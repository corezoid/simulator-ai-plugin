# Security Policy

## Supported versions

The plugin is released as a single line; the latest published version on the marketplace
receives security fixes. Please reproduce any issue against the current `main` before reporting.

## Reporting a vulnerability

**Do not open a public GitHub issue for security problems.**

Report privately using GitHub's
[private vulnerability reporting](https://github.com/corezoid/simulator-ai-plugin/security/advisories/new),
or email **security@corezoid.com** with:

- a description of the issue and its impact,
- steps to reproduce (a minimal MCP call sequence or graph file is ideal),
- the plugin version and your environment (OS, Go version, gateway URL).

We aim to acknowledge reports within 3 business days and to provide a remediation timeline
after triage. Please give us a reasonable window to release a fix before any public disclosure.

## Scope

In scope:

- the Go MCP server (`plugins/simulator/mcp-server/`) — auth handling, the API client, and the
  engine tools that touch the filesystem (`pullGraphFile`/`pushGraphFile`,
  `uploadActorPicture(Bulk)`),
- handling of credentials, tokens, API keys (`SIMULATOR_API_SECRET`), and `.env` material.

Out of scope:

- vulnerabilities in the upstream Simulator.Company backend / `pong-server` (report to the
  platform team),
- issues that require a malicious local `.env` the user wrote themselves.

## Handling of secrets

- TLS verification is on by default; the server warns when it would send a token over plaintext
  HTTP to a non-local host, and **refuses to start** when that credential is a long-lived
  `SIMULATOR_API_SECRET` (loopback exempt; `SIMULATOR_ALLOW_INSECURE_API_SECRET=1` overrides).
- Tokens and `.env` are never logged or committed. If you find a path where they leak, that is
  in scope — please report it.
- `SIMULATOR_API_SECRET` is read-only to the plugin: it is never written back to `.env`, never
  logged (only the *fact* that API-key mode is active), and never included in telemetry — the
  one-time email opt-in runs from `login`, which is disabled in that mode.

## Telemetry

The MCP server sends anonymous tool-call telemetry to a Corezoid-owned ingest endpoint
(`www.corezoid.com`, the same process corezoid-ai-plugin uses — events carry `product: "simulator"`
so the two plugins' data stays distinguishable). Each event carries: timestamp, tool name, call
duration, a fixed error-type enum (no free-form error text), the API hostname currently in use
(host only — no path or query), the serving transport, the server version, a random
per-installation UUID (`~/.simulator/installation_id`), and the connected MCP client's name/version
(e.g. Claude Code, Codex, Kiro).

**Never sent:** tokens, workspace/actor/form/graph identifiers, or any graph/actor/form content.

Set `SIMULATOR_ANALYTICS_DISABLED=1` to opt out entirely. After the first successful `login`, and
only if the connected client supports MCP elicitation, you are offered a one-time opt-in to
include your email address in telemetry — declining is always honored, and the choice (asked-once
flag, and the email if you opted in) is stored in `~/.simulator/preferences.json`, editable or
deletable at any time.
