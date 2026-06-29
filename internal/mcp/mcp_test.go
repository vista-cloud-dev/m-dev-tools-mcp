package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/vista-cloud-dev/m-dev-tools-mcp/internal/introspect"
)

const schemaJSON = `{"schemaVersion":"1.0","tool":"m","version":"1.0","commands":[
  {"path":["lint"],"help":"Lint M source.","flags":[{"name":"check","type":"bool"}],"args":[{"name":"paths","type":"list"}]},
  {"path":["push"],"help":"Write back.","args":[{"name":"rest","type":"list"}]},
  {"path":["lsp"],"help":"server"}
]}`

// fakeRun answers `m schema` with the canned tree and any tool call with a
// recorded argv echoed back inside a fake envelope.
func fakeRun(_ *testing.T, gotArgv *[]string, exit int, stdout, stderr string) RunFunc {
	return func(_ context.Context, _ string, argv []string) ([]byte, []byte, int, error) {
		if len(argv) > 0 && argv[0] == "schema" {
			return []byte(schemaJSON), nil, 0, nil
		}
		*gotArgv = argv
		return []byte(stdout), []byte(stderr), exit, nil
	}
}

// roundtrip feeds one request line through dispatch and decodes the response.
func roundtrip(t *testing.T, s *Server, line string) response {
	t.Helper()
	var in, out strings.Builder
	in.WriteString(line + "\n")
	if err := s.Serve(context.Background(), strings.NewReader(in.String()), &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	text := strings.TrimSpace(out.String())
	if text == "" {
		return response{} // notification: no reply
	}
	var resp response
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("decode response %q: %v", text, err)
	}
	return resp
}

func newServer(run RunFunc) *Server {
	prof, _ := introspect.ParseProfile("default")
	return New("m", prof, run)
}

func TestInitializeEchoesProtocol(t *testing.T) {
	s := newServer(fakeRun(t, nil, 0, "", ""))
	resp := roundtrip(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`)
	if resp.Error != nil {
		t.Fatalf("initialize error: %+v", resp.Error)
	}
	res := resp.Result.(map[string]any)
	if res["protocolVersion"] != "2024-11-05" {
		t.Errorf("protocolVersion = %v, want echoed 2024-11-05", res["protocolVersion"])
	}
	caps := res["capabilities"].(map[string]any)
	if _, ok := caps["tools"]; !ok {
		t.Errorf("capabilities missing tools: %v", caps)
	}
	if si := res["serverInfo"].(map[string]any); si["name"] != "m-dev-tools-mcp" {
		t.Errorf("serverInfo.name = %v", si["name"])
	}
}

func TestToolsListExposesProfiled(t *testing.T) {
	s := newServer(fakeRun(t, nil, 0, "", ""))
	resp := roundtrip(t, s, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if resp.Error != nil {
		t.Fatalf("tools/list error: %+v", resp.Error)
	}
	tools := resp.Result.(map[string]any)["tools"].([]any)
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.(map[string]any)["name"].(string)] = true
	}
	if !names["lint"] || !names["push"] {
		t.Errorf("expected lint + push tools, got %v", names)
	}
	if names["lsp"] {
		t.Errorf("lsp must be excluded by the default profile, got %v", names)
	}
}

func TestToolsCallBuildsArgvAndReturnsEnvelope(t *testing.T) {
	var gotArgv []string
	env := `{"schemaVersion":"1.0","command":"lint","ok":true,"exit":0,"data":{"findings":0}}`
	s := newServer(fakeRun(t, &gotArgv, 0, env, ""))
	resp := roundtrip(t, s, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"lint","arguments":{"check":true,"paths":["A.m"]}}}`)
	if resp.Error != nil {
		t.Fatalf("tools/call error: %+v", resp.Error)
	}
	if strings.Join(gotArgv, " ") != "lint --check A.m --output json" {
		t.Errorf("argv = %v, want [lint --check A.m --output json]", gotArgv)
	}
	res := resp.Result.(map[string]any)
	if res["isError"].(bool) {
		t.Errorf("isError should be false on exit 0")
	}
	content := res["content"].([]any)[0].(map[string]any)
	if !strings.Contains(content["text"].(string), `"findings":0`) {
		t.Errorf("content text should carry the envelope, got %v", content["text"])
	}
}

func TestToolsCallErrorEnvelopeFromStderr(t *testing.T) {
	var gotArgv []string
	errEnv := `{"ok":false,"exit":4,"error":{"code":"ENGINE_UNRESOLVED","exit":4}}`
	s := newServer(fakeRun(t, &gotArgv, 4, "", errEnv))
	resp := roundtrip(t, s, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"lint","arguments":{}}}`)
	res := resp.Result.(map[string]any)
	if !res["isError"].(bool) {
		t.Errorf("isError should be true on non-zero exit")
	}
	content := res["content"].([]any)[0].(map[string]any)
	if !strings.Contains(content["text"].(string), "ENGINE_UNRESOLVED") {
		t.Errorf("error envelope (from stderr) should be surfaced, got %v", content["text"])
	}
}

func TestUnknownToolIsParamsError(t *testing.T) {
	s := newServer(fakeRun(t, nil, 0, "", ""))
	resp := roundtrip(t, s, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"nope","arguments":{}}}`)
	if resp.Error == nil || resp.Error.Code != codeParams {
		t.Errorf("unknown tool: want params error, got %+v", resp.Error)
	}
}

func TestNotificationGetsNoReply(t *testing.T) {
	s := newServer(fakeRun(t, nil, 0, "", ""))
	resp := roundtrip(t, s, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if resp.JSONRPC != "" || resp.Result != nil || resp.Error != nil {
		t.Errorf("notification must produce no response, got %+v", resp)
	}
}

func TestUnknownMethod(t *testing.T) {
	s := newServer(fakeRun(t, nil, 0, "", ""))
	resp := roundtrip(t, s, `{"jsonrpc":"2.0","id":6,"method":"frobnicate"}`)
	if resp.Error == nil || resp.Error.Code != codeMethod {
		t.Errorf("unknown method: want -32601, got %+v", resp.Error)
	}
}
