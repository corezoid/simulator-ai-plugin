---
name: simulator
displayName: Simulator.Company
version: 2.4.0
description: BPM, graph, and financial-tracking toolkit for the Simulator.Company platform. Exposes the Simulator REST API as MCP tools plus 16 skills covering actors, forms, graphs, layers, accounts, transactions, transfers, charts, smart forms, meetings, reactions, and attachments.
author:
  name: Simulator.Company
  url: https://simulator.company
homepage: https://doc.simulator.company
repository: https://github.com/corezoid/simulator-ai-plugin
license: MIT
keywords:
  - simulator
  - bpm
  - business-process
  - graph
  - financial
  - actors
  - forms
  - mcp
---

# Simulator.Company Power for AWS Kiro

A Kiro Power that brings the [Simulator.Company](https://simulator.company)
platform — actors, graphs, forms, financial accounts, transactions, and
dashboards — directly into your Kiro workspace as MCP tools and skills.

## Install

```sh
git clone https://github.com/corezoid/simulator-ai-plugin
cd simulator-ai-plugin
plugins/simulator/scripts/install-kiro.sh "$YOUR_KIRO_WORKSPACE"
```

Open the workspace in Kiro. The simulator MCP server registers under
`.kiro/settings/mcp.json`, the steering file under `.kiro/steering/`, and
every skill under `.kiro/skills/<name>/`. `install-kiro.sh` hard-copies the
skills into the workspace and resolves the `$CLAUDE_PLUGIN_ROOT` token in
each `SKILL.md` to the absolute plugin path, so reference docs at
`plugins/simulator/docs/...` resolve correctly under Kiro (Kiro does not
substitute the token on its own).

Re-running `install-kiro.sh` is idempotent: it refreshes the workspace
overlay in place.

## What it does

- **Actors & forms** — create, search, update, link, and inspect every
  business-process actor with the full form-schema overlay.
- **Graph layouts** — pull and push whole layers as YAML, auto-cluster nodes
  by domain (`compactGraphLayout`), prune sprawling edges (`pruneLongEdges`),
  enumerate placements in one call (`getAllLayerPlacements`).
- **Pictures & icons** — `uploadActorPicture` and bulk variant with SVG→PNG
  auto-rasterisation and SHA-256 dedup.
- **Financial accounts** — currencies, account names, transactions,
  transfers, balances, counters, and triggers.
- **Charts & dashboards** — line / bar / area / funnel charts pinned to a
  layer; pulled at runtime from real accounts and counters.
- **Smart forms** — design, deploy, and run CDU/Script smart-form
  applications.

## MCP tools (highlights)

| Tool | What it does |
|---|---|
| `login` | OAuth2 browser login; saves token to `~/.simulator/`. |
| `set-workspace` | Pick the active workspace by id or name. |
| `pullGraphFile` / `pushGraphFile` | Export / sync a layer as YAML. |
| `createActor` / `updateActor` / `deleteActor` | Single-actor CRUD. |
| `createActors` | Bulk create up to 50 actors per call. |
| `compactGraphLayout` | Auto-layout actors with domain-clustering. |
| `pruneLongEdges` | Delete edges exceeding a Manhattan distance threshold. |
| `getAllLayerPlacements` | One call returns every placement on a layer. |
| `uploadActorPicture` / `uploadActorPictureBulk` | Icons with SVG→PNG. |
| `createForm` / `updateForm` / `getForms` | Form-template lifecycle. |
| `createAccountName` / `createCurrency` / `addFormAccount` | Account setup. |
| `createTransaction` / `createTransfer` | Move value between accounts. |
| `createChart` / `createDashboard` | Visualise account / counter data. |

The full tool list is generated from the OpenAPI spec; ~190 operations are
exposed.

## Skills

Each skill is auto-loaded from `.kiro/skills/<name>/SKILL.md` and routes on
trigger phrases declared in the SKILL.md frontmatter. Highlights:

- `simulator` — universal entry point and platform overview.
- `simulator-graph` — graph layouts, actor links, layer canvas operations.
- `simulator-forms` — form templates, field types, system forms.
- `simulator-finance` — accounts, transactions, transfers, currencies.
- `simulator-charts` — charts, dashboards, time-series.
- `simulator-actors` / `simulator-tasks` / `simulator-attachments` — focused
  helpers for the corresponding entity.
- `simulator-smart-forms` (+ `-logic`, `-runtime`) — smart-form design and
  execution.
- `simulator-meetings` / `simulator-chat` / `simulator-reactions` —
  collaboration features.
- `simulator-init` — first-time environment setup.
- `simulator-access` — sharing, groups, API keys.

## Same codebase, three hosts

This repository ships the same plugin payload to:

- **Claude Code** — via `claude plugin install simulator@simulator`.
- **Codex** — via `codex plugin add simulator@simulator`.
- **AWS Kiro** — via this Power.

One Git tag → one GitHub Release → artifacts for all three hosts.
