# Simulator.Company — Claude Code & Codex Plugin

> **Status:** stable — released, actively maintained. Supported clients: Claude Code ≥ 1.x, Codex. Go 1.24+ required. macOS and Linux tested.

A plugin for [Claude Code](https://claude.ai/code) and [Codex](https://codex.openai.com) that connects the [Simulator.Company](https://simulator.company) platform to Claude via MCP (Model Context Protocol). Claude gets direct access to the Simulator REST API and domain knowledge to manage actors, graphs, forms, and financial accounts through natural conversation.

## What it does

The plugin bundles a Go MCP server that exposes the full Simulator.Company public API as MCP tools and provides specialist skills that teach Claude the platform's entity model and common workflows:

| Skill                | Activate with                                            | Covers                                                  |
|----------------------|----------------------------------------------------------|---------------------------------------------------------|
| `simulator`          | "use Simulator", "call Simulator API"                    | Full platform overview, all entities, MCP tools         |
| `simulator-init`     | "setup", "connect to simulator", "login to simulator"    | OAuth login, workspace selection, environment setup     |
| `simulator-graph`    | "create actor", "link nodes", "add to layer"             | Actors, links, layers, graph traversal, bulk push/pull  |
| `simulator-forms`    | "create form", "design template", "field structure"      | Form templates, field types, system forms               |
| `simulator-finance`  | "record transaction", "account balance", "transfer funds"| Accounts, transactions, transfers, currencies           |

## Requirements

- [Claude Code](https://claude.ai/code) or [Codex](https://codex.openai.com) installed
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

### Updating

```bash
claude plugin update simulator@simulator   # Claude Code
codex plugin update simulator@simulator    # Codex
```

Restart Claude Code / Codex after updating to apply the new version.

## Authentication

On the first Simulator operation Claude detects that no token is present and runs the `login` tool automatically — your browser opens at `account.corezoid.com` for OAuth2 sign-in and the session continues without interruption. After login Claude uses MCP elicitation to let you pick a workspace from the list returned by the account API.

The token is saved to `.env` in your working directory (mode `0600`) and reused on every subsequent session. When it expires, the login flow triggers again automatically.

You can also trigger login manually at any time:

```
log in to Simulator
```

### Static token (optional)

If you prefer to manage the token yourself, set it in `.env` or export it before starting Claude Code or Codex:

```bash
export ACCESS_TOKEN=your_token_here
```

The static token takes priority over saved credentials.

## Configuration

| Environment variable          | Required | Description                                                                 |
|-------------------------------|----------|-----------------------------------------------------------------------------|
| `ACCESS_TOKEN`                | No       | Static token — overrides OAuth2 saved credentials                           |
| `ACCESS_TOKEN_EXPIRES_AT`     | No       | Token expiry timestamp (RFC 3339) — written automatically after OAuth login |
| `ACCOUNT_URL`                 | No       | Override the default account URL (`https://account.corezoid.com`)           |
| `WORKSPACE_ID`                | No       | Default workspace ID (`accId`) — set automatically after `set-workspace`    |
| `SIMULATOR_PROFILE`           | No       | Environment profile: `local` \| `prod` (default `prod`); also via `--profile` |
| `SIMULATOR_API_BASE_URL`      | No       | Override the profile's API base URL (e.g. `http://localhost:9000/papi/1.0`)  |
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

The MCP server exposes a **curated, typed tool set (~46 tools)** scoped to the core
scenarios — forms, actors, accounts, transactions, graph building, applications/smart forms
— rather than the entire REST surface. Each tool maps to a backend operation by its
`operationId`; a drift gate keeps the set in sync with the live `/papi/1.0` contract.

**Curated API operations** (one tool per backend operation):

| Domain        | Tools                                                                                  |
|---------------|----------------------------------------------------------------------------------------|
| Forms         | `createForm` `getForm` `getForms` `updateForm` `deleteForm` `setFormStatus`            |
| Actors        | `createActor` `getActor` `getActorByRef` `updateActor` `deleteActor` `setActorStatus`  |
| Accounts      | `createAccount` `getAccounts` `getBalance` `updateAccount` `deleteAccount` `createCurrency` `getCurrencies` `createAccountName` `getAccountNames` |
| Transactions  | `createTransaction` `finalizeTransaction` `getTransactions` `createTransfer` `getTransfer` |
| Graph         | `createLink` `massLink` `getEdgeTypes` `getLayerActors` `manageLayerActors`            |
| Applications  | `createApplication` `createSmartForm` `listSmartForms` `manageAppContent`              |
| Auth          | `login` `set-workspace`                                                                |

**Engine tools** (multi-call workflows + client-side computation):

| Tool                     | Description                                                                                          |
|--------------------------|------------------------------------------------------------------------------------------------------|
| `pullGraphFile`          | Fetch all actors and edges from a layer and write them to `<layerId>.yaml` in the working directory  |
| `pushGraphFile`          | Read `<layerId>.yaml` and sync it with the server layer: create / update / remove to match the file  |
| `getAllLayerPlacements`  | Return every actor placement on a layer in one paginated call                                        |
| `compactGraphLayout`     | Auto-layout a layer into domain-clustered grids (replaces the pull → edit → push loop)               |
| `pruneLongEdges`         | Delete edges longer than a distance threshold; preserves hierarchy edges                             |
| `uploadActorPicture` / `uploadActorPictureBulk` | Set actor pictures from URL / file / base64; auto-rasterise SVG → PNG; bulk dedupes by SHA-256 |
| `createChart`            | Create a dashboard chart actor (dynamic `actorFilter` or explicit accounts mode)                     |

## Architecture

```
Claude Code / Codex
  └── simulator MCP server (go run ./cmd/server --profile local|prod)
        ├── config      local / prod profiles (API base + account URL)
        ├── auth        login (OAuth2 PKCE → .env), set-workspace
        ├── tools       curated typed operations (forms, actors, accounts,
        │               transactions, graph, apps) — one tool per backend op
        ├── engines     pullGraphFile, pushGraphFile, compactGraphLayout,
        │               pruneLongEdges, getAllLayerPlacements, uploadActorPicture(Bulk), createChart
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
Specialist for form template design:
- Create custom forms with typed fields (text, number, select, date, file, formula, reference)
- Define default account structures within forms (auto-created for every new actor)
- Work with system forms (Graph, Layer, Event, Script/CDU, Account, Currency, Transaction...)
- Update, version, and manage form status

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

### `/software-migration-onramp`
Discovery facilitator for the Smart Company Onramp migration project. Runs a structured
5-phase discovery dialog and writes the resulting actor graph to disk. See its
[`README.md`](plugins/simulator/skills/software-migration-onramp/README.md) and the
`prompts/` specs.

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
├── Makefile                     # build / vet / test / discovery / run-local / run-prod
├── CHANGELOG.md
├── CLAUDE.md                    # Repo guide for Claude Code (points to AGENTS.md)
├── AGENTS.md                    # Repo guide for coding agents (canonical)
├── docs/                        # Project / contributor documentation
│   ├── ARCHITECTURE.md          # Plugin & MCP-server architecture
│   └── INTEGRATION.md           # pong-server integration plan & status
├── public/                      # Generated AI-discovery artifacts (llms.txt, .well-known/skills/index.json)
└── plugins/simulator/           # Plugin root (CLAUDE_PLUGIN_ROOT for both Claude Code and Codex)
    ├── .claude-plugin/
    │   └── plugin.json          # Claude Code plugin manifest
    ├── .codex-plugin/
    │   └── plugin.json          # Codex plugin manifest
    ├── .mcp.json                # MCP server configuration (go run ./cmd/server)
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
    │   ├── simulator-forms/                # Forms specialist skill
    │   ├── simulator-finance/              # Finance specialist skill
    │   ├── simulator-charts/               # Dashboard charts specialist skill
    │   └── software-migration-onramp/      # Migration discovery facilitator
    └── docs/                    # Plugin-shipped reference (referenced by skills)
        ├── entities/            # Entity reference docs
        └── user-flows/          # End-to-end walkthroughs
```

## Debugging

Run the server directly against a profile and enable verbose logging with `SIMULATOR_DEBUG`:

```bash
cd plugins/simulator/mcp-server
SIMULATOR_DEBUG=1 go run ./cmd/server --profile local
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

## License

MIT
