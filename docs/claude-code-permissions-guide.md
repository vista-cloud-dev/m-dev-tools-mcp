# Claude Code Permissions Guide

A clean-room, machine-agnostic blueprint for configuring Claude Code permissions so
that **safe, high-frequency work never prompts**, **dangerous work always confirms**,
and **catastrophic work is impossible** — identically on every Linux/Unix/macOS machine
you use.

Two layers, two jobs:

- **The model is twofold.** Universal rules go once in **`~/.claude/settings.json`** (user
  level). Stack-specific rules go in each repo's **`<repo>/.claude/settings.json`** — the
  whole point of separate repos is that they hold different projects in different languages
  that need different permissions.
- **Job 1 — greenfield bootstrap.** Stand up a brand-new machine (no Claude installed)
  with the full config in ~10 minutes.
- **Job 2 — gold standard for auditing.** Diff an *existing* machine against this baseline
  to find drift, dedupe per-repo cruft, and end the "why is it asking me *again*?" loop.

- **Distilled from a real cleanup:** a permissions audit of 63 Claude Code sessions
  (1,353 Bash calls across several repos) — which found that **≈56% of every permission
  prompt was a read-only command** (`ls`/`cat`/`grep`/`find`/read-only `git`) — generalized
  here to any stack. The concrete anti-patterns that audit turned up are folded into
  [§9.6](#96-lessons-learned-from-a-real-cleanup).
- **Verified against (May 2026):** the official Claude Code docs —
  [permissions](https://code.claude.com/docs/en/permissions.md),
  [permission-modes](https://code.claude.com/docs/en/permission-modes.md),
  [settings](https://code.claude.com/docs/en/settings.md),
  [sandboxing](https://code.claude.com/docs/en/sandboxing.md).
- **Anthropic's own starter configs:**
  <https://github.com/anthropics/claude-code/tree/main/examples/settings>.

> **TL;DR**
> 1. Put **universal** rules once in `~/.claude/settings.json` (master) — they then apply
>    in every repo on the machine.
> 2. Put **stack-specific** rules in each repo's committed `.claude/settings.json`
>    (Go ≠ Node ≠ Python ≠ containers).
> 3. Leave `settings.local.json` **gitignored and empty** as the "Yes, don't ask again" sink.
> 4. On current Claude Code, most basic read-only rules (`ls`, `cat`, `grep`, …) are
>    **already built in** and never prompt ([§1.5](#15-built-in-read-only-commands-many-allow-rules-are-redundant)) — the rules that actually buy you
>    silence are the *non*-read-only ones (`git add/commit/push`, build/test, `jq`, `awk`).

---

## Contents

1. [How permissions actually work](#1-how-permissions-actually-work)
2. [The two-layer model](#2-the-two-layer-model)
3. [Greenfield bootstrap (new machine, step by step)](#3-greenfield-bootstrap)
4. [Layer 1 — master user settings (`~/.claude/settings.json`)](#4-layer-1--master-user-settings)
5. [Layer 2 — per-repo, by language / tool](#5-layer-2--per-repo-by-language--tool)
6. [The local sink (`settings.local.json`)](#6-the-local-sink)
7. [Reference: the universal ask & deny lists](#7-reference-the-universal-ask--deny-lists)
8. [Caveats — where prefix matching lies to you](#8-caveats)
9. [Auditing an existing machine against this gold standard](#9-auditing-an-existing-machine)
10. [Optional: lock the deny floor on shared machines](#10-optional-lock-the-deny-floor)
11. [Appendix: complete copy-paste files](#11-appendix)

---

## 1. How permissions actually work

Eight facts. Every recommendation below follows from them; all are from the official docs.

### 1.1 Rule syntax

A rule is `Tool` or `Tool(specifier)`. For shells the specifier is a command pattern.

| Form | Matches |
|---|---|
| `Bash(npm test)` | **only** the exact string `npm test` |
| `Bash(npm test:*)` | `npm test` **and** `npm test -- --watch` (bare command *and* any args) |
| `Bash(npm test *)` | `npm test --watch` but **not** bare `npm test` (space-`*` needs a trailing arg) |
| `Bash(npm *)` | anything starting `npm ` |
| `Bash(* --version)` | anything ending ` --version` — wildcards may appear **anywhere** |

**This guide uses the `:*` (colon-star) form everywhere**, because it matches the command
*with or without* arguments. The space-`*` form is what Claude writes when you click
*"Yes, don't ask again"*; equivalent for the with-args case but misses the bare command.
The `:*` suffix is only special at the **end** — `Bash(git:* push)` treats `:` literally.

> Prefix rules are literal from the **start**. `git log` and `git --no-pager log` are
> *different* prefixes; each needs its own rule.

### 1.2 Compound commands can't smuggle past the rules

Claude Code splits on `&&`, `||`, `;`, `|`, `|&`, `&`, and newlines, then checks **each
sub-command independently**. If `ls:*` is allowed but `rm` is not, `ls && rm x` still
**prompts**. This is what makes a broad read-only allow list safe — an allowed prefix
can't wrap a disallowed command. (PowerShell rules behave the same via AST parsing.)

When you approve a compound command with "don't ask again", Claude saves a **separate
rule per sub-command** (up to 5), not the whole string — so `git status && npm test`
saves a rule for `npm test` alone.

### 1.3 Precedence within the merged ruleset: deny → ask → allow, first match wins

- **Deny always beats allow.** Nothing overrides a deny — not `--allowedTools`, not a hook.
- **Ask always beats allow.** This is the lever: `allow` a broad prefix, `ask` on the
  dangerous variant (allow `git push:*`, ask `git push --force:*`).
- A **bare-tool deny** (`"Bash"`) removes the tool from Claude's context entirely; a
  **scoped deny** (`Bash(rm:*)`) leaves the tool available and blocks only matches.

### 1.4 The file hierarchy

Settings are read from these locations, **merged additively for permissions**, with this
precedence (highest first):

1. **Managed / enterprise** — *cannot* be overridden, even by CLI args
   - Linux/WSL: `/etc/claude-code/managed-settings.json` (+ `…/managed-settings.d/`)
   - macOS: `/Library/Application Support/ClaudeCode/managed-settings.json` (+ `…/managed-settings.d/`)
2. **Command-line args** (`--allowedTools`, `--disallowedTools`, `--add-dir`, …)
3. **Local project** — `<repo>/.claude/settings.local.json` (gitignored, personal)
4. **Shared project** — `<repo>/.claude/settings.json` (committed, team)
5. **User** — `~/.claude/settings.json` (lowest)

The two you maintain by hand are **#5 (user/master)** and **#4 (per-repo)**; #3 is the
auto-filled local sink. Managed (#1) is an optional hard floor for locked machines ([§10](#10-optional-lock-the-deny-floor)).

> **`settings.json` is read only from the repo you launch in — not from parent folders.**
> The docs are explicit: *"settings.json keys load from the current working directory's
> `.claude/` folder with no parent-directory fallback, alongside your user
> `~/.claude/settings.json` and managed settings."* So you can't drop a shared file in a
> folder *above* your repos and have it apply — it would simply never load. That's exactly
> why this guide is twofold: universal rules in the user file, everything else in the repo
> you're actually in. (`--add-dir` / `additionalDirectories` extend *file access* to other
> paths, but never load permission rules.)

Because permissions **merge** rather than override, a deny in your user file still beats an
allow in a repo file. Only deny/ask/allow precedence matters across the merged set, not
which file a rule came from.

### 1.5 Built-in read-only commands (many allow rules are redundant)

Current Claude Code ships a **non-configurable** allowlist of read-only commands that
**never prompt in any mode**:

> `ls`, `cat`, `echo`, `pwd`, `head`, `tail`, `grep`, `find`, `wc`, `which`, `diff`,
> `stat`, `du`, `cd`, and the read-only forms of `git`.

Consequence for this guide: explicit `Bash(ls:*)`, `Bash(cat:*)`, `Bash(grep:*)`, read-only
`git status/log/diff/show`, etc. are **belt-and-suspenders** on a current install — they
don't *hurt*, and they keep the config self-documenting and correct on older versions, but
the rules that actually *remove prompts you're hitting today* are the ones **not** on this
list: text-shaping tools that can write (`sort`, `awk`, `jq`, `cut`, `tr`), and all
mutating commands (`git add/commit/push`, build/test runners, `mkdir`, `chmod`).

Caveat: a read-only command **still prompts** when an *unquoted glob* is present and the
command has write/exec-capable flags — `find`, `sort`, `sed`, `git` — because the glob
could expand to something like `-delete`. So an explicit `Bash(find:*)` allow still earns
its place.

### 1.6 Wrappers: some are stripped, many are not

Before matching, Claude strips a **fixed** set of process wrappers, so `Bash(go test:*)`
also matches `timeout 30 go test`:

> stripped: `timeout`, `time`, `nice`, `nohup`, `stdbuf`, and **bare** `xargs`
> (no flags).

But these are **not** stripped and will defeat your intuition:

- **Environment / exec runners pass their inner command through the wildcard.**
  `direnv exec`, `devbox run`, `mise exec`, `npx`, **`docker exec`**, **`podman exec`**.
  A rule like `Bash(podman exec:*)` matches **whatever comes after `exec`**, including
  `podman exec ctr rm -rf /data`. **Allowing `*exec*`/`*run*` on a runner = trusting
  arbitrary commands inside it.** To stay tight, write the inner command in the rule:
  `Bash(podman exec mycontainer pytest:*)`.
- **`watch`, `setsid`, `ionice`, `flock` always prompt** and cannot be pre-approved by a
  prefix rule.
- **`find -exec` / `find -delete` always prompt** even with `Bash(find:*)` allowed. (This
  is *safer* than older guidance suggested — the destructive `find` forms are not
  auto-approved.)

### 1.7 Path rules for `Read` / `Edit` / `Write`

Gitignore-style globs with **anchored roots**:

| Pattern | Resolves to |
|---|---|
| `Read(//etc/hosts)` | the **absolute** path `/etc/hosts` (note the **double** slash) |
| `Read(/src/**)` | `<project-root>/src/**` (single slash = project root, **not** filesystem root) |
| `Read(~/.ssh/**)` | home dir |
| `Read(src/**)` / `Read(./src/**)` | `<cwd>/src/**` |
| `Read(.env)` ≡ `Read(**/.env)` | a file named `.env` at any depth |
| `Read(//**/.env)` | every `.env` anywhere on the filesystem |

`Edit` rules cover all edit tools; `Read`/`Edit` deny rules also cover the file-reading
Bash commands Claude recognizes (`cat`, `head`, `tail`, `sed`) — but **not** files opened
by an arbitrary subprocess (a Python/Node script). For that you need
[sandboxing](#83-redirection-and-indirect-writes-arent-analyzed).

**Symlinks:** an *allow* applies only if **both** the link and its target match; a *deny*
applies if **either** matches. You cannot dodge a secret-file deny via a symlink.

### 1.8 `defaultMode` — the baseline behavior

| Mode | What it does |
|---|---|
| `default` | Prompt on first use of each tool. **Recommended** with a good allow list. |
| `acceptEdits` | Auto-approves edits + common fs commands (`mkdir`,`touch`,`mv`,`cp`,…) inside the working dir / `additionalDirectories`. |
| `plan` | Read-only exploration; no source edits. |
| `auto` | Auto-approve with a background safety classifier (research preview). Best placed in `~/.claude/settings.json`; complements, never replaces, deny rules. |
| `dontAsk` | Auto-*denies* anything not pre-approved via `permissions.allow` (read-only built-ins still run). A good non-interactive sandbox model. |
| `bypassPermissions` | Skips all prompts except an `rm -rf /` / `rm -rf ~` circuit breaker. **Isolated/throwaway envs only.** |

Keep **`default`**. A strong allow list + `default` = near-zero friction on safe work while
anything new still surfaces. (Advanced alternative: [sandboxing](#83-redirection-and-indirect-writes-arent-analyzed)
with `autoAllowBashIfSandboxed` runs Bash without prompts *inside an OS boundary* — a
different, stronger trade-off than allow-listing.)

---

## 2. The two-layer model

Put each rule at the **broadest scope where it is still safe**. There are exactly two
layers you maintain, plus the auto-filled local sink and an optional machine floor.

```
┌─ LAYER 1 · USER  ~/.claude/settings.json   [every repo on this machine] ────────┐
│   The universal baseline — same on every machine you use.                       │
│   allow: recoverable git + text tools + WebSearch + trusted domains             │
│   ask  : rm / force-push / sudo / curl / installs / outward gh   (universal)    │
│   deny : catastrophic rm + secret files                          (universal)    │
├─ LAYER 2 · PER-REPO  <repo>/.claude/settings.json   [committed, team-shared] ───┤
│   This project's stack — different per repo, because the languages differ.      │
│   allow: this stack's build/test/lint (go / node / python / rust / containers)  │
│   ask  : this repo's destructive targets (make clean, terraform apply, …)       │
└─────────────────────────────────────────────────────────────────────────────────┘
     · LOCAL SINK   <repo>/.claude/settings.local.json   [gitignored, personal]
       left empty; the "Yes, don't ask again" button fills it. Prune periodically. (§6)
     · OPTIONAL FLOOR  managed-settings.json   [root-owned, unoverridable]  (§10)
```

The rule of thumb for *which* layer:

- **Safe (or recoverable, or universally dangerous) on any machine for any project?**
  → Layer 1 (`git add`, `jq`, `rm`→ask, secret-file deny).
- **Specific to this project's language, build, or tooling?** → Layer 2 (`go test`,
  `npm run`, `terraform apply`→ask).

That's it — two files to maintain, no extra tier.

---

## 3. Greenfield bootstrap

A brand-new Linux/Unix/macOS box, no Claude installed. ~10 minutes.

### Step 1 — Install Claude Code

```bash
# Native installer (recommended; self-updating):
curl -fsSL https://claude.ai/install.sh | bash
#   …or via npm if you prefer:
npm install -g @anthropic-ai/claude-code

claude --version        # confirm
claude                  # first run walks you through auth
```

> Verify the current install command at <https://code.claude.com/docs/en/setup> — the
> installer URL is the one thing in this guide most likely to change.

### Step 2 — Lay down the master user settings (Layer 1)

`~/.claude/` is created on first run. Drop in [Appendix A](#a-master--claudesettingsjson)
verbatim:

```bash
mkdir -p ~/.claude
$EDITOR ~/.claude/settings.json     # paste Appendix A
```

This alone kills the bulk of cross-repo prompt fatigue everywhere on the machine.

### Step 3 — Per-repo: add the stack's rules (Layer 2)

In each repo, paste the matching language block from [§5](#5-layer-2--per-repo-by-language--tool) into
`.claude/settings.json` and commit it:

```bash
cd ~/work/some-go-service
mkdir -p .claude
$EDITOR .claude/settings.json        # paste the Go block (or Node/Python/…)
git add .claude/settings.json && git commit -m "chore: add Claude permissions"
```

### Step 4 — Gitignore the local sink (per repo or globally)

```bash
# Per repo:
printf '\n# Claude Code — personal/machine-local permissions\n.claude/settings.local.json\n' \
  >> .gitignore

# …or once for the whole machine:
git config --global core.excludesFile ~/.config/git/ignore
printf '.claude/settings.local.json\n' >> ~/.config/git/ignore
```

Keep `.claude/settings.json` **committed** (shared); keep `.claude/settings.local.json`
**ignored** (personal). Done.

---

## 4. Layer 1 — master user settings

`~/.claude/settings.json` applies to **every repo on this machine**. It holds the
*universal* rules: things that are safe (or recoverable, or universally dangerous)
regardless of stack. This is the single most important file — get it identical on every
machine and most prompt fatigue is gone.

What goes here, and why:

| Category | Examples | Rationale |
|---|---|---|
| Text tools that can write | `jq`, `awk`, `sort`, `cut`, `tr`, `column` | high-frequency, not in the built-in read-only set ([§1.5](#15-built-in-read-only-commands-many-allow-rules-are-redundant)) |
| Recoverable git mutations | `git add/commit/mv/stash/push`, `git restore --staged` | local & revertable; you do these constantly |
| Recoverable fs ops | `mkdir`, `touch`, `chmod +x` | create-only; can't destroy data |
| Read-only web | `WebSearch`, `WebFetch(domain:…)` for trusted docs | research without prompts; **never** `WebFetch(domain:*)` |
| Universal **ask** | `rm`, force-push, `sudo`, `curl`/`wget`, installs, outward `gh` | dangerous everywhere → confirm everywhere ([§7](#7-reference-the-universal-ask--deny-lists)) |
| Universal **deny** | catastrophic `rm`, secret files | irreversible / exfiltration risk ([§7](#7-reference-the-universal-ask--deny-lists)) |

The basic read-only utilities (`ls`/`cat`/`grep`/read-only `git`/…) are included in
Appendix A for explicitness and old-version safety, but per [§1.5](#15-built-in-read-only-commands-many-allow-rules-are-redundant) they're already free
on a current install.

> **`git checkout:*` and `git push:*` are judgment calls.** `git checkout -- <file>`
> discards uncommitted edits; `git push:*` is outward-facing (paired with an `ask` on
> `--force`). They're in Appendix A because they're extremely frequent and the blast
> radius is bounded — but if you want them gated, move them from `allow` to `ask`.
> Likewise `gh api:*` is allowed for convenience and **also auto-approves `gh api -X POST/DELETE`**
> (which can create/delete repos) — drop it to `ask` if that trade-off bothers you.

Full file: [Appendix A](#a-master--claudesettingsjson).

---

## 5. Layer 2 — per-repo, by language / tool

Each block below is a **complete, standalone, committed `.claude/settings.json`** for a
stack. It assumes Layer 1 is in place, so it only adds what's stack-specific. Drop the
matching block into the repo's `.claude/` directory and commit it.

These run **your repo's own code** (`make`, `npm run`, `go test`, …). That's appropriate
for repos you own; reconsider before allow-listing them in a third-party clone ([§8.4](#84-runners-execute-your-repos-code)).

### Go

```json
{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "permissions": {
    "allow": [
      "Bash(go build:*)", "Bash(go test:*)", "Bash(go vet:*)", "Bash(go run:*)",
      "Bash(go fmt:*)", "Bash(gofmt:*)", "Bash(go generate:*)", "Bash(go list:*)",
      "Bash(go env:*)", "Bash(go version)", "Bash(go mod tidy)",
      "Bash(go mod download)", "Bash(go mod verify)", "Bash(go mod why:*)",
      "Bash(golangci-lint run:*)", "Bash(staticcheck:*)",
      "Bash(make build:*)", "Bash(make test:*)", "Bash(make lint:*)",
      "Bash(make run:*)", "Bash(make tidy)", "Bash(make schema:*)",
      "Bash(make dist:*)", "Bash(make all)"
    ],
    "ask": [
      "Bash(make clean:*)"
    ]
  }
}
```

> `go install`/`go get` stay in the **master ask** list (they fetch + run code). `make clean`
> deletes build output → `ask`. Matches this repo's Makefile targets
> (`all build clean dist lint run schema test tidy`).

### Node / TypeScript

```json
{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "permissions": {
    "allow": [
      "Bash(npm run:*)", "Bash(npm test:*)", "Bash(npm ci)",
      "Bash(pnpm run:*)", "Bash(pnpm test:*)", "Bash(pnpm install --frozen-lockfile)",
      "Bash(yarn test:*)", "Bash(yarn build:*)",
      "Bash(node:*)", "Bash(tsc:*)", "Bash(npx tsc:*)",
      "Bash(npx vitest:*)", "Bash(npx jest:*)",
      "Bash(npx eslint:*)", "Bash(npx prettier --check:*)"
    ]
  }
}
```

> `npm ci` / `pnpm install --frozen-lockfile` install from the committed lockfile
> (deterministic) → allow. `npm install`/`npm i` can mutate the lockfile and run install
> scripts → stays in **master ask**. Remember `npx <x>` passes `<x>` through ([§1.6](#16-wrappers-some-are-stripped-many-are-not)), so
> `Bash(npx:*)` would be over-broad — list specific `npx` tools as above.

### Python

```json
{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "permissions": {
    "allow": [
      "Bash(pytest:*)", "Bash(python -m pytest:*)", "Bash(python3 -m pytest:*)",
      "Bash(python -m py_compile:*)", "Bash(python3 -m py_compile:*)",
      "Bash(ruff check:*)", "Bash(ruff format --check:*)",
      "Bash(mypy:*)", "Bash(pyright:*)",
      "Bash(uv run:*)", "Bash(uv sync)", "Bash(uv lock --check)",
      "Bash(tox:*)", "Bash(nox:*)"
    ]
  }
}
```

> `pip install` / `uv pip install` / `uv add` stay in **master ask**. `uv run` executes
> project code (like `make`) → fine for your own repo.

### Rust

```json
{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "permissions": {
    "allow": [
      "Bash(cargo build:*)", "Bash(cargo test:*)", "Bash(cargo check:*)",
      "Bash(cargo clippy:*)", "Bash(cargo fmt:*)", "Bash(cargo doc:*)",
      "Bash(cargo run:*)", "Bash(cargo bench:*)", "Bash(rustc --version)"
    ]
  }
}
```

> `cargo install` stays in **master ask**.

### JVM (Maven / Gradle)

```json
{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "permissions": {
    "allow": [
      "Bash(mvn test:*)", "Bash(mvn verify:*)", "Bash(mvn compile:*)", "Bash(mvn package:*)",
      "Bash(./gradlew test:*)", "Bash(./gradlew build:*)", "Bash(./gradlew check:*)",
      "Bash(gradle test:*)", "Bash(gradle build:*)"
    ]
  }
}
```

### Containers — read-only / inspection

For repos where you only ever *inspect* containers.

```json
{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "permissions": {
    "allow": [
      "Bash(docker ps:*)", "Bash(docker images:*)", "Bash(docker inspect:*)",
      "Bash(docker info:*)", "Bash(docker version)", "Bash(docker logs:*)",
      "Bash(docker context ls)", "Bash(docker context show)",
      "Bash(podman ps:*)", "Bash(podman images:*)", "Bash(podman image ls:*)",
      "Bash(podman inspect:*)", "Bash(podman info:*)", "Bash(podman logs:*)",
      "Bash(podman version)", "Bash(podman --version)"
    ]
  }
}
```

### Containers — dev sandbox (allow with eyes open)

For a repo whose container *is* the dev environment. **`docker exec`/`podman exec`/`run`
pass their inner command through the wildcard ([§1.6](#16-wrappers-some-are-stripped-many-are-not)) — allowing `*exec:*` trusts
arbitrary commands inside the container.** Acceptable when the container is disposable;
otherwise scope to specific inner commands. Destructive container ops are gated behind `ask`.

```json
{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "permissions": {
    "allow": [
      "Bash(docker exec:*)", "Bash(docker run:*)", "Bash(docker cp:*)",
      "Bash(docker build:*)", "Bash(docker compose:*)",
      "Bash(podman exec:*)", "Bash(podman run:*)", "Bash(podman cp:*)",
      "Bash(podman build:*)", "Bash(podman compose:*)", "Bash(podman pull:*)"
    ],
    "ask": [
      "Bash(docker rm:*)", "Bash(docker rmi:*)", "Bash(docker system prune:*)",
      "Bash(docker image prune:*)", "Bash(docker volume prune:*)",
      "Bash(podman rm:*)", "Bash(podman rmi:*)", "Bash(podman image rm:*)",
      "Bash(podman image prune:*)", "Bash(podman container prune:*)",
      "Bash(podman system prune:*)", "Bash(podman volume rm:*)",
      "Bash(podman volume prune:*)", "Bash(podman machine rm:*)"
    ]
  }
}
```

### Infrastructure as Code (Terraform)

```json
{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "permissions": {
    "allow": [
      "Bash(terraform fmt:*)", "Bash(terraform validate:*)", "Bash(terraform plan:*)",
      "Bash(terraform init:*)", "Bash(terraform providers:*)", "Bash(tflint:*)"
    ],
    "ask": [
      "Bash(terraform apply:*)", "Bash(terraform destroy:*)",
      "Bash(terraform import:*)", "Bash(terraform state rm:*)"
    ]
  }
}
```

### A project that ships its own CLI

Many repos build a command-line tool you then drive (`mytool`, a `bin/` script). Allow its
**read/inspect** subcommands; gate **state-changing** verbs behind `ask`. Crucially, do
**not** allow a bare `Bash(mytool:*)` if the tool has write subcommands (`init`, `apply`,
`deploy`) — the wildcard would cover those too. List the safe subcommands explicitly.

```json
{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "permissions": {
    "allow": [
      "Bash(mytool status:*)", "Bash(mytool list:*)", "Bash(mytool doctor:*)",
      "Bash(mytool logs:*)", "Bash(mytool config get:*)", "Bash(mytool version)",
      "Bash(make verify:*)", "Bash(make check:*)", "Bash(make help:*)", "Bash(make -n:*)"
    ],
    "ask": [
      "Bash(mytool init:*)", "Bash(mytool apply:*)", "Bash(mytool deploy:*)",
      "Bash(mytool bootstrap:*)", "Bash(mytool recreate:*)", "Bash(mytool db:*)"
    ]
  }
}
```

> `make -n` is a dry run (safe). The lesson from auditing real CLIs: a single
> `Bash(toolname:*)` allow silently green-lights every destructive subcommand the tool
> grows later — enumerate the read verbs instead, and let new ones prompt.

> **Mixed-language repo?** Just merge the relevant blocks' `allow`/`ask` arrays into one
> `.claude/settings.json`. There's no conflict — permissions are additive.

---

## 6. The local sink

`<repo>/.claude/settings.local.json` is **personal and gitignored**. Leave it for the
*"Yes, don't ask again"* button and machine-specific one-offs.

- **Keep it out of git** ([Step 4](#step-4--gitignore-the-local-sink-per-repo-or-globally)) — it holds your paths and machine quirks.
- **Prune it periodically.** This is where one-shot noise accumulates: a giant one-off
  `printf`/`awk` literal, a `…/tasks/<id>.output` path, a `/tmp/probe` — none of which will
  ever match again. Delete anything that's not a stable, recurring command.
- **Promote, don't duplicate.** If a rule shows up in *several* repos' local files, it's
  universal — move it up to Layer 1 and delete the copies. If it's stack-specific, move it
  into that repo's committed `settings.json`. The [§9](#9-auditing-an-existing-machine) audit finds these.

---

## 7. Reference: the universal ask & deny lists

These live in **Layer 1** (`~/.claude/settings.json`), or the [managed floor](#10-optional-lock-the-deny-floor) if
you want them unoverridable. `ask` overrides `allow`, so you can allow a broad prefix and
still confirm the dangerous variant.

### Always-ask (confirm every time)

```text
# filesystem destruction
Bash(rm:*)  Bash(rmdir:*)  Bash(git rm:*)  Bash(git clean:*)

# git history / force-push  (pair with allowing git push:* and git branch:*)
Bash(git push --force:*)  Bash(git push -f:*)  Bash(git push --force-with-lease:*)
Bash(git reset --hard:*)  Bash(git rebase:*)  Bash(git filter-branch:*)
Bash(git branch -D:*)  Bash(git tag -d:*)  Bash(git reflog expire:*)  Bash(git gc:*)

# privilege / processes / recursive perms
Bash(sudo:*)  Bash(kill:*)  Bash(pkill:*)  Bash(killall:*)  Bash(chmod -R:*)  Bash(chown:*)

# network fetch & pipe-to-shell  (can download/run code or exfiltrate)
Bash(curl:*)  Bash(wget:*)

# software installs (supply-chain surface)
Bash(brew install:*)  Bash(brew tap:*)  Bash(brew uninstall:*)
Bash(npm install:*)  Bash(npm i:*)  Bash(npm uninstall:*)  Bash(pnpm add:*)
Bash(pip install:*)  Bash(pip3 install:*)  Bash(uv pip install:*)  Bash(uv add:*)
Bash(go install:*)  Bash(go get:*)  Bash(cargo install:*)  Bash(gem install:*)  Bash(pipx install:*)

# outward-facing GitHub mutations
Bash(gh pr create:*)  Bash(gh pr merge:*)  Bash(gh pr close:*)
Bash(gh repo create:*)  Bash(gh repo delete:*)
Bash(gh release create:*)  Bash(gh release delete:*)  Bash(gh secret:*)
```

> Tip: for a project that hammers a *local* API, add the loopback form to that repo's
> allow list — `Bash(curl http://localhost:*)` / `Bash(curl http://127.0.0.1:*)` — rather
> than allowing `curl:*` globally. (Argument-constraining curl rules are fragile; see [§8.5](#85-argument-constraining-rules-are-fragile).)

### Deny (never — the hard floor)

```text
# catastrophic / irreversible  (everyday rm is ask, above; these are the unrecoverable extremes)
Bash(rm -rf /)  Bash(rm -rf /*)  Bash(rm -rf ~)  Bash(rm -rf ~/*)
Bash(rm -rf $HOME)  Bash(rm -rf $HOME/*)  Bash(rm --no-preserve-root:*)
Bash(sudo rm:*)  Bash(dd:*)  Bash(mkfs:*)  Bash(mkfs.*:*)

# secret protection — deny can't be overridden, and matches via symlink-or-target
Read(**/.env*)
Read(**/*.pem)  Read(**/*.key)  Read(**/*.p12)
Read(**/id_rsa)  Read(**/id_ed25519)  Read(**/.npmrc)  Read(**/.pgpass)
Read(~/.ssh/**)  Read(~/.aws/**)  Read(~/.config/gcloud/**)
Read(**/secrets/**)  Read(**/credentials)
```

> `Read(**/.env*)` blocks **every** dotenv variant, including the harmless
> `.env.example` / `.env.sample` templates. Since deny can't be overridden, if you need
> Claude to read a template either rename it without the leading dot (`env.example`) or
> drop that one rule. Add `Edit(**/.env*)` / `Write(**/.env*)` if you also want to block
> creating/clobbering dotenv files (that also stops Claude scaffolding a new `.env`).

---

## 8. Caveats

Bash allow-listing is prefix matching, not semantic analysis. Internalize these or the
allow list will surprise you.

### 8.1 In-place / escape-hatch flags
A prefix allow covers a command's destructive modes too: `sed -i`, `cp` overwrites,
`tar --delete`. The safety net is the deny list (irreversible cases) plus the fact that
you review output. Good news from [§1.6](#16-wrappers-some-are-stripped-many-are-not): `find -delete`/`-exec` and `watch`/`flock`
are **not** auto-approved — they always prompt.

### 8.2 Runners pass their payload through
`podman exec`, `docker exec`, `npx`, `devbox run`, `mise exec`, `direnv exec` are **not**
stripped wrappers — a `Bash(podman exec:*)` allow matches *any* inner command. Scope them
to the inner command (`Bash(podman exec api pytest:*)`) unless the target is disposable.

### 8.3 Redirection and indirect writes aren't analyzed
`Bash(echo:*)` matches `echo x > important.txt`; the rule sees `echo …`, not the
`>`-clobber. And Read/Edit deny rules don't cover files a Python/Node subprocess opens
itself. This is why [§7](#7-reference-the-universal-ask--deny-lists) protects secret *files* explicitly — and why, for true OS-level
enforcement, you enable **[sandboxing](https://code.claude.com/docs/en/sandboxing.md)**
(filesystem + network boundaries that apply to Bash *and its children*, merged with your
Read/Edit deny and `WebFetch` rules). With `autoAllowBashIfSandboxed: true` (default),
sandboxed Bash runs without prompts inside the boundary — a strong alternative to broad
allow lists for the "stop asking me" goal.

### 8.4 Runners execute your repo's code
`make`, `npm run`, `node`, `python`, `uv run`, `cargo run` run arbitrary code from the
project. Allowing them = trusting *that repo's* scripts. Appropriate for your own repos;
reconsider in third-party clones.

### 8.5 Argument-constraining rules are fragile
`Bash(curl http://github.com/ *)` does *not* reliably restrict curl to GitHub — it misses
`curl -X GET http://github.com`, `https://…`, redirects, `URL=… && curl $URL`, and extra
spaces. For real URL control, deny `curl`/`wget` and use `WebFetch(domain:…)`, or a
PreToolUse hook. Don't lean on argument patterns for security.

### 8.6 `auto` mode complements, never replaces, deny
`defaultMode: "auto"` adds a background classifier, but it's a research preview and sits
*on top of* the rules. The deny list is still your floor.

---

## 9. Auditing an existing machine

The reverse workflow: you already have Claude on a box with years of accreted, drifted
permissions, and you want to pull it up to this gold standard. Work top-down.

### 9.1 Inventory every settings file

```bash
# user + managed
ls -la ~/.claude/settings.json /etc/claude-code/managed-settings.json \
       "/Library/Application Support/ClaudeCode/managed-settings.json" 2>/dev/null

# every project file under your code roots
find ~/work ~/dev ~/src -type f \
     \( -name 'settings.json' -o -name 'settings.local.json' \) \
     -path '*/.claude/*' 2>/dev/null
```

Inside a repo, `/permissions` shows the **effective merged** ruleset *and which file each
rule came from* — the fastest way to see what's actually in force.

### 9.2 Diff the master against the gold standard

```bash
# normalize + sort both, then diff:
diff <(jq -S '.permissions' ~/.claude/settings.json) \
     <(jq -S '.permissions' /path/to/appendix-A.json)
```

Anything the gold standard has that you don't → add. Anything you have that's a one-shot
or over-broad (bare `"Bash"`, `WebFetch(domain:*)`) → fix.

### 9.3 Diff a repo against its language block

```bash
# compare a repo's committed file to the canonical Go block (saved as go-block.json):
diff <(jq -S '.permissions' .claude/settings.json) \
     <(jq -S '.permissions' /path/to/go-block.json)
```

### 9.4 Find duplication to promote, and noise to prune

```bash
# rules that appear in MANY repo files probably belong in Layer 1:
find ~/work -path '*/.claude/settings*.json' -exec sh -c \
  'jq -r ".permissions | (.allow//[])+(.ask//[])+(.deny//[]) | .[]" "$1"' _ {} \; \
  2>/dev/null | sort | uniq -c | sort -rn | head -40
```

High counts that are *universal* → promote to Layer 1 and delete the per-repo copies.
High counts that are *stack-specific* → make sure each such repo has the language block.
Then open each `settings.local.json` and delete one-shot literals ([§6](#6-the-local-sink)).

### 9.5 Harvest real prompts from your transcripts
Run the **`/fewer-permission-prompts`** skill: it scans your session transcripts for the
read-only Bash/MCP calls you keep approving and proposes an allowlist — a data-driven way
to top up Layers 1 and 2 from what *this* machine actually does. (A run like this — 63
sessions, 1,353 Bash calls — is what produced the baseline in this guide.)

### 9.6 Lessons learned from a real cleanup
The recurring anti-patterns a real audit (63 sessions across several repos) turned up.
Check for each on an existing machine — they're the highest-value fixes:

- **Bare `"Bash"` in an allow list.** Every shell command then runs silently unless an
  ask/deny catches it — so `curl … | sh`, `sudo …`, and anything unforeseen execute
  *unseen*. Replace it with the explicit Layer 1 + Layer 2 allow lists so anything new
  **prompts**. (Reserve bare `"Bash"` only for a deliberately throwaway, non-interactive
  `dontAsk` sandbox — never an interactive repo.)
- **`"defaultMode": "auto"` in a *project* file is a no-op.** Only `~/.claude/settings.json`
  (and managed) is honored for mode, and only on a supporting model. Delete it from repo
  files; set it once at the user level if you actually want it.
- **The same rule re-granted across many repos.** `WebSearch`, `git add/commit/push`,
  read-only git, trusted `WebFetch` domains, copied into repo after repo. Anything
  *universal* belongs in Layer 1 exactly once — promote it and delete the duplicates
  ([§9.4](#94-find-duplication-to-promote-and-noise-to-prune)).
- **One-shot noise accumulating in `settings.local.json`.** Giant one-off `printf`/`awk`
  literals, `…/tasks/<id>.output` paths, `/tmp` probes — none will ever match again. Prune
  them ([§6](#6-the-local-sink)).
- **An unignored local file leaking personal paths.** If `.claude/settings.local.json`
  isn't gitignored, your machine-specific paths get committed into the shared repo. Add it
  to `.gitignore` (or a global `~/.config/git/ignore`) — [Step 4](#step-4--gitignore-the-local-sink-per-repo-or-globally).
- **Good destructive-`ask` rules trapped at the repo level.** A solid `rm`/force-push/prune
  ask list living in one repo protects only that repo. The *universal* gates belong in
  Layer 1 ([§7](#7-reference-the-universal-ask--deny-lists)); keep only the repo-specific
  destroyers (e.g. `terraform destroy`, `make clean`, container prune) in Layer 2.

---

## 10. Optional: lock the deny floor

*Not part of the two-layer model — only for shared, regulated, or single-purpose machines.*

If you administer a machine where users must not be able to loosen the safety floor,
deploy a **managed settings** file that **no user or project setting can override**
([§1.3](#13-precedence-within-the-merged-ruleset-deny--ask--allow-first-match-wins)). Requires root.

- Linux/WSL: `/etc/claude-code/managed-settings.json`
- macOS: `/Library/Application Support/ClaudeCode/managed-settings.json`

Keep it **minimal** — only the hard floor, so it never fights day-to-day work:

```json
{
  "permissions": {
    "deny": [
      "Bash(rm -rf /)", "Bash(rm -rf /*)", "Bash(rm -rf ~)", "Bash(rm -rf ~/*)",
      "Bash(rm -rf $HOME)", "Bash(rm -rf $HOME/*)", "Bash(rm --no-preserve-root:*)",
      "Bash(sudo rm:*)", "Bash(dd:*)", "Bash(mkfs:*)", "Bash(mkfs.*:*)",
      "Read(**/.env*)", "Read(**/*.pem)", "Read(**/*.key)", "Read(**/*.p12)",
      "Read(**/id_rsa)", "Read(**/id_ed25519)", "Read(**/.npmrc)", "Read(**/.pgpass)",
      "Read(~/.ssh/**)", "Read(~/.aws/**)", "Read(~/.config/gcloud/**)",
      "Read(**/secrets/**)", "Read(**/credentials)"
    ],
    "disableBypassPermissionsMode": "disable"
  }
}
```

> On a personal laptop you don't need this — the same deny list living in
> `~/.claude/settings.json` (Layer 1, Appendix A) already protects you. Managed settings
> only add value when the floor must survive a user editing their own files.

---

## 11. Appendix

### A. Master — `~/.claude/settings.json`

```json
{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "permissions": {
    "defaultMode": "default",
    "allow": [
      "Bash(ls:*)", "Bash(cat:*)", "Bash(head:*)", "Bash(tail:*)", "Bash(wc:*)",
      "Bash(grep:*)", "Bash(rg:*)", "Bash(find:*)", "Bash(tree:*)",
      "Bash(echo:*)", "Bash(printf:*)", "Bash(pwd)", "Bash(date:*)",
      "Bash(which:*)", "Bash(command -v:*)", "Bash(type:*)", "Bash(file:*)", "Bash(stat:*)",
      "Bash(sort:*)", "Bash(uniq:*)", "Bash(cut:*)", "Bash(tr:*)", "Bash(column:*)",
      "Bash(jq:*)", "Bash(yq:*)", "Bash(awk:*)", "Bash(sed -n:*)",
      "Bash(diff:*)", "Bash(comm:*)",
      "Bash(realpath:*)", "Bash(dirname:*)", "Bash(basename:*)", "Bash(du:*)", "Bash(df:*)",
      "Bash(env)", "Bash(printenv:*)", "Bash(uname:*)", "Bash(hostname)", "Bash(id)",
      "Bash(mkdir:*)", "Bash(touch:*)", "Bash(chmod +x:*)",

      "Bash(git status:*)", "Bash(git --no-pager status:*)",
      "Bash(git log:*)", "Bash(git --no-pager log:*)",
      "Bash(git diff:*)", "Bash(git --no-pager diff:*)",
      "Bash(git show:*)", "Bash(git --no-pager show:*)",
      "Bash(git branch:*)", "Bash(git rev-parse:*)", "Bash(git rev-list:*)",
      "Bash(git ls-files:*)", "Bash(git ls-remote:*)", "Bash(git remote -v)",
      "Bash(git remote get-url:*)", "Bash(git describe:*)", "Bash(git blame:*)",
      "Bash(git shortlog:*)", "Bash(git tag -l:*)", "Bash(git fetch:*)",
      "Bash(git check-ignore:*)", "Bash(git stash list:*)", "Bash(git config --get:*)",

      "Bash(git add:*)", "Bash(git commit:*)", "Bash(git mv:*)",
      "Bash(git stash:*)", "Bash(git restore --staged:*)",
      "Bash(git switch:*)", "Bash(git checkout:*)", "Bash(git push:*)",

      "Bash(gh api:*)", "Bash(gh pr view:*)", "Bash(gh pr list:*)",
      "Bash(gh pr diff:*)", "Bash(gh pr checks:*)", "Bash(gh run view:*)",
      "Bash(gh run list:*)", "Bash(gh issue view:*)", "Bash(gh issue list:*)",

      "WebSearch",
      "WebFetch(domain:github.com)", "WebFetch(domain:raw.githubusercontent.com)",
      "WebFetch(domain:code.claude.com)", "WebFetch(domain:docs.claude.com)",
      "WebFetch(domain:pkg.go.dev)", "WebFetch(domain:developer.mozilla.org)",
      "WebFetch(domain:docs.python.org)", "WebFetch(domain:stackoverflow.com)",

      "Skill(update-config)"
    ],
    "ask": [
      "Bash(rm:*)", "Bash(rmdir:*)", "Bash(git rm:*)", "Bash(git clean:*)",
      "Bash(git push --force:*)", "Bash(git push -f:*)", "Bash(git push --force-with-lease:*)",
      "Bash(git reset --hard:*)", "Bash(git rebase:*)", "Bash(git filter-branch:*)",
      "Bash(git branch -D:*)", "Bash(git tag -d:*)",
      "Bash(git reflog expire:*)", "Bash(git gc:*)",
      "Bash(sudo:*)", "Bash(kill:*)", "Bash(pkill:*)", "Bash(killall:*)",
      "Bash(chmod -R:*)", "Bash(chown:*)",
      "Bash(curl:*)", "Bash(wget:*)",
      "Bash(brew install:*)", "Bash(brew tap:*)", "Bash(brew uninstall:*)",
      "Bash(npm install:*)", "Bash(npm i:*)", "Bash(npm uninstall:*)", "Bash(pnpm add:*)",
      "Bash(pip install:*)", "Bash(pip3 install:*)", "Bash(uv pip install:*)", "Bash(uv add:*)",
      "Bash(go install:*)", "Bash(go get:*)", "Bash(cargo install:*)",
      "Bash(gem install:*)", "Bash(pipx install:*)",
      "Bash(gh pr create:*)", "Bash(gh pr merge:*)", "Bash(gh pr close:*)",
      "Bash(gh repo create:*)", "Bash(gh repo delete:*)",
      "Bash(gh release create:*)", "Bash(gh release delete:*)", "Bash(gh secret:*)"
    ],
    "deny": [
      "Bash(rm -rf /)", "Bash(rm -rf /*)", "Bash(rm -rf ~)", "Bash(rm -rf ~/*)",
      "Bash(rm -rf $HOME)", "Bash(rm -rf $HOME/*)", "Bash(rm --no-preserve-root:*)",
      "Bash(sudo rm:*)", "Bash(dd:*)", "Bash(mkfs:*)", "Bash(mkfs.*:*)",
      "Read(**/.env*)",
      "Read(**/*.pem)", "Read(**/*.key)", "Read(**/*.p12)",
      "Read(**/id_rsa)", "Read(**/id_ed25519)", "Read(**/.npmrc)", "Read(**/.pgpass)",
      "Read(~/.ssh/**)", "Read(~/.aws/**)", "Read(~/.config/gcloud/**)",
      "Read(**/secrets/**)", "Read(**/credentials)"
    ]
  }
}
```

> Notes: this baseline adds `git switch` (safer branch-switch than `checkout`), `sed -n`
> (read-only sed), `yq`, environment probes (`env`/`printenv`/`uname`), and read-only `gh`
> subcommands (`pr view/list/diff/checks`, `run view/list`, `issue view/list`), with generic
> doc domains you can swap for your own. `gh api:*` is retained with the
> [§4](#4-layer-1--master-user-settings) trade-off (it also auto-approves `gh api -X POST/DELETE`).
> The basic read-only utilities are kept for explicitness though
> [§1.5](#15-built-in-read-only-commands-many-allow-rules-are-redundant) makes them redundant on current versions.

### B. Per-repo blocks
The Go / Node / Python / Rust / JVM / container / Terraform / own-CLI blocks are in
[§5](#5-layer-2--per-repo-by-language--tool). Each is a complete committed `.claude/settings.json`; merge their arrays
for a mixed-language repo.

### C. Local — `<repo>/.claude/settings.local.json`
Leave for the *"Yes, don't ask again"* button. Gitignored. Prune one-shots ([§6](#6-the-local-sink)).
