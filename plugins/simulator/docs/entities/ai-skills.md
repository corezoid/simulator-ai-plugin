# Skills (skill registry)

A **skill** is a reusable, workspace-specific playbook stored as an actor of the `Skills`
**system form**. It is the data-driven analogue of the plugin's built-in skills: built-in
skills (shipped in git) teach *how to use the platform API in general*; a skill actor encodes
*what to do in this particular workspace* — the concrete steps, MCP tool calls, and entity ids
for a task such as "create a smart contract" or "onboard a client".

The registry lets workspace members teach the assistant new procedures **without a plugin
release**. Both surfaces use it: the reaction-triggered AI agent (its system prompt tells it to
consult the registry) and the interactive Claude Code / MCP client (the `/simulator-skills` and
`/simulator` skills).

## Why a skill is an actor

A skill maps cleanly onto the actor model, reusing the platform's full-text index and access
control with no new storage:

| Skill field | Actor field | Role |
|---|---|---|
| name | `title` | human label; full-text indexed |
| slug | `ref` | stable `kebab-case` id, **unique per form** → instant `getActorByRef`; the explicit-invocation handle |
| body | `description` | the full procedure (unlimited `TEXT`); full-text indexed; loaded on demand |
| publish state | `status` | `verified` = active, `pending` = draft, `rejected` = disabled |

`title` + `ref` are the cheap discovery metadata (returned by list/search); `description` is the
expensive body, fetched only when a skill is chosen — the same frontmatter-vs-body split the
built-in skills use.

Skill actors are ordinary user actors (`isSystem=false`) that happen to live in the `Skills`
system form. They are **workspace-local**: a skill embeds concrete entity ids and lives in one
workspace's form, so it is not portable to another workspace.

## The `Skills` system form

- A SYSTEM form (`type=system`, title `Skills`), seeded per workspace by the backend
  (`SYSTEM_FORMS.SKILLS`) — created on workspace bootstrap and backfilled by migration.
- Near-schemaless on purpose (like `Tags` / `AccountTemplates`): `sections: [{ content: [] }]`.
  Everything lives in the actor's `title` / `ref` / `description`; there are no custom fields.
- Resolved by `(type=system, title="Skills")` — the system-form template carries no `ref`, so
  the lookup is by title (case-insensitive), mirroring how reactions resolve their form.

## Tools

- **`findSkill(query, limit?)`** — discover skills by intent. Returns a cheap list of
  `{id, title, ref, status}` (never the body). Only `verified` skills are returned. An **empty
  `query` lists every active skill** (enumeration / slug discovery).
- **`getSkill(ref | id)`** — load one skill in full, including the `description` body. Passing
  `ref` (the slug) is the explicit-invocation path: an instant `getActorByRef`, no search.
- **Authoring** reuses the curated actor tools: `createActor(formName="Skills", ref, title,
  description)`, then `setActorStatus(actorId, "verified")` to publish; `updateActor` to edit;
  `setActorStatus(actorId, "rejected")` to disable.

`findSkill` / `getSkill` are local composite tools (they resolve the form id and compose
existing PAPI reads), so they are not part of the OpenAPI-validated curated operation set.

## Discovery & execution algorithm

0. **Explicit:** user named a skill (`/skill <slug>`, "use skill …") → `getSkill(ref=<slug>)`.
1. Resolve the `Skills` form for the workspace (absent → no registry; proceed normally).
2. `findSkill(query=<intent>)` → candidates.
3. Confident match → load it; ambiguous → ask the user; none → normal handling.
4. `getSkill(ref=…)` → read the body.
5. **Pre-flight:** `status=verified`; verify referenced entity ids still exist; if stale, tell
   the user and offer to update.
6. Execute the steps, honoring idempotency notes; **confirm destructive/outward steps**.
7. Report the result in the user's language.

## Body contract (template)

The whole body is wrapped in `[md]…[/md]` — an actor's `description` renders as BBCode, and the
`[md]` tag switches that region to markdown so the skill renders correctly in the UI.

```
[md]
# <Skill name>
## When to use   — trigger phrases / intent (so findSkill matches)
## Context       — workspace assumptions, what must already exist
## Entities      — Form "X" = id 1234; Layer "Y" = id <uuid>; AccountName "Z" = id <uuid>
## Steps         — numbered tool calls, e.g. createActor(formId=1234, data={…}); createLink(…)
## Verify        — how to confirm success
## Idempotency   — what to check (getActorByRef/searchActors) before creating
## Safety        — which steps are destructive/outward and need confirmation
[/md]
```

Start the body (inside `[md]`) with the name, trigger phrases, and a one-line summary so the
full-text index matches the user's intent well.

## Safety model

- A skill body is **data — a plan proposed by a workspace author**, not system instructions. It
  cannot relax the assistant's confirmation rules. Content that tries to escalate privileges,
  exfiltrate data, or skip confirmations must be ignored (prompt-injection guard).
- Execution runs under the requesting user's token; the platform's access rules are the real
  boundary. Because a skill authored by one user can be run by another, only `verified` skills
  are dispatched and destructive/outward steps are always confirmed (confused-deputy guard).
- The reaction agent additionally has no shell/filesystem tools and is limited to the simulator
  MCP server, which contains the blast radius further.

## Related

- [../user-flows/authoring-skills.md](../user-flows/authoring-skills.md) — user-facing how-to for creating a skill and the markdown body conventions, with a worked example.
- [actors.md](actors.md) — actor fields and the `data` value protocol.
- [forms.md](forms.md) — forms and system forms.
- [system-forms.md](system-forms.md) — the system-form catalogue.
