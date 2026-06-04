# Architecture

This document describes the internal architecture of the **simulator-ai-plugin** — how
the plugin is packaged, how the Go MCP server turns the Simulator.Company REST API into
MCP tools, how authentication and workspace context work, and how the supporting build
tooling regenerates the embedded artifacts.

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
│   ├── simulator-finance        │   calls MCP tools over stdio/SSE     │
│   ├── simulator-charts         │                                      │
│   └── software-migration-...─┘                                        │
└───────────────────────────────────────┬─────────────────────────────┘
                                         │ MCP (stdio by default, SSE optional)
                                         ▼
┌─────────────────────────────────────────────────────────────────────┐
│  Go MCP server  (`go run .` from plugins/simulator/mcp-server)        │
│                                                                       │
│   main.go ── flags / modes / .env bootstrap                           │
│      │                                                                │
│      ▼                                                                │
│   app/mcp-server (LoadSwaggerServer)                                  │
│      ├── embedded OpenAPI spec (specs.go → swagger/*.full.json)       │
│      ├── auto-generated tools  (one per REST operation, ~174)         │
│      ├── custom tools          (11 hand-written, see §4)              │
│      └── executeOperation ── HTTP ─▶ Simulator REST API (/papi/1.0)   │
│                                                                       │
│   app/auth ── OAuth2 PKCE login + .env credential storage             │
│   app/swagger ── spec loader (HTTP/file → models.SwaggerSpec)         │
│   app/models ── OpenAPI/Swagger type definitions                      │
└───────────────────────────────────────┬─────────────────────────────┘
                                         │ HTTPS (Bearer token, accId query param)
                                         ▼
                          Simulator.Company REST API (mw.simulator.company)
```

Two layers cooperate:

- **Skills** carry *domain knowledge* — what an actor/form/account is, which tools to call
  in what order, common workflows. They are plain markdown and ship no code.
- **The MCP server** is a generic *Swagger→MCP bridge* — it knows nothing about the
  business domain; it mechanically exposes every REST operation plus a handful of
  hand-written convenience tools.

This separation is deliberate: the API surface can grow (regenerate the spec) without
touching the skills, and the skills can evolve without redeploying the server.

---

## 2. Repository layout

```
simulator-ai-plugin/
├── .claude-plugin/marketplace.json   # Claude Code marketplace listing
├── .agents/plugins/marketplace.json  # Codex marketplace listing
├── .mcp.json                         # Root MCP launcher (marketplace install)
├── Makefile                          # enrich-spec / check-spec / discovery / build / vet / test
├── CHANGELOG.md
├── CLAUDE.md                         # Repo guide for Claude Code (points to AGENTS.md)
├── AGENTS.md                         # Repo guide for coding agents (canonical)
├── context7.json                     # Context7 docs indexing config
├── docs/                             # Project / contributor documentation
│   └── ARCHITECTURE.md               # ← this file
├── public/                           # Generated AI-discovery artifacts
│   ├── llms.txt
│   └── .well-known/skills/index.json
└── plugins/simulator/                # CLAUDE_PLUGIN_ROOT — what gets installed
    ├── .claude-plugin/plugin.json    # Claude Code manifest
    ├── .codex-plugin/plugin.json     # Codex manifest
    ├── .mcp.json                     # Plugin MCP launcher (go run . --spec simulator)
    ├── docs/                         # Plugin-shipped reference (entities, user-flows)
    ├── skills/                       # 7 skills (markdown only)
    └── mcp-server/                   # Go MCP server (see §3)
```

The plugin is published in **two manifests** (Claude Code and Codex) that both point at
`plugins/simulator/`. The root `.mcp.json` is used when the plugin is installed from a
marketplace; the plugin-level `.mcp.json` is used when running from a local clone. Both
launch the same `go run .` command — there is no build step.

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

> **Rewrite in progress.** The sections below describe the **legacy** server (root
> `main.go` + `app/mcp-server`) — the generic Swagger→MCP bridge that exposes all 185
> operations. A **new layered server** now lives alongside it under `cmd/server` +
> `internal/` and is the target implementation:
>
> ```
> cmd/server/        thin entrypoint: resolve profile → apiclient → curated tools → stdio
> internal/config/   local/prod profiles (env + profiles.json overridable)
> internal/apiclient/ HTTP: base URL, auth header, accId injection, timeouts, error mapping
> internal/tools/    curated typed operation registry (op.go) + per-domain tool files
>                    (forms, actors, accounts, transactions, graph, apps) + auth helpers
> ```
>
> Layering: `cmd → tools → {apiclient, config}` + reuse of `app/auth`; no package imports
> "upward". It exposes a curated ~36-tool set for the core scenarios rather than the full
> passthrough. The client-side engines (graph sync, layout, chart, upload) are not yet
> ported. See [`INTEGRATION.md`](INTEGRATION.md) for the plan and migration status; `.mcp.json`
> still launches the legacy server until the engines move over.

```
mcp-server/
├── main.go            # entry point: flags, run modes, .env bootstrap
├── specs.go           # //go:embed of the full OpenAPI spec
├── swagger/           # bundled OpenAPI specs (see §3.4)
├── app/
│   ├── auth/          # oauth.go (PKCE) + credentials.go (.env persistence)
│   ├── swagger/       # loader.go — parse spec from URL or file
│   ├── models/        # models.go — SwaggerSpec / Endpoint / Definition types
│   └── mcp-server/    # the bridge itself + all custom tools
└── cmd/
    ├── enrichspec/    # regenerate the embedded full spec from the live API
    └── gendiscovery/  # regenerate public/ discovery artifacts from SKILL.md files
```

### 3.1 Run modes (`main.go`)

`main.go` bootstraps logging, loads `.env` from the working directory, parses flags, and
selects one of three modes:

| Mode      | Trigger                          | Behaviour                                                        |
|-----------|----------------------------------|-----------------------------------------------------------------|
| **stdio** | default                          | Standard MCP transport over stdin/stdout (how hosts launch it)  |
| **SSE**   | `--sse` (+ `--addr`)             | HTTP server with Server-Sent Events for remote/multi-client use |
| **CLI**   | positional args: `<tool> k=v …`  | Run a single tool once and exit — handy for scripting/testing   |

In MCP mode the server always tees debug output to `/tmp/simulator.log`. Verbose
per-request logging (including request bodies) is gated behind `SIMULATOR_DEBUG`.

### 3.2 Tool-generation pipeline (`app/mcp-server/server.go`)

`LoadSwaggerServer` is the heart of the bridge:

1. **Load spec** — `specs.go` embeds `swagger/sim-public-swagger.full.json` via
   `//go:embed`; `app/swagger/loader.go` parses it into `models.SwaggerSpec`. (A different
   spec can be loaded from a URL/file via flags for development.)
2. **Build operations** — walk every `path` × `method`, apply include/exclude filters,
   producing one operation record each.
3. **Register auto-generated tools** — for each operation:
   - tool name = `operationId` (fallback: `<method>-<path-segments>`);
   - the JSON request schema is expanded into typed MCP parameters where possible,
     otherwise a single JSON-string `body` parameter is used;
   - a few operations are **special-cased** because the model needs help:
     `createActor` (resolve `formName`→`formId`, cache the new actor), `massLink`
     (inject the `layerId`), `createLink` (look up the hierarchy edge-type id).
4. **Register custom tools** — the 11 hand-written tools in §4 are added on top.
5. **Execute** — when a tool is called, `executeOperation` builds the HTTP request
   (Bearer token from `.env`, `accId` workspace query param auto-injected), sends it via
   the shared `apiHTTPClient()` (with a request timeout and connection reuse), and returns
   the response body to the model.

### 3.3 Workspace & auth context

Every API call needs a Bearer token and a workspace id (`accId`). Both live in `.env`:

- the token is written by the `login` tool (see §5);
- the workspace id is written by `set-workspace` as `WORKSPACE_ID` and auto-appended to
  every outgoing request as the `accId` query parameter.

A static `ACCESS_TOKEN` in the environment overrides saved OAuth credentials.

### 3.4 The two swagger specs

| File                              | Ops | Role                                                                 |
|-----------------------------------|-----|----------------------------------------------------------------------|
| `sim-public-swagger-all.json`     | 80  | Hand-curated spec — reuse source carrying canonical operationIds     |
| `sim-public-swagger.full.json`    | 185 | **Embedded & served at runtime** — full `/papi/1.0` surface          |

The live API doc (`https://mw.simulator.company/api/1.0/doc/json`) is served *without*
`operationId`/`summary`. The `enrichspec` tool (§6) pulls it, back-fills deterministic
camelCase operationIds and summaries, and **reuses** the curated spec so the canonical
operationIds the server special-cases (`createActor`, `getForm`, `manageLayer`,
`createLink`, `massLink`, …) are preserved. The result is `…full.json`, which `specs.go`
embeds. The curated spec is kept only as the reuse source, not served directly.

---

## 4. Custom (hand-written) tools

These 11 tools are registered in `LoadSwaggerServer` in addition to the auto-generated
operations. They exist because they wrap multi-call workflows, do client-side computation,
or massage payloads the raw REST endpoints get wrong.

| Tool                     | Source file              | Purpose                                                                                  |
|--------------------------|--------------------------|------------------------------------------------------------------------------------------|
| `login`                  | `server.go` + `app/auth` | OAuth2 PKCE flow; opens the browser and writes the token to `.env`                       |
| `set-workspace`          | `server.go`              | Persist the active `WORKSPACE_ID` (`accId`) to `.env`                                     |
| `createActors`           | `server.go`              | Bulk-create up to 50 actors in one call; resolves `formName`→`formId`                     |
| `pullGraphFile`          | `sync_graph.go`          | Export a layer's actors + edges to `<layerId>.yaml` for local editing                    |
| `pushGraphFile`          | `sync_graph.go` / `push_graph.go` | Diff a local YAML against the server layer and create/update/delete to match    |
| `getAllLayerPlacements`  | `get_layer_placements.go`| Paginate `/graph_layers/paginated/{layerId}` to return every actor placement in one call |
| `compactGraphLayout`     | `compact_layout.go`      | Auto-layout a layer into domain-clustered grids (one call replaces pull→edit→push)        |
| `pruneLongEdges`         | `prune_edges.go`         | Delete edges longer than a Manhattan-distance threshold; preserves hierarchy edges       |
| `uploadActorPicture`     | `upload.go` + `svg.go`   | Upload an actor picture (URL/file/base64); auto-rasterises SVG→PNG, can inject a fill colour |
| `uploadActorPictureBulk` | `upload.go`              | Set pictures on up to 500 actors; dedupes identical sources by SHA-256                   |
| `createChart`            | `create_chart.go`        | Create a dashboard chart actor (dynamic `actorFilter` or explicit accounts mode)         |

Notable client-side helpers behind these tools:

- **SVG rasterisation** (`svg.go`) — pure-Go `oksvg`+`rasterx`, capped at 4096×4096, so the
  graph UI (which can't render SVG storage paths) always gets a PNG.
- **Graph sync** (`sync_graph.go`, `push_graph.go`) — the largest and most delicate logic:
  a bidirectional diff that detects unchanged actors, minimises API calls, and handles
  cascading deletes. Form name→id maps are cached per-workspace under a `sync.RWMutex`.

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

- **Storage**: plaintext `.env` in the working directory, mode `0600`. Writes are
  serialised under a mutex and token + expiry are written in a single pass to avoid
  half-written credential files.
- **Defaults**: `ACCOUNT_URL` → `https://account.corezoid.com`; OAuth client id is
  overridable via `SIMULATOR_OAUTH_CLIENT_ID` (on-prem deployments set their own).
- **TLS**: certificate verification is **on by default**; `--insecure` is required to talk
  to self-signed gateways.

---

## 6. Build & regeneration tooling

There is **no build step to run the plugin** — hosts launch it with `go run .`. The
`Makefile` targets exist to regenerate the committed artifacts and to gate CI:

| Target            | Command                                            | What it does                                                              |
|-------------------|----------------------------------------------------|---------------------------------------------------------------------------|
| `make enrich-spec`| `go run ./cmd/enrichspec --input <live> --reuse … --output …full.json` | Regenerate the embedded full spec from the live API doc        |
| `make check-spec` | `go run ./cmd/enrichspec --input <live> --check …` | **Drift gate** — fails if upstream added ops missing from the embedded spec |
| `make discovery`  | `(cd mcp-server) go run ./cmd/gendiscovery --root ../../..` | Regenerate `public/llms.txt` and `public/.well-known/skills/index.json` |
| `make build`      | `go build ./...`                                   | Compile the server                                                        |
| `make vet`        | `go vet ./...`                                     | Static checks                                                             |
| `make test`       | `go test ./...`                                    | Run tests (currently none — see §7)                                       |

### `cmd/enrichspec`

Pulls the live OpenAPI doc, then for each operation tries an exact (`method`+full path)
then a fuzzy (normalised path) match against the curated spec to reuse a known
operationId/summary; otherwise it generates a deterministic camelCase id
(`<verb><Resource>[ByParam]`), de-duplicating collisions. It then asserts that all
canonical operationIds the server special-cases are present, failing loudly if not.

### `cmd/gendiscovery`

Walks `plugins/simulator/skills/`, parses each `SKILL.md` frontmatter, and emits the two
AI-discovery files under `public/`. This replaced an earlier Python script so the repo has
a single Go toolchain.

---

## 7. Known gaps & risks

These are documented for transparency and as a backlog for contributors:

- **No automated tests.** `go test ./...` reports `[no test files]` across all packages.
  The highest-value targets are the graph-sync diff (`sync_graph.go` / `push_graph.go`,
  the most complex logic in the repo) and the auth/credential flow.
- **Silently ignored errors.** Several `_ = json.Unmarshal(...)` / `_ = auth.Save(...)`
  sites swallow failures; a failed token persist or a malformed API response can pass
  unnoticed. Wrapping these in a logged warning would make failures visible.
- **`--insecure` not fully plumbed.** The flag exists in `main.go` but the swagger loader
  and OAuth client construct their own TLS config; verify the flag reaches every client
  before relying on it.
- **Large `server.go`.** Tool registration, special-casing and execution all live in one
  ~3k-line file; extracting the custom-tool registrations would improve navigability.

---

## 8. Where to go next

- Entity model and field semantics → [`plugins/simulator/docs/entities/`](../plugins/simulator/docs/entities/README.md)
- End-to-end walkthroughs → [`plugins/simulator/docs/user-flows/`](../plugins/simulator/docs/user-flows/README.md)
- API operation catalogue → [`plugins/simulator/skills/simulator/references/api-operations.md`](../plugins/simulator/skills/simulator/references/api-operations.md)
- Running / developing the server → [`plugins/simulator/mcp-server/README.md`](../plugins/simulator/mcp-server/README.md)
- Working in this repo as an agent → [`AGENTS.md`](../AGENTS.md)
