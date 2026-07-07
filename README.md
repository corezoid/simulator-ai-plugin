# Simulator.Company — Claude Code, Codex & Kiro Plugin

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go 1.25+](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/dl/)
[![MCP](https://img.shields.io/badge/MCP-2025--03--26-555)](https://modelcontextprotocol.io)
[![Release](https://img.shields.io/github/v/release/corezoid/simulator-ai-plugin?sort=semver)](https://github.com/corezoid/simulator-ai-plugin/releases)
[![Claude Code](https://img.shields.io/badge/Claude%20Code-plugin-D97757)](https://claude.ai/code)

> **Status:** stable — released, actively maintained. Supported clients: Claude Code ≥ 1.x, Codex, AWS Kiro. Go 1.24+ required. macOS and Linux tested.

A plugin for [Claude Code](https://claude.ai/code), [Codex](https://codex.openai.com) and [AWS Kiro](https://kiro.dev) that connects the [Simulator.Company](https://simulator.company) platform to Claude via MCP (Model Context Protocol). Claude gets direct access to the Simulator REST API and domain knowledge to manage actors, graphs, forms, and financial accounts through natural conversation.

## What it does

The plugin bundles a Go MCP server that exposes the full Simulator.Company public API as MCP tools and provides specialist skills that teach Claude the platform's entity model and common workflows:

| Skill                | Activate with                                            | Covers                                                  |
|----------------------|----------------------------------------------------------|---------------------------------------------------------|
| `simulator`          | "use Simulator", "call Simulator API"                    | Full platform overview, all entities, MCP tools         |
| `simulator-init`     | "setup", "connect to simulator", "login to simulator"    | OAuth login, workspace selection, environment setup     |
| `simulator-skills`   | "run skill", "use playbook", "/skill <slug>", "save as skill", "what skills do I have" | Skill registry: discover/run saved playbooks (`findSkill`/`getSkill`) and author new ones — the data-driven analogue of these built-in skills |
| `simulator-graph`    | "create actor", "link nodes", "add to layer"             | Actors, links, layers, graph traversal, bulk push/pull  |
| `simulator-forms`    | "create form", "design template", "Account Template"     | Form templates (Account Templates), field classes, system forms |
| `simulator-actors`   | "create a record", "fill in a template", "update actor data" | Actor instances of a form, the `data` value protocol, search & filter |
| `simulator-smart-forms` | "smart form", "CDU", "edit page config", "push smart form" | Smart Form lifecycle, pages, CDU protocol, releases  |
| `simulator-smart-forms-logic` | "add logic to smart form", "wire corezoid to smart form", "/get /send process" | Brief generator for the Corezoid process(es) bound to a Smart Form; delegates to `corezoid-create` / `corezoid-edit`; `procId` binding |
| `simulator-finance`  | "record transaction", "account balance", "transfer funds"| Accounts, transactions, transfers, currencies, counters |
| `simulator-charts`   | "chart", "dashboard", "visualise on layer"               | Dashboard charts & time-series visualisation on layers  |
| `simulator-reactions`| "comment on this actor", "reply", "pin comment"          | Reactions: comments / events / approvals / ratings (threaded) |
| `simulator-chat`     | "write a message to user N", "DM", "open a chat with"    | Messaging: send a message to a user, p2p/group chats (Events-form actors; messages are comment reactions) |
| `simulator-tasks`    | "create a task", "assign to", "who approves", "needs signature" | Tasks/assignments: an Events-form actor + executor (`execute`) / approver (`sign`) / legal signer (`ds`) roles |
| `simulator-agents`   | "delegate to", "assign this to X", "can X do this", "who should do this", "ask X's agent" | Digital-twin agents: discover a person by competency (`findAgent`), load their "# Agent" profile (`getAgent`), then do the task, find a better person, or escalate (task / p2p) |
| `simulator-meetings` | "schedule a meeting", "recurring meeting", "agenda", "join link" | Meetings/video/SIP rooms (Events-form actor, `scheduleMeeting`): schedule, recurrence, agenda, persistent rooms, public & in-app join links |
| `simulator-attachments` | "upload a file", "attach document", "rename file"     | Files: upload, attach/detach to actors & reactions      |
| `simulator-access`   | "share with", "grant access", "who can edit this"        | Access rules: grant/revoke view/modify/… on objects     |

## Requirements

- [Claude Code](https://claude.ai/code), [Codex](https://codex.openai.com) or [AWS Kiro](https://kiro.dev) installed
- [Go 1.24+](https://go.dev/dl/) available in `PATH` (the MCP server runs via `go run`, no build step needed)
  ```bash
  brew install golang        # macOS
  sudo apt install golang    # Ubuntu/Debian
  ```
  Verify with `go version`.
- A Simulator.Company account

## Installation

### Claude Code

**From the GitHub marketplace:**

```bash
claude plugin marketplace add corezoid/simulator-ai-plugin
claude plugin install simulator@simulator
```

**Or from a local clone:**

```bash
git clone https://github.com/corezoid/simulator-ai-plugin
claude plugin marketplace add ./simulator-ai-plugin
claude plugin install simulator@simulator
```

### Codex

**From the GitHub marketplace:**

```bash
codex plugin marketplace add corezoid/simulator-ai-plugin
codex plugin install simulator@simulator
```

**Or from a local clone:**

```bash
git clone https://github.com/corezoid/simulator-ai-plugin
codex plugin marketplace add ./simulator-ai-plugin
codex plugin install simulator@simulator
```

No build step, no extra setup. The MCP server starts automatically on first use.

### AWS Kiro

Kiro has no marketplace command and does not resolve the `$CLAUDE_PLUGIN_ROOT` token that Claude Code and Codex substitute at skill-load time, so it installs from a local clone via a helper script that copies the plugin into the workspace's `.kiro/` and resolves that token to an absolute path:

```bash
git clone https://github.com/corezoid/simulator-ai-plugin
cd simulator-ai-plugin
plugins/simulator/scripts/install-kiro.sh [workspace-dir]   # defaults to $KIRO_WORKSPACE_DIR or the current dir
```

This writes the MCP entry to `<workspace>/.kiro/settings/mcp.json`, symlinks the steering file into `.kiro/steering/`, and hard-copies each skill into `.kiro/skills/<name>/` with `$CLAUDE_PLUGIN_ROOT` replaced by the absolute plugin path. Open the workspace in Kiro and the MCP server, skills, and steering are picked up automatically. The plugin is also published to [kiro.dev/powers](https://kiro.dev/powers) — see [`POWER.md`](POWER.md).

### Updating

```bash
claude plugin update simulator@simulator   # Claude Code
codex plugin update simulator@simulator    # Codex
```

Restart Claude Code / Codex after updating to apply the new version. For AWS Kiro, `git pull` the clone and re-run `plugins/simulator/scripts/install-kiro.sh` (it is idempotent) to refresh the workspace overlay.

## Authentication

Simulator runs on many environments (cloud, on-prem, local dev), so the first step is to
**choose one**. The `set-environment` tool takes a cloud preset — `mw.simulator.company`
(default) or `sim.simulator.company` — or a custom/local URL (host or full URL; `/papi/1.0`
is appended if omitted). It fetches that gateway's **public config** to derive the correct
OAuth account URL — the platform authenticates through the `account` system, and one account
may back several environments, so the auth URL is determined per gateway rather than fixed.
The chosen API base and account URL are saved to `.env` (`SIMULATOR_API_BASE_URL`,
`ACCOUNT_URL`). Switching environment later clears the token + workspace and requires a fresh
`login`.

Next Claude runs the `login` tool — your browser opens for OAuth2 sign-in against the chosen
environment's account URL and the token is saved. Claude then lists your workspaces with
`getWorkspaces` and lets you pick one **by name**; `set-workspace` (by `name` or `accId`)
saves the choice as `WORKSPACE_ID`. You never need to know the workspace id.

The token is saved to `.env` in your working directory (mode `0600`) and reused on every subsequent session. When it expires, the login flow triggers again automatically.

You can also trigger login manually at any time:

```
log in to Simulator
```

### Static token (optional)

If you prefer to manage the token yourself, set it in `.env` or export it before starting Claude Code, Codex or Kiro:

```bash
export ACCESS_TOKEN=your_token_here
```

The static token takes priority over saved credentials.

## Configuration

| Environment variable          | Required | Description                                                                 |
|-------------------------------|----------|-----------------------------------------------------------------------------|
| `ACCESS_TOKEN`                | No       | Static token — overrides OAuth2 saved credentials                           |
| `ACCESS_TOKEN_EXPIRES_AT`     | No       | Token expiry timestamp (RFC 3339) — written automatically after OAuth login |
| `ACCOUNT_URL`                 | No       | OAuth account (SA) URL — set automatically by `set-environment` (derived from the gateway's public config); overrides the default `https://account.corezoid.com` |
| `WORKSPACE_ID`                | No       | Default workspace ID (`accId`) — set automatically after `set-workspace`    |
| `SIMULATOR_PROFILE`           | No       | Environment profile: `local` \| `prod` (default `prod`); also via `--profile` |
| `SIMULATOR_API_BASE_URL`      | No       | API base URL — set automatically by `set-environment`; overrides the profile (e.g. `http://localhost:9000/papi/1.0`) |
| `SIMULATOR_ACCOUNT_URL`       | No       | Override the profile's OAuth account (SA) URL                                |
| `SIMULATOR_OAUTH_CLIENT_ID`   | No       | OAuth2 client ID — on-prem deployments with a custom authorization server should set this to their own client ID; cloud (account.corezoid.com) users do not need it |

All values are read from a `.env` file in the current working directory at startup, and the `login` / `set-workspace` tools persist their results back to that file.

## Usage

Once installed, just talk to Claude naturally:

```
Create a business process graph for customer onboarding with three steps:
Document Collection → Review → Approval. Add all steps to a layer.
```

```
Create a Car form template with fields: make, model, year, color, VIN.
Add financial accounts: purchase value (USD asset), maintenance costs (USD expense),
and a mileage counter (km).
```

```
Record a $450 maintenance transaction on the Toyota Camry actor.
Then show me all accounts and their current balances.
```

```
Search for all actors of form type "Task" on the "Main Process" layer.
```

```
Pull layer 1a2b3c4d-... to a local YAML, let me edit it, then push it back.
```

## MCP Tools

The MCP server exposes a **curated, typed tool set (~95 tools)** scoped to the core
scenarios — forms, actors, accounts, transactions, graph building, applications/smart forms
— rather than the entire REST surface. Each tool maps to a backend operation by its
`operationId`; a drift gate keeps the set in sync with the live `/papi/1.0` contract.

Read tools take an optional **`filter`** parameter — a comma-separated allow-list of fields
to return (e.g. `id,title,data.status`; dotted paths pick nested `data` fields). The backend
prunes the response to just those fields, so prefer it whenever only part of an entity is
needed to keep responses (and token cost) small. Available on every read/lookup/list tool:
`getActor`, `getActorByRef`, `searchActors`, `searchLayerActors`, `filterActors`, `getForm`,
`getForms`, `searchForms`, `getAccount`, `getAccounts`, `getBalance`, `getCurrencies`, `getAccountNames`,
`getTransactions`, `getTransfer`, `getRelatedActors`, `getLinkedActors`, `getActorLinks`,
`getLayerActors`, `getEdgeTypes`, `searchAll`, `getWorkspaces`. (For `getLayerActors`/`searchAll` it projects
the actor/node items.)

**Curated API operations** (one tool per backend operation):

| Domain        | Tools                                                                                  |
|---------------|----------------------------------------------------------------------------------------|
| Forms         | `createForm` `getForm` `getForms` `searchForms` `updateForm` `deleteForm` `setFormStatus` `createFormAccount` `getFormAccounts` `removeFormAccount` `getLinkedForms` `getFormsTree` |
| Actors        | `createActor` `getActor` `getActorByRef` `searchActors` `searchLayerActors` `filterActors` `updateActor` `deleteActor` `setActorStatus` `getSystemActor` `getCorezoidProcesses` |
| Accounts      | `createAccount` `getAccount` `getAccounts` `getBalance` `getChildAccounts` `updateAccount` `setAccountAmount` `deleteAccount` `createAccountPair` `createCurrency` `getCurrencies` `searchCurrencies` `createAccountName` `getAccountNames` `updateAccountName` `searchAccountNames` `setAccountFormula` `getAccountFormula` |
| Account tags & triggers | `saveAccountActors` (link Tags/AccountTriggers actors to a pair or one account) `getDataFieldActorsByActor` `saveDataFieldActorsByActor` `getDataFieldActorsByForm` `saveDataFieldActorsByForm` (data triggers on a data field) |
| Counters      | `saveCounters` `setCounters` `getCounters` |
| Access rules  | `getAccessRules` `saveAccessRules` `getTemplateActorsAccess` `saveTemplateActorsAccess` `getTreeLayerAccess` `saveTreeLayerAccess` `bulkSaveAccessRules` `bulkSaveAccountPairsAccessRules` `requestAccess` |
| Transactions  | `createTransaction` `finalizeTransaction` `atomCreateTransaction` `getTransactions` `getAccountTransactions` `getTransactionByRef` `createTransfer` `createTransferTwoStep` `getTransfer` `getTransferByRef` `filterTransfers` |
| Graph (links) | `createLink` `massLink` `getEdge` `updateEdge` `deleteEdge` `existLink` `deleteEdgesByNodes` `getEdgeTypes` `getLayerActors` `getLayerActorsPaginated` `getRelatedActors` `getLinkedActors` `getActorLinks` `manageLayerActors` `moveActors` `existLayerElement` `cleanGraphLayer` `layerStats` |
| Reactions     | `createReaction` `updateReaction` `deleteReaction` `getReactions` `getReactionsStats` `markReactionsRead` `getPinnedReactions` `togglePinnedReaction` |
| Attachments   | `getAttachments` `getActorAttachments` `addAttachments` `updateAttachment` `removeAttachments` `uploadBase64` `readAttachment` (download & read a file's content — text inline, images as a viewable block, PDFs/binary as an embedded resource) |
| Search        | `searchAll` (global text/semantic search across actors & users)                        |
| Skill registry | `findSkill` (discover saved skill playbooks by intent; empty query lists all published) `getSkill` (load one in full by `ref`/slug or `id`) — actors of the `Skills` system form; local composite tools (outside the drift gate), see `/simulator-skills` |
| Digital-twin agents | `findAgent` (discover people by competency over their twin profiles; empty query lists members) `getAgent` (load a person's "# Agent" profile by `userId`/`actorId`) — user twins on the `System` form; local composite tools (outside the drift gate), see `/simulator-agents` |
| Public links  | `generatePublicLink` `getPublicLink` `revokePublicLink` (shareable `/m/<hash>` join link to an actor — meeting / SIP access without login) |
| Meetings      | `getTranscription` (read a meeting call's speech transcription — summarize / extract action items; needs a live room) |
| Smart Forms (runtime) | `appGetPage` `appSendForm` (drive any Smart Form / CDU app via the `get`/`send` page protocol — render a page, submit a form; Corezoid supplies data & control flow. Universal primitives for the `simulator-smart-forms-runtime` skill) |
| Users         | `getUsers` `getUser` `searchUsers` (workspace members — resolve a userId/groupId for sharing) |
| Setup         | `set-environment` (cloud preset or custom/local URL) `login` `getWorkspaces` `set-workspace` (by accId or name) |


**Engine tools** (multi-call workflows + client-side computation):

| Tool                     | Description                                                                                          |
|--------------------------|------------------------------------------------------------------------------------------------------|
| `pullGraphFile`          | Fetch all actors and edges from a layer and write them to `<layerId>.yaml` in the working directory  |
| `pushGraphFile`          | Read `<layerId>.yaml` and sync it with the server layer: create / update / remove to match the file  |
| `getAllLayerPlacements`  | Return every actor placement on a layer in one paginated call                                        |
| `compactGraphLayout`     | Auto-layout a layer into domain-clustered grids (replaces the pull → edit → push loop)               |
| `pruneLongEdges`         | Delete edges longer than a distance threshold; preserves hierarchy edges                             |
| `uploadActorPicture` / `uploadActorPictureBulk` | Set actor pictures from URL / file / base64; auto-rasterise SVG → PNG; bulk dedupes by SHA-256 |
| `createSmartForm`        | Create a new Smart Form actor with develop + production environments                                 |
| `updateSmartFormEnv`     | Update Corezoid credentials bound to a Smart Form environment (develop or production)                |
| `pullSmartForm`          | Download all env file trees of a Smart Form to `<actorId>/<env>/` with `.manifest.json`              |
| `pushSmartForm`          | Diff local develop files against `.manifest.json`, validate, and push changed files in one batch     |
| `deploySmartForm`        | Deploy one Smart Form env to another (develop → production); creates a new release                   |
| `listReleases`           | List releases for a Smart Form environment                                                           |
| `diffReleases`           | Show added / removed / modified files between two releases                                           |
| `rollbackRelease`        | Roll back to a prior release (forward-only: creates a new active release)                            |
| `getFileHistory`         | List version history for a Smart Form file (fileId from `.manifest.json`)                            |
| `getFileVersion`         | Fetch the source of one specific file version                                                        |
| `rollbackFile`           | Restore a file to a prior version                                                                    |
| `listTrash`              | List soft-deleted objects in a Smart Form environment                                                |
| `restoreFromTrash`       | Restore a soft-deleted object from trash                                                             |
| `createChart`            | Create a dashboard chart actor (dynamic `actorFilter` or explicit accounts mode)                     |
| `buildLink`              | Build an absolute web-app deep-link (actor / event / chat / layer / transaction / …) for the user to click; resolves the web base + workspace automatically, and defaults to the user's open actor/layer from the UI context when present |

## Architecture

```
Claude Code / Codex / Kiro
  └── simulator MCP server (go run ./cmd/server --profile local|prod)
        ├── config      cloud presets + local / prod profiles (API base + account URL)
        ├── auth        set-environment (public config → account URL), login (OAuth2 PKCE → .env), set-workspace
        ├── tools       curated typed operations (forms, actors, accounts,
        │               transactions, graph, apps) — one tool per backend op
        ├── engines     pullGraphFile, pushGraphFile, compactGraphLayout,
        │               pruneLongEdges, getAllLayerPlacements, uploadActorPicture(Bulk), createChart, buildLink
        └── apiclient   HTTP → Simulator /papi/1.0 (local :9000 or mw gateway)
```

The server exposes a curated, typed tool set declared in Go (not a generic spec passthrough);
a drift gate validates those declarations against the backend's `papi-openapi.json`. Skills
add the domain knowledge on top. For the full design — profiles, the tool registry, engines,
the drift gate, and the auth flow — see [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md).

### Simulator.Company Entity Model

```
Workspace (accId)
  ├── Forms          — templates defining actor structure and field types
  │     └── Actors  — instances (nodes in the business process graph)
  │           ├── Links        — directed edges connecting actors
  │           ├── Layers       — visual views with actor positions
  │           ├── Accounts     — financial/metric tracking (asset, expense, counter...)
  │           │     ├── Transactions  — credits and debits on a single account
  │           │     └── Transfers     — atomic movement between two accounts
  │           ├── Reactions    — comments, approvals, ratings
  │           └── Attachments  — file storage
  ├── Currencies     — units of value for accounts (USD, EUR, Km, Units...)
  ├── Account Names  — category labels for accounts
  └── Link Types     — categories for edges between actors
```

## Skills reference

### `/simulator`
Universal Simulator assistant. Knows the full platform model, all entity types, and common workflow sequences. Use this when you need guidance across multiple domains or want to explore what's possible.

### `/simulator-init`
Environment setup assistant — runs `login`, picks a workspace, and saves credentials to `.env`. Use it the first time you connect to Simulator or when switching workspaces.

### `/simulator-graph`
Specialist for graph structure operations:
- Create, update, search, and delete actors
- Create single or bulk links between actors
- Manage layers — add actors with positions, search by form or text, move between layers
- Traverse the graph — get linked actors, actor links, global layer membership
- Pull a whole layer to YAML, edit locally, push it back

### `/simulator-forms`
Specialist for form template (Account Template / «Шаблон рахунків») design:
- Create custom forms as `sections[]` of typed field items (`edit`, `check`, `radio`, `select`, `multiSelect`, `calendar`, `upload`, `label`, `button`, `image`)
- Use static or dynamic `select` sources (layer, actorFilter, actors, currencies, accountNames, workspaceMembers, …)
- Work with system forms (Graph, Layer, Event, Script/CDU, Account, Currency, Transaction...)
- Update, version, and manage form status; attach accounts to the actors created from the form

### `/simulator-actors`
Specialist for actor instances (the *records* of a form / Account Template):
- Create and update actors with a `data` object keyed by the form's field `id`s (`item_<digits>`)
- Get the per-class `data` value shapes right (string / number / boolean / option arrays / `{type,title,value}` references / calendar objects)
- Read by UUID or `(formId, ref)`, set status, delete
- Search across the workspace (`searchActors`) and list/rank a form's actors, optionally by account balance (`filterActors`)

### `/simulator-smart-forms`
Specialist for Smart Form (CDU / Script / Application) authoring:
- Pull all env files to disk with `pullSmartForm`, push changes with `pushSmartForm`
- Edit page `config` (grid → form → section → item), `locale`, `viewModel`, `definitions`, `styles`
- Full CDU page protocol: 28 component types, templating (`[[locale]]` / `{{viewModel}}` / `$ref`), change protocol (200/205/302)
- Deploy `develop → production` as an immutable release, list/diff/rollback releases
- File history, version restore, and trash management

### `/simulator-finance`
Specialist for financial and metric tracking:
- Set up currencies and account name categories
- Create accounts of any type on actors (asset, liability, expense, income, counter, state)
- Record immediate or 2-step (authorize → complete/cancel) transactions
- Create atomic multi-account transfers
- Query balances, transaction history, and filter transfers

### `/simulator-charts`
Specialist for dashboard charts and time-series visualisation on graph layers — builds
chart actors via `createChart` (dynamic `actorFilter` or explicit accounts mode).

## Project structure

```
simulator-ai-plugin/
├── .claude-plugin/
│   ├── marketplace.json         # Claude Code marketplace listing (points to plugins/simulator)
│   └── plugin.json              # Claude Code plugin manifest (root-level install target)
├── .mcp.json                    # Root-level MCP server config (used when installed via marketplace)
├── .agents/
│   └── plugins/
│       └── marketplace.json     # Codex marketplace listing (points to plugins/simulator)
├── Makefile                     # build / vet / test / discovery / run-local / run-prod / inspect
├── CHANGELOG.md
├── CLAUDE.md                    # Repo guide for Claude Code (points to AGENTS.md)
├── AGENTS.md                    # Repo guide for coding agents (canonical)
├── docs/                        # Project / contributor documentation
│   ├── ARCHITECTURE.md          # Plugin & MCP-server architecture
│   └── INTEGRATION.md           # pong-server integration plan & status
├── public/                      # Generated AI-discovery artifacts (llms.txt, .well-known/skills/index.json)
└── plugins/simulator/           # Plugin root ($CLAUDE_PLUGIN_ROOT for Claude Code, Codex and Kiro)
    ├── .claude-plugin/
    │   └── plugin.json          # Claude Code plugin manifest
    ├── .codex-plugin/
    │   └── plugin.json          # Codex plugin manifest
    ├── .kiro-plugin/
    │   └── plugin.json          # AWS Kiro plugin manifest
    ├── .mcp.json                # MCP server configuration (go run ./cmd/server)
    ├── .mcp.kiro.json           # Kiro MCP entry (copied to .kiro/settings/mcp.json by install-kiro.sh)
    ├── steering/                # Kiro steering file (Kiro's always-on repo guide)
    ├── scripts/
    │   └── install-kiro.sh      # Installs the plugin into a Kiro workspace's .kiro/
    ├── mcp-server/              # Go MCP server source (see mcp-server/README.md)
    │   ├── cmd/server/          # entry point: profile → tools → stdio
    │   ├── cmd/gendiscovery/    # regenerate public/ discovery artifacts
    │   ├── go.mod / go.sum
    │   ├── internal/
    │   │   ├── config/          # local/prod profiles
    │   │   ├── apiclient/       # HTTP client (auth, accId, timeouts)
    │   │   ├── tools/           # curated typed tools + testdata (drift spec, eval)
    │   │   └── engines/         # graph sync, layout, prune, upload, chart
    │   └── app/auth/            # OAuth2 PKCE flow + .env credential storage
    ├── skills/
    │   ├── simulator/                      # Universal assistant skill
    │   │   ├── SKILL.md
    │   │   └── references/api-operations.md
    │   ├── simulator-init/                 # Environment setup skill
    │   ├── simulator-graph/                # Graph specialist skill
    │   ├── simulator-forms/                # Forms (Account Templates) specialist skill
    │   ├── simulator-actors/               # Actor-instance / data-protocol specialist skill
    │   ├── simulator-smart-forms/          # Smart Form (CDU) authoring specialist skill
    │   ├── simulator-finance/              # Finance specialist skill
    │   └── simulator-charts/               # Dashboard charts specialist skill
    └── docs/                    # Plugin-shipped reference (referenced by skills)
        ├── entities/            # Entity reference docs
        └── user-flows/          # End-to-end walkthroughs
```

## Local development

Run the plugin from this repo (developing it, or testing against a local `pong-server`),
not from the marketplace.

**Requirements:** Go 1.24+, and a backend — a local `pong-server` on `http://localhost:9000`
(profile `local`) or the public gateway (profile `prod`).

### 1. Pick the environment

Create `plugins/simulator/mcp-server/.env`:

```
SIMULATOR_PROFILE=local      # or prod
```

Running with the **`local` profile** (via `SIMULATOR_PROFILE=local` or `--profile local`) does
two things for development:

- the server starts pointed at a local `pong-server` on `http://localhost:9000`;
- **`set-environment` additionally offers a `local` preset** (`localhost:9000`) — i.e.
  `set-environment(preset="local")` works. In the default `prod` profile that preset is
  hidden, so end users are only offered the cloud gateways (`mw` / `sim`) and a custom URL.

`login` / `set-workspace` (and `set-environment`) write `ACCESS_TOKEN` / `WORKSPACE_ID` /
`SIMULATOR_API_BASE_URL` / `ACCOUNT_URL` back into this same file.

### 2. Connect it in Claude Code

Pick **one** way (don't combine — two would register the `simulator` server twice):

- **Plugin dir (recommended for dev):** start Claude Code pointing at the repo —
  ```bash
  claude --plugin-dir /Users/<you>/PJ/control/simulator-ai-plugin/plugins/simulator
  ```
  To run against the **local** backend, prefix the launch with `SIMULATOR_PROFILE` — the
  MCP server inherits it from the Claude Code process, so you don't have to edit `.env`:
  ```bash
  SIMULATOR_PROFILE=local claude --plugin-dir /Users/<you>/PJ/control/simulator-ai-plugin/plugins/simulator
  ```
  (This targets `pong-server` on `:9000` and makes `set-environment` offer the `local`
  preset; without it the server defaults to the `prod` profile. An env var set this way
  wins over `mcp-server/.env`. Equivalently: put `SIMULATOR_PROFILE=local` in
  `plugins/simulator/mcp-server/.env`.)
- **Local marketplace install:**
  ```
  /plugin marketplace add /Users/<you>/PJ/control/simulator-ai-plugin/plugins/simulator
  /plugin install simulator@simulator
  /reload-plugins
  ```
- **Project auto-load:** simply opening this repo as your project loads the root
  `.mcp.json` (a project-scoped server) — approve it when Claude Code asks.

Verify with `/mcp` — you should see **simulator** ✓ with ~50 tools.

> **Avoiding conflicts with the installed (prod) plugin.** If you also have the published
> `simulator@simulator` plugin installed, it registers the **same** `simulator` MCP server
> and the same skills — so two copies collide. While developing locally, **disable the prod
> plugin**:
> ```
> /plugin                       # toggle simulator@simulator OFF (or: claude plugin disable simulator@simulator)
> ```
> Re-enable it when you're done. Disabling is the only clean option if you're testing the
> **skills** (they reference tools by bare name, so with both active they'd drive whichever
> server wins — usually prod, against the prod backend).
>
> If you must run both at once (e.g. to call your dev tools explicitly via
> `mcp__simulator-dev__*` while keeping prod for normal use), rename the server in the root
> `.mcp.json` from `simulator` to `simulator-dev` so the MCP server names don't clash. The
> prod backend vs your local backend stay separate anyway — each instance reads its own
> `.env` (the installed plugin's dir vs `plugins/simulator/mcp-server/.env`).

### 3. Authenticate and choose a workspace

```
log in to Simulator          # OAuth in the browser → token saved to .env
which workspaces do I have?  # getWorkspaces → list by name
work in <workspace name>     # set-workspace(name=…) → saves WORKSPACE_ID
```

### 4. Restart after you change the plugin

Edited Go code, a tool, `.env`, `.mcp.json`, or a skill? Reload — **no reinstall needed**:

```
/reload-plugins
```

This kills and relaunches the Go MCP server (`go run ./cmd/server`), re-reading the source
and `.env`. Check `/mcp` again if a server shows as failed (see its **Errors** tab in
`/plugin`).

### Run / test outside Claude Code

```bash
make run-local      # go run ./cmd/server --profile local
make run-prod       # against the public gateway
make inspect        # MCP Inspector web UI wrapping the server (PROFILE=local|prod)
make test           # unit + scenario + drift + eval tests
make lint           # golangci-lint v2 (gosec clean; style backlog)
make eval           # behavioural eval, dry (needs ANTHROPIC_API_KEY)
make eval-live      # behavioural eval executing tools against the backend
```

`make inspect` launches the [MCP Inspector](https://github.com/modelcontextprotocol/inspector)
(needs Node.js / `npx`) — a browser UI to list and call the server's tools and read its
resources by hand. It prints a `localhost:6274` URL with a session token; open it, hit
**Connect**, then browse **Tools** / **Resources**. Authenticate first (the `login` tool or
a valid `.env`) since calls hit the real backend. See the
[MCP server README](plugins/simulator/mcp-server/README.md#inspecting--debugging-with-the-mcp-inspector)
for the headless `--cli` mode.

## Debugging

Run the server directly against a profile (it logs startup config and request errors to
stderr):

```bash
cd plugins/simulator/mcp-server
go run ./cmd/server --profile local
```

Tests cover config, the HTTP client, the curated tools (scenarios + `-race`), the backend
drift gate, and the eval scenarios:

```bash
go test ./...
```

## Compatibility

| Component         | Supported versions            | Notes                                       |
|-------------------|-------------------------------|---------------------------------------------|
| Claude Code       | ≥ 1.x                         | MCP protocol 2025-03-26                     |
| Codex             | current stable                | Same MCP server, same skills                |
| AWS Kiro          | current stable                | Same MCP server, same skills; install via `scripts/install-kiro.sh` (no marketplace) |
| Go toolchain      | 1.24+ (module declares 1.25)  | Required to run the MCP server via `go run` |
| macOS             | 13 Ventura and later          | Tested on arm64 and amd64                   |
| Linux             | Ubuntu 22.04+, Debian 12+     | amd64 tested                                |
| Windows           | not tested                    | Likely works; PRs welcome                   |

> **Note:** If your Go installation is older than the module's `go` directive, the toolchain manager will try to download a newer version from `proxy.golang.org`. In air-gapped environments set `GOTOOLCHAIN=local` and install a matching Go version manually.

## Links

- [Simulator.Company](https://simulator.company)
- [API Documentation](https://doc.simulator.company)
- [Claude Code](https://claude.ai/code)
- [MCP Protocol](https://modelcontextprotocol.io)

## Contributing & support

- [Contributing guide](CONTRIBUTING.md) — layout, build/verify, conventions
- [Code of Conduct](CODE_OF_CONDUCT.md)
- [Security policy](SECURITY.md) — report vulnerabilities privately
- [Open an issue](https://github.com/corezoid/simulator-ai-plugin/issues)

## License

[MIT](LICENSE) © Simulator.Company (Corezoid)
