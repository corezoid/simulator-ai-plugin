# Changelog

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
