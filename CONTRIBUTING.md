# Contributing to the Simulator.Company Plugin

Thanks for your interest in improving the plugin. This document covers how the repo is laid
out, how to build and verify changes, and the conventions we expect in a pull request.

## Repository layout

| Path                                  | What lives there                                                  |
|---------------------------------------|-------------------------------------------------------------------|
| `plugins/simulator/mcp-server/`       | Go MCP server (`cmd/server` + `internal/{config,apiclient,tools,engines}`) |
| `plugins/simulator/skills/`           | The shipped skills (`simulator`, `simulator-graph`, …)            |
| `plugins/simulator/docs/`             | Reference docs loaded by skills at runtime (`$CLAUDE_PLUGIN_ROOT/docs/…`) |
| `docs/`                               | Contributor docs (`ARCHITECTURE.md`, `INTEGRATION.md`)            |
| `public/`                             | **Generated** discovery artifacts — do not hand-edit              |
| `.claude-plugin/`, `.agents/`         | Plugin + marketplace manifests                                    |

The canonical, tool-agnostic guide is [`AGENTS.md`](AGENTS.md) — read it first. Architecture
is in [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md).

## Prerequisites

- [Go 1.25+](https://go.dev/dl/) in `PATH` (the module declares `go 1.25`; the MCP server runs via `go run`)
- `make`
- Optionally [`golangci-lint`](https://golangci-lint.run/) v2 for `make lint`

## Build & verify

All commands run from the repo root.

```bash
make build    # compile the MCP server
make vet      # go vet
make test     # unit tests (config, apiclient, tools, drift gate, eval)
make lint     # golangci-lint v2 (advisory; gosec must stay clean)
```

Before opening a PR, run at minimum:

```bash
make build && make vet && make test
```

## Conventions

- **Curated tools live in Go**, declared as typed `Operation`s under
  `internal/tools/<domain>.go` — they are *not* generated from a spec. New or renamed tools
  must validate against the drift gate (`internal/tools/testdata/papi-openapi.json`). Refresh
  that spec from pong-server with `yarn dump-openapi`; never write it by hand.
- **Generated files** (`public/*`) are produced by `make discovery`. If you change a skill's
  frontmatter, regenerate them — don't hand-edit.
- **Skills.** Edit the skill's `SKILL.md` to change behaviour. Reference docs that skills load
  at runtime must stay under `plugins/simulator/docs/` (only the plugin dir ships on install).
- **When you add or rename a tool**, update the MCP-tools table in [`README.md`](README.md) and
  §4 of [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md).
- **Security.** Keep TLS verification on by default. Never log or commit tokens or `.env`.
- **Versioning.** Bump the plugin version in *all* manifests (`.claude-plugin/`, `.agents/`)
  and add a `CHANGELOG.md` entry together, in the same change.

## Commit messages

Follow the existing conventional-commit style used in the history, e.g.:

```
feat(mcp-server): add filterActors tool
fix(engines): reject non-UUID layerId before filesystem access
docs: document set-environment in README
chore(skills): regenerate discovery artifacts
```

## Branching model

We follow a simple `develop`-integration flow:

- **`develop`** is the integration branch and the **default base for every feature/fix PR.**
- **`main`** holds releases only. It receives changes **exclusively** by promoting
  `develop` (a release PR `develop` → `main`) or via a `release/*` / `hotfix/*` branch.
- Do **not** open feature/fix PRs against `main` — a CI guard
  (`.github/workflows/guard-base-branch.yml`) fails any PR into `main` whose source
  branch isn't `develop`, `release/*`, or `hotfix/*`.

## Pull requests

1. Branch off `develop` (e.g. `git switch -c feat/my-change develop`).
2. Target **`develop`** as the PR base (it's the repo default, so this is automatic).
3. Keep the change focused; update docs and the changelog alongside code.
4. Ensure `make build && make vet && make test` pass.
5. Fill in the PR template.

## Reporting issues

Use the issue templates. For anything security-sensitive, follow
[`SECURITY.md`](SECURITY.md) instead of opening a public issue.
