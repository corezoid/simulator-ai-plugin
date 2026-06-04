# CLAUDE.md

Guidance for Claude Code when working in the **simulator-ai-plugin** repository.

> The canonical, tool-agnostic instructions live in [`AGENTS.md`](AGENTS.md) — repo
> overview, layout, commands, conventions, and gotchas. **Read it first.** This file only
> adds Claude Code–specific notes; keep substantive guidance in `AGENTS.md` to avoid drift.

## Quick orientation

- **What it is:** an MCP plugin connecting the Simulator.Company platform to Claude — a Go
  MCP server (`plugins/simulator/mcp-server/`) exposing the REST API as tools, plus 7
  skills (`plugins/simulator/skills/`).
- **Design:** [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md).
- **Verify changes:** `make build && make vet` from the repo root.

## Claude Code–specific notes

- **Skills.** The `simulator*` skills here are the ones surfaced as `/simulator`,
  `/simulator-graph`, etc. When editing a skill's behaviour, edit its `SKILL.md`; if you
  change the frontmatter, regenerate discovery artifacts with `make discovery` (don't
  hand-edit `public/`).
- **`$CLAUDE_PLUGIN_ROOT` = `plugins/simulator/`.** Skills load reference docs as
  `$CLAUDE_PLUGIN_ROOT/docs/entities/*.md`. Those files must stay under
  `plugins/simulator/docs/` (only the plugin dir is shipped on install). Contributor docs
  go in the repo-root `docs/`.
- **MCP tools.** When you add or rename a tool, update the MCP-tools table in the root
  [`README.md`](README.md) and §4 of [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md).
- **Don't hand-edit generated files:** `swagger/sim-public-swagger.full.json` (use
  `make enrich-spec`) and `public/*` (use `make discovery`).

## House rules

- No tests exist yet — if you change graph-sync logic (`sync_graph.go` / `push_graph.go`),
  be extra careful and consider adding the first tests.
- Keep TLS verification on by default; never log or commit tokens / `.env`.
- Bump the plugin version in all manifests + `CHANGELOG.md` together.
