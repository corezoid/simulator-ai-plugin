---
inclusion: always
---

# Simulator.Company — workspace guardrails

These rules apply to every interaction with the Simulator MCP server in this
workspace. They mirror the always-on guardrails the `simulator` skill carries
inside Claude Code / Codex installs; here they ship as a Kiro steering file
so Kiro applies them without depending on skill auto-routing.

## Workspace Context Check (MANDATORY FIRST STEP)

**Exception — global-id reads need no `accId`.** If the request targets a
specific object by its **global id** and only **reads** it (no create/update
in a workspace), skip this whole check and call the tool directly:
`getActor`/`deleteActor` (by `actorId`), `getActorByRef`,
`getAccounts`/`getBalance`/`getTransactions` (by `actorId`/`accountId`),
`getForm` (by `formId`), reactions/attachments by actor id, etc. These
endpoints resolve the object by its id and do **not** require workspace
context — do **not** ask for `accId` or hunt for a `.env`/workspace for them.
(The `accId` returned in the response can seed later workspace-scoped calls.)

**Otherwise, before doing anything else**, verify the WorkspaceID (`accId`)
is known:

1. Check whether `accId` is already known: current message, conversation
   history, or `WORKSPACE_ID` env var / `.env` file in the project directory.
2. If `accId` is **not** provided, immediately ask the user — **in their own
   language** (English, Ukrainian, or Russian) — which workspace to work in,
   i.e. for the Workspace ID (`accId`). If they haven't set up the environment
   yet, suggest running `set-environment` → `login` → `getWorkspaces` →
   `set-workspace`. Do **not** call any other MCP tools until the user
   provides `accId`.
3. Once `accId` is known, proceed normally and use it in all subsequent API
   calls.

## Tool routing

- Each public Simulator REST endpoint is exposed as one MCP tool whose name
  matches the swagger `operationId` (camelCase). Path parameters (`{accId}`,
  `{formId}`, …) become named arguments.
- For broader platform context — actors / forms / graph / finance / charts —
  the matching skill under `.kiro/skills/simulator-*` carries the deep
  reference docs. Activate the relevant one when the user signals intent.

## Language policy

- Internal instructions and field names stay in English.
- Reply to the user **in the language they wrote in** (English, Ukrainian, or
  Russian). Never hardcode a non-English sentence for Claude to say.

## Output discipline

`getActor`, `getForm`, `updateActor`, and `updateForm` return responses that
can exceed 30k tokens (full access lists, doubled form schemas, member
metadata). For >10 such calls in a row, delegate to a background agent with
explicit "do not echo responses" instructions; otherwise the main context
window fills up after a handful of operations.
