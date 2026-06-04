# AGENTS.md

Guidance for coding agents (Codex, Claude Code, etc.) working in the **simulator-ai-plugin**
repository. Humans: see the root [`README.md`](README.md) (usage) and
[`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) (design).

## What this repo is

A plugin for Claude Code and Codex that connects the **Simulator.Company** platform to the
host via MCP. It bundles:

- a **Go MCP server** (`plugins/simulator/mcp-server/`) that exposes the Simulator
  `/papi/1.0` REST API as MCP tools — the full surface (**185 operations**) plus 11
  hand-written convenience tools;
- **7 skills** (`plugins/simulator/skills/`) — markdown that teaches the model the
  platform's entity model and common workflows.

Read [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) before making non-trivial changes.

## Layout (the parts you'll touch)

```
plugins/simulator/mcp-server/   Go MCP server (Go 1.24+)
  main.go                       flags, run modes, .env bootstrap
  specs.go                      //go:embed of swagger/sim-public-swagger.full.json
  app/mcp-server/               the Swagger→MCP bridge + the 11 custom tools
  app/auth/                     OAuth2 PKCE + .env credential storage
  app/{swagger,models}/         spec loader + OpenAPI types
  cmd/{enrichspec,gendiscovery} regen tools (spec / discovery artifacts)
  swagger/                      curated reuse source (80) + embedded full (185) specs
plugins/simulator/skills/       7 skills (markdown only, ship with the plugin)
plugins/simulator/docs/         entity & user-flow reference (ships with the plugin)
docs/                           contributor docs (ARCHITECTURE.md) — repo-level
public/                         generated AI-discovery artifacts (do not hand-edit)
```

## Commands

All `make` targets run from the repo root (recipes `cd` into the module):

```bash
make build         # go build ./...
make vet           # go vet ./...
make test          # go test ./...        (no tests yet)
make enrich-spec   # regenerate swagger/sim-public-swagger.full.json from the live API
make check-spec    # CI drift gate: fail if upstream added ops missing from the full spec
make discovery     # regenerate public/llms.txt + public/.well-known/skills/index.json
```

Run the server directly (no build step — hosts use `go run .`):

```bash
cd plugins/simulator/mcp-server
go run .                         # stdio MCP transport
go run . --sse --addr :8080      # SSE transport
go run . getCompanies            # CLI mode: run one tool and exit
```

Before committing Go changes, run `make build` and `make vet`. Set `SIMULATOR_DEBUG=1` for
verbose logs (`/tmp/simulator.log` in MCP mode).

## Conventions & gotchas

- **No build step ships.** Hosts launch the server with `go run .`. Don't add a compiled
  binary to the repo.
- **The served spec is `sim-public-swagger.full.json`** (embedded via `specs.go`). Don't
  hand-edit it — regenerate with `make enrich-spec`. The curated
  `sim-public-swagger-all.json` (80) is the **reuse source** for enrichment and the source
  of truth for canonical operationIds (`createActor`, `getForm`, `manageLayer`,
  `createLink`, `massLink`, …). If you change those, keep the curated spec in sync.
- **`public/` is generated.** Edit `SKILL.md` frontmatter, then `make discovery`.
- **Entity/user-flow docs must stay under `plugins/simulator/docs/`.** Skills reference
  them as `$CLAUDE_PLUGIN_ROOT/docs/...` and only `plugins/simulator/` is copied on
  install. Moving them out breaks installed plugins. Contributor/architecture docs go in
  the repo-root `docs/`.
- **Versioning.** The plugin version appears in `.claude-plugin/{plugin,marketplace}.json`,
  `.agents/plugins/marketplace.json` and `CHANGELOG.md` — bump them together.
- **TLS is verified by default.** `--insecure` is only for self-signed on-prem gateways.
- **Custom tools** are registered in `app/mcp-server/server.go` (`LoadSwaggerServer`); the
  largest/most delicate logic is the bidirectional graph sync in `sync_graph.go` /
  `push_graph.go` — change it carefully (it has no tests; see ARCHITECTURE §7).

## Adding a custom MCP tool

1. Add the handler in a new file under `app/mcp-server/` (mirror `create_chart.go` etc.).
2. Register it with `mcp.NewTool(...)` inside `LoadSwaggerServer` in `server.go`.
3. `make build && make vet`.
4. Document it in the root [`README.md`](README.md) MCP-tools table and
   [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) §4.

## Security

This repo touches OAuth tokens (stored in `.env`, mode `0600`). Never log token values,
never commit a `.env`, and keep TLS verification on by default.
