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

The MCP server exposes a small set of hand-written tools plus every operation discovered in the bundled Simulator OpenAPI spec (80+ endpoints). All API-derived tools follow the operation IDs from the swagger spec (e.g. `createActor`, `getCompanies`, `searchActors`, `createTransfer`, …).

| Tool             | Description                                                                                          |
|------------------|------------------------------------------------------------------------------------------------------|
| `login`          | Authenticate via OAuth2 PKCE (opens browser); elicits account URL + workspace and writes them to `.env` |
| `set-workspace`  | Save the active workspace ID (`accId`) to `.env` as `WORKSPACE_ID`                                   |
| `pullGraphFile`  | Fetch all actors and edges from a layer and write them to `<layerId>.yaml` in the working directory  |
| `pushGraphFile`  | Read `<layerId>.yaml` and sync it with the server layer: create / update / remove to match the file  |
| `createActors`   | Bulk-create up to 50 actors in one call; returns the list of new actor IDs                           |
| `<operationId>`  | One tool per Simulator REST operation (auto-generated from the bundled OpenAPI spec)                 |

## Architecture

```
Claude Code / Codex
  └── simulator MCP server (go run .)
        ├── Auth          login (OAuth2 PKCE → .env), set-workspace
        ├── Graph helpers pullGraphFile, pushGraphFile, createActors
        └── REST passthrough
              └── one MCP tool per operation in sim-public-swagger.json
                  (createActor, searchActors, createLink, createLayer,
                   createForm, createAccount, createTransaction, createTransfer, …)
```

The MCP server is a generic Swagger→MCP bridge that reads the bundled `swagger/sim-public-swagger.json` and converts every endpoint into an MCP-callable operation. Skills add the domain knowledge on top.

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
└── plugins/simulator/           # Plugin root (CLAUDE_PLUGIN_ROOT for both Claude Code and Codex)
    ├── .claude-plugin/
    │   └── plugin.json          # Claude Code plugin manifest
    ├── .codex-plugin/
    │   └── plugin.json          # Codex plugin manifest
    ├── .mcp.json                # MCP server configuration (go run . --spec simulator)
    ├── mcp-server/              # Go MCP server source
    │   ├── main.go
    │   ├── specs.go
    │   ├── go.mod / go.sum
    │   ├── app/
    │   │   ├── auth/            # OAuth2 PKCE flow + .env credential storage
    │   │   ├── mcp-server/      # MCP server, push/pull graph handlers
    │   │   ├── models/          # OpenAPI data models
    │   │   └── swagger/         # Swagger loader
    │   ├── swagger/             # Bundled OpenAPI specs
    │   │   ├── sim-public-swagger.json
    │   │   └── sim-public-swagger-all.json
    │   └── info/
    ├── skills/
    │   ├── simulator/                      # Universal assistant skill
    │   │   ├── SKILL.md
    │   │   └── references/api-operations.md
    │   ├── simulator-init/                 # Environment setup skill
    │   ├── simulator-graph/                # Graph specialist skill
    │   ├── simulator-forms/                # Forms specialist skill
    │   └── simulator-finance/              # Finance specialist skill
    └── docs/                    # Entity and user-flow documentation
        ├── entities/
        └── user-flows/
```

## Debugging

The MCP server always writes debug output to `/tmp/simulator.log` when running in MCP mode. View it with:

```bash
tail -f /tmp/simulator.log
```

In CLI mode, you can invoke a single tool without starting the MCP transport:

```bash
cd plugins/simulator/mcp-server
go run . <tool-name> key=value ...
# e.g.
go run . getCompanies
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
