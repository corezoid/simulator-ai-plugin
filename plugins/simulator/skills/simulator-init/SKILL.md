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

## Step 1 — Authenticate

Call MCP tool **`login`** with no arguments:

```
login()
```

It opens a browser for OAuth2 (PKCE) sign-in against the profile's account URL and saves
the token to `.env` as `ACCESS_TOKEN`. (Profile: `prod` → `account.corezoid.com`, `local`
→ `account.pre.corezoid.com`.)

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

## Variables saved to `.env`

| Variable | Set during |
|---|---|
| `ACCOUNT_URL` | Step 1 — Account URL elicitation |
| `ACCESS_TOKEN` | Step 1 — OAuth2 authentication |
| `ACCESS_TOKEN_EXPIRES_AT` | Step 1 — Token expiry |
| `WORKSPACE_ID` | Step 1 — Workspace selection |
