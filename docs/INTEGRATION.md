# Plan — natural-language control of `pong-server` via MCP

> **Status: proposal / plan only.** No code or skills are changed by this document. It
> records the target design agreed for driving the `pong-server` (`control-api`) public
> API in natural language through the simulator MCP plugin.

## 1. Goal

Operate the `pong-server` platform in natural language: create and manage **forms**,
**actors**, **accounts**, **transactions**, **graphs** (links + layers), and
**applications / smart forms** — through a small, well-designed set of MCP tools plus the
existing domain skills.

## 2. Context — the plugin already targets this backend

`pong-server` (`control-api`) is the backend the plugin's bundled spec describes. The
alignment is exact, not coincidental:

| `pong-server` fact | Evidence | Consequence |
|---|---|---|
| Public API prefix `/papi/1.0/*` | `apiRegister.js:206-217` | Matches the plugin's `/papi/1.0` spec |
| OpenAPI 3.1 served at `/api/1.0/doc/json` (prod filters to `/papi/*`) | `swaggerRegister.js:16-59` | This is the spec source `enrichspec` pulls (via the `mw.` gateway) |
| `@fastify/swagger` emits `summary`/`description` but **no `operationId`** | `swaggerRegister.js`, route schemas | The whole `enrichspec` / operationId back-fill exists to compensate for this |
| Auth: `Authorization: Bearer <SA-token>` or `Simulator <jwt>`; SA = `account[.pre].corezoid.com`; `accId` from URL/query | `checkPublicApiAuth.js:128-207`, `checkAccAccess.js:9-22` | The plugin's OAuth2-PKCE → JWT + `accId` auto-inject fit this model |
| Entities: Actor, Form, Edge(link), Layer, Account, AccountName, Currency, Transaction, Transfer, AccessRule, Attachment, Reaction | `models/index.js:51-100` | Exactly the plugin's skill domain |
| ~230 public routes across 36 modules | `apiRegister.js:103-204` | Plugin's 185 ops ≈ 80% — but we will deliberately cover far less (see §3) |

**Conclusion:** we are not building a new solution. We curate, target, and harden the
existing spec-driven MCP bridge.

## 3. Scope decision — core scenarios only (NOT full coverage)

Covering all ~230 public routes is overhead and hurts LLM tool-selection accuracy. We
expose only the operations needed to complete these end-to-end scenarios:

1. **Forms CRUD** — define field schema + default accounts
2. **Actors CRUD** — instances of forms (graph nodes)
3. **Accounts CRUD** — financial/metric accounts on actors
4. **Transactions** — record on accounts (immediate + 2-step), transfers between accounts
5. **Graph building** — links + layers + placement/layout
6. **Applications** — build apps
7. **Smart Forms** — CDU / Script smart forms

Everything else (tasks, sip, communications, proxy, realtime, invites, users, push tokens,
…) is **off by default**, switchable on if a future scenario needs it.

## 4. Environment selection via config (local / prod)

The server picks an environment from config, not hardcoded URLs. Proposed model:

```jsonc
// profiles.json — read at startup (overridable by env / --profile)
{
  "active": "local",                       // ← or SIMULATOR_PROFILE=local|prod
  "profiles": {
    "local": {
      "apiBaseUrl": "http://localhost:9000",
      "accountUrl": "https://account.pre.corezoid.com",
      "specSource": "http://localhost:9000/api/1.0/doc/json"
    },
    "prod": {
      "apiBaseUrl": "https://mw.simulator.company",
      "accountUrl": "https://account.corezoid.com",
      "specSource": "embedded"             // //go:embed snapshot for determinism
    }
  }
}
```

- Resolution order: `--profile` flag → `SIMULATOR_PROFILE` env → `active` in config →
  default `prod` (backward-compatible).
- Each field individually overridable by env (`SIMULATOR_API_BASE_URL`,
  `SIMULATOR_ACCOUNT_URL`, `SIMULATOR_SPEC_URL`) so secrets/URLs stay out of the file.
- `.mcp.json` selects the profile via its `env` block — one launcher, two environments.
- Auth tokens stay in `.env` (per-environment), never in `profiles.json`.

Code touch points (for later implementation): `main.go` flag/env resolution + a tiny
profile loader; the existing `--baseUrl`/`--url` and `ACCOUNT_URL` plumbing already exists
and just gets fed from the resolved profile.

## 5. Tool design — "proper tools", not raw passthrough

### Principles

- **A tool models a user intent, not a REST endpoint.** One tool may compose several REST
  calls (e.g. *create actor with its accounts*); we do not expose 5 near-duplicate
  variants of one endpoint.
- **Typed parameters, not JSON blobs.** Where the auto-generated tool degrades to a single
  `body` JSON string, the curated tool expands real typed fields — LLMs call typed tools
  far more reliably.
- **Consistent naming:** `verbNoun` (`createForm`, `getForm`, `updateForm`, `deleteForm`,
  `searchForms`). Names match operationIds so skills, allowlist, and backend agree.
- **Read-before-write coherence:** every domain has get/search so the model can ground IDs
  before mutating.
- **Safety on destructive ops:** deletes and bulk ops support `dryRun` / return a diff;
  transactions use idempotency refs (`TransactionsUniqueRef`/`TransfersUniqueRef`).

### Two layers

- **Layer A — curated passthrough (allowlisted):** straightforward 1:1 CRUD that already
  has a good schema. Implemented by restricting the auto-generator to the allowlist below.
- **Layer B — hand-written ergonomic tools:** composites, typed wrappers over blob
  endpoints, and safety-wrapped mutations.

### Curated catalogue (target ≈ 40 tools, vs 185)

Status: `[exists]` already in the plugin · `[promote]` exists as raw passthrough, upgrade
to typed/safe · `[new]` to be written.

**1. Forms CRUD** — `forms.js`, `formAccounts.js`
| Tool | Status | Notes |
|---|---|---|
| `createForm(name, fields[], defaultAccounts[]?)` | promote | typed field schema + optional default accounts |
| `getForm` / `getFormByRef` | exists | |
| `getForms` / `searchForms(query)` | exists / promote | |
| `updateForm` | exists | |
| `deleteForm` | exists | confirm/dryRun |
| `setFormStatus(formId, status)` | exists | template/active |

**2. Actors CRUD** — `actors.js`
| Tool | Status |
|---|---|
| `createActor(formId\|formName, data, title?, color?, picture?)` | exists (special-cased) |
| `createActors(bulk ≤50)` | exists |
| `getActor` / `getActorByRef` / `getActorByObjId` | exists |
| `searchActors` / `filterActors` | exists |
| `updateActor` / `updateActorByRef` | exists |
| `deleteActor` / `deleteActorByRef` | exists (confirm/dryRun) |
| `setActorStatus` | exists |

**3. Accounts CRUD** — `accounts.js`, `accountNames.js`, `currencies.js`, `counters.js`
| Tool | Status | Notes |
|---|---|---|
| `createAccount(actorId, name, currency, type)` | promote | type ∈ asset/liability/expense/income/counter/state |
| `getAccounts(actorId)` / `getBalance` / `getCounters` | promote | |
| `updateAccount` / `deleteAccount` | promote | |
| `createCurrency` / `getCurrencies` | exists | |
| `createAccountName` / `getAccountNames` | exists | |

**4. Transactions & transfers** — `transactions.js`, `transfers.js`
| Tool | Status | Notes |
|---|---|---|
| `createTransaction(accountId, amount, type, ref?)` | promote | immediate or 2-step authorize |
| `completeTransaction` / `cancelTransaction` | new | 2-step finalisation |
| `getTransactions` / `searchTransactions` | promote | |
| `createTransfer(from, to, amount, ref?)` | promote | atomic; idempotent via ref |
| `getTransfers` | promote | |

**5. Graph building** — `graphLayers.js`, `edgeTypes.js`, actor edges
| Tool | Status |
|---|---|
| `createLayer` / `manageLayer` | promote / exists (special-cased) |
| `createLink` / `massLink` | exists (special-cased) / exists |
| `getEdgeTypes` | exists |
| `getLayerActors` / `getAllLayerPlacements` | exists |
| `getActorLinks` / `getLinkedActors` | exists |
| `pullGraphFile` / `pushGraphFile` | exists |
| `compactGraphLayout` / `pruneLongEdges` / `createChart` | exists |

**6 & 7. Applications & Smart Forms** — `applications.js`, `appContent.js`, `smartForms.js`, pages
| Tool | Status |
|---|---|
| `createApplication` (read via `getActor`) | new |
| `createSmartForm` / `getSmartForm` / `updateSmartForm` / `listSmartForms` | new |
| `createAppPage` / `manageAppContent` | new |

### Scenario → tools coverage (completeness check)

| Scenario | Tools used |
|---|---|
| Create a form with fields + accounts | `createForm` |
| Create actors of that form | `createActor` / `createActors` |
| Add accounts & record money | `createAccount`, `createTransaction`, `createTransfer` |
| Build a process graph | `createLayer`, `createActor`, `createLink`/`massLink`, `manageLayer`, `compactGraphLayout` |
| Build an application with smart forms | `createApplication`, `createSmartForm`, `createAppPage` |

The existing skills (`simulator-graph/forms/finance`) are updated to reference only the
curated tools.

## 6. How to obtain the API spec (recommended)

The backend's `@fastify/swagger` omits `operationId`, which is the root cause of the
current fuzzy-matching machinery. Since you own `pong-server`, fix it at the source:

**Recommended pipeline:**

```
pong-server  (add explicit operationId to /papi route schemas)
   └─ fastify.swagger()  ─►  papi-openapi.json   (filtered to /papi/*, WITH operationId)
         ├─ committed in pong-server repo (CI artifact)   ──┐
         └─ served live at /api/1.0/doc/json                │
                                                            ▼
   simulator-ai-plugin
     ├─ local profile:  fetch the live URL  (always fresh, no regen step)
     ├─ prod profile:   embedded snapshot + `make check-spec` drift gate
     └─ allowlist:      curated ~40 operationIds  →  MCP tools
```

Why this is best:
- **`operationId` at the source** makes tool names a stable contract owned by the backend;
  `enrichspec`'s fuzzy reuse becomes unnecessary (or a thin fallback).
- A **committed `papi-openapi.json`** means the spec is reviewable in PRs and reproducible
  without a running server.
- **Local = live fetch** (fast iteration), **prod = embedded snapshot** (deterministic
  startup, offline-safe), with the drift gate catching upstream additions.

### Best way for the agent (Claude) to read the spec, in priority order

1. **A committed `papi-openapi.json` with `operationId`** in `pong-server` — one
   authoritative file to read. *Preferred.*
2. **Live `GET /api/1.0/doc/json`** when a target instance is running.
3. **Parse `src/packages/controlMain/schemas/**`** directly — last resort; reconstructs
   intent from Fastify schemas but is not a real OpenAPI document.

**Decided:** `operationId` is added at the source (see §9). `enrichspec` and the
185-op full-spec embed are therefore retired — the plugin consumes a clean, curated
`papi-openapi.json` directly.

## 7. Rewrite decision & target architecture

The current `mcp-server` is hard to follow: `server.go` alone is **3107 lines** mixing spec
loading, operation building, tool registration, special-casing, execution, and custom-tool
wiring. Decisions #1 (operationId at source) + curated scope delete much of the reason that
complexity exists (`enrichspec` ~405 LOC, full-spec embed, fuzzy matching, fallback
naming).

**Verdict: rewrite the frame, keep the engines.** A from-scratch greenfield would recklessly
discard proven domain logic (graph sync, layout, charts, OAuth); a timid in-place refactor of
a 3k-line file is hard and most of it should be deleted anyway. So:

- **Rewrite (greenfield, small, tested):** entrypoint, server/transport wiring, spec
  loading, the generic OpenAPI→tool registry, the API client, config/profiles.
- **Port with cleanup + tests:** OAuth + credentials (`auth/`), graph sync
  (`push_graph.go`+`sync_graph.go`, ~1.5k LOC — the risky bit, test first), `compact_layout`,
  `prune_edges`, `get_layer_placements`, `upload`+`svg`, `create_chart`.
- **Delete:** `enrichspec`, the full/curated `sim-public-swagger*.json`, the 3k-line
  `server.go`, fuzzy-match + fallback-naming. Keep `gendiscovery` (orthogonal).

SDK: keep **`mark3labs/mcp-go`** (already integrated, mature); no switch unless a concrete
need appears.

### Target package layout

```
mcp-server/
  cmd/server/main.go        # thin: resolve profile → build deps → run transport
  internal/
    config/                 # profile resolution: flags > env > profiles.json > default
    auth/                   # [PORT] OAuth2 PKCE + .env credential store
    apiclient/              # HTTP: base URL, Bearer, accId inject, timeout, retry, error map
    spec/                   # load+parse curated papi-openapi.json (embed=prod, file/url=local)
    mcp/                    # stdio/sse transport wiring over mark3labs/mcp-go
    tools/
      registry.go           # one curated OpenAPI op (typed) -> one MCP tool
      forms.go actors.go accounts.go transactions.go graph.go apps.go
    graph/                  # [PORT] pure sync/layout/prune engines (unit-testable)
    media/                  # [PORT] upload + svg rasterize
    domain/                 # shared DTOs + enums (account types, statuses)
  papi-openapi.json         # committed curated spec (operationId from source)
  embed.go                  # //go:embed papi-openapi.json (prod profile)
  testdata/
```

Layering rule (the thing the current code lacks): `cmd → tools → {apiclient, spec, graph,
media, auth} → {config, domain}`. No package imports "upward". `tools.BuildAll(reg, deps)`
replaces the 3k-line `server.go`: simple CRUD is generated from the curated spec via
`registry`; composites/safety tools are hand-written in the per-domain files.

### Migration sequence & status

1. ✅ **pong-server**: `operationId` added to the 35 curated `/papi` route schemas (inline
   spread) + `tools/dump-openapi.js` + `yarn dump-openapi` script. Branch
   `feat/papi-operation-ids`. See §9. *(running the dump needs the datastore stack up — path (a))*
2. ✅ **New skeleton** with tests: `internal/config` (profiles), `internal/apiclient`,
   `internal/tools` registry (`op.go`), `cmd/server` stdio entrypoint. Unit + scenario tests
   (`internal/...`), incl. a `-race` concurrency test.
3. ✅ **Auth**: reused from `app/auth` (OAuth PKCE + creds), driven by `accountUrl` from the
   resolved profile. *(kept in `app/auth`; moves to `internal/auth` when `app/` is deleted)*
4. ✅ **Curated CRUD** for forms / actors / accounts / transactions declared as typed
   operations and registered via the registry.
5. ✅ **Engines ported** → `internal/engines` (pull/push graph sync, `compactGraphLayout`,
   `pruneLongEdges`, `getAllLayerPlacements`, `createChart`) reusing the proven legacy logic,
   registered via `engines.RegisterTools`. *(thin runtime config + cosmetic form-name stubs;
   sync-diff unit tests still TODO)*
6. ✅ **Media ported** (`upload` + `svg` rasterise) → `internal/engines`.
7. ✅ **Apps + smart-form tools** (`apps.go`): createApplication, createSmartForm,
   listSmartForms, manageAppContent. *(getApplication dropped — no backend route; the drift
   gate in step 10 caught it. Read an app via getActor.)*
8. ✅ **Skills updated** to the curated set (formId-before-createActor; `manageLayer`→
   `manageLayerActors`; per-skill curated-names notes) and **`.mcp.json` switched** to
   `go run ./cmd/server`.
9. ✅ **Legacy deleted**: `app/mcp-server`, root `main.go`/`specs.go`, `cmd/enrichspec`,
   `app/{models,swagger}`, `sim-public-swagger*.json`.
10. ✅ **Spec drift gate** (path (a)) — **active**: `internal/tools/drift_test.go` validates every
    curated op (method/path/operationId) against the committed
    `testdata/papi-openapi.json` (187 ops, dumped from the live backend via `yarn dump-openapi`).
    It already caught a phantom `getApplication` (no backend route — removed). Refresh the spec
    by re-running the dump and copying it over. *(A future step can switch the registry to
    generate tools from this spec; the gate already guards the hand-declared ops against drift.)*
11. ✅ **Eval harness** — structural: `eval-scenarios.json` (10 NL scenarios → expected tool
    sequences) + `eval_test.go` asserting every referenced tool is real. **Behavioural:**
    `cmd/evalrunner` (`make eval`) spawns the real server (`tools/list`), drives a Claude model
    through each prompt via the Anthropic API in a bounded tool-use loop (stubbed results — no
    backend writes), and checks the expected tools were called. Opt-in: skips without
    `ANTHROPIC_API_KEY`.
12. ✅ **Graph-sync unit tests** — `internal/engines/graphsync_test.go`: the diff's decision
    primitives (`formIDFromLayerActor`, form-name cache + nested resolution, actor-formId cache,
    override) plus an end-to-end `pushGraph` test driving the full diff (fetch → diff → delete)
    against a mock backend.

> **Resolved follow-ups:** `createActor` now accepts `formName` (resolved to the form id);
> optional boolean **query** flags are omitted when false (so `false` is not misread as
> "enabled"). Remaining: switching the registry to spec-generated tools is still optional
> (the drift gate already guards the hand-declared ops).

## 8. Environment auth (local mirrors production)

Both profiles use the **same OAuth2-PKCE flow**; only `accountUrl` (SA) and `apiBaseUrl`
differ. Local prerequisite: the local `pong-server` must trust the same SA the plugin logs
into (`account.pre.corezoid.com`) — i.e. its `auth.saSecretKey` / registered OAuth `control`
client point at that SA. No static-token path is required for normal use.

## 9. pong-server changes — operationId at source

In `pong-server` (separate repo), add an explicit `operationId` to each `/papi` route
schema, matching the curated camelCase names in §5 (`createForm`, `createActor`,
`createAccount`, `createTransaction`, …):

```js
// e.g. src/packages/controlMain/schemas/forms/forms.js
schema: { operationId: 'createForm', summary: '…', tags: ['forms'], body: {…} }
```

Then emit the spec as a build artifact (filtered to `/papi/*`, with operationIds):

```js
// scripts/dump-openapi.js — after app.ready()
fs.writeFileSync('papi-openapi.json', JSON.stringify(app.swagger(), null, 2))
```

This makes tool names a backend-owned contract and lets the plugin drop all fuzzy matching.

## 10. Resolved decisions

- ✅ **operationId at source** — yes, add in `pong-server` (§9).
- ✅ **Curated tool list (§5)** — confirmed.
- ✅ **Local auth** — same OAuth scheme as production, config-selected SA/base URL (§4, §8).
- ✅ **Rewrite** — frame rewritten, domain engines ported (§7).
