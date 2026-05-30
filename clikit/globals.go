// Package clikit is the shared CLI convention layer for the m-cli Go toolchain.
//
// Every Go CLI in the toolchain (m-cli, irissync, vista-meta, kids-vc,
// m-dev-tools-mcp, …) is built on this package so they share one command
// grammar, one --output/JSON contract, one error/exit-code ladder, and one
// TTY-gated styling layer. See m-cli-go-toolchain-spec.md §5 / §5.5.
package clikit

// OutputFormat is the resolved render mode for a single invocation.
type OutputFormat string

const (
	// FormatText renders human-facing output (styled when stdout is a TTY).
	FormatText OutputFormat = "text"
	// FormatJSON renders the machine-readable envelope (§5.5).
	FormatJSON OutputFormat = "json"
)

// Globals are the flags every CLI in the toolchain shares. Embed it in the
// root command struct, e.g.:
//
//	type CLI struct {
//	    clikit.Globals
//	    Fmt FmtCmd `cmd:"" help:"…"`
//	}
type Globals struct {
	Output  string `short:"o" enum:"text,json,auto" default:"auto" help:"Output: text (styled on a TTY), json (machine-readable), or auto."`
	NoColor bool   `name:"no-color" env:"NO_COLOR" help:"Disable ANSI styling even on a TTY."`
	Verbose bool   `short:"v" help:"Verbose diagnostics to stderr."`
}
