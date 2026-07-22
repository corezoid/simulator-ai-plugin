# AGENTS.md

Guidance for coding agents (Codex, Claude Code, etc.) working in the **simulator-ai-plugin**
repository. Humans: see the root [`README.md`](README.md) (usage) and
[`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) (design).

## What this repo is

A plugin for Claude Code, Codex, and AWS Kiro that connects the **Simulator.Company** platform
(backend: `pong-server` / `control-api`) to the host via MCP. It bundles:

- a **Go MCP server** (`plugins/simulator/mcp-server/`) that exposes the Simulator
  `/papi/1.0` public API as a **curated, typed set of ~95 MCP tools** (declared in Go, not a
  generic spec passthrough), scoped to the core scenarios;
- **14 skills** (`plugins/simulator/skills/`) — markdown that teaches the model the
  platform's entity model and common workflows.

Read [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) before making non-trivial changes.

## Layout (the parts you'll touch)

```
plugins/simulator/mcp-server/   Go MCP server (Go 1.24+, mark3labs/mcp-go)
  cmd/server/                   entry point: profile → apiclient → tools → stdio
  cmd/gendiscovery/             regenerate public/ discovery artifacts
  cmd/evalrunner/               behavioural eval (opt-in; needs ANTHROPIC_API_KEY)
  internal/config/              cloud presets + local/prod profiles (env + profiles.json overridable)
  internal/apiclient/           HTTP client: base URL, auth header, accId, timeouts, errors
  internal/tools/               curated typed Operation registry (op.go) + per-domain files
    testdata/                   papi-openapi.json (drift gate) + eval-scenarios.json
  internal/engines/             graph sync, layout, prune, placements, upload, chart
  app/auth/                     set-environment (public config → account URL) + OAuth2 PKCE + .env credential storage
plugins/simulator/skills/       14 skills (markdown only, ship with the plugin)
plugins/simulator/docs/         entity & user-flow reference (ships with the plugin)
docs/                           contributor docs (ARCHITECTURE.md, INTEGRATION.md) — repo-level
public/                         generated AI-discovery artifacts (do not hand-edit)
```

## Commands

All `make` targets run from the repo root (recipes `cd` into the module):

```bash
make build         # go build ./...
make vet           # go vet ./...
make lint          # golangci-lint run ./...   (golangci-lint v2; gosec clean, style backlog open)
make test          # go test ./...   — config, apiclient, tools (scenarios, -race, drift, eval), engines
make discovery     # regenerate public/llms.txt + public/.well-known/skills/index.json
make run-local     # go run ./cmd/server --profile local   (dev pong-server :9000)
make run-prod      # go run ./cmd/server --profile prod
make eval          # behavioural eval, dry — model picks tools, canned fixtures (opt-in)
make eval-skills   # behavioural eval, dry, with the SKILL.md files injected as system prompt
make eval-live     # behavioural eval executing tools against the backend (throwaway workspace)
```

Run the server directly (no build step — hosts use `go run ./cmd/server`):

```bash
cd plugins/simulator/mcp-server
go run ./cmd/server --profile local   # targets pong-server :9000; logs to stderr
```

The **`local` profile** (`--profile local` or `SIMULATOR_PROFILE=local`) targets a
pong-server on `:9000` **and** makes `set-environment` offer the `local` preset
(`localhost:9000`); the default `prod` profile hides it (cloud `mw`/`sim` + custom URL only).

Before committing Go changes, run `make build && make vet && make test`.

**Dev loop in Claude Code:** load the plugin from the repo with `claude --plugin-dir <repo>`
(or a local marketplace install, or the project-scoped root `.mcp.json` — pick one).
Set the profile either by prefixing the launch (`SIMULATOR_PROFILE=local claude --plugin-dir
<repo>` — the server inherits it) or in `plugins/simulator/mcp-server/.env`
(`SIMULATOR_PROFILE=local`). After editing
code/tools/skills/`.env`, run **`/reload-plugins`** to relaunch the MCP server — no
reinstall. Verify with `/mcp`. Full guide: README → "Local development".

## Conventions & gotchas

- **No build step ships.** Hosts launch the server with `go run ./cmd/server`. Don't add a
  compiled binary to the repo.
- **Tools are declared in Go, curated.** Each domain file under `internal/tools/`
  (`forms.go`, `actors.go`, …) declares a slice of typed `Operation`s; `build.go` registers
  them. The server exposes ~95 tools, not the full REST surface — keep it curated.
- **Drift gate.** `internal/tools/drift_test.go` validates every declared op (method, path,
  operationId) against `internal/tools/testdata/papi-openapi.json` (the backend contract,
  dumped by pong-server `yarn dump-openapi`). After changing tools or the backend, refresh
  that spec and run `go test ./internal/tools/ -run TestSpecDrift`.
- **operationIds live at the backend source.** `pong-server` declares them on its `/papi`
  route schemas; the plugin matches them. Keep names in sync across both repos.
- **`public/` is generated.** Edit `SKILL.md` frontmatter, then `make discovery`.
- **Skill language: English-first, trilingual in practice.** We work mainly in three
  languages — **English (primary), Ukrainian, Russian**. Write all skill/doc *prose and
  behavioural instructions in English* (one maintainable source). **Never hardcode a
  non-English sentence for Claude to say** — instead instruct it to reply *in the user's own
  language*. Two deliberate exceptions stay multilingual: (1) **activation triggers** in
  `description:` frontmatter may include uk/ru example phrases (they improve auto-activation
  when users type in those languages); (2) **product/UI terms** are kept verbatim (e.g. the
  Account-Template alias «Шаблон рахунків»). Example *data* values should be language-neutral.
- **Entity/user-flow docs must stay under `plugins/simulator/docs/`.** Skills reference them
  as `$CLAUDE_PLUGIN_ROOT/docs/...` and only `plugins/simulator/` is copied on install.
  Contributor/architecture docs go in the repo-root `docs/`. The token name is host-imposed:
  Claude Code and Codex both resolve `$CLAUDE_PLUGIN_ROOT` via text substitution at
  skill-load time (anthropics/claude-code#48230, #47789, #44057). Renaming it breaks both
  hosts. AWS Kiro doesn't substitute the token at all — `install-kiro.sh` and the release
  generator hard-copy the skills and `sed`-replace the token with the absolute plugin path
  at install time instead. MCP subprocesses are not skill loading: Codex does not expose
  `CLAUDE_PLUGIN_ROOT` there, so `.mcp.json` must keep the user's `PWD` as
  `SIMULATOR_WORK_DIR` and resolve the installed plugin root separately.
- **Branching model.** `develop` is the integration branch and the **default base for
  every feature/fix PR**; `main` is release-only and receives changes solely by promoting
  `develop` (or a `release/*` / `hotfix/*` branch). Never open a feature/fix PR against
  `main` — a CI guard (`.github/workflows/guard-base-branch.yml`) rejects any PR into `main`
  whose head isn't `develop`, `release/*`, or `hotfix/*`.
- **Versioning & releases.** The version is minted at **release time**, not per PR — many PRs
  land on `develop` during the week, so a per-PR bump just makes contributors collide on the
  same number.
  - **In a PR:** do **not** touch the version. Append a one-line entry under the
    `## [Unreleased]` section at the top of `CHANGELOG.md` (under `### Added` / `### Changed` /
    `### Fixed`). Appending a bullet avoids the merge conflicts a version bump causes.
  - **At release (promoting `develop` → `main`):** run `make release VERSION=x.y.z`. It rewrites
    `## [Unreleased]` into a dated `## [x.y.z]` section (and starts a fresh empty `Unreleased`),
    then bumps the version in lockstep across the **six** files that carry it:
    `plugins/simulator/.claude-plugin/plugin.json`,
    `plugins/simulator/.codex-plugin/plugin.json`,
    `plugins/simulator/.kiro-plugin/plugin.json`,
    `.claude-plugin/marketplace.json`,
    `.agents/plugins/marketplace.json`,
    `POWER.md` (frontmatter). The script does not commit/tag/push — review the diff, commit,
    merge to `main`, then tag `vx.y.z`. The `Release` workflow reads the `## [x.y.z]` CHANGELOG
    section for the GitHub release notes, so the section must exist before the tag.
- **Linter.** `make lint` runs golangci-lint v2 (config `plugins/simulator/mcp-server/.golangci.yml`,
  shared with sibling Go services). `gosec` is clean (real findings fixed; trusted-input taint
  false positives are documented in the `gosec.excludes` list). The broader `default: all` set
  still has a style/modernization backlog, so lint is **advisory** — not yet in the pre-commit
  `build && vet && test` gate.
- **TLS is verified by default.** `--insecure` is only for self-signed on-prem gateways.
- **Graph sync** (`internal/engines/sync_graph.go` + `push_graph.go`, ~1.5k LOC) is the most
  delicate logic — change it carefully. Core helpers and the diff / inject paths now have
  unit tests (`graphsync_test.go`: form-id/UUID/name-cache helpers, `injectMassLinkData`,
  `injectManageLayerData`, `fetchHierarchyEdgeTypeID`, `updateGraphActor`, layer-actor
  pagination, plus an end-to-end `pushGraph` delete path via `httptest`). The full
  create/recreate orchestration and edge-placement branches are only partly covered — extend
  coverage when you touch them.

## Adding a curated MCP tool

1. Add an `Operation` to the relevant `internal/tools/<domain>.go` slice (method, path
   template, typed params), matching the backend's `operationId`.
2. `make build && make vet && make test` (the drift gate confirms it matches the backend).
3. Document it in the root [`README.md`](README.md) tool table and
   [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) §4.
4. **Add an eval scenario** in `internal/tools/testdata/eval-scenarios.json` — every curated
   tool should have at least one. The structural test (`eval_test.go`) fails if a scenario
   names a tool that isn't registered (or an `argChecks.tool` not listed in `tools`). Use
   `argChecks` (`mustContain` / `mustNotContain` / `mustMatch`) to assert on the model's
   arguments where the shape matters (data keys, value discriminators, regression guards).
   Scenario knobs for robustness:
   - A `tools` entry may be an **any-of group** `"a|b"` — satisfied if the model calls *either*
     (for genuinely interchangeable tools, e.g. `"getTransfer|getTransferByRef"`).
   - **`dryOnlyTools`** lists follow-up tools required only in dry mode — the second step of a
     real-dependency flow (e.g. `finalizeTransaction` after authorize) that dry fixtures let the
     model reach but a placeholder-id 404 stops it from reaching live. An `argCheck` can carry
     `"dryOnly": true` for the same reason.
   - For **live** (`make eval-live`): the runner scores tool *selection*, not backend success
     (a 404 still counts as "called"). Give prompts concrete ids/refs (or have them discover
     real entities via `getUsers`/`getForms`) so the model actually issues the call; add a
     **dry fixture** in `evalrunner/main.go` for any new read tool a multi-step chain reads from.

For a multi-call/computational tool, add it under `internal/engines/` and register it in
`engines/register.go` instead.

## CI

`.github/workflows/ci.yml` runs on every push (main/develop) and PR:
- **`gate`** (required, no secrets): `make build` → `vet` → `test` → discovery freshness
  (`make discovery` then `git diff --exit-code -- public/` — a PR that edited `SKILL.md`
  frontmatter but forgot `make discovery` fails here) → `make lint` (advisory,
  `continue-on-error`).
- **`behavioural-eval`** (opt-in, `workflow_dispatch` only — it spends Anthropic credits):
  `make eval` with `ANTHROPIC_API_KEY` from repo secrets.

## Security

This repo touches OAuth tokens (stored in `.env`, mode `0600`). Never log token values,
never commit a `.env`, and keep TLS verification on by default.
