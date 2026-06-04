# Architecture

This document describes the internal architecture of the **simulator-ai-plugin** — how
the plugin is packaged, how the Go MCP server exposes the Simulator.Company (`pong-server`)
public API as a curated set of MCP tools, how authentication and workspace context work,
and how the server is kept in sync with the backend contract.

It is aimed at contributors working on the plugin itself. End users only need the root
[`README.md`](../README.md); skill consumers should read the per-skill `SKILL.md`
files and the [entity docs](../plugins/simulator/docs/entities/README.md).

---

## 1. High-level picture

```
┌─────────────────────────────────────────────────────────────────────┐
│  Claude Code / Codex host                                             │
│                                                                       │
│   Skills (markdown)                  MCP client                       │
│   ├── simulator              ──┐                                      │
│   ├── simulator-init           │   domain knowledge + tool-call       │
│   ├── simulator-graph          │   guidance is injected into the      │
│   ├── simulator-forms          ├─▶ model context; the model then      │
│   ├── simulator-finance        │   calls MCP tools over stdio         │
│   ├── simulator-charts         │                                      │
│   └── software-migration-...─┘                                        │
└───────────────────────────────────────┬─────────────────────────────┘
                                         │ MCP (stdio)
                                         ▼
┌─────────────────────────────────────────────────────────────────────┐
│  Go MCP server  (`go run ./cmd/server` from plugins/simulator/mcp-server) │
│                                                                       │
│   cmd/server ── resolve profile · load .env · wire deps · serve stdio │
│      │                                                                │
│      ▼                                                                │
│   internal/config   ── local / prod profiles (env + profiles.json)    │
│   internal/tools    ── curated typed Operation registry → MCP tools   │
│   internal/engines  ── client-side engines (graph sync, layout, chart)│
│   app/auth          ── OAuth2 PKCE login + .env credential storage    │
│      │                                                                │
│      ▼                                                                │
│   internal/apiclient ── HTTP: base URL · Bearer · accId · timeouts ─▶ │
└───────────────────────────────────────┬─────────────────────────────┘
                                         │ HTTPS (Authorization, accId path/query)
                                         ▼
              Simulator.Company public API  (/papi/1.0 — local :9000 or mw gateway)
```

Two layers cooperate:

- **Skills** carry *domain knowledge* — what an actor/form/account is, which tools to call
  in what order, common workflows. They are plain markdown and ship no code.
- **The MCP server** exposes a *curated, typed tool set* (~46 tools) scoped to the core
  scenarios — forms, actors, accounts, transactions, graph building, applications/smart
  forms — rather than the entire REST surface. Each tool is a compile-time descriptor, not
  a generic passthrough; a drift gate keeps those descriptors honest against the backend.

This separation is deliberate: the tool set can evolve without touching the skills, and the
skills can evolve without redeploying the server.

---

## 2. Repository layout

```
simulator-ai-plugin/
├── .claude-plugin/marketplace.json   # Claude Code marketplace listing
├── .agents/plugins/marketplace.json  # Codex marketplace listing
├── .mcp.json                         # Root MCP launcher (marketplace install)
├── Makefile                          # build / vet / test / discovery / run-local / run-prod
├── CHANGELOG.md
├── CLAUDE.md                         # Repo guide for Claude Code (points to AGENTS.md)
├── AGENTS.md                         # Repo guide for coding agents (canonical)
├── context7.json                     # Context7 docs indexing config
├── docs/                             # Project / contributor documentation
│   ├── ARCHITECTURE.md               # ← this file
│   └── INTEGRATION.md                # pong-server integration plan & status
├── public/                           # Generated AI-discovery artifacts
│   ├── llms.txt
│   └── .well-known/skills/index.json
└── plugins/simulator/                # CLAUDE_PLUGIN_ROOT — what gets installed
    ├── .claude-plugin/plugin.json    # Claude Code manifest
    ├── .codex-plugin/plugin.json     # Codex manifest
    ├── .mcp.json                     # Plugin MCP launcher (go run ./cmd/server)
    ├── docs/                         # Plugin-shipped reference (entities, user-flows)
    ├── skills/                       # 7 skills (markdown only)
    └── mcp-server/                   # Go MCP server (see §3)
```

The plugin is published in **two manifests** (Claude Code and Codex) that both point at
`plugins/simulator/`. The root `.mcp.json` is used when the plugin is installed from a
marketplace; the plugin-level `.mcp.json` is used when running from a local clone. Both
launch the same `go run ./cmd/server` command — there is no build step.

> **Why docs live in two places.** Top-level `docs/` holds contributor/architecture docs
> for people working *on* the repo. The entity and user-flow reference under
> `plugins/simulator/docs/` is **shipped with the installed plugin** and referenced by
> skills via `$CLAUDE_PLUGIN_ROOT/docs/...` (where `$CLAUDE_PLUGIN_ROOT` =
> `plugins/simulator`). Only `plugins/simulator/` is copied on install, so that reference
> material must stay inside the plugin or the skills break for installed users.

---

## 3. Go MCP server

Source: `plugins/simulator/mcp-server/`. Module:
`github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server` (Go 1.24+).
SDK: `github.com/mark3labs/mcp-go`.

```
mcp-server/
├── cmd/server/        # entry point: resolve profile → build deps → register tools → stdio
├── cmd/gendiscovery/  # regenerate public/ discovery artifacts from SKILL.md files
├── internal/
│   ├── config/        # profile resolution: flag > env > profiles.json > built-in default
│   ├── apiclient/     # HTTP client: base URL, auth header, accId injection, timeout, errors
│   ├── tools/         # curated typed Operation registry (op.go) + per-domain tool files
│   │                  #   forms, actors, accounts, transactions, graph, apps + auth helpers
│   │   └── testdata/  # papi-openapi.json (drift gate) + eval-scenarios.json
│   └── engines/       # client-side engines: graph pull/push sync, compact layout, prune,
│                      #   layer placements, picture upload (+ SVG raster), chart
└── app/auth/          # OAuth2 PKCE flow + .env credential storage (shared)
```

Layering rule: `cmd → tools → {apiclient, config, engines, auth}`; no package imports
"upward". `tools.BuildAll(server, client, profile)` plus `engines.RegisterTools(server)`
replace what used to be a single 3k-line registration function.

### 3.1 Run mode & entry point (`cmd/server/main.go`)

The server is stdio-only (the transport MCP hosts launch). On start it:

1. parses `--profile` and `--insecure`;
2. loads `.env` from the working directory (without overriding the live environment);
3. resolves the active profile (§3.2);
4. builds the `apiclient` (base URL from the profile; the Authorization header is read from
   saved credentials per request, so a mid-session `login` takes effect without a restart);
5. registers the curated tools (`tools.BuildAll`) and the engine tools
   (`engines.RegisterTools`);
6. serves over stdio.

`SIMULATOR_DEBUG` may be used by individual components for verbose logging.

### 3.2 Profiles & environment (`internal/config`)

The target backend is chosen by **profile**, resolved in order: `--profile` flag →
`SIMULATOR_PROFILE` env → `profiles.json` `active` → built-in default (`prod`).

| Profile | API base URL                          | Account (SA) URL                  |
|---------|----------------------------------------|-----------------------------------|
| `local` | `http://localhost:9000/papi/1.0`       | `https://account.pre.corezoid.com`|
| `prod`  | `https://mw.simulator.company/papi/1.0`| `https://account.corezoid.com`    |

Each field is overridable via `SIMULATOR_API_BASE_URL`, `SIMULATOR_ACCOUNT_URL`,
`SIMULATOR_OAUTH_CLIENT_ID`, or a `profiles.json` in the working directory. Tokens never
live in `profiles.json` — only in `.env`.

### 3.3 Tool model (`internal/tools`)

Tools are **declared in Go**, not generated from a spec at runtime. `op.go` defines an
`Operation` (name = operationId, HTTP method, path template, typed `Param`s) and a generic
`register()` that turns any `Operation` into a typed MCP tool whose handler maps
arguments → path/query/body → one `apiclient.Do` call. The per-domain files
(`forms.go`, `actors.go`, `accounts.go`, `transactions.go`, `graph.go`, `apps.go`) each
declare a slice of `Operation`s; `build.go` registers them all plus the `login` /
`set-workspace` helpers.

Conventions baked into the registry:

- `accId` path/query params default to the active workspace when omitted;
- a POST/PUT whose object-body fields are all omitted still sends `{}`;
- required params missing → a clear error result (not a malformed request).

### 3.4 Workspace & auth context

Every API call needs an `Authorization` header and a workspace id (`accId`). Both come from
`.env`: the token is written by `login`; `WORKSPACE_ID` is written by `set-workspace`. The
`apiclient` injects `accId` into path/query params that need it, and guards the live
workspace value with an `RWMutex` (set-workspace mutates it while tool calls read it).

### 3.5 Spec drift gate

`internal/tools/drift_test.go` validates every curated `Operation` (method, path,
operationId) against `internal/tools/testdata/papi-openapi.json` — the OpenAPI spec dumped
from the live backend (where `operationId`s are now declared at the source; see
[`INTEGRATION.md`](INTEGRATION.md) §9). If the file is absent the test skips; when present
it fails on any divergence between the plugin's declared tools and the backend contract.
Refresh the spec with pong-server's `yarn dump-openapi` and copy it into `testdata/`.

---

## 4. Curated tool set & engines

The server registers ~46 tools in two groups.

**Curated API operations** (`internal/tools`, declared per domain) — one MCP tool per
backend operation, with typed parameters:

| Domain        | Tools                                                                                  |
|---------------|----------------------------------------------------------------------------------------|
| Forms         | `createForm` `getForm` `getForms` `searchForms` `updateForm` `deleteForm` `setFormStatus` |
| Actors        | `createActor` `getActor` `getActorByRef` `searchActors` `searchLayerActors` `filterActors` `updateActor` `deleteActor` `setActorStatus` |
| Accounts      | `createAccount` `getAccounts` `getBalance` `updateAccount` `deleteAccount` `createCurrency` `getCurrencies` `createAccountName` `getAccountNames` |
| Transactions  | `createTransaction` `finalizeTransaction` `getTransactions` `createTransfer` `getTransfer` |
| Graph         | `createLink` `massLink` `getEdgeTypes` `getLayerActors` `getRelatedActors` `manageLayerActors` |
| Applications  | `createApplication` `createSmartForm` `listSmartForms` `manageAppContent`              |
| Search        | `searchAll` (global text/semantic search across actors & users)                        |
| Setup         | `login` `getWorkspaces` `set-workspace` (by accId or name) |

**Engine tools** (`internal/engines`) — multi-call workflows and client-side computation
ported from the original implementation:

| Tool                     | Source                   | Purpose                                                              |
|--------------------------|--------------------------|----------------------------------------------------------------------|
| `pullGraphFile`          | `sync_graph.go`          | Export a layer's actors + edges to `<layerId>.yaml`                  |
| `pushGraphFile`          | `sync_graph.go` / `push_graph.go` | Diff a local YAML against the layer and create/update/delete |
| `getAllLayerPlacements`  | `get_layer_placements.go`| Return every placement on a layer in one paginated call             |
| `compactGraphLayout`     | `compact_layout.go`      | Auto-layout a layer into domain-clustered grids                     |
| `pruneLongEdges`         | `prune_edges.go`         | Delete edges longer than a distance threshold; preserves hierarchy  |
| `uploadActorPicture(Bulk)`| `upload.go` + `svg.go`  | Set actor pictures (URL/file/base64); auto-rasterise SVG→PNG        |
| `createChart`            | `create_chart.go`        | Create a dashboard chart actor (dynamic filter or explicit accounts) |

Engines share a small runtime config (`engines.Configure`: base URL + TLS) and read the
auth header / `WORKSPACE_ID` per call. The graph sync (`sync_graph.go` + `push_graph.go`,
~1.5k LOC) is the most delicate logic: a bidirectional diff that minimises API calls and
handles cascading deletes; its form name→id cache is per-workspace under a `sync.RWMutex`.

---

## 5. Authentication (`app/auth`)

```
login tool
  │
  ├─ oauth.go: generate PKCE verifier + challenge + random CSRF state
  ├─ start a local callback HTTP server on a random free port
  ├─ open the browser at <ACCOUNT_URL>/oauth2/authorize?…&state=…&code_challenge=…
  ├─ wait for the redirect (5-min timeout); reject if returned state ≠ sent state
  ├─ exchange the code at /oauth2/token → simulator_token (JWT)
  ├─ parse the JWT `exp` claim (fallback: conservative 12h window)
  └─ credentials.go: atomically write ACCESS_TOKEN + ACCESS_TOKEN_EXPIRES_AT to .env (0600)
```

- **Storage**: plaintext `.env` in the working directory, mode `0600`. Writes are serialised
  under a mutex and token + expiry are written in a single pass.
- **Account URL** comes from the resolved profile (`account.corezoid.com` for prod,
  `account.pre.corezoid.com` for local); the OAuth client id is overridable via
  `SIMULATOR_OAUTH_CLIENT_ID`. Local mirrors production — same PKCE flow, different SA.
- **TLS**: certificate verification is **on by default**; `--insecure` is for self-signed
  on-prem gateways.

---

## 6. Build & tooling

There is **no build step to run the plugin** — hosts launch it with `go run ./cmd/server`.
`Makefile` targets (run from the repo root; recipes `cd` into the module):

| Target          | What it does                                                                |
|-----------------|------------------------------------------------------------------------------|
| `make build`    | `go build ./...`                                                            |
| `make vet`      | `go vet ./...`                                                              |
| `make lint`     | `golangci-lint run ./...` (golangci-lint v2; `gosec` clean, style backlog)   |
| `make test`     | `go test ./...` — config, apiclient, tools (scenarios, `-race`), drift, eval |
| `make discovery`| Regenerate `public/llms.txt` + `public/.well-known/skills/index.json`       |
| `make run-local`| `go run ./cmd/server --profile local`                                       |
| `make run-prod` | `go run ./cmd/server --profile prod`                                        |

**Spec source.** Tool `operationId`s are declared at the backend source (pong-server
`/papi` route schemas). pong-server's `yarn dump-openapi` emits `papi-openapi.json`, which
is copied into `internal/tools/testdata/` to drive the drift gate (§3.5). See
[`INTEGRATION.md`](INTEGRATION.md).

### `cmd/gendiscovery`

Walks `plugins/simulator/skills/`, parses each `SKILL.md` frontmatter, and emits the two
AI-discovery files under `public/`.

---

## 7. Testing & known gaps

The module ships tests under `internal/`:

- **config** — profile resolution (defaults, env overrides, unknown-profile error);
- **apiclient** — request building (method/path/query/body/auth) and non-2xx → `APIError`;
- **tools** — scenario tests driving handlers against a mock server, accId defaulting,
  required-param enforcement, empty-object body, `formName` resolution, boolean-query
  omission, a `-race` concurrency test, the spec **drift gate**, and the **eval** check;
- **engines** — graph-sync diff: decision primitives (`formIDFromLayerActor`, form-name
  cache + nested resolution, actor-formId cache/override) plus an end-to-end `pushGraph`
  test against a mock backend; tool-registration smoke test + base-URL config.

Eval harness:

- **Structural** (`internal/tools/eval_test.go`): asserts every tool named in
  `eval-scenarios.json` exists.
- **Behavioural** (`cmd/evalrunner`): spawns the real server (`tools/list`) and drives a
  Claude model through each prompt via the Anthropic API in a bounded tool-use loop.
  - `make eval` (dry): tool calls answered with a stub `{}` — read-only, no backend; checks
    the model reaches for the expected tools. Scenarios tagged `liveOnly` (those needing
    real ids/state) are skipped. Verified passing against the live model.
  - `make eval-live` (`--execute`): forwards tool calls to the MCP server so they run against
    the real backend; entities created during a run are tracked and best-effort deleted at
    the end. **Use a throwaway workspace** — `login` + `set-workspace` first.
  - Opt-in either way — skips without `ANTHROPIC_API_KEY`.

Backlog:

- **Spec-driven registry (future).** Tools are hand-declared; the drift gate guards them
  against backend drift, so generating them from the spec is optional cleanup, not required.
- **Live behavioural eval in CI.** `cmd/evalrunner` is read-only (stubbed tool results);
  executing tool calls against a throwaway workspace would test end-to-end behaviour too.

---

## 8. Where to go next

- Integration plan & migration status → [`INTEGRATION.md`](INTEGRATION.md)
- Entity model and field semantics → [`plugins/simulator/docs/entities/`](../plugins/simulator/docs/entities/README.md)
- End-to-end walkthroughs → [`plugins/simulator/docs/user-flows/`](../plugins/simulator/docs/user-flows/README.md)
- API operation catalogue → [`plugins/simulator/skills/simulator/references/api-operations.md`](../plugins/simulator/skills/simulator/references/api-operations.md)
- Running / developing the server → [`plugins/simulator/mcp-server/README.md`](../plugins/simulator/mcp-server/README.md)
- Working in this repo as an agent → [`AGENTS.md`](../AGENTS.md)
