---
name: simulator-init
description: >
  Simulator.Company environment setup specialist. Use when the user wants to connect
  to Simulator.Company, set up credentials, authenticate, configure the workspace,
  or start working with Simulator for the first time.
  Activate when the user says "init", "setup", "connect to simulator", "login to simulator",
  "configure simulator", "get started with simulator", or "set workspace".
---

# Initialize Simulator Environment

You are a specialist in setting up the Simulator.Company working environment using the `simulator` MCP server.

## Step 0 — Choose an environment

Simulator runs on many environments (cloud, on-prem, and local dev). Ask the user which one
they want, then call **`set-environment`**. Offer the presets the tool advertises in its
`preset` parameter (don't invent others):

- **Simulator Cloud** — `mw` = `mw.simulator.company` (default) or `sim` = `sim.simulator.company`
- **Custom / on-prem** — they paste a server URL or host (e.g. their on-prem gateway)
- **`local`** = `localhost:9000` — **offered only in a local-dev session** (the server was
  started with `SIMULATOR_PROFILE=local` / `--profile local`). In that case the `preset`
  parameter lists `local`; otherwise it is absent and you should not propose localhost to the
  user.

```
set-environment(preset="mw")                  # cloud (default); or preset="sim"
set-environment(preset="local")               # ONLY in a local-dev session
set-environment(url="https://my-onprem.example.com")   # custom / on-prem
```

`set-environment` reads that gateway's **public config** to derive the correct OAuth account
URL (one account may back several environments, so the auth URL is not fixed per gateway). It
saves the choice to `.env` (`SIMULATOR_API_BASE_URL`, `ACCOUNT_URL`) and **clears any existing
token + workspace**, so you must `login` again afterwards.

## Step 1 — Authenticate

Call MCP tool **`login`** with no arguments:

```
login()
```

It opens a browser for OAuth2 (PKCE) sign-in against the account URL chosen in Step 0 and
saves the token to `.env` as `ACCESS_TOKEN`.

## Step 2 — Choose a workspace (by name, no id needed)

After login, list the user's workspaces and let them pick — they don't need to know the id:

```
getWorkspaces()        → returns [{id, name}, …]
```

Show the names, ask which one, then save the choice with **`set-workspace`** — by name
(resolved automatically) or by id:

```
set-workspace(name="<workspace name>")     # resolves the id for you
set-workspace(accId="<accId>")             # if you already know the id
```

This writes `WORKSPACE_ID` to `.env`; it becomes the default `accId` for every other tool.

---

## Step 3 — Done

Confirm setup is complete:

> "Simulator environment is ready.
> Workspace: `WORKSPACE_ID=<accId>`
> Token saved to `.env`. You can now use all Simulator tools."

---

## Switching environment later

To move to a different environment at any time, call `set-environment` again (by `preset` or
`url`). It re-derives the account URL and **clears the token + workspace**, so re-run `login`
and `set-workspace` afterwards.

---

## Variables saved to `.env`

| Variable | Set during |
|---|---|
| `SIMULATOR_API_BASE_URL` | Step 0 — Environment selection |
| `ACCOUNT_URL` | Step 0 — Environment selection (derived from public config) |
| `ACCESS_TOKEN` | Step 1 — OAuth2 authentication |
| `ACCESS_TOKEN_EXPIRES_AT` | Step 1 — Token expiry |
| `WORKSPACE_ID` | Step 2 — Workspace selection |
