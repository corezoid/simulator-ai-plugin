# Changelog

## [Unreleased]

### Added
- **Server-side field selection (`filter`) on every read tool — token-saving.** All read/lookup/list tools now expose an optional `filter` query parameter: a comma-separated allow-list of fields to return (e.g. `id,title,data.status`; dotted paths pick nested `data` fields). The backend prunes the response to just those fields via `filterActorData` / `filterData`, so a tool call can fetch only what it needs instead of the full entity. Centralised as `fieldFilterParam` in `internal/tools/op.go`.
  - Tools whose backend already supported `filter`: added it to `getActor`, `getBalance`, `getTransactions`, `searchActors`, `searchLayerActors`, `filterActors`, and **fixed** the two pre-existing `filter` params on `getAccounts` and `getRelatedActors` whose descriptions wrongly called them data-filter expressions (the backend always treats `filter` as field selection).
  - **Added `filter` support in pong-server** for the remaining read routes and wired the matching tools: `getActorByRef`, `getForm`, `getForms`, `searchForms`, `getCurrencies`, `getAccountNames`, `getTransfer`, `getEdgeTypes`, `getLayerActors`, `listSmartForms`, `searchAll`, `getWorkspaces`. Each backend route declares `filter` on its Fastify querystring schema and applies `filterActorData` (actor/node responses: `getActorByRef`, `getLayerActors` nodes, `searchAll` actor results — user results untouched, `objType` preserved) or `filterData` (everything else); `workspaces` clones session-held objects before projection.
  - The drift gate (`(method, path, operationId)` only) still passes; refresh `testdata/papi-openapi.json` via pong-server's `yarn dump-openapi` once the backend changes deploy so the dumped spec carries the new params. Documented in README / `docs/ARCHITECTURE.md`.

### Removed
- **`software-migration-onramp` skill.** Dropped the migration discovery facilitator skill (and its `prompts/` specs); the plugin now ships 6 skills. Removed its references from README / `docs/ARCHITECTURE.md` / AGENTS / the `simulator` skill and regenerated the discovery artifacts (`public/`).

### Added
- **Local dev preset is now profile-gated.** `set-environment` offers the `local` preset (`localhost:9000`) only in a local-dev session (startup profile `local`, i.e. `SIMULATOR_PROFILE=local` / `--profile local`); the default `prod` profile advertises only the cloud gateways (`mw`, `sim`) + custom URL, so end users are never prompted with localhost. Documented the local-dev invocation in README / AGENTS, and dropped the stale `SIMULATOR_DEBUG` references (it was never read in code).
- **Multi-environment support + `set-environment` tool.** Before login the user now chooses an environment — a cloud preset (`mw.simulator.company` default, `sim.simulator.company`) or a custom/local/on-prem URL (host or full URL; `/papi/1.0` appended if omitted). Since pong-server authenticates through the `account` system and one account may back several sim environments, the OAuth account URL is no longer fixed per gateway: `set-environment` fetches the gateway's public, unauthenticated config (`getConfigReq` → `data.saUrl`) to derive it, persists `SIMULATOR_API_BASE_URL` + `ACCOUNT_URL` to `.env`, and clears any token + workspace (so you re-`login`). The environment can be switched at any time mid-session — the `apiclient` and engine base URLs are mutated in place (guarded by `RWMutex`) so it takes effect without a restart, and `login` follows the chosen account URL. New `apiclient.FetchPublicConfig`; `config.CloudEnvironments` presets + `NormalizeAPIBaseURL`; `config.Resolve` honours a persisted `ACCOUNT_URL` on restart. Documented in README / `docs/ARCHITECTURE.md` and the `simulator-init` / `simulator` skills.
- **`filterActors` tool** (`GET /papi/1.0/actors_filters/{formId}`) — list/rank the actors of a form by an account's **current balance** in a single server-side query. Set `linkedToActorId` to scope candidates to one anchor actor's graph neighbours (hierarchy link, both directions), answering "the actors related to X whose account N balance is > / < a value"; `accountNameId`+`currencyId` select the account, `amountFrom`/`amountTo` set the threshold, `orderBy=balance` ranks. Filters on current balance only (turnover-over-period stays a per-actor `getAccounts(from,to)` read; the backend's period-balance path is still disabled). Declared the matching `operationId: 'filterActors'` on the pong-server public route and refreshed the drift spec. Re-introduces, in curated Go form, the `filterActors` tool dropped in the rewrite. Documented in README / `docs/ARCHITECTURE.md` and the `simulator-finance` / `simulator-graph` skills.

### MCP server rewrite
- Replaced the monolithic `app/mcp-server/server.go` Swagger→MCP bridge with a **layered server** under `cmd/server/` + `internal/`: `config` (local/prod profiles, env/`profiles.json`-overridable), `apiclient` (auth header + accId injection + timeouts + error mapping, workspace guarded by `RWMutex`), `tools` (a curated, typed `Operation` registry → one MCP tool per backend operation, declared per domain), and `engines` (the ported client-side tools). OAuth/credentials reused from `app/auth`.
- **Curated tool set (~46 tools)** scoped to the core scenarios — forms / actors / accounts / transactions / graph (links & layers) / applications & smart forms — plus the engine tools `pullGraphFile`, `pushGraphFile`, `getAllLayerPlacements`, `compactGraphLayout`, `pruneLongEdges`, `uploadActorPicture(Bulk)`, `createChart`. No more 185-op passthrough.
- **Environment by profile**: `--profile local|prod` (or `SIMULATOR_PROFILE`); local targets a dev pong-server on `:9000` with the same OAuth flow as prod. `.mcp.json` now launches `go run ./cmd/server`.
- **Backend operationIds at the source** + a **spec drift gate**: tool declarations are validated against `internal/tools/testdata/papi-openapi.json` (dumped from the live backend); it already caught a phantom `getApplication` tool. Plus a structural **eval harness** (`eval-scenarios.json`).
- **Tests**: first tests in the module — config, apiclient, tool scenarios (incl. `-race`), drift gate, eval, engine registration.
- **Removed** the legacy tree: `app/mcp-server`, `app/{models,swagger}`, root `main.go`/`specs.go`, `cmd/enrichspec`, and the bundled `sim-public-swagger*.json` (the full-spec embed + enrichspec generator are no longer needed).
- Known limitations: `createActor` takes a numeric `formId` (no `formName` resolution); behavioural (LLM-in-the-loop) eval is a CI/manual step. See `docs/INTEGRATION.md`.
- Clarified that account `amount`/balances are stored as their real decimal value and currency `precision`/`decimals` is display-only (never scale by `10^precision`) — in the `createTransaction`/`finalizeTransaction`/`getAccounts`/`getBalance`/`createCurrency` tool descriptions, the `simulator-finance` skill, and `docs/entities/accounts.md`.
- **Security hardening** of the engine tools: reject non-UUID `layerId`/`actorId`/`filterActorId`/`accountNameId` arguments before any filesystem or API access (closes path traversal in `pullGraphFile`/`pushGraphFile` and URL/query injection); escape graph-file-derived IDs interpolated into API URLs; restrict the `uploadActorPicture(Bulk)` `localPath` source to image extensions with a 25 MiB cap; HTML-escape reflected values in the OAuth callback page; warn when the API base URL would send the auth token over plaintext HTTP to a non-local host. Added `internal/engines/security_test.go`.
- **Linter**: added `golangci-lint` v2 (`make lint`, config `plugins/simulator/mcp-server/.golangci.yml` shared with sibling services). Fixed the real `gosec` findings it surfaced — `ReadHeaderTimeout` on the OAuth callback server (G112), tighter file/dir permissions on written graph files and discovery artifacts (G306/G301); documented the remaining trusted-input taint false positives in `gosec.excludes`. `gosec` is clean; the broader `default: all` set still has a style/modernization backlog (lint is advisory, not yet gating). Documented `make lint` in README / AGENTS / CLAUDE / ARCHITECTURE.
- Synced the lagging `.agents/plugins/marketplace.json` version (was `1.5.0`) with the other manifests.

## [1.5.0]

### Full API coverage
- The MCP server now exposes the **full `/papi/1.0` public API surface (185 operations, up from 48)**. Added a Go generator (`cmd/enrichspec`) that pulls the live OpenAPI doc (`https://mw.simulator.company/api/1.0/doc/json` — served without `operationId`/`summary`), back-fills deterministic camelCase operationIds and summaries, and **reuses the hand-curated specs** so the canonical operationIds the server special-cases depend on (`createActor`, `getForm`, `manageLayer`, `createLink`, `massLink`, …) are preserved. `specs.go` now embeds the generated `swagger/sim-public-swagger.full.json`; the curated `sim-public-swagger-all.json` (80) is kept as the reuse source.
- Added a `Makefile`: `enrich-spec` (regenerate), `check-spec` (drift gate — fails if upstream adds ops missing from the embedded spec), `discovery`, plus `build`/`vet`/`test`.

### Security & reliability fixes
- **OAuth2 CSRF `state`**: the PKCE flow now generates a random `state`, sends it in the authorization URL, and rejects callbacks whose `state` does not match (prevents authorization-code injection).
- **HTTP client hardening**: introduced a single shared `apiHTTPClient()` with a request `Timeout` (the main `executeOperation` client previously had none and could hang a goroutine forever) and connection reuse. TLS certificate verification is now **on by default**; pass `--insecure` only for self-signed gateways (previously verification was hardcoded off in ~10 places).
- **`.env` writes** are now serialised under a mutex and the token + expiry are written in a single pass (no more races / half-written credential files); a token with no derivable `exp` falls back to a conservative 12h window instead of "never expires".
- **`papiGET` (layer pull)** now checks the HTTP status, so a 401/500 no longer silently parses as an empty layer.
- **SVG rasterization** is capped at 4096×4096 (was unbounded → OOM via large `pngWidth/pngHeight`); `looksLikeSVG` precedence bug fixed; fill-check regex hoisted out of the hot loop.
- **base64 image input** now accepts URL-safe and unpadded variants, not only `StdEncoding`.
- **`pushGraphFile`**: an edge whose endpoint isn't placed on the layer is no longer counted as a placed edge (and logs a warning) instead of inflating `EdgesCreated`.
- **`compactGraphLayout`**: cluster height now scales to the tallest cluster, so large clusters no longer overlap the row below.
- Resource-read path check now requires a separator (no sibling-dir prefix bypass); removed dead `setRequestSecurity`; fixed the `set-workspace` description (`WORKSPACE_ID`, not `SIMULATOR_ACC_ID`) and the advertised server name typo (`swagger-mcp`).

### Repo hygiene
- Removed the dead duplicate `./app/` tree (stale copy importing the defunct `git.corezoid.com/mw161089sar/...` paths; canonical code lives under `plugins/simulator/mcp-server/`).
- Rewrote the AI-discovery generator in Go (`cmd/gendiscovery`, replacing `scripts/generate-discovery.py`) so the repo has a single Go toolchain; `public/llms.txt` + `public/.well-known/skills/index.json` are regenerated by `make discovery`.
- Fixed the root `.mcp.json` (it launched a nonexistent `scripts/start-mcp.sh` and passed the obsolete `SIMULATOR_TOKEN`); it now mirrors the working `go run` launcher pointing at `plugins/simulator/mcp-server`.
- Synced the plugin version to `1.5.0` across `.claude-plugin/marketplace.json` and `.agents/plugins/marketplace.json`.

## [1.4.0]

- Add `uploadActorPictureBulk` MCP tool: set pictures on up to 500 actors per call, dedupes identical source images by SHA-256 so the same icon is uploaded once and reused, supports `picture` shortcut to bind an already-uploaded storage path without re-uploading bytes.
- Auto-rasterise SVG sources to PNG inside `uploadActorPicture` (and bulk variant) via pure-Go `oksvg`+`rasterx`: defaults to 256×256, optional `pngWidth`/`pngHeight` overrides, and `svgFillColor` injects a brand colour on the `<svg>` root for monochrome simpleicons. The graph UI doesn't render SVG storage paths, so callers no longer need a local `rsvg-convert` install.

## [1.3.5]

- Add `pruneLongEdges(layerId, maxDistancePx?, bucketThreshold?, preserveParentEdges?, dryRun?)` MCP tool. Walks every edge on a layer, deletes those whose Manhattan distance between endpoints exceeds `maxDistancePx` (default 600 px). By default keeps edges where either endpoint is a hierarchy bucket (≥ `bucketThreshold` incoming edges, default 3). `dryRun:true` previews without deleting. Returns scanned/deleted/kept_short/kept_parent counts plus up to 10 example deletions.

## [1.3.4]

- Add `compactGraphLayout(layerId, strategy)` MCP tool. Implements the `domain-clusters` strategy: actors with `>= bucketThreshold` incoming edges become cluster headers, their children are arranged in a grid under them, and the clusters themselves are laid out in a super-grid (default 4 columns). Orphans stack in a Misc zone. Tunable via `clustersPerRow` / `nodesPerRow` / `nodeDX` / `nodeDY`. One MCP call replaces the full pull → YAML → reposition → push loop. Strategy arg is reserved for future `hierarchical` / `force-directed` layouts.

## [1.3.3]

- Add `getAllLayerPlacements(layerId)` MCP tool: returns every `(actorId, laId, formId, title, position)` row on a layer in one call. The existing `getLayerActorsByFormId` requires the caller to enumerate every formId in use on the layer (often 15+); this tool walks `/graph_layers/paginated/{layerId}?type=nodes` internally instead, paginating to completion.

## [1.3.2]

- Fix `pushGraphFile` not propagating actor positions to the canvas. The internal `updatePositions` helper was sending a bare JSON array to `PUT /graph_layers/actors/{layerId}` and passing `laId` as an integer; the endpoint expects `{"items": [...]}` with `id` as a string and silently no-ops otherwise. Positions in YAML now reach the server on every push.


## [1.3.1]

- Rename edge title fields to camelCase.

## [1.3.0]

- Rebrand to simulator-ai-plugin and ship MCP server.
