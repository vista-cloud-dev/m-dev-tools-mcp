// Package introspect turns the `m` toolchain's reflected `m schema` (the
// aggregated busybox tree, spec §5.5) into MCP tool definitions and back into
// `m` argv for a tool call. It is the whole substance of the thin MCP wrapper:
// the server does not define a parallel surface — it mirrors whatever `m schema`
// reports, so the agent surface can never drift from the CLI ([N11]).
package introspect

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/vista-cloud-dev/m-dev-tools-mcp/clikit"
)

// Tool is one MCP tool derived from an `m` command.
type Tool struct {
	Name        string         `json:"name"`
	Path        []string       `json:"-"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema"`

	flags []clikit.SchemaFlag
	args  []clikit.SchemaArg
}

// Profile gates which commands are exposed as tools.
type Profile struct {
	Name string
	deny map[string]bool // tool names to exclude
}

// nonRPC commands aren't request/response shaped (servers, long-running, or the
// introspection surface itself) so they're never exposed as tools.
var nonRPC = map[string]bool{
	"lsp": true, "watch": true,
	"schema": true, "version": true, "install-completions": true,
}

// ParseProfile resolves a named exposure profile:
//   - default: every request/response command (excludes the nonRPC set).
//   - safe:    default minus the DB writer (`push`).
//   - all:     everything, including the nonRPC commands (escape hatch).
func ParseProfile(name string) (Profile, error) {
	switch name {
	case "", "default":
		return Profile{Name: "default", deny: copyDeny(nonRPC)}, nil
	case "safe":
		d := copyDeny(nonRPC)
		d["push"] = true
		return Profile{Name: "safe", deny: d}, nil
	case "all":
		return Profile{Name: "all", deny: map[string]bool{}}, nil
	default:
		return Profile{}, fmt.Errorf("unknown profile %q (want default|safe|all)", name)
	}
}

func copyDeny(m map[string]bool) map[string]bool {
	out := make(map[string]bool, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (p Profile) exposes(path []string, name string) bool {
	if len(path) > 0 && p.deny[path[0]] {
		return false
	}
	return !p.deny[name]
}

// FromSchema maps every exposed command in doc to an MCP tool.
func FromSchema(doc clikit.SchemaDoc, prof Profile) []Tool {
	var tools []Tool
	for _, c := range doc.Commands {
		if len(c.Path) == 0 {
			continue
		}
		name := strings.Join(c.Path, "_")
		if !prof.exposes(c.Path, name) {
			continue
		}
		tools = append(tools, Tool{
			Name:        name,
			Path:        c.Path,
			Description: c.Help,
			InputSchema: inputSchema(c),
			flags:       c.Flags,
			args:        c.Args,
		})
	}
	return tools
}

// inputSchema renders a command's flags + positionals as a JSON Schema object.
func inputSchema(c clikit.SchemaCommand) map[string]any {
	props := map[string]any{}
	var required []string
	for _, f := range c.Flags {
		props[f.Name] = propSchema(f.Type, f.Enum, f.Help)
	}
	for _, a := range c.Args {
		props[a.Name] = propSchema(a.Type, a.Enum, a.Help)
		if a.Required {
			required = append(required, a.Name)
		}
	}
	schema := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func propSchema(typ string, enum []string, help string) map[string]any {
	p := map[string]any{}
	switch typ {
	case "bool":
		p["type"] = "boolean"
	case "int", "uint":
		p["type"] = "integer"
	case "float":
		p["type"] = "number"
	case "list":
		p["type"] = "array"
		p["items"] = map[string]any{"type": "string"}
	case "enum":
		p["type"] = "string"
		if len(enum) > 0 {
			p["enum"] = enum
		}
	default:
		p["type"] = "string"
	}
	if help != "" {
		p["description"] = help
	}
	return p
}

// BuildArgv builds the `m` argv for a tool call from a JSON arguments object:
// the command path, the provided flags (a true bool as a bare `--flag`, others
// as `--flag value`), the positional args in declared order, and a trailing
// `--output json` so the result is always the machine envelope (§5.5).
func (t Tool) BuildArgv(args map[string]any) []string {
	argv := append([]string{}, t.Path...)

	flagByName := map[string]clikit.SchemaFlag{}
	for _, f := range t.flags {
		flagByName[f.Name] = f
	}
	for _, f := range t.flags {
		v, ok := args[f.Name]
		if !ok {
			continue
		}
		if f.Type == "bool" {
			if b, _ := v.(bool); b {
				argv = append(argv, "--"+f.Name)
			}
			continue
		}
		for _, s := range toStrings(v) {
			argv = append(argv, "--"+f.Name, s)
		}
	}
	for _, a := range t.args {
		v, ok := args[a.Name]
		if !ok {
			continue
		}
		argv = append(argv, toStrings(v)...)
	}
	return append(argv, "--output", "json")
}

// toStrings coerces a JSON value (scalar or array) to one or more CLI tokens.
func toStrings(v any) []string {
	switch x := v.(type) {
	case nil:
		return nil
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			out = append(out, scalar(e))
		}
		return out
	case []string:
		return x
	default:
		return []string{scalar(v)}
	}
}

func scalar(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		// JSON numbers decode as float64; render integers without a decimal.
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%g", x)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// MBinary resolves the `m` busybox: the M_BIN override, then alongside this
// server binary, then $PATH. A miss is a deterministic error (the §3.3 ladder).
func MBinary() (string, error) {
	if override := os.Getenv("M_BIN"); override != "" {
		if isExecutable(override) {
			return override, nil
		}
		return "", clikit.Fail(clikit.ExitRuntime, "M_NOT_FOUND",
			fmt.Sprintf("m binary not found at M_BIN=%s", override),
			"point M_BIN at the built `m`, or put it alongside this server or on $PATH")
	}
	if self, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(self), "m")
		if isExecutable(cand) {
			return cand, nil
		}
	}
	if p, err := exec.LookPath("m"); err == nil {
		return p, nil
	}
	return "", clikit.Fail(clikit.ExitRuntime, "M_NOT_FOUND", "m binary not found",
		"install `m` on $PATH or set M_BIN=/path/to/m")
}

func isExecutable(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0
}

// LoadSchema runs `<mbin> schema` and parses the aggregated command tree.
func LoadSchema(ctx context.Context, mbin string) (clikit.SchemaDoc, error) {
	out, err := exec.CommandContext(ctx, mbin, "schema").Output()
	if err != nil {
		return clikit.SchemaDoc{}, fmt.Errorf("m schema: %w", err)
	}
	var doc clikit.SchemaDoc
	if err := json.Unmarshal(out, &doc); err != nil {
		return clikit.SchemaDoc{}, fmt.Errorf("m schema: invalid JSON: %w", err)
	}
	return doc, nil
}
