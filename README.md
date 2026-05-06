# Simulator.Company — Claude Code & Codex Plugin

A plugin for [Claude Code](https://claude.ai/code) and [Codex](https://codex.openai.com) that connects the [Simulator.Company](https://simulator.company) platform to Claude via MCP (Model Context Protocol). Claude gets direct access to the Simulator REST API and domain knowledge to manage actors, graphs, forms, and financial accounts through natural conversation.

## What it does

The plugin exposes the full Simulator.Company public API as MCP tools and provides four specialist skills that teach Claude the platform's entity model and common workflows:

| Skill | Activate with | Covers |
|---|---|---|
| `simulator` | "use Simulator", "call Simulator API" | Full platform overview, all entities |
| `simulator-graph` | "create actor", "link nodes", "add to layer" | Actors, links, layers, graph traversal |
| `simulator-forms` | "create form", "design template", "field structure" | Form templates, field types, system forms |
| `simulator-finance` | "record transaction", "account balance", "transfer funds" | Accounts, transactions, transfers, currencies |

Claude uses three MCP tools under the hood — `list_opers`, `get_oper`, `run_oper` — to discover and execute any of the 80+ Simulator API operations.

## Requirements

- [Claude Code](https://claude.ai/code) or [Codex](https://codex.openai.com) installed
- [Go 1.21+](https://go.dev/dl/) available in `PATH` (the MCP server runs via `go run`, no build step needed)
- A Simulator.Company account with an API bearer token

## Installation

**1. Set your API token** (add to `~/.zshrc` or `~/.bashrc` and reload the shell):

```bash
export SIMULATOR_TOKEN=your_bearer_token_here
```

**2. Add the plugin marketplace and install:**

```bash
claude plugin marketplace add corezoid/simulator-ai-plugin
claude plugin install simulator@simulator
```

**Or install from a local clone:**

```bash
git clone https://github.com/corezoid/simulator-ai-plugin
claude plugin marketplace add ./simulator-ai-plugin
claude plugin install simulator@simulator
```

That's it. No build step, no additional setup. Claude Code starts the MCP server automatically via `go run` on first use.

## Configuration

| Environment variable | Required | Description |
|---|---|---|
| `SIMULATOR_TOKEN` | Yes | Bearer token for Simulator.Company API |
| `SIMULATOR_URL` | No | Override the default API base URL |

The token is passed securely through the Claude Code plugin environment — it never appears in config files.

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

## Architecture

```
Claude Code
  └── simulator MCP server (go run .)
        ├── list_opers  → discover available API operations
        ├── get_oper    → get full schema of an operation
        └── run_oper    → execute API call with parameters
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

### `/simulator-graph`
Specialist for graph structure operations:
- Create, update, search, and delete actors
- Create single or bulk links between actors
- Manage layers — add actors with positions, search by form or text, move between layers
- Traverse the graph — get linked actors, actor links, global layer membership

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
│   └── plugin.json              # Claude Code plugin manifest
├── .mcp.json                    # MCP server configuration (Claude Code)
├── .agents/
│   └── plugins/
│       └── marketplace.json     # Codex marketplace listing
├── plugins/
│   └── simulator/               # Codex plugin root
│       ├── .codex-plugin/
│       │   └── plugin.json      # Codex plugin manifest
│       ├── .mcp.json            # MCP server configuration (Codex)
│       ├── scripts/
│       │   └── start-mcp.sh    # Codex wrapper → delegates to repo-root script
│       └── skills/
│           ├── simulator/
│           │   ├── SKILL.md             # Universal assistant skill
│           │   └── references/
│           │       └── api-operations.md  # Complete operation ID reference
│           ├── simulator-graph/
│           │   └── SKILL.md
│           ├── simulator-forms/
│           │   └── SKILL.md
│           └── simulator-finance/
│               └── SKILL.md
├── scripts/
│   └── start-mcp.sh             # MCP server startup (go run, no build needed)
├── swagger/
│   └── sim-public-swagger.json  # Bundled Simulator.Company OpenAPI spec
├── app/
│   ├── mcp-server/              # MCP server implementation (Go)
│   ├── models/                  # OpenAPI data models
│   └── swagger/                 # Swagger loader
├── resources/
│   └── docs/
│       ├── entities/            # Entity documentation
│       └── user-flows/          # Workflow documentation
└── main.go
```

## Links

- [Simulator.Company](https://simulator.company)
- [API Documentation](https://doc.simulator.company)
- [Claude Code](https://claude.ai/code)
- [MCP Protocol](https://modelcontextprotocol.io)

## License

MIT
