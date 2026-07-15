# Changelog

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
