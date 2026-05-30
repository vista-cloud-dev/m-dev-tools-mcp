package clikit

import (
	"errors"
	"fmt"
)

// Exit codes — the toolchain-wide ladder (spec §3.3). Every CLI uses these.
const (
	ExitOK      = 0 // success
	ExitRuntime = 1 // runtime error (IO / engine / parse)
	ExitUsage   = 2 // usage error (bad flags/args)
	ExitCheck   = 3 // --check / lint found findings or drift
	ExitRefused = 4 // engine-bound op refused (no engine / substrate unavailable)
)

// Error is the deterministic, machine-parseable error object. Commands return
// it (via Fail) so agents and CI branch on code+exit, not on prose (§5.5).
type Error struct {
	Code    string `json:"code"`
	Exit    int    `json:"exit"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

func (e *Error) Error() string { return e.Message }

// Fail builds a deterministic Error for a command to return.
func Fail(exit int, code, message, hint string) *Error {
	return &Error{Code: code, Exit: exit, Message: message, Hint: hint}
}

// exitOf maps any error to an exit code (clikit.Error keeps its own).
func exitOf(err error) int {
	var e *Error
	if errors.As(err, &e) {
		return e.Exit
	}
	return ExitRuntime
}

// RenderError prints an error: the JSON envelope in JSON mode, otherwise a
// styled "Error: …" + optional hint on stderr.
func RenderError(c *Context, err error) {
	var e *Error
	if !errors.As(err, &e) {
		e = &Error{Code: "RUNTIME", Exit: ExitRuntime, Message: err.Error()}
	}
	if c.JSON() {
		_ = writeJSON(c.Stderr, Envelope{SchemaVersion: SchemaVersion, Command: c.Command, OK: false, Exit: e.Exit, Error: e})
		return
	}
	fmt.Fprintf(c.Stderr, "%s %s\n", c.th.err.render(c.Color, c.gl.Err+" Error:"), e.Message)
	if e.Hint != "" {
		fmt.Fprintln(c.Stderr, c.th.hint.render(c.Color, "  "+c.gl.Arrow+" hint: "+e.Hint))
	}
}
