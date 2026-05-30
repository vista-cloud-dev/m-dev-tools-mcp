package clikit

import (
	"reflect"
	"strings"

	"github.com/alecthomas/kong"
)

// SchemaVersion is the semver of the machine surface (envelope + schema).
// Additive fields bump the minor; removals/renames bump the major (§5.5).
const SchemaVersion = "1.0"

// SchemaDoc is the JSON `schema` output: the full command/flag/enum tree,
// reflected from the Kong model so it can never drift from the real surface.
type SchemaDoc struct {
	SchemaVersion string          `json:"schemaVersion"`
	Tool          string          `json:"tool"`
	Version       string          `json:"version"`
	GlobalFlags   []SchemaFlag    `json:"globalFlags,omitempty"`
	Commands      []SchemaCommand `json:"commands"`
}

// SchemaCommand describes one (sub)command.
type SchemaCommand struct {
	Path  []string     `json:"path"`
	Help  string       `json:"help,omitempty"`
	Flags []SchemaFlag `json:"flags,omitempty"`
	Args  []SchemaArg  `json:"args,omitempty"`
}

// SchemaFlag describes one flag.
type SchemaFlag struct {
	Name     string   `json:"name"`
	Short    string   `json:"short,omitempty"`
	Type     string   `json:"type"`
	Enum     []string `json:"enum,omitempty"`
	Default  string   `json:"default,omitempty"`
	Required bool     `json:"required,omitempty"`
	Help     string   `json:"help,omitempty"`
}

// SchemaArg describes one positional argument.
type SchemaArg struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Enum     []string `json:"enum,omitempty"`
	Required bool     `json:"required,omitempty"`
	Help     string   `json:"help,omitempty"`
}

// BuildSchema reflects a parsed Kong model into a SchemaDoc.
func BuildSchema(k *kong.Kong, tool, version string) SchemaDoc {
	doc := SchemaDoc{SchemaVersion: SchemaVersion, Tool: tool, Version: version}

	global := map[string]bool{}
	for _, f := range k.Model.Flags {
		if f.Name == "help" {
			continue
		}
		global[f.Name] = true
		doc.GlobalFlags = append(doc.GlobalFlags, flagSchema(f))
	}

	var walk func(n *kong.Node, prefix []string)
	walk = func(n *kong.Node, prefix []string) {
		for _, child := range n.Children {
			path := append(append([]string{}, prefix...), child.Name)
			if child.Type == kong.CommandNode {
				cmd := SchemaCommand{Path: path, Help: child.Help}
				for _, f := range child.Flags {
					if f.Name == "help" || global[f.Name] {
						continue
					}
					cmd.Flags = append(cmd.Flags, flagSchema(f))
				}
				for _, p := range child.Positional {
					cmd.Args = append(cmd.Args, SchemaArg{
						Name: p.Name, Type: valueType(p), Enum: splitEnum(p.Enum),
						Required: p.Required, Help: p.Help,
					})
				}
				doc.Commands = append(doc.Commands, cmd)
			}
			walk(child, path)
		}
	}
	walk(k.Model.Node, nil)
	return doc
}

func flagSchema(f *kong.Flag) SchemaFlag {
	sf := SchemaFlag{
		Name: f.Name, Type: valueType(f.Value), Default: f.Default,
		Required: f.Required, Enum: splitEnum(f.Enum), Help: f.Help,
	}
	if f.Short != 0 {
		sf.Short = string(f.Short)
	}
	return sf
}

func valueType(v *kong.Value) string {
	if v.Enum != "" {
		return "enum"
	}
	if v.Target.IsValid() {
		switch v.Target.Kind() {
		case reflect.Bool:
			return "bool"
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return "int"
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return "uint"
		case reflect.Float32, reflect.Float64:
			return "float"
		case reflect.Slice:
			return "list"
		}
	}
	return "string"
}

func splitEnum(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// SchemaCmd is the reusable `schema` subcommand. Embed it in any CLI:
//
//	Schema clikit.SchemaCmd `cmd:"" help:"Emit the command tree as JSON."`
//
// It always emits JSON (the machine surface), regardless of --output.
type SchemaCmd struct{}

// Run emits the reflected schema. clikit.Run binds the *kong.Kong.
func (SchemaCmd) Run(c *Context, k *kong.Kong) error {
	return c.EmitJSON(BuildSchema(k, k.Model.Name, Version))
}
