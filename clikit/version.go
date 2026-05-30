package clikit

import "runtime"

// Build metadata, injected at link time:
//
//	go build -ldflags "-X github.com/vista-cloud-dev/m-dev-tools-mcp/clikit.Version=$VER \
//	                    -X …/clikit.Commit=$SHA -X …/clikit.Date=$DATE"
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// VersionCmd is the reusable `version` subcommand. Embed it in any CLI:
//
//	Version clikit.VersionCmd `cmd:"" help:"Show version and build info."`
type VersionCmd struct{}

type versionInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
	Go      string `json:"go"`
}

// Run prints version + build info (styled text or the JSON envelope).
func (VersionCmd) Run(c *Context) error {
	info := versionInfo{Version: Version, Commit: Commit, Date: Date, Go: runtime.Version()}
	return c.Result(info, func() {
		c.KV(
			[2]string{"version", c.Accent(info.Version)},
			[2]string{"commit", info.Commit},
			[2]string{"built", info.Date},
			[2]string{"go", info.Go},
		)
	})
}
