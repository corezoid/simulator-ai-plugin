# Tasks (Events-form actors)

A **task** in Simulator.Company is not a separate entity — it is an **actor of the
`Events` system form** with **no `chatType`** set. The Events form is the combined
template used for calendar events, SIP meetings, chats *and* tasks; an **empty/absent
`chatType`** is what marks an Events actor as a plain event/task rather than a chat.

A task's **roles are access rules on the task actor** (keyed by `userId`), not data
fields, and its **lifecycle is reactions** (`done` / `sign` / `ds` / `reject`) posted under it.

This is the model behind *"create a task, assign an executor, add approvers / required
signatures"*: create an Events actor → grant `execute` / `sign` / `ds` via access rules
→ the assignees complete and approve it with reactions.

---

## Roles = access privileges

The people on a task are expressed as **access rules** (`saveAccessRules`,
`objType="actor"`, `objId=<taskId>`). The `privs` object carries the role:

| Role | `privs` flag | Meaning | Acts via reaction |
|---|---|---|---|
| Executor / assignee | `execute` | Does the work | `done` |
| Approver | `sign` | Approves / signs off | `sign` (or `reject`) |
| Legal signer | `ds` | Qualified digital signature (legal docs) | `ds` |
| Watcher | `view` only | Read-only visibility | — |

- `view`, `modify`, `remove` are **required booleans** in every rule; `sign`, `ds`,
  `execute` are **optional**. Setting any privilege implies `view`.
- **`reactionOrders`** `{sign, ds, execute}` (positive integers) order multi-step
  sign-off: give approver 1 `reactionOrders.sign=1`, approver 2 `=2`, etc. The same
  applies to `ds` for sequential legal signing.
- One person can hold several roles in **one** rule (`{execute:true, sign:true}`);
  different people get **separate** rules.
- The **creator is the owner implicitly** — never add a self access-rule.
- Grants are applied **asynchronously**: `saveAccessRules` returns a job `taskId` (the
  async-task id, unrelated to the task *actor*); propagation takes a moment.

See the `simulator-access` skill / access-rules docs for the full privilege model.

## The task actor (Events form)

Create with the generic `createActor` on the Events form — there is **no dedicated
"create task" endpoint**:

```
createActor(
  formName="Events",                       # or formId=<per-workspace Events id>
  title="<task name>",                     # short label
  description="<the task body / instructions>",   # the full task text goes HERE
  data={ "startDate": <nowUnixSec>, "endDate": <deadlineUnixSec> })
```

- **The task body goes in the actor's `description`.** `title` is the short label; the
  task's full instructions are the actor's top-level **`description`** field (the
  `description` argument of `createActor`) — *not* a comment reaction and *not* `data`.
  Comment reactions are for later discussion, not the task body.
- `startDate` / `endDate` are **required** by the Events form and are stored as **plain
  unix seconds** on Events (not the nested `{startDate,endDate,timeZoneOffset}` calendar
  object the generic actor `data` protocol describes). Use `startDate` = start/now and
  `endDate` = the **deadline**.
- **No `chatType`** — that field (set to `p2p`/`group`) is what turns an Events actor
  into a chat. A task leaves it empty.
- The Events form **id is per-workspace** (system templates are copied into each
  workspace with a fresh auto-increment id) — resolve it via
  `getForms(formTypes="system")` (match `title == "Events"`) or pass `formName="Events"`
  to `createActor`. Never hardcode it.

See [`chats.md`](chats.md) and [`system-forms.md`](system-forms.md) for the full Events
field list.

## Lifecycle = reactions

The assigned users drive the task to completion with reactions (see
[`reactions.md`](reactions.md) — the Reactions system form lists these types):

- **`done`** — the executor marks the work finished.
- **`sign`** — an approver signs off (approval).
- **`reject`** — an approver declines (put the reason in the `description` argument).
- **`ds`** — a qualified digital signature for legal documents.
- **`freeze`** — put the task on hold.

All are created with `createReaction(type=<one of the above>, actorId="<taskId>")`; the
reaction text always rides in the `description` argument (there is no `content` argument).

Read progress with `getReactions(actorId="<taskId>", view="flat")` and tally `done` /
`sign` / `ds` / `reject` against the people granted `execute` / `sign` / `ds`.

## Finding tasks

```
# Tasks (Events actors that are NOT chats):
filterActors(formId=<EventsFormId>, q="chatType=")

# Tasks assigned to a given user as executor:
filterActors(formId=<EventsFormId>, q="chatType=", members="<userId>:execute")
```

The `members` filter is a comma-separated `userId:priv` access-member list
(e.g. `4210:execute,4310:sign`); `q="chatType="` excludes chats (empty chatType).

## Scopes

Task operations use the same scopes as the underlying entities: `control.events:actors.*`
(the Events actor), access-rule management, and reaction creation. See `reactions.md`,
`actors.md`, and the access-rules docs.
