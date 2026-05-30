// Command m-dev-tools-mcp is a thin Model Context Protocol server over the `m`
// toolchain (spec §5.5, [N11]). It introspects the aggregated `m schema`,
// exposes each command as an MCP tool, and returns m's `--output json`
// envelopes and deterministic error objects unchanged — a wrapper over the
// reflected surface, never a parallel one.
//
// Try:
//
//	m-dev-tools-mcp tools --output json     # the tools exposed for a profile
//	m-dev-tools-mcp tools --profile safe    # drop DB writers
//	m-dev-tools-mcp serve                   # speak MCP over stdio (for an agent host)
//	m-dev-tools-mcp schema | jq .           # this CLI's own command tree
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/willabides/kongplete"

	"github.com/vista-cloud-dev/m-dev-tools-mcp/clikit"
	"github.com/vista-cloud-dev/m-dev-tools-mcp/internal/introspect"
	"github.com/vista-cloud-dev/m-dev-tools-mcp/internal/mcp"
)

// CLI is the root command grammar (spec §5).
type CLI struct {
	clikit.Globals

	Serve serveCmd `cmd:"" help:"Run the MCP server (JSON-RPC 2.0 over stdio)."`
	Tools toolsCmd `cmd:"" help:"List the tools exposed for a profile (inspection of the m schema mapping)."`

	Schema  clikit.SchemaCmd  `cmd:"" help:"Emit this CLI's command/flag/enum tree as JSON."`
	Version clikit.VersionCmd `cmd:"" help:"Show version and build info."`

	InstallCompletions kongplete.InstallCompletions `cmd:"" help:"Install shell tab-completions."`
}

func main() {
	cli := &CLI{}
	os.Exit(clikit.Run(
		"m-dev-tools-mcp",
		"m-dev-tools-mcp — a thin MCP server over the m toolchain's reflected schema.",
		cli, &cli.Globals,
	))
}

// opts are the flags both server-facing commands share.
type opts struct {
	Profile string `default:"default" enum:"default,safe,all" help:"Exposure profile: default (RPC-shaped commands), safe (also drops the DB writer), or all."`
	MBin    string `name:"m-bin" type:"path" help:"Path to the m busybox (else $M_BIN, alongside this binary, or $PATH)."`
}

func (o opts) resolve() (string, introspect.Profile, error) {
	prof, err := introspect.ParseProfile(o.Profile)
	if err != nil {
		return "", introspect.Profile{}, clikit.Fail(clikit.ExitUsage, "BAD_PROFILE", err.Error(), "")
	}
	if o.MBin != "" {
		if fi, err := os.Stat(o.MBin); err != nil || fi.IsDir() || fi.Mode()&0o111 == 0 {
			return "", prof, clikit.Fail(clikit.ExitRuntime, "M_NOT_FOUND",
				fmt.Sprintf("--m-bin %q is not an executable file", o.MBin), "")
		}
		return o.MBin, prof, nil
	}
	mbin, err := introspect.MBinary()
	return mbin, prof, err
}

// --- serve -------------------------------------------------------------------

type serveCmd struct{ opts }

func (c *serveCmd) Run(_ *clikit.Context) error {
	mbin, prof, err := c.resolve()
	if err != nil {
		return err
	}
	srv := mcp.New(mbin, prof, nil)
	if err := srv.Serve(context.Background(), os.Stdin, os.Stdout); err != nil {
		return clikit.Fail(clikit.ExitRuntime, "SERVE", err.Error(), "")
	}
	return nil
}

// --- tools (inspection) ------------------------------------------------------

type toolsCmd struct{ opts }

func (c *toolsCmd) Run(cc *clikit.Context) error {
	mbin, prof, err := c.resolve()
	if err != nil {
		return err
	}
	doc, err := introspect.LoadSchema(context.Background(), mbin)
	if err != nil {
		return clikit.Fail(clikit.ExitRuntime, "M_SCHEMA", err.Error(),
			"is the m busybox reachable? set --m-bin or $M_BIN")
	}
	tools := introspect.FromSchema(doc, prof)
	return cc.Result(map[string]any{"profile": prof.Name, "count": len(tools), "tools": tools}, func() {
		cc.Title(fmt.Sprintf("%d tools (profile: %s)", len(tools), prof.Name))
		for _, t := range tools {
			fmt.Fprintf(cc.Stdout, "%s  %s\n", cc.Accent(t.Name), cc.Faint(t.Description))
		}
	})
}
