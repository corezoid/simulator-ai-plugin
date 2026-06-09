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
- handling of credentials, tokens, and `.env` material.

Out of scope:

- vulnerabilities in the upstream Simulator.Company backend / `pong-server` (report to the
  platform team),
- issues that require a malicious local `.env` the user wrote themselves.

## Handling of secrets

- TLS verification is on by default; the server warns when it would send a token over plaintext
  HTTP to a non-local host.
- Tokens and `.env` are never logged or committed. If you find a path where they leak, that is
  in scope — please report it.
