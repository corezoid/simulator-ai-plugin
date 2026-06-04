# AGENTS.md

Guidance for coding agents (Codex, Claude Code, etc.) working in the **simulator-ai-plugin**
repository. Humans: see the root [`README.md`](README.md) (usage) and
[`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) (design).

## What this repo is

A plugin for Claude Code and Codex that connects the **Simulator.Company** platform
(backend: `pong-server` / `control-api`) to the host via MCP. It bundles:

- a **Go MCP server** (`plugins/simulator/mcp-server/`) that exposes the Simulator
  `/papi/1.0` public API as a **curated, typed set of ~46 MCP tools** (declared in Go, not a
  generic spec passthrough), scoped to the core scenarios;
- **7 skills** (`plugins/simulator/skills/`) — markdown that teaches the model the
  platform's entity model and common workflows.

Read [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) before making non-trivial changes.

## Layout (the parts you'll touch)

```
plugins/simulator/mcp-server/   Go MCP server (Go 1.24+, mark3labs/mcp-go)
  cmd/server/                   entry point: profile → apiclient → tools → stdio
  cmd/gendiscovery/             regenerate public/ discovery artifacts
  cmd/evalrunner/               behavioural eval (opt-in; needs ANTHROPIC_API_KEY)
  internal/config/              local/prod profiles (env + profiles.json overridable)
  internal/apiclient/           HTTP client: base URL, auth header, accId, timeouts, errors
  internal/tools/               curated typed Operation registry (op.go) + per-domain files
    testdata/                   papi-openapi.json (drift gate) + eval-scenarios.json
  internal/engines/             graph sync, layout, prune, placements, upload, chart
  app/auth/                     OAuth2 PKCE + .env credential storage
plugins/simulator/skills/       7 skills (markdown only, ship with the plugin)
plugins/simulator/docs/         entity & user-flow reference (ships with the plugin)
docs/                           contributor docs (ARCHITECTURE.md, INTEGRATION.md) — repo-level
public/                         generated AI-discovery artifacts (do not hand-edit)
```

## Commands

All `make` targets run from the repo root (recipes `cd` into the module):

```bash
make build         # go build ./...
make vet           # go vet ./...
make test          # go test ./...   — config, apiclient, tools (scenarios, -race, drift, eval), engines
make discovery     # regenerate public/llms.txt + public/.well-known/skills/index.json
make run-local     # go run ./cmd/server --profile local   (dev pong-server :9000)
make run-prod      # go run ./cmd/server --profile prod
make eval          # behavioural eval — drives a model through eval-scenarios.json (opt-in)
```

Run the server directly (no build step — hosts use `go run ./cmd/server`):

```bash
cd plugins/simulator/mcp-server
SIMULATOR_DEBUG=1 go run ./cmd/server --profile local
```

Before committing Go changes, run `make build && make vet && make test`.

## Conventions & gotchas

- **No build step ships.** Hosts launch the server with `go run ./cmd/server`. Don't add a
  compiled binary to the repo.
- **Tools are declared in Go, curated.** Each domain file under `internal/tools/`
  (`forms.go`, `actors.go`, …) declares a slice of typed `Operation`s; `build.go` registers
  them. The server exposes ~46 tools, not the full REST surface — keep it curated.
- **Drift gate.** `internal/tools/drift_test.go` validates every declared op (method, path,
  operationId) against `internal/tools/testdata/papi-openapi.json` (the backend contract,
  dumped by pong-server `yarn dump-openapi`). After changing tools or the backend, refresh
  that spec and run `go test ./internal/tools/ -run TestSpecDrift`.
- **operationIds live at the backend source.** `pong-server` declares them on its `/papi`
  route schemas; the plugin matches them. Keep names in sync across both repos.
- **`public/` is generated.** Edit `SKILL.md` frontmatter, then `make discovery`.
- **Entity/user-flow docs must stay under `plugins/simulator/docs/`.** Skills reference them
  as `$CLAUDE_PLUGIN_ROOT/docs/...` and only `plugins/simulator/` is copied on install.
  Contributor/architecture docs go in the repo-root `docs/`.
- **Versioning.** The plugin version appears in `.claude-plugin/{plugin,marketplace}.json`,
  `.agents/plugins/marketplace.json` and `CHANGELOG.md` — bump them together.
- **TLS is verified by default.** `--insecure` is only for self-signed on-prem gateways.
- **Graph sync** (`internal/engines/sync_graph.go` + `push_graph.go`, ~1.5k LOC) is the most
  delicate logic — change it carefully; dedicated unit tests are still a backlog item.

## Adding a curated MCP tool

1. Add an `Operation` to the relevant `internal/tools/<domain>.go` slice (method, path
   template, typed params), matching the backend's `operationId`.
2. `make build && make vet && make test` (the drift gate confirms it matches the backend).
3. Document it in the root [`README.md`](README.md) tool table and
   [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) §4; consider a line in
   `internal/tools/testdata/eval-scenarios.json`.

For a multi-call/computational tool, add it under `internal/engines/` and register it in
`engines/register.go` instead.

## Security

This repo touches OAuth tokens (stored in `.env`, mode `0600`). Never log token values,
never commit a `.env`, and keep TLS verification on by default.
