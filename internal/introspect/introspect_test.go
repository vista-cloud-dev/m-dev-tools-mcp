package introspect

import (
	"reflect"
	"sort"
	"testing"

	"github.com/vista-cloud-dev/clikit"
)

func sampleDoc() clikit.SchemaDoc {
	return clikit.SchemaDoc{
		SchemaVersion: "1.0", Tool: "m", Version: "1.0",
		Commands: []clikit.SchemaCommand{
			{Path: []string{"lint"}, Help: "Lint M source.", Flags: []clikit.SchemaFlag{
				{Name: "check", Type: "bool", Help: "Exit non-zero on findings."},
				{Name: "profile", Type: "enum", Enum: []string{"default", "xindex"}, Default: "default"},
			}, Args: []clikit.SchemaArg{
				{Name: "paths", Type: "list", Help: "Files to lint."},
			}},
			{Path: []string{"push"}, Help: "Write back to IRIS.", Args: []clikit.SchemaArg{
				{Name: "rest", Type: "list"},
			}},
			{Path: []string{"kids", "decompose"}, Help: "Split a .KID.", Args: []clikit.SchemaArg{
				{Name: "kid-file", Type: "string", Required: true},
			}},
			{Path: []string{"lsp"}, Help: "Run the language server."},
			{Path: []string{"watch"}, Help: "Watch."},
			{Path: []string{"schema"}, Help: "Emit schema."},
			{Path: []string{"version"}, Help: "Version."},
		},
	}
}

func toolNames(ts []Tool) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.Name
	}
	sort.Strings(out)
	return out
}

func TestFromSchemaDefaultProfileExcludesNonRPC(t *testing.T) {
	prof, _ := ParseProfile("default")
	tools := FromSchema(sampleDoc(), prof)
	got := toolNames(tools)
	want := []string{"kids_decompose", "lint", "push"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("default tools = %v, want %v (lsp/watch/schema/version excluded)", got, want)
	}
}

func TestFromSchemaSafeProfileDropsWriters(t *testing.T) {
	prof, _ := ParseProfile("safe")
	got := toolNames(FromSchema(sampleDoc(), prof))
	for _, n := range got {
		if n == "push" {
			t.Errorf("safe profile must not expose push; got %v", got)
		}
	}
}

func TestParseProfileUnknown(t *testing.T) {
	if _, err := ParseProfile("bogus"); err == nil {
		t.Error("ParseProfile(bogus): want error")
	}
}

func TestInputSchemaShape(t *testing.T) {
	prof, _ := ParseProfile("default")
	var lint Tool
	for _, tl := range FromSchema(sampleDoc(), prof) {
		if tl.Name == "lint" {
			lint = tl
		}
	}
	props, ok := lint.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("InputSchema has no properties: %#v", lint.InputSchema)
	}
	check, ok := props["check"].(map[string]any)
	if !ok || check["type"] != "boolean" {
		t.Errorf("check flag schema = %#v, want type boolean", props["check"])
	}
	profile, ok := props["profile"].(map[string]any)
	if !ok {
		t.Fatalf("missing profile property")
	}
	if _, ok := profile["enum"]; !ok {
		t.Errorf("enum flag should carry an enum list: %#v", profile)
	}
	if _, ok := props["paths"]; !ok {
		t.Errorf("positional arg `paths` should be a property: %#v", props)
	}
}

func TestBuildArgvFlagsAndPositionals(t *testing.T) {
	prof, _ := ParseProfile("default")
	var lint Tool
	for _, tl := range FromSchema(sampleDoc(), prof) {
		if tl.Name == "lint" {
			lint = tl
		}
	}
	argv := lint.BuildArgv(map[string]any{
		"check":   true,
		"profile": "xindex",
		"paths":   []any{"A.m", "B.m"},
	})
	// path, then flags (bool as bare flag, enum as --name value), positionals,
	// and a trailing --output json.
	joined := argv
	mustContain := func(sub ...string) {
		for i := 0; i+len(sub) <= len(joined); i++ {
			if reflect.DeepEqual(joined[i:i+len(sub)], sub) {
				return
			}
		}
		t.Errorf("argv %v missing subsequence %v", joined, sub)
	}
	if joined[0] != "lint" {
		t.Errorf("argv[0] = %q, want lint", joined[0])
	}
	mustContain("--check")
	mustContain("--profile", "xindex")
	mustContain("A.m")
	mustContain("B.m")
	mustContain("--output", "json")
}

func TestBuildArgvGroupPath(t *testing.T) {
	prof, _ := ParseProfile("default")
	var dec Tool
	for _, tl := range FromSchema(sampleDoc(), prof) {
		if tl.Name == "kids_decompose" {
			dec = tl
		}
	}
	argv := dec.BuildArgv(map[string]any{"kid-file": "p.KID"})
	if len(argv) < 3 || argv[0] != "kids" || argv[1] != "decompose" {
		t.Fatalf("group path argv = %v, want it to start with [kids decompose]", argv)
	}
	if argv[2] != "p.KID" {
		t.Errorf("positional not placed: %v", argv)
	}
}

func TestBuildArgvOmitsFalseBoolAndAbsent(t *testing.T) {
	prof, _ := ParseProfile("default")
	var lint Tool
	for _, tl := range FromSchema(sampleDoc(), prof) {
		if tl.Name == "lint" {
			lint = tl
		}
	}
	argv := lint.BuildArgv(map[string]any{"check": false})
	for _, a := range argv {
		if a == "--check" || a == "--profile" {
			t.Errorf("false/absent flags must be omitted, got %v", argv)
		}
	}
}
