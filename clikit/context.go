package clikit

import (
	"encoding/json"
	"io"
	"os"

	"golang.org/x/term"
)

// Envelope is the stable --output json wrapper for every command result
// (spec §5.5). One shape across the whole toolchain.
type Envelope struct {
	SchemaVersion string       `json:"schemaVersion"`
	Command       string       `json:"command,omitempty"`
	OK            bool         `json:"ok"`
	Exit          int          `json:"exit"`
	Data          any          `json:"data,omitempty"`
	Diagnostics   []Diagnostic `json:"diagnostics,omitempty"`
	Error         *Error       `json:"error,omitempty"`
}

// Diagnostic is one lint/diagnostic finding (the editor↔CI shared shape).
type Diagnostic struct {
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Col      int    `json:"col,omitempty"`
	Rule     string `json:"rule,omitempty"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// Context carries the resolved output mode + styling into a command's Run.
// Bind it via clikit.Run; command methods take Run(cc *clikit.Context) error.
//
// All styling lives on this one type (see style.go for the toolkit and
// spinner.go for the live elements). Every primitive is a no-op unless Color
// is set, so JSON and piped output stay clean automatically.
type Context struct {
	Stdout  io.Writer
	Stderr  io.Writer
	Format  OutputFormat
	Color   bool
	Verbose bool
	Command string

	th      theme
	gl      Glyph
	unicode bool
}

// NewContext resolves the format/color for this invocation from the globals
// and the TTY state of stdout. It also picks the glyph set: full Unicode on a
// UTF-8 terminal, an ASCII fallback otherwise.
func NewContext(g *Globals, command string) *Context {
	tty := term.IsTerminal(int(os.Stdout.Fd()))
	format := resolveFormat(g.Output, tty)
	color := format == FormatText && tty && !g.NoColor
	unicode := supportsUnicode()
	gl := glyphsUnicode
	if !unicode {
		gl = glyphsASCII
	}
	return &Context{
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Format:  format,
		Color:   color,
		Verbose: g.Verbose,
		Command: command,
		th:      newTheme(),
		gl:      gl,
		unicode: unicode,
	}
}

func resolveFormat(output string, tty bool) OutputFormat {
	switch output {
	case "json":
		return FormatJSON
	case "text":
		return FormatText
	default: // "auto": styled text on a TTY, JSON when piped/redirected (§3.3)
		if tty {
			return FormatText
		}
		return FormatJSON
	}
}

// JSON reports whether the machine-readable envelope should be emitted.
func (c *Context) JSON() bool { return c.Format == FormatJSON }

// Result renders a command's result: the JSON envelope in JSON mode, otherwise
// the human-facing text produced by the text closure.
func (c *Context) Result(data any, text func()) error {
	if c.JSON() {
		return c.emit(Envelope{SchemaVersion: SchemaVersion, Command: c.Command, OK: true, Exit: ExitOK, Data: data})
	}
	if text != nil {
		text()
	}
	return nil
}

// Diagnostics renders a result that carries lint-style findings.
func (c *Context) Diagnostics(data any, diags []Diagnostic, text func()) error {
	if c.JSON() {
		return c.emit(Envelope{SchemaVersion: SchemaVersion, Command: c.Command, OK: true, Exit: ExitOK, Data: data, Diagnostics: diags})
	}
	if text != nil {
		text()
	}
	return nil
}

// EmitJSON pretty-prints an arbitrary value as JSON (used by `schema`).
func (c *Context) EmitJSON(v any) error { return c.emit(v) }

func (c *Context) emit(v any) error { return writeJSON(c.Stdout, v) }

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
