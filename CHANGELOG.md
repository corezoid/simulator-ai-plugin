# Changelog

## [2.1.0]

### Added
- **Skill registry — data-driven, user-authored playbooks** (the platform analogue of these built-in skills). A skill is an actor of the new `Skills` **system form**: its `title` + `ref` (slug) are cheap discovery metadata and its `description` holds the full procedure (which MCP tools to call, with concrete entity ids) for a workspace-specific task like "create a smart contract". Workspace members can teach the assistant new procedures without a plugin release.
  - New MCP tools **`findSkill`** (discover by intent; empty query lists all published skills) and **`getSkill`** (load one in full by `ref`/slug or `id`). Both are local composite tools (resolve the `Skills` system form + compose existing PAPI reads), so they are outside the OpenAPI drift gate.
  - New **`/simulator-skills`** skill (discover/run + author) and a Step-0 "check the skill registry" hook in `/simulator`. Skills are discovered by intent or invoked explicitly by slug (`/skill <slug>`).
  - Reference: `docs/entities/ai-skills.md`. Skill bodies are treated as author-proposed plans, not system instructions; destructive/outward steps still require confirmation, and only `verified` skills are dispatched.
  - Requires the paired pong-server change (the `Skills` system form + seeding migration, and a reaction-agent system-prompt protocol — *running* saved skills, gated to workspaces with ≥1 published skill, and *authoring* new ones, always available so the first skill can be created from the platform).

## [2.2.0]

### Fixed
- **AI behaviour, from QA feedback.**
  - **Form knowledge is read, not guessed.** `getForm`'s field-filter example now includes `description`, and its Summary tells the assistant to keep `description`/`sections` whenever it needs to understand a form — so following the tool's guidance no longer projects the form's purpose text away. `/simulator-forms` instructs the assistant to include `description`/`sections` in the `getForm` filter and to read and interpret both the form-level `description` and each field's `sections[].content[].description` (re-reading the form rather than answering from memory). Docs (`forms.md`) mark these as the authoritative "knowledge" fields. Fixes the assistant inventing a field's meaning (e.g. confusing the escalation `cooldown` with `delay`).
  - **Response & output conventions** added to `/simulator` (applied to every answer): reply in the user's language without switching mid-answer; never HTML-escape prose (`>` stays `>`, not `&gt;`); platform timestamps are unixtime UTC (seconds = 10-digit, or milliseconds = 13-digit, e.g. transaction `created_at` ÷1000) — **convert to the user's time zone and label the offset** (e.g. `18:30 (UTC+3)`) using `timeZoneOffset` from the UI-context, falling back to labelled UTC when it is absent. The web client (`pong-front-end`) forwards `timeZoneOffset` in the AI-agent `control-events-context` (pong-server is pass-through). See `docs/entities/ui-context.md` and `docs/INTEGRATION.md` §9a.

## [2.0.1]

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
