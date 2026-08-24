# Changelog

## [Unreleased]

### Changed
- simulator-app-generator: design quality is now an explicit deliverable — a gated design brief
  in Phase 3 (§5.3a), tokens seeded in Phase 4, an acceptance-criteria quality bar (§10.1) and a
  mandatory human visual pass (§10.2)

## [2.7.0]

### Changed
- [CE-15707] Sync the edge-hole link contract into the Simulator plugin

## [2.6.0]

### Added
- anonymous tool-call analytics + opt-in email (#86)

### Changed
- bump actions/setup-go from 6 to 7 (#80)
- bump github.com/mark3labs/mcp-go from 0.55.1 to 0.57.0 in /plugins/simulator/mcp-server (#84)
- CE-15765 feat(graph): exportGraph/importGraph/uploadGraphFile/getTaskStatus (#90)
- CE-15784 docs(links): closing an actor hole keeps each link's kind (#85)

### Fixed
- mime type, conflict detection, dup guard (#79)
- reconcile missing file id after create (#78)
- set customize_response:false on callback nodes (#82)

## [2.5.0]

### Added
- actor geolocation fields on createActor/updateActor (#74)

### Changed
- CE-15667 feat(actors): filterActors linkedToActorDirection param (#77)
- add CDU UI pattern recipes & DOM/protocol notes (#70)
- document extra.reverseEdge link-direction flag

### Fixed
- self-healing MCP path resolution in dev checkouts, bump to 2.4.1 (#72)
- pushSmartForm Windows path bug, bump to 2.4.1 (#71)
- getAccounts defaults to limit=100; getActor validates the UUID up front (#69)

<!-- PRs: add your entry under ## [Unreleased] (### Added / Changed / Fixed).
     Do NOT bump the version or add a dated section — that is minted at release
     time by `make release VERSION=x.y.z`. See AGENTS.md → Versioning & releases. -->

## [Unreleased]

### Added
- **`simulator-app-generator` skill.** Generates a complete multi-page Smart Form app from a set
  of existing Corezoid process ids plus a product description: pulls every process, derives its
  real input/output contract from its `api_rpc_reply` nodes (declared `params` drift and are only
  a cross-check), designs a page map that covers all of them, builds the Smart Form and the
  bridging Corezoid middleware process that calls them via `api_rpc`, binds both envs, then
  verifies end-to-end (`lint-process` + `pushSmartForm`, synthetic `run-task` payloads, and a
  live `appGetPage`/`appSendForm` drive) with a self-repair loop. Adds
  `docs/user-flows/app-generation.md` with the extraction algorithm, middleware skeleton, and
  test-payload catalogue.
- **Anonymous tool-call telemetry + opt-in email.** The MCP server now sends anonymous usage
  events (tool name, duration, error type, API hostname, transport, server version, a
  per-installation UUID, and MCP client name/version) to the same Corezoid ingest process
  corezoid-ai-plugin uses, tagged `product: "simulator"` so the two stay distinguishable
  downstream. No tokens, workspace/actor/form identifiers, or graph content are ever sent. Opt out
  entirely with `SIMULATOR_ANALYTICS_DISABLED=1`. After the first successful `login`, clients that
  support MCP elicitation are offered a one-time opt-in to include an email address, stored in
  `~/.simulator/preferences.json`. New `internal/telemetry` package; wired via
  `server.WithToolHandlerMiddleware` in `app/mcpserver.New` so it covers every registered tool
  without touching individual handlers. See README's Telemetry section and SECURITY.md for the
  full field list.
- **Graph import/export tools — `exportGraph`, `importGraph`, `uploadGraphFile`, `getTaskStatus`.** Wraps the pong-server async task API so a workspace graph (actors, edges, forms, and optionally attachments / transactions / processes / users / balances) can be exported to a `.graph` archive or re-imported, mirroring the UI's Export/Import buttons — distinct from the existing `pullGraphFile`/`pushGraphFile` developer sync tools, which edit a single layer's YAML and never touch `.graph` archives. `exportGraph` requires at least one of `actors`/`forms`/`allWorkspace`; `uploadGraphFile` accepts a `.graph` file as base64 or a public URL (capped at 100 MiB either way) and returns a storage `fileName` for `importGraph`; `getTaskStatus` polls a task by id and, for a completed export, returns a ready-to-share `downloadUrl` alongside the raw `details.file.fileName`.

### Fixed
- **`serverInfo.version` reported `2.1.0` while every manifest was at `2.7.0` (#89).** The version
  returned in the MCP `initialize` handshake came from two stale Go consts (`cmd/server`'s and
  `mcpserver.defaultVersion`) that `scripts/release.sh` never bumped — it only touched the six
  manifests. Collapsed them into one exported source of truth, `mcpserver.DefaultVersion`, which
  `cmd/server` now reads; `release.sh` bumps it in lockstep with the manifests, and a new
  `TestDefaultVersionMatchesManifest` fails CI if it ever drifts again. Also bumped
  `.kiro-plugin/plugin.json`, which had fallen a release behind (`2.5.0`).
- **`loadSysForms` cached transient failures and could serve valid data alongside a stale error
  (#87).** The success path never cleared `sysFormsErr`, and both failure paths cached the error
  with `sysFormsLoaded=true`; in the stateless (SSE) server, a failing and a succeeding request
  racing on the same workspace could leave the cache permanently `{validForms, staleErr}`, after
  which every caller (`if sysErr != nil …`) silently stopped resolving form-name→id until restart.
  Now only successful loads are cached (a failure is retried, never poisoned), the success write
  runs under a double-check so a concurrent winner is reused rather than clobbered, and the unused
  `sysFormsErr` field is removed.
- **`createEdgeLink` ignored the per-item `error` flag from `mass_links` (#88).** The response's
  `error bool` was parsed but never checked, so an `{error:true, data:{id:…}}` item would be read
  as a created edge and recorded as a live link — a silent ghost. The success branch is now guarded
  on `!resp.Data[0].Error`. Defensive: the current backend strips the id from a failed item, so
  there is no active data loss today, but the contract is now enforced.
- **`pushSmartForm` could not see cross-file token defects, so an unresolved `[[key]]` shipped
  silently.** `cduschema.ValidateFile` is per-file by signature — it can never tell whether a
  page's `[[key]]` resolves against the locale files or whether a `{{key}}` has a viewModel default
  — and the save endpoint stores page source opaquely, so nothing reported it until a literal
  `[[key]]` appeared in the browser. New `cduschema.ValidateTree` audits the whole env tree (not
  only the files being written: deleting a viewModel default breaks an untouched page) and
  `pushSmartForm` runs it alongside the per-file pass. A missing **locale** key is an error (locale
  resolves from files only; nothing at runtime can supply it); a missing **viewModel** default is a
  warning (the bound process may fill it per request). Also reports a `label`/`image` bound to a
  default of `""` (the renderer rejects an empty value), a default no page references, and — for a
  *literal* `contentLoop` — the exact entries missing a key the template uses. Placeholders inside a
  *templated* `contentLoop` are backend-filled and never reported, so list pages stay quiet; only a
  section's `content` is loop-scoped, and `regexp`/`mask` are skipped entirely so a character class
  like `^[[:alpha:]]+$` is not read as a locale token. A locale miss blocks only when the page or one
  of the locale files feeding it is part of the push — the same miss in an untouched page is
  pre-existing debt and is reported as a warning, because `pushSmartForm` has no force flag and
  aborting on it would strand an unrelated fix. Warnings are surfaced on a successful push under a
  new `warnings` field.
- **`pushSmartForm` accepted item keys and nested shapes the renderer then rejected.** The swagger
  sets `additionalProperties: false` nowhere, so the save endpoint stores a typo'd `visibilty` or a
  `head[].id` verbatim and the defect surfaces only as a console error in the browser — the item
  simply never hides, or the column list renders empty. `cduschema` now derives, from the bundled
  swagger, the property surface of every item `class` and of the nested spots where the schema *is*
  precise (`extra`, `options[]`, a table's `head[]` / `body[]`), and `ValidateFile` rejects a key no
  variant declares. The allowlist is the **union** over every schema variant of a class, because the
  swagger splits one class across type variants that each redeclare only part of the surface
  (`value` is on `Edit-int` but not on `Edit-default`) — checking a single variant would reject
  valid config. The union is then widened with the fields the
  renderer accepts but the swagger omits — the §4 base envelope (`value`, `required`, `error`,
  `errorMsg`, `submitOnChange`, `extra`) plus the §5 rows the swagger under-describes
  (`mainMenu.options`, `carousel.items`, `comments.title`, `timer.extra.duration`,
  `file.extra.{downloadUrl,uploadUrl,auth}`, `upload.extra.compression`,
  `attachment.extra.downloadUrl`) — because the derived union alone was NARROWER than the documented
  protocol and rejected those shapes outright. `TestProbeDocumentedKeysPresentInUnion` now walks the
  whole §5 table rather than a hand-picked subset, so a swagger update that drops a real key fails
  the tests instead of blocking users' pushes; a class the swagger never described (`row`,
  `draggable`) keeps no rule and is skipped rather than rejected. Also transcribed the renderer-only
  rules the swagger cannot express (it carries no `minLength` anywhere): a `label`/`image` `value`
  may not be an empty string, and an `image` `value` may not be a `data:` URI — the renderer proxies
  it through `/api/1.0/image?src=`, which rejects the scheme with `400 "URL is not allowed"`.
  Documented in `cdu-page-protocol.md` §5.1 / §10.1.
- **App-generator docs: the callback `api` node snippet was incomplete and failed `lint-process`.**
  All three copies of it (`simulator-smart-forms-logic` §2.7/§2.8, `simulator-app-generator` §7.2,
  `app-generation.md` §4.4) omitted `format`, `send_sys`, `debug_info`, `cert_pem` and
  `max_threads`. Following them verbatim produced a process that fails the JSON-schema gate
  (`missing property 'max_threads'`) and the `UNDERSPECIFIED API CALL NODES` check — whose
  real-world symptom is a server commit that hangs ~15–20 s then reports `no response from server`.
  All three snippets are now complete, with the cost of trimming them spelled out. Also documents
  that `extra.code` may be templated (`"{{respCode}}"` with `extra_type.code:"number"`), so one
  callback node can serve 200/205/302 instead of one node per code.
- **App-generator docs: the `err_node_id` invariant drove authors into a lint-flagged
  anti-pattern.** §7.7 required an `obj_type: 3` escalation for *every* fallible node; for an error
  path with no work to do that produces a passthrough escalation (`lint-process` flags it), and
  padding it with a throwaway `set_param` earns `UNUSED SET_PARAM` **plus** `SHARED ERROR CLUSTERS`.
  Now states both shapes — straight to an `obj_type: 2` final when there is no logic, escalation
  only when there is — plus the two facts that make "every branch still answers the runtime"
  reachable: an escalation's `go` may rejoin the happy path, and each branch needs its own callback
  node so nine branches don't share one error terminal.
- **App-generator docs: generated apps looked broken by default.** The two platform defaults that
  wreck an unstyled app — `.section__content` shipping its own grey background plus
  `padding: 20px 16px 0`, and `[data-class="grid-one-column"]` being capped and centred — were
  documented only in `simulator-styles`, which the app-generator reached for in Phase 8, long after
  the pages were authored. New §6.1 makes the resets, the app shell and the per-page
  `grid.styleClass` hook part of Phase 4; Phase 8 is now explicitly about branding rather than
  rescue.
- **Contract extraction missed real inputs in two node shapes.** `api_copy` carries its payload in
  `data`/`data_type` (it has no `extra` field at all) and an `api` node with `format: "raw"` carries
  it in `raw_body`; neither was mentioned anywhere in the plugin, and `raw_body` had zero
  occurrences. A scan reading only `extra` reports such processes as input-free — and, in a real
  run, reported a non-existent "sends an empty email" defect in a correct process. §1.7 /
  `simulator-app-generator` §3.6 now table all three carriers, and §3.9 asks for the field you read
  to be named before any defect is reported about someone else's backend.
- **Side-effect classification scored unreachable nodes.** Corezoid never prunes orphans, so a
  repurposed process keeps every sender it ever had: the reference `Cashback Categories` has 126
  nodes of which **6** are reachable, and scoring the whole bag marks a safe read-only process as
  `likely` and excludes it from probing. New §2.1 requires a BFS from Start (over `to_node_id` +
  `err_node_id` + `semaphors[].to_node_id`) before scoring, notes that unreachable reply nodes
  otherwise invent outcome branches, and adds a `nodes: "<reachable> of <total>"` manifest field.
- **A declared input can be dead.** The mirror of the documented output-side `params` drift: a
  process may declare an `input` no node ever reads, because the value actually comes from a
  state-diagram read (`{{conv[<id>].ref[SessionData].Session}}` — one workspace-wide session, not a
  per-user token). New §1.7a plus a `deadDeclaredInputs` manifest field.
- **Two verified backend facts the extraction docs were missing.** §1.3: a reply value may be a
  literal JSON *string* rather than a `{{var}}` — such a process is a constant source, so
  `run-task` returns empty task data and the schema must be read off the literal. And Corezoid
  translation refs inside that literal resolve to the **empty string**, not to themselves, so a
  regex hunting `t'([A-Za-z]+)` in the value can never match and a label map keyed on the ref is
  dead code — recover from a surviving sibling field instead (the reference FAQ used `url`). §1.8:
  `create-alias` accepts only `a-z`, `0-9` and `-`, so an underscored sub-process name
  (`chudo_get`) is rejected outright; pick the dashed form up front.
- **App-generator docs: "seed every `{{key}}`" was unqualified and produced false positives.**
  Placeholders inside a `contentLoop` section's `content` are **loop-scoped** — substituted from the
  entries the backend returns — so they are not viewModel keys and must not be seeded or reported as
  undefaulted (only the array binding itself, e.g. `{{promos_loop}}`, needs a default). Phase 4 now
  states the exclusion. Also aligns §9.1 with `pushSmartForm`'s new cross-file token audit (locale
  misses are errors, viewModel misses warnings) and tells the reader to actually read `warnings`,
  since they do not block a push; and §9.3's L3 assertion now covers unresolved `[[` as well as
  `{{`.
- **App-generator docs: the canonical node table contradicted its own error-cluster rule.** §4.1
  listed a single Callback GET, a single Callback SEND and one Error final while the paragraph below
  it required a callback node and error terminals **per branch** — and §7.8 told the brief to
  instantiate that table verbatim, so following it earned the `SHARED ERROR CLUSTERS` the same PR
  documents. §4.1 is now a spine plus a GET/SEND branch template instantiated per page and per
  button; only the logic-free Success final is shared.
- **App-generator docs: the side-effect heuristic banned every reader from `/get`.** "`api` node
  with a non-GET method → strong" marks any POST-to-read process `likely`, which §2.2/§5.1a then
  forbids on a page's `/get` — including the reference `Transactions history`, which the coverage
  table itself puts there. The method is now an explicitly weak signal; the strong ones are a
  mutating verb in the URL path or callee name, an `api_copy`, and an outbound reply nothing reads.
- **App-generator docs: a session token in the `302 query` was only conditionally discouraged.** The
  query is the page URL — history, `Referer`, proxy logs, and any link the user shares hands over the
  session. A domain session token is now treated as sensitive by default: park it in a state process
  or actor and carry an opaque id; a bearer token reaches a URL only when the backend leaves no
  alternative and the user has been told.
- **App-generator docs: node-budget guidance counted the wrong nodes, and two size levers were
  undocumented.** The "split above ~60 nodes / 8 pages" threshold gave no hint that error clusters
  scale mechanically with the number of fallible nodes — `layout-process` measures **32%** of both
  reference handlers as error nodes (77 → ~52 business, 74 → ~50) — so following it splits graphs
  that are comfortably inside it. §7.1 / §4.1a now say to budget business nodes only. §4.1a also
  gains a fourth pattern, **mapping an outcome in one Code node instead of a `go_if_const` tree**
  (7 mappers instead of 7 conditions plus ~20 builders and their clusters on the reference `/send`
  handler), stated with both sides of the trade: it removes the unbracketed-nested-`param` silent
  fallthrough entirely, but hides that branching from the Corezoid canvas — so the `path` / `page` /
  `buttonId` dispatches stay real condition nodes. §7.5 adds the two structural defences that stop
  `submitOnChange` correctness depending on a hand-maintained id list: prefer zero such fields when
  the page has no cascade, and make the dispatch default a no-op ack rather than a submit.
- **`simulator-styles`: silent Less compile failures had no documented check.** A Less error is
  emitted as a `/* Less Error … */` comment at serve time and the page renders unstyled;
  `pushSmartForm` does not compile CSS and `appGetPage` never returns it, so nothing reported it.
  Adds a verified local recipe, including the three traps that break the obvious command: Smart Form
  partials have no `.less` extension, `lessc` cannot read a `<(…)` process-substitution path
  (`EBADF`), and the npm package is `less` — a package literally named `lessc` exists and is not the
  compiler.
- **Smart Form visibility placeholders rejected by `pushSmartForm`.** Page configs may use a pure
  `{{viewModelKey}}` placeholder for form, section, and rendered-item `visibility`; validation now
  accepts that server-resolved form while still rejecting malformed or embedded placeholders.
- **`appGetPage` / `appSendForm` could not carry a page `query`, making the platform's own session
  pattern untestable.** A Smart Form is stateless: a `302` answers `{nextPage, query}` and the next
  page reads it as `body.query.*`, which is how apps carry a session token across navigation.
  Neither runtime tool accepted it, so a logged-in page could not be rendered at all — driving one
  produced a cold page that looked like a backend bug. Both `appGetPage` and `appSendForm` gain
  `query`, flattened into the URL query string exactly as the renderer sends it — including on the
  `appSendForm` POST, whose handler reads the query off the URL and forwards it to the process as
  `body.query` (a body-borne `query` is overwritten there and never arrives). New `InQueryMap`
  param kind in `internal/tools/op.go` does
  the flattening — plain `InQuery` would have sent the whole object as one opaque value, silently
  dropping the session; it rejects a non-object and skips blank keys / nil values (a nil would
  otherwise render as the literal `"null"`).
- **Telemetry: unsynchronized `telemetryEmail` read/write.** The opt-in email was stored in a plain
  `var string`, written by `AskForEmailOnce` (after `login`) and read by `Middleware` on every tool
  call — safe under the current single-threaded stdio transport, but a data race under `go test
  -race` if a concurrent transport (HTTP/SSE) were ever added. Now uses `atomic.Pointer[string]`,
  matching the `atomic.Bool` discipline already used for the telemetry `enabled` flag.

## [2.5.0] - 2026-07-14

### Added
- **Actor geolocation — `geoPosition` / `geoName` on `createActor` / `updateActor` (#74).** An actor now carries an optional real-world position independent of its form `data`: `geoPosition` — `{"lat": <number>, "lon": <number>}` in WGS84 decimal degrees, or `null` to clear — and `geoName` — a location-name string (max 255 chars), or `null`. Coordinates are validated backend-side: latitude is hard-bounded to -90..90 (out of range rejected), longitude outside ±180 wraps cyclically, and values round to 6 decimals; `lat`/`lon` are set as a pair. Documented in `/simulator-actors` and `docs/entities/actors.md`; drift snapshot and an eval scenario updated.

## [2.4.2]

### Fixed
- **Kiro MCP server path resolution in a dev checkout.** `.mcp.kiro.json`'s `${KIRO_PLUGIN_ROOT:-$PWD/.kiro/..}` fallback resolved to the repo root (not `plugins/simulator/`) when a developer opened the repo directly in Kiro without running `install-kiro.sh`, so the server tried (and failed) to run `<repo>/mcp-server/run.sh`. It now probes for `mcp-server/run.sh` at that path and falls back to `plugins/simulator/` when missing, with a final guard that prints a clear error and exits if neither candidate has it. `install-kiro.sh` also now `sed`-resolves the same fallback to the absolute plugin path (escaping `\`, `&`, and the `#` delimiter) when generating a workspace's `mcp.json`, instead of a plain copy, so an external workspace's config no longer depends on `KIRO_PLUGIN_ROOT` at all. README's Kiro install instructions updated to match.

## [2.4.1]

### Fixed
- **`pushSmartForm` failed on Windows when creating files in a new subfolder (e.g. a new Smart Form page `pages/<id>/config`).** Phase 2 mapped the server's create response back to local paths using `filepath.Dir`, which yields backslash-separated paths on Windows and misses the slash-keyed folder map — so the push aborted with "server did not return id for created file …" even though the server had already created the folder and files (a subsequent `pullSmartForm` showed them). The response mapping now reuses `resolveParentID` (the same `ToSlash`-normalized lookup used when POSTing), keeping the key consistent across OSes. macOS/Linux behaviour is unchanged (`ToSlash` is a no-op there).

## [2.4.0]

### Added
- extend agents to any actor, not just user twins
- add /simulator-agents digital-twin agent skill + findAgent/getAgent
- add /simulator-agents digital-twin agent skill + findAgent/getAgent
- expose hole field on createLink; document edge-hole
- add simulator-styles skill (#61)
- resolve target entity before creating; offer Total for debit/credit pairs (#60)
- add AWS Kiro support (#42)

### Changed
- fix Codex test step — no Plugin Directory GUI in CLI
- fix Codex plugin commands (install→add, update flow)
- record CDU Smart Forms doc changes under 2.4.0 (#68)
- record CDU Smart Forms doc changes under 2.4.0
- detect submitOnChange by buttonId, not buttonData.action (#67)
- document CDU rendering gotchas (#62)
- getForm filter guidance — request `form`, not `sections` (#66)
- document CDU form links & button.extra spec (#64)
- note #60 skill behaviour under 2.3.0
- release v2.1.0

## [2.4.0]

### Added
- **`/simulator-styles` — Smart Form (CDU) styling skill (#61).** A dedicated specialist for the `style` / `styles/` (Less/CSS) layer of Smart Forms: theme tokens, page/form/section layout, component re-skinning, reusable patterns, and design-system approaches. It reuses the existing `pullSmartForm` / `pushSmartForm` / `deploySmartForm` cycle and consumes the `styleClass` hooks that `/simulator-smart-forms` attaches — the two skills hand off explicitly (structure vs. styling). Ships with a new rendered-DOM reference (`docs/user-flows/cdu-dom-tree-reference.md`) mapping each page-config element to the tag tree and stable class hooks the CSS must target. Also corrects the CDU section/layout model in `cdu-page-protocol.md` and `/simulator-smart-forms`: a section has no `footer` slot, the wizard header class is `steps` (not `stepper`), and `row` is authored via the base `row`/`w` fields (the renderer synthesizes the component) with `w` as a relative weight.
- **CDU Smart Form rendering gotchas (#62).** New §12 in `cdu-page-protocol.md` (with pointers from `/simulator-smart-forms` and `/simulator-smart-forms-logic`) documenting non-obvious renderer behaviour verified against `control-cdu`: a `row` renders as a CSS `table`, so its items do **not** wrap on desktop (use `display:inline-block` items in `content` for a wrapping grid) — though `row` does collapse to one column at the mobile breakpoint; `styleClass` on a `row` is dropped from the DOM (style the leaf components); a `radio`/`select` option "dot" is a JSS-hashed `<i>` (not the `<input>`) — restyle via `[class*="icon"]`/`[class*="content"]` and drive the selected state off the `.checked` class (one `button` per option is the version-proof card-picker alternative); and a `visibility:"visible"` field hidden via CSS still submits (the idiomatic hidden value carrier).
- **CDU form links & full `button.extra` spec (#64).** `cdu-page-protocol.md` and `/simulator-smart-forms` now document that `[url=https://…]text[/url]` bbcode renders a clickable `<a target="_blank">` (raw `<a>` HTML is escaped — use `[url]`; `[iurl]` gives a same-tab link), and complete the `button.extra` reference: `url` (opens the URL instead of submitting) with `target` (`_self`/`_blank`, honoured by newer renderers — older ones open the same tab), `action:'logout'`, `request` (a bare `fetch` before submit, which proceeds only if it resolves), `autoSubmit {interval,maxCount}` (interval clamped 5–60s), `options[]` (a click menu that bypasses `url`/`request`/`action`/submit), `icon`, `rounded`, `mobileVisible`.

### Fixed
- **`getForm` filter guidance (#66).** The tool Summary told callers to keep `sections`, but `filter` projects **top-level keys only** and a form's fields live under `form.sections`, so `filter=…,sections` (and the dotted `form.sections`) returned nothing. It now says to request `form`; the top-level `description` (the form's purpose) is unchanged.
- **`submitOnChange` detection in `/simulator-smart-forms-logic` (#67).** §2.3a/§2.9.3 taught detecting a field change via `body.buttonData.action != ""` and listed `check`/`radio` as emitting a non-empty `action` — but only `select` sends `buttonData` (verified against `control-cdu`); `radio`, `check`, `toggle`, `edit`, … send `{}`, so that guard silently routed those changes to the real-submit path (the wizard "jumping a step" / a half-filled form persisted). Detection now dispatches on `body.buttonId` (which §2.3 already recommends); `action` stays only as a `select` `select`-vs-`filter` signal.

## [2.3.0]

### Added
- **`pictureObject` on `createActor` / `updateActor` — custom-image ("napkin") nodes (#57).** An actor can now render a custom image AS the node body instead of a standard form node: pass `pictureObject` `{"img": "data:image/png;base64,…" (PNG/SVG data URI), "width": 800, "height": 8, "type": "napkin"}`. The image is anchored at its centre and keeps the source aspect ratio (set `width`; `height` follows) — e.g. a wide-and-short source PNG for a thin divider line. Uses: dividers/separators, a custom shape or icon the form catalogue does not cover, or an embedded picture/logo on a graph. Documented in `docs/entities/actors.md` and `/simulator-graph`.
- **`ref` on `updateActor` — re-key an actor in place (#59).** `updateActor` now accepts an optional `ref` body param, reaching parity with `createActor`: an existing actor can be given or reassigned its external business key (1–255 chars, unique per `formId`) without recreating it (which would mint a new UUID and break its links/placements). Resolve it afterwards with `getActorByRef(formId, ref)`; omit `ref` to leave it unchanged. (The backend `PUT /actors/actor/{formId}/{actorId}` accepts `ref` in the body even though it is absent from that endpoint's request schema in the drift spec — verified live; the drift gate only checks method/path/operationId, so it stays green.)
- **`getLayerActorsPaginated` — paginated layer reads for large graphs (#58).** `getLayerActors` loads a whole layer in one call and the backend rejects it with a `400` (`Layer is too large (… nodes, … edges, total: …). Maximum allowed: N.`) once nodes+edges exceed the layer-size cap (~300), which previously left the assistant unable to read big layers. The new curated tool maps to `GET /graph_layers/paginated/{actorId}` and returns one page of either `nodes` or `edges` (`type`, `limit` ≤ 50, `offset`, `filter`); walk `offset` per `type` to traverse a layer of any size. `getLayerActors` now documents the size cap and points at the paginated tool, and `/simulator-graph` defaults to the paginated read (call `layerStats`, then page) rather than the whole-layer call. The graph skill also now teaches how to extract the layer `actorId` from a pasted graph URL (`.../graph/<graphActorId>/layers/<layerActorId>` → the segment after `/layers/`) and is explicit that "read the nodes on a graph/layer" means reading that layer's placements — never a workspace-wide `searchActors`/`filterActors`, which previously pulled in unrelated chats, daily reports and other forms. `buildLink`'s `layer` help/params were also reworded to use the same graph-URL vocabulary (`/graph/<graphActorId>/layers/<layerActorId>`) so the two docs agree on which segment is the graph folder vs. the layer.
- **Edge styling surfaced on `manageLayerActors` (#55).** The edge `layerSettings` description now documents the full set of pong-server edge-styling keys that already ride through the passthrough `items` array — `lineStyle` (`solid`/`dashed`/`dotted`), `curveStyle` (`curved`/`rounded`/`roundedDownward`/`straight`), `color` (6-digit `#RRGGBB` hex, no shorthand/alpha), `width` (integer ≥ 1) and `routingPoints` (`{w,d}` manual-routing waypoints) — so an edge's colour, thickness and curve are now discoverable and settable. A new "Styling edges" section in `/simulator-graph` shows the pattern (to restyle an edge already on the layer, delete its placement and re-create it). Documentation only — the fields already reached the wire.
- **Skill registry — data-driven, user-authored playbooks** (the platform analogue of these built-in skills). A skill is an actor of the new `Skills` **system form**: its `title` + `ref` (slug) are cheap discovery metadata and its `description` holds the full procedure (which MCP tools to call, with concrete entity ids) for a workspace-specific task like "create a smart contract". Workspace members can teach the assistant new procedures without a plugin release.
  - New MCP tools **`findSkill`** (discover by intent; empty query lists all published skills) and **`getSkill`** (load one in full by `ref`/slug or `id`). Both are local composite tools (resolve the `Skills` system form + compose existing PAPI reads), so they are outside the OpenAPI drift gate.
  - New **`/simulator-skills`** skill (discover/run + author) and a Step-0 "check the skill registry" hook in `/simulator`. Skills are discovered by intent or invoked explicitly by slug (`/skill <slug>`).
  - Reference: `docs/entities/ai-skills.md`. Skill bodies are treated as author-proposed plans, not system instructions; destructive/outward steps still require confirmation, and only `verified` skills are dispatched.
  - Requires the paired pong-server change (the `Skills` system form + seeding migration, and a reaction-agent system-prompt protocol — *running* saved skills, gated to workspaces with ≥1 published skill, and *authoring* new ones, always available so the first skill can be created from the platform).
- **`/simulator-agents` — digital-twin agents: talk to a person as an agent and delegate work** (the people-analog of the skill registry above). Every workspace user has a 1:1 twin actor (`systemObjType="user"` on the `System` form) whose `description` holds an "# Agent" competency profile (System instructions + Knowledge — what they do, what they know, whether they fit a task); the registry is the workspace's user twins instead of the `Skills` form. The delegation procedure — discover the right person, load and adopt their profile, then either do the task autonomously (under the caller's access), find a better-suited person, or hand the decision to the user (create a task / send a p2p message) — is layered on top and **reuses** the `/simulator-tasks` and `/simulator-chat` flows.
  - New MCP tools **`findAgent`** (discover people by competency over their twin profiles via `searchAll`; empty query lists members via `getUsers`) and **`getAgent`** (load one person's "# Agent" profile in full by `userId` — get-or-creates the twin — or `actorId`). Both are local composite tools (`agents.go`, resolve the `System` system form + compose existing PAPI reads), so — like `findSkill`/`getSkill` — they are outside the OpenAPI drift gate.
  - New **`/simulator-agents`** skill and reference doc `docs/entities/digital-twin-agents.md` (the twin-as-agent model, the "# Agent" profile format, `findAgent`/`getAgent`, inject-as-instruction, the caller-access boundary), plus cross-references from `/simulator`, `/simulator-chat`, and `/simulator-tasks`.
  - A twin profile is treated as **data, not system instructions** (prompt-injection guard, mirroring the skill registry): it cannot escalate privileges or skip confirmations, and everything runs under the caller's PAPI access — the twin never grants the target person's privileges. Outward/destructive actions still require confirmation. Populating the "# Agent" `description` is done by an external factory from git activity and is out of scope here.
  - **Any actor can be an agent, not just user twins.** An agent is now defined as **any actor** whose `description` holds an "# Agent" profile — the common case is a person's user twin, but a non-user system actor (a service/bot/device twin) or a plain business actor (a team, department, organization, process, service) can be one too. `findAgent` gains an optional **`formId`** (a numeric form id, tolerant of a JSON-number value): it still defaults to the user-twin registry (`System` form), but a form id targets another **agent-registry** form; an empty query enumerates the chosen registry (`getUsers` for the default, `filterActors` for a `formId`). The search is always form-scoped — findAgent never runs an unscoped workspace-wide actor search (that would rank arbitrary actors as agents, since `/search`'s `filter` is field projection, not an "# Agent" predicate); use the general `searchActors`/`searchAll` tools for cross-form discovery. `getAgent` documents that `actorId` loads **any** agent actor (not only a person twin). The `/simulator-agents` skill and `docs/entities/digital-twin-agents.md` are reframed around agent-as-actor: discovery branches on the result's `systemObjType` (a `"user"` twin vs a candidate non-user agent, whose "# Agent" profile is confirmed via `getAgent` before use), and delegation offers the choices that fit the agent — task / p2p message for a person, or routing to the members behind a non-user agent / triggering a runnable actor. Person twins remain the default and behave exactly as before (backward compatible).

### Fixed
- **`buildLink` open-layer URL.** An id-less `entity=layer` link now builds the canonical `/graph/<graphActorId>/layers/<layerActorId>` shape: the open graph (folder, from the UI context's `activeGraph`) fills the `/graph/` slot and the open layer (`activeLayer`) is the focused element. Previously it dropped `activeLayer` into the `/graph/` slot and never used `activeGraph` at all, yielding `/graph/<layerId>/layers`. Platforms that send only `activeLayer` (no `activeGraph`), or a degenerate `activeGraph == activeLayer`, keep the old fallback (`/graph/<layerId>/layers`) so existing links still resolve; in that fallback any caller-supplied `focusId` is dropped rather than appended, since the layer already occupies the path slot (avoids a malformed `/graph/<layer>/layers/<focusId>`). Requires the paired pong-server change: the public `/graph_layers/paginated/{actorId}` route now declares `operationId: 'getLayerActorsPaginated'`. The committed drift spec (`internal/tools/testdata/papi-openapi.json`) is updated to match; re-dump it from pong-server (`yarn dump-openapi`) on the next refresh.
- **AI behaviour, from QA feedback.**
  - **Form knowledge is read, not guessed.** `getForm`'s field-filter example now includes `description`, and its Summary tells the assistant to keep `description`/`sections` whenever it needs to understand a form — so following the tool's guidance no longer projects the form's purpose text away. `/simulator-forms` instructs the assistant to include `description`/`sections` in the `getForm` filter and to read and interpret both the form-level `description` and each field's `sections[].content[].description` (re-reading the form rather than answering from memory). Docs (`forms.md`) mark these as the authoritative "knowledge" fields. Fixes the assistant inventing a field's meaning (e.g. confusing the escalation `cooldown` with `delay`).
  - **Response & output conventions** added to `/simulator` (applied to every answer): reply in the user's language without switching mid-answer; never HTML-escape prose (`>` stays `>`, not `&gt;`); platform timestamps are unixtime UTC (seconds = 10-digit, or milliseconds = 13-digit, e.g. transaction `created_at` ÷1000) — **convert to the user's time zone and label the offset** (e.g. `18:30 (UTC+3)`) using `timeZoneOffset` from the UI-context, falling back to labelled UTC when it is absent. The web client (`pong-front-end`) forwards `timeZoneOffset` in the AI-agent `control-events-context` (pong-server is pass-through). See `docs/entities/ui-context.md` and `docs/INTEGRATION.md` §9a.
  - **Resolve *which* entity to create before touching domain data (#60).** A new "Step 0.5" in `/simulator` tells the assistant to resolve a named business entity ("smart contract", "supplier", …) before calling any entity-specific tool: search template forms first (`searchForms`/`getForms` on `title`/`description`), then check for prior actors of the matched form (`searchActors`/`searchAll`), and only create after the user confirms. A workspace-specific business form takes priority over any platform system mechanism that merely sounds related — incidentally stumbling onto `AccountTriggers`/`Tags` while inspecting data is a data point to mention, not a substitute for the form search, so the task no longer silently pivots into configuring a similarly-named mechanism. An already-resolved term is reused within the same conversation instead of re-searching/re-asking. Fixes the assistant defaulting to `AccountTriggers` for "smart contract".
  - **`/simulator-finance` offers Total for directional account pairs (#60).** When `getAccounts` returns both a `debit` and a `credit` account for the same `(nameId, currencyId)` pair, the assistant now offers **Total** (the net balance, `credit.amount − debit.amount`) as a third choice alongside Debit/Credit, and asks which the user wants before proceeding. Total is always computed, never a stored row — matching pong-server's own `incomeType: 'total'` semantics (`credit − debit` over the two directional rows).

## [2.2.0]

### Added
- add AWS Kiro support (#42)

## [2.2.0]

### Added
- AWS Kiro support. The same plugin payload now installs on Kiro alongside the existing Claude Code and Codex hosts via a symmetric overlay: `plugins/simulator/.kiro-plugin/plugin.json`, `plugins/simulator/.mcp.kiro.json`, `plugins/simulator/steering/simulator.md`, and a root-level `POWER.md` distribution manifest for kiro.dev/powers.
- `plugins/simulator/scripts/install-kiro.sh` sets up an existing Kiro workspace from a cloned repo: copies the MCP entry, symlinks the steering file, hard-copies each skill into `.kiro/skills/<name>/`, and `sed`-substitutes `$CLAUDE_PLUGIN_ROOT` in every `SKILL.md` with the absolute plugin path (Kiro does not substitute the token on its own, unlike Claude Code and Codex). Idempotent — re-run after a `git pull` to refresh the workspace overlay.

### Notes
- The canonical `SKILL.md` files keep `$CLAUDE_PLUGIN_ROOT` — Claude Code and Codex both resolve that exact token via host-side text substitution (anthropics/claude-code#48230, #47789, #44057) and renaming it would break doc loading on both. The install-time `sed` substitution in `install-kiro.sh` is the only host-specific bit.
- There is no release-zip Kiro overlay artifact in this version. A pre-built zip would still need a post-extract substitution step (the token can only be resolved to an absolute path that the user actually checked out), so the clone + `install-kiro.sh` path is currently the only correct install flow for Kiro.

## [2.1.0]

### Added
- `updateLayerPositions` MCP tool — reposition actors already present on a layer within the same layer (use `moveActors` to move actors between layers).
- `updateAccountName` gains a `transferOnly` boolean parameter; transfer-only behaviour documented in the finance skill and the accounts entity doc (mirrors pong-server CE-15565).

### Changed
- Bump `github.com/mark3labs/mcp-go` from 0.54.1 to 0.55.0.
- CI and release workflows: bump `actions/checkout` v4→v7, `actions/setup-go` v5→v6, `actions/upload-artifact` v4→v7, `actions/attest-build-provenance` v2→v4, `softprops/action-gh-release` v2→v3.

### Fixed
- `buildLink` chat deep-links now point at a conversation correctly: `/chats/<acc>/list/chats/<chatActorId>?tab=chat` (the stream segment defaults to the standard `chats` stream and the required `?tab=chat` query is included). `id` is the chat-actor UUID; omit it to open the chat list.
- AI agent now finds files the user attached to their **triggering message**. The message is a reaction under the root actor, so its files live on the reaction, not the actor — the agent was calling `getActorAttachments` on the root actor and reporting "no attachments". Documented that an actor's own attachments and the triggering message's attachments are **two distinct sets**, both read via `getActorAttachments(<id>)` → `readAttachment` (the actor for "files on this actor", the triggering reaction for "the file I sent"). Added an `activeReaction` field to the UI context (`control-events-context`) so the platform can hand the trigger id directly, with a `getReactions(... orderValue=DESC)` fallback when it's absent. (Populating `activeReaction` requires a matching pong-server change.)
- UI-context guidance now states that `activeActor` (the actor the user is *viewing*) outranks the *root actor* the agent was triggered on: "this / the current / the open actor" resolves to `activeActor`. Fixes the AI agent answering about the root/console actor instead of the on-screen one. (Paired with a pong-server `buildPrompt` change that asserts the same priority in the agent's prompt.)

## [2.0.0]

First public release of the Simulator.Company plugin for Claude Code / Codex.

### Added
- MCP server (Go) wrapping the Simulator.Company REST API and exposing ~100 curated tools across actors, forms, graph, finance, access, reactions, attachments, charts, users and Smart Forms.
- Authentication: API-key flow plus OAuth2 PKCE with MCP Elicitation; TLS verification on by default.
- Curated tool surface declared as typed operations and validated against the backend OpenAPI spec by a drift gate.
- Skills covering the master router plus actors, forms, graph, finance, access, reactions, attachments, charts, chat, meetings, tasks, init, and Smart Forms (author, logic, runtime).
- Smart Form runtime — `appGetPage` / `appSendForm` drive any CDU / Script mini-app conversationally; convention-free discovery via the form's own title / description / tags.
- Local helper tools: `buildLink`, `getBbcodeTags`, `readAttachment` (text / image / binary-aware).
- Plugin manifests for Claude Code and Codex, plus the local agents marketplace.
- Architecture and per-entity documentation, contributor guides, and MIT license.
