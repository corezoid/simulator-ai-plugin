# CLAUDE.md

Guidance for Claude Code when working in the **simulator-ai-plugin** repository.

> The canonical, tool-agnostic instructions live in [`AGENTS.md`](AGENTS.md) — repo
> overview, layout, commands, conventions, and gotchas. **Read it first.** This file only
> adds Claude Code–specific notes; keep substantive guidance in `AGENTS.md` to avoid drift.

## Quick orientation

- **What it is:** an MCP plugin connecting the Simulator.Company platform to Claude — a Go
  MCP server (`plugins/simulator/mcp-server/`) exposing the REST API as tools, plus 6
  skills (`plugins/simulator/skills/`).
- **Design:** [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md).
- **Verify changes:** `make build && make vet` from the repo root (`make lint` is available —
  golangci-lint v2, advisory: `gosec` clean, style backlog open).

## Claude Code–specific notes

- **Skills.** The `simulator*` skills here are the ones surfaced as `/simulator`,
  `/simulator-graph`, etc. When editing a skill's behaviour, edit its `SKILL.md`; if you
  change the frontmatter, regenerate discovery artifacts with `make discovery` (don't
  hand-edit `public/`). **Write instructions in English** and have Claude reply in the
  user's own language — never hardcode a non-English sentence for it to say. See the
  "Skill language" convention in [`AGENTS.md`](AGENTS.md) for the trilingual policy
  (en primary; uk/ru activation triggers and product aliases are the kept exceptions).
- **`$CLAUDE_PLUGIN_ROOT` = `plugins/simulator/`.** Skills load reference docs as
  `$CLAUDE_PLUGIN_ROOT/docs/entities/*.md`. Those files must stay under
  `plugins/simulator/docs/` (only the plugin dir is shipped on install). Contributor docs
  go in the repo-root `docs/`. Claude Code and Codex both resolve this exact token via
  text substitution at skill-load time; renaming it breaks doc loading on both hosts
  (anthropics/claude-code#48230, #47789, #44057). For AWS Kiro — which does no such
  substitution — `install-kiro.sh` and the release-zip generator hard-copy the skills
  and `sed`-replace the token with the absolute plugin path at install time.
- **MCP tools.** When you add or rename a tool, update the MCP-tools table in the root
  [`README.md`](README.md) and §4 of [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md).
- **Curated tools live in Go** under `internal/tools/<domain>.go` (declared as typed
  `Operation`s), not generated from a spec. The drift gate
  (`internal/tools/testdata/papi-openapi.json`) validates them against the backend.
- **Don't hand-edit generated files:** `public/*` (use `make discovery`). Refresh the drift
  spec from pong-server (`yarn dump-openapi`), don't write it by hand.

## House rules

- Graph-sync logic (`internal/engines/sync_graph.go` / `push_graph.go`) now has unit tests for
  its core helpers and diff / inject paths (`graphsync_test.go`), but the full create/recreate
  orchestration and edge-placement branches are only partly covered — change it carefully and
  extend the tests when you touch those paths.
- Keep TLS verification on by default; never log or commit tokens / `.env`.
- **Don't bump the plugin version in a PR.** A PR only appends a bullet under the
  `## [Unreleased]` section at the top of `CHANGELOG.md` (`### Added` / `Changed` / `Fixed`).
  The version — across the six manifests plus a dated `CHANGELOG` section — is minted once at
  release time by `make release VERSION=x.y.z`. See the "Versioning & releases" convention in
  [`AGENTS.md`](AGENTS.md).
