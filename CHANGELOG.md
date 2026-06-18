# Changelog

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
