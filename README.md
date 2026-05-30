# m-dev-tools-mcp

**A thin [Model Context Protocol](https://modelcontextprotocol.io) server over
the `m` toolchain.** It introspects the busybox's reflected **`m schema`** (the
aggregated command tree — native `fmt`/`lint`/`test`/… plus the dispatched
`irissync`/`kids-vc` namespaces) and exposes each command as an MCP tool. Every
result is m's `--output json` envelope; every failure is m's deterministic error
object — passed straight through. It is a **wrapper over the reflected surface,
not a parallel one** (spec §5.5), so the agent surface can never drift from the
CLI.

```sh
m-dev-tools-mcp tools --output json    # the tools exposed for a profile (inspection)
m-dev-tools-mcp tools --profile safe   # drop the DB writer (push)
m-dev-tools-mcp serve                  # speak MCP (JSON-RPC 2.0 over stdio) to an agent host
```

The server locates the `m` busybox via `--m-bin`, then `$M_BIN`, then alongside
this binary, then `$PATH`.

## How it works

```
  agent host ──MCP/JSON-RPC over stdio──► m-dev-tools-mcp
                                            │  tools/list ─► `m schema` ─► one tool per command
                                            │  tools/call ─► `m <cmd> … --output json`
                                            ▼
                                          m (busybox) ─► native cmd, or dispatch ─► irissync / kids-vc
```

- **`tools/list`** runs `m schema` and maps each command to an MCP tool: the
  command path becomes the tool name (`kids decompose` → `kids_decompose`), and
  the flags/positionals become a JSON-Schema `inputSchema`.
- **`tools/call`** rebuilds the `m` argv from the tool arguments, appends
  `--output json`, runs `m`, and returns the envelope as the tool result —
  marking a non-zero exit as `isError` so the agent branches on it. m's error
  envelopes (on stderr) are surfaced unchanged.
- **`initialize`** echoes the client's `protocolVersion` and advertises the
  `tools` capability. `ping` and `notifications/initialized` are handled.

## Profiles

`--profile` gates which commands are exposed:

| Profile | Exposes |
|---|---|
| `default` | every request/response command (excludes the non-RPC `lsp`/`watch` servers and the `schema`/`version`/`install-completions` meta commands) |
| `safe` | `default` minus the DB writer (`push`) |
| `all` | everything, including the non-RPC commands (escape hatch) |

## Build, test, CI

Scaffolded from [`go-cli-template`](https://github.com/vista-cloud-dev/go-cli-template)
with a vendored `clikit` (the m-cli convention). Static (`CGO_ENABLED=0`,
`-trimpath`); the only dependency cost is the shared CLI framework — the MCP loop
is pure stdlib. CI is the org `go-ci` reusable workflow (golangci-lint +
schema-contract + cross-compile matrix).

```sh
make test     # CGO_ENABLED=1 go test -race -cover ./...
make lint     # golangci-lint
make build    # static binary → dist/m-dev-tools-mcp
make schema   # the machine surface (this CLI's own tree)
```

## Licensing

Apache-2.0 (`LICENSE`/`NOTICE`), per the toolchain-wide policy; reconciliation is
deferred to project completion.
