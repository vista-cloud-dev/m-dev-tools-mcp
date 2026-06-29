// Package mcp is a thin Model Context Protocol server (JSON-RPC 2.0 over stdio,
// newline-delimited) that exposes the `m` toolchain to agents. It is a wrapper
// over the reflected `m schema` (spec §5.5, [N11]) — not a parallel surface:
// every tool is one `m` command, every result is m's `--output json` envelope,
// and every failure is m's deterministic error object passed straight through.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"

	"github.com/vista-cloud-dev/clikit"
	"github.com/vista-cloud-dev/m-dev-tools-mcp/internal/introspect"
)

// ProtocolVersion is the MCP revision this server defaults to when a client
// doesn't pin one in `initialize`.
const ProtocolVersion = "2025-06-18"

// RunFunc invokes the `m` binary; injected so the server is testable without a
// real toolchain. It returns the child's stdout, stderr, and exit code.
type RunFunc func(ctx context.Context, mbin string, argv []string) (stdout, stderr []byte, exit int, err error)

// Server speaks MCP over a reader/writer pair.
type Server struct {
	mbin    string
	profile introspect.Profile
	run     RunFunc
	name    string
	version string
}

// New builds a server. If run is nil the default exec runner is used.
func New(mbin string, profile introspect.Profile, run RunFunc) *Server {
	if run == nil {
		run = ExecRun
	}
	return &Server{mbin: mbin, profile: profile, run: run, name: "m-dev-tools-mcp", version: clikit.Version}
}

// ExecRun is the production runner: exec `m` and capture its streams.
func ExecRun(ctx context.Context, mbin string, argv []string) ([]byte, []byte, int, error) {
	cmd := exec.CommandContext(ctx, mbin, argv...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	switch {
	case err == nil:
		return out.Bytes(), errb.Bytes(), 0, nil
	case isExit(err):
		var ee *exec.ExitError
		errors.As(err, &ee)
		return out.Bytes(), errb.Bytes(), ee.ExitCode(), nil
	default:
		return nil, nil, 0, err
	}
}

func isExit(err error) bool {
	var ee *exec.ExitError
	return errors.As(err, &ee)
}

// --- JSON-RPC framing --------------------------------------------------------

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

const (
	codeParse     = -32700
	codeMethod    = -32601
	codeParams    = -32602
	codeInternal  = -32603
	bufMaxBytes   = 16 << 20 // tool args / schemas can be large
	bufStartBytes = 64 << 10
)

// Serve runs the message loop until stdin closes. Each line is one JSON-RPC
// message; requests get a response line, notifications get none.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, bufStartBytes), bufMaxBytes)
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)

	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			_ = enc.Encode(response{JSONRPC: "2.0", Error: &rpcError{Code: codeParse, Message: "parse error"}})
			continue
		}
		resp, reply := s.dispatch(ctx, req)
		if reply {
			if err := enc.Encode(resp); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}

// isNotification reports whether a message has no id (so it gets no response).
func isNotification(id json.RawMessage) bool {
	return len(id) == 0 || string(id) == "null"
}

func (s *Server) dispatch(ctx context.Context, req request) (response, bool) {
	if isNotification(req.ID) {
		return response{}, false // e.g. notifications/initialized — no reply
	}
	base := response{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		base.Result = s.initialize(req.Params)
	case "ping":
		base.Result = map[string]any{}
	case "tools/list":
		result, rerr := s.toolsList(ctx)
		if rerr != nil {
			base.Error = rerr
		} else {
			base.Result = result
		}
	case "tools/call":
		result, rerr := s.toolsCall(ctx, req.Params)
		if rerr != nil {
			base.Error = rerr
		} else {
			base.Result = result
		}
	default:
		base.Error = &rpcError{Code: codeMethod, Message: "method not found: " + req.Method}
	}
	return base, true
}

func (s *Server) initialize(params json.RawMessage) map[string]any {
	version := ProtocolVersion
	if len(params) > 0 {
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if json.Unmarshal(params, &p) == nil && p.ProtocolVersion != "" {
			version = p.ProtocolVersion
		}
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
		"serverInfo":      map[string]any{"name": s.name, "version": s.version},
	}
}

// loadTools runs `m schema` and maps the aggregated tree to MCP tools.
func (s *Server) loadTools(ctx context.Context) ([]introspect.Tool, error) {
	stdout, stderr, exit, err := s.run(ctx, s.mbin, []string{"schema"})
	if err != nil {
		return nil, err
	}
	if exit != 0 {
		return nil, fmt.Errorf("m schema exited %d: %s", exit, bytes.TrimSpace(stderr))
	}
	var doc clikit.SchemaDoc
	if err := json.Unmarshal(stdout, &doc); err != nil {
		return nil, fmt.Errorf("m schema: invalid JSON: %w", err)
	}
	return introspect.FromSchema(doc, s.profile), nil
}

func (s *Server) toolsList(ctx context.Context) (map[string]any, *rpcError) {
	tools, err := s.loadTools(ctx)
	if err != nil {
		return nil, &rpcError{Code: codeInternal, Message: err.Error()}
	}
	list := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		list = append(list, map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": t.InputSchema,
		})
	}
	return map[string]any{"tools": list}, nil
}

func (s *Server) toolsCall(ctx context.Context, params json.RawMessage) (map[string]any, *rpcError) {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{Code: codeParams, Message: "invalid params: " + err.Error()}
	}
	tools, err := s.loadTools(ctx)
	if err != nil {
		return nil, &rpcError{Code: codeInternal, Message: err.Error()}
	}
	var tool *introspect.Tool
	for i := range tools {
		if tools[i].Name == p.Name {
			tool = &tools[i]
			break
		}
	}
	if tool == nil {
		return nil, &rpcError{Code: codeParams, Message: "unknown tool: " + p.Name}
	}

	argv := tool.BuildArgv(p.Arguments)
	stdout, stderr, exit, err := s.run(ctx, s.mbin, argv)
	if err != nil {
		return nil, &rpcError{Code: codeInternal, Message: fmt.Sprintf("running m %v: %v", argv, err)}
	}
	// m's success envelope is on stdout; its deterministic error envelope is on
	// stderr. Surface whichever carries the payload, and mark a non-zero exit as
	// a tool error so the agent branches on it.
	text := bytes.TrimSpace(stdout)
	if len(text) == 0 {
		text = bytes.TrimSpace(stderr)
	}
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": string(text)}},
		"isError": exit != 0,
	}, nil
}
