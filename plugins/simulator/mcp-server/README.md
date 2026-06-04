# Simulator MCP server

The Go MCP server that backs the `simulator` plugin. It exposes the Simulator.Company
(`pong-server`) `/papi/1.0` public API as a curated, typed set of MCP tools and handles
OAuth2 login and workspace context.

See [`docs/ARCHITECTURE.md`](../../../docs/ARCHITECTURE.md) for the design and
[`docs/INTEGRATION.md`](../../../docs/INTEGRATION.md) for the pong-server integration plan
and status.

## Requirements

- Go **1.24+** (`go version`). The module's `go` directive may request a newer toolchain;
  in air-gapped environments set `GOTOOLCHAIN=local` and install a matching Go manually.

## Running

Hosts (Claude Code / Codex) launch the server automatically via `go run ./cmd/server` —
there is no build step. Pick the target backend with `--profile`:

```bash
go run ./cmd/server --profile local   # dev pong-server on :9000, pre SA
go run ./cmd/server --profile prod    # public gateway (default)
```

The server reads `ACCESS_TOKEN`, `WORKSPACE_ID`, etc. from a `.env` file in the **current
working directory**. The `login` and `set-workspace` tools write back to it. See the root
[`README.md`](../../../README.md#configuration) for the full env-var table.

### Flags

| Flag         | Default | Description                                                     |
|--------------|---------|-----------------------------------------------------------------|
| `--profile`  | `prod`  | Environment profile: `local` \| `prod` (or `SIMULATOR_PROFILE`) |
| `--insecure` | off     | Skip TLS verification (self-signed on-prem gateways)            |

Profiles are overridable per-field via `SIMULATOR_API_BASE_URL`, `SIMULATOR_ACCOUNT_URL`,
`SIMULATOR_OAUTH_CLIENT_ID`, or a `profiles.json` in the working directory. `SIMULATOR_DEBUG`
enables verbose logging.

## Layout

```
mcp-server/
├── cmd/server/        # entry point: profile → apiclient → curated + engine tools → stdio
├── cmd/gendiscovery/  # regenerate public/ discovery artifacts from SKILL.md files
├── internal/
│   ├── config/        # local/prod profiles (env + profiles.json overridable)
│   ├── apiclient/     # HTTP: base URL, auth header, accId injection, timeouts, errors
│   ├── tools/         # curated typed operation registry (op.go) + per-domain tools:
│   │                  #   forms, actors, accounts, transactions, graph, apps + auth helpers
│   │   └── testdata/  # papi-openapi.json (drift gate) + eval-scenarios.json
│   └── engines/       # graph sync, layout, prune, placements, picture upload, chart
└── app/auth/          # OAuth2 PKCE flow + .env credential storage
```

## Make targets

Run from the repository root (recipes `cd` into this module):

```bash
make build         # go build ./...
make vet           # go vet ./...
make test          # go test ./...   — config, apiclient, tools (scenarios, -race, drift, eval), engines
make discovery     # regenerate public/llms.txt + public/.well-known/skills/index.json
make run-local     # go run ./cmd/server --profile local
make run-prod      # go run ./cmd/server --profile prod
```

## Keeping in sync with the backend

Tool `operationId`s are declared at the backend source (pong-server `/papi` route schemas).
Regenerate the contract spec in pong-server with `yarn dump-openapi` and copy it to
`internal/tools/testdata/papi-openapi.json`; the drift gate (`go test ./internal/tools/ -run
TestSpecDrift`) then fails on any divergence between the plugin's declared tools and the
backend. See [`docs/INTEGRATION.md`](../../../docs/INTEGRATION.md).
