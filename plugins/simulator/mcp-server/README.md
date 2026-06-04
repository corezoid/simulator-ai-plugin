# Simulator MCP server

The Go MCP server that backs the `simulator` plugin. It exposes the Simulator.Company
(`pong-server`) `/papi/1.0` REST API as MCP tools and handles OAuth2 login and workspace
context.

> **Two servers during the rewrite.** The module currently contains both:
> - the **new** layered server (`cmd/server` + `internal/`) — config-driven profiles, a
>   thin `apiclient`, and a curated, typed tool set for the core scenarios. This is the
>   target implementation.
> - the **legacy** server (root `main.go` + `app/mcp-server`) — the Swagger→MCP bridge that
>   exposes all 185 operations plus 11 client-side convenience tools. Still wired into
>   `.mcp.json` until the engines are ported to the new server.
>
> See [`docs/ARCHITECTURE.md`](../../../docs/ARCHITECTURE.md) for the design and
> [`docs/INTEGRATION.md`](../../../docs/INTEGRATION.md) for the rewrite plan and status.

## Requirements

- Go **1.24+** (`go version`). The module's `go` directive may request a newer toolchain;
  in air-gapped environments set `GOTOOLCHAIN=local` and install a matching Go manually.

## Running

**New server** (curated, config-driven) — pick an environment with `--profile`:

```bash
go run ./cmd/server --profile local   # dev pong-server on :9000, pre SA
go run ./cmd/server --profile prod     # public gateway (default)
```

**Legacy server** (full Swagger→MCP bridge) — launched by `.mcp.json` today via `go run .`:

```bash
go run .                               # stdio (default MCP transport)
go run . --sse --addr localhost:8080   # SSE transport for remote / multi-client use
go run . getCompanies                  # CLI mode — run a single tool once and exit
```

The server reads `ACCESS_TOKEN`, `WORKSPACE_ID`, `ACCOUNT_URL`, etc. from a `.env` file in
the **current working directory**. The `login` and `set-workspace` tools write back to it.
See the root [`README.md`](../../../README.md#configuration) for the full env-var table.

### Flags

New server (`cmd/server`):

| Flag         | Default | Description                                                            |
|--------------|---------|------------------------------------------------------------------------|
| `--profile`  | `prod`  | Environment profile: `local` \| `prod` (or `SIMULATOR_PROFILE`)        |
| `--insecure` | off     | Skip TLS verification (self-signed on-prem gateways)                   |

Profiles are overridable per-field via `SIMULATOR_API_BASE_URL`, `SIMULATOR_ACCOUNT_URL`,
`SIMULATOR_OAUTH_CLIENT_ID`, or a `profiles.json` in the working directory.

Legacy server (root `main.go`):

| Flag         | Default            | Description                                              |
|--------------|--------------------|----------------------------------------------------------|
| `--spec`     | embedded full spec | Load a different OpenAPI spec (URL or file)              |
| `--sse`      | off                | Use SSE transport instead of stdio                       |
| `--addr`     | `localhost:8080`   | Bind address for SSE mode                                |
| `--insecure` | off                | Skip TLS verification (self-signed on-prem gateways)     |

Set `SIMULATOR_DEBUG=1` for verbose per-request logging. In MCP mode, debug output is also
written to `/tmp/simulator.log`:

```bash
tail -f /tmp/simulator.log
```

## Layout

```
mcp-server/
├── cmd/server/        # NEW entry point: profile → apiclient → curated tools → stdio
├── internal/          # NEW layered implementation
│   ├── config/        # local/prod profiles (env + profiles.json overridable)
│   ├── apiclient/     # HTTP: base URL, auth header, accId injection, timeouts, errors
│   └── tools/         # curated typed operation registry (op.go) + per-domain tools:
│                      #   forms, actors, accounts, transactions, graph, apps + auth helpers
├── main.go            # LEGACY entry point: flags, run modes, .env bootstrap
├── specs.go           # LEGACY //go:embed of swagger/sim-public-swagger.full.json
├── swagger/           # LEGACY bundled OpenAPI specs (curated reuse + generated full)
├── app/
│   ├── auth/          # OAuth2 PKCE flow + .env credential storage (shared by both servers)
│   ├── swagger/       # LEGACY spec loader (URL/file → models.SwaggerSpec)
│   ├── models/        # LEGACY OpenAPI/Swagger type definitions
│   └── mcp-server/    # LEGACY Swagger→MCP bridge + the 11 custom tools
└── cmd/
    ├── enrichspec/    # LEGACY regenerate the embedded full spec from the live API
    └── gendiscovery/  # regenerate public/ discovery artifacts from SKILL.md files
```

## Make targets

Run from the repository root (recipes `cd` into this module):

```bash
make build         # go build ./...
make vet           # go vet ./...
make test          # go test ./...   (covers internal/... — config, apiclient, tools)
make enrich-spec   # regenerate swagger/sim-public-swagger.full.json from the live API
make check-spec    # drift gate: fail if upstream added ops missing from the embedded spec
make discovery     # regenerate public/llms.txt + public/.well-known/skills/index.json
```

`make check-spec` is the CI gate that catches upstream API additions; run `make enrich-spec`
to pick them up, then commit the regenerated `…full.json`.
