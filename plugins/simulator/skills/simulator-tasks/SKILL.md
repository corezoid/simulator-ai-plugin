---
name: simulator-tasks
description: >
  Simulator.Company task & assignment specialist — creating a task and assigning who
  does it, who approves it, and whose signature it needs. In Simulator a task is an
  actor of the Events system form (no chatType), and the roles are access privileges,
  not data fields: the executor gets `execute`, approvers get `sign`, and a legal
  document's signers get `ds`. Completion and sign-off are then posted as `done` /
  `sign` / `reject` reactions. Use when the user wants to "create a task", "assign a
  task to", "set an executor/assignee", "add approvers", "require a signature", "who
  signs this", "make a to-do". Also covers **order / directive documents** that tell a
  specific person to do something — an "order/наказ addressed to X" is a task whose
  addressee is the executor. Activate on "create a task", "assign to", "task for",
  "needs approval", "who approves", "requires signature", "sign-off", "order", "decree",
  "directive", "create an order for", "створи задачу", "постав задачу", "признач виконавця",
  "додай погоджувача", "потрібен підпис", "хто погоджує", "хто підписує", "наказ",
  "створи наказ", "склади наказ", "наказ на", "розпорядження", "доручення", "создай задачу",
  "назначь исполнителя", "добавь согласующего", "нужна подпись", "кто подписывает",
  "приказ", "создай приказ", "распоряжение", "поручение". For a 1:1/group chat use
  `simulator-chat`; for the reaction mechanics use `simulator-reactions`; for access in
  general use `simulator-access`.
---

> **Curated tool names (v2 server):** `getForms`, `searchUsers`, `getUsers`,
> `createActor`, `getActorByRef`, `filterActors`, `saveAccessRules`, `getAccessRules`,
> `createReaction`, `getReactions`. Call them by these exact names. There is **no**
> dedicated "create task" / "assign" tool — a task is composed from these.

# Simulator.Company Task Specialist

In Simulator.Company a **task is an actor of the `Events` system form** with **no
`chatType`** (an empty/absent `chatType` is what separates a task / plain event from a
chat). The Events form is the same one used for calendar events, SIP meetings, and chats
— `chatType` is what disambiguates them.

> **Orders / directives are tasks.** A request to create an **order / directive document**
> aimed at a person — «створи наказ **на** Салімова про …», «склади розпорядження для …»,
> "create an order for X to …", ru «приказ на …» — is a task, not a plain event. The
> **addressee** ("на <кого>" / "for <whom>") is the **executor**: create the Events actor,
> then **grant that person `execute`** via `saveAccessRules` (step 4 below). Putting the
> name only in the title/description is the common mistake — without the `execute` rule the
> person is not assigned and gets no `done` action. Resolve the addressee's `userId` with
> `searchUsers` first; if other people are named to approve/sign, give them `sign`/`ds`.
>
> **If the workspace has a dedicated «наказ»/order form (with example documents), mirror an
> example — don't just `createActor` and stop.** Read one existing order of that form
> (`getActor` + `getAccessRules`) and reproduce **how it assigns the responsible person**:
> typically an `execute` **access rule** for the addressee (`saveAccessRules`), and/or a
> responsible/executor **data field** on the form (a `workspaceMembers` dynamic-select whose
> value is the user id — see the actor `data` protocol). Set whichever the example uses, and
> **always grant the addressee access** — an order with no access rule for the person never
> appears in their events. Creating the actor alone is not "assigned".

A task's **roles are access privileges on the task actor**, not data fields:

| Role | Privilege | How they act on the task |
|---|---|---|
| **Executor / assignee** (виконавець) | `execute` | Marks the work complete with a **`done`** reaction |
| **Approver** (хто погоджує) | `sign` | Approves with a **`sign`** reaction, or declines with **`reject`** |
| **Legal-document signer** (підпис на юр. документі) | `ds` | Applies a qualified digital signature with a **`ds`** reaction |
| **Watcher** | `view` only | Can see the task; no action |

> `view` is implied when any other privilege is set. One person can hold several roles
> (e.g. `{execute:true, sign:true}`) — that's one rule with both flags. Different people
> get separate rules.

There is **no server-side "a task must have an executor / must be approved in order"
enforcement** — a task is composed from `createActor` + `saveAccessRules` + reactions,
and **this skill owns that discipline**.

> Read `$PLUGIN_ROOT/docs/entities/tasks.md` for the full model, and
> `chats.md` for the shared Events-form field list.

Reply to the user in **their own language**.

---

## Test case: "create a task, assign it, and require a signature"

### 1 — Resolve the Events form id (per workspace)

```
getForms(formTypes="system")    # find the row whose title == "Events" → its id
```

The Events form id differs per workspace — don't hardcode it. (You may instead pass
`formName="Events"` to `createActor` and let it resolve the id.)

### 2 — Resolve the people (executor, approvers, signers)

Every grantee is a real workspace `userId` — resolve it, never guess:

```
searchUsers(query="<name>")     # → that person's userId (e.g. 4210)
getUsers()                      # browse all members if you need to pick
```

Collect: the **executor's** userId, each **approver's** userId, and (for a legal
document) each **DS signer's** userId.

### 3 — Create the task (an Events actor, no chatType)

```
createActor(
  formName="Events",                       # or formId=<EventsFormId>
  title="<task name>",                     # short label, e.g. "Prepare Q3 supplier contract"
  description="<the task body / instructions>",   # the full text of the task goes HERE
  data={ "startDate": <nowUnixSeconds>,    # required by the Events form
         "endDate":   <deadlineUnixSeconds> })   # the due date
```

- **The task body goes in the actor's `description`.** `title` is the short label; the
  full instructions / brief of the task are the actor's **`description`** field (the
  `description` argument of `createActor`) — not a comment reaction and not `data`.
- `startDate` / `endDate` are **required** by the Events form and are **plain unix
  seconds** on Events (not the nested calendar object the generic `data` protocol uses).
  Use `startDate` = now (or the scheduled start) and `endDate` = the **deadline**.
- **Do not set `chatType`** — a task is not a chat. Setting it would turn the actor into
  a chat (see `simulator-chat`).

### 4 — Assign the roles (one `saveAccessRules` call)

Grant each person their role on the task actor. `execute` = does it, `sign` = approves
it, `ds` = legally signs it. `view`/`modify`/`remove` are required booleans; the
role flags are optional.

```
saveAccessRules(objType="actor", objId="<taskId>", rules=[
  # Executor — can act on and complete the task:
  { "action":"create", "data":{ "userId": <executorId>,
      "privs":{ "view":true, "modify":true, "remove":false, "execute":true } } },

  # Approver(s) — must sign off. reactionOrders.sign sets the order (1 = first):
  { "action":"create", "data":{ "userId": <approver1Id>,
      "privs":{ "view":true, "modify":false, "remove":false, "sign":true },
      "reactionOrders":{ "sign": 1 } } },
  { "action":"create", "data":{ "userId": <approver2Id>,
      "privs":{ "view":true, "modify":false, "remove":false, "sign":true },
      "reactionOrders":{ "sign": 2 } } }
])
```

For a **legal document** that needs a qualified e-signature, grant `ds` instead of (or
in addition to) `sign`, and order signers with `reactionOrders.ds`:

```
saveAccessRules(objType="actor", objId="<taskId>", rules=[
  { "action":"create", "data":{ "userId": <signerId>,
      "privs":{ "view":true, "modify":false, "remove":false, "ds":true },
      "reactionOrders":{ "ds": 1 } } }
])
```

> `saveAccessRules` is applied **asynchronously** and returns a **`taskId`** (the async
> job id — unrelated to the task actor). Grants take a moment to propagate; that's fine.
> The creator is the **owner implicitly** — do not add a self access-rule.

### 5 — (Optional) Add follow-up notes / kick off

The task body itself is the actor's `description` (step 3). Use a **`comment` reaction**
only for *additional* discussion or to ping someone — not for the task body:

```
createReaction(type="comment", actorId="<taskId>", description="<a follow-up note>")
```

To notify the executor through the platform AI agent (acts under the requesting user's
access), add `extra={mcp:true}` — see `simulator-reactions`.

### 6 — Lifecycle (how the task is completed & approved)

These reactions are posted **by the assigned users** (your job is to set the task up so
they can):

- **Executor** finishes → `createReaction(type="done", actorId="<taskId>")`.
- **Approver** signs off → `createReaction(type="sign", actorId="<taskId>")`, or declines
  → `createReaction(type="reject", actorId="<taskId>", description="<reason>")`.
- **DS signer** applies the qualified signature → `createReaction(type="ds", actorId="<taskId>")`.
- Read progress: `getReactions(actorId="<taskId>", view="flat")` — count `done` / `sign` /
  `ds` / `reject` against the people you assigned.

> The reaction text (a comment body, a rejection reason) always goes in the
> **`description`** argument of `createReaction` — there is no `content` argument.

---

## Variations

- **One person is both executor and approver.** Combine flags in a single rule:
  `privs:{ view:true, execute:true, sign:true }`.
- **Reorder / change approvers.** `action:"update"` with a new `reactionOrders`, or
  `action:"delete"` (grantee id only) to remove someone.
- **Assign to a group.** Use `groupId` instead of `userId` in the rule (resolve it with
  `getUser(type="group")`).
- **Many tasks at once.** Create each Events actor, then `bulkSaveAccessRules` (≤50
  objects) to assign roles across all of them in one call.
- **Find a person's tasks.** `filterActors(formId=<EventsFormId>, q="chatType=",
  members="<userId>:execute")` — Events actors with an empty chatType where that user
  has the execute role. (Use `q="chatType="` to exclude chats.)

## Safety & correctness rules

- **No `chatType` on a task.** An Events actor with a `chatType` is a chat, not a task —
  leave it empty/absent.
- **Roles are access rules, never `data` fields.** Don't store executor/approver ids in
  `data`; grant `execute` / `sign` / `ds` via `saveAccessRules`.
- **Resolve every `userId` first** (`searchUsers` / `getUsers`) — grantees must belong to
  the workspace or the rule is rejected. Don't guess ids.
- **Confirm before assigning on someone's behalf** if it notifies them — `saveAccessRules`
  (default `notify=true`) and reactions notify the people involved. Use `notify=false`
  to stay quiet.
- **`execute` vs `sign` vs `ds`.** `execute` = the doer; `sign` = an approval/sign-off;
  `ds` = a legally-binding digital signature (use for юридичні документи). Pick by intent.
- `startDate`/`endDate` are **required** and are plain unix **seconds** on Events.

---

## Relationship to the other skills

| Skill | Boundary |
|---|---|
| `simulator-chat` | The *chat* use of the Events form (`chatType` set); a task has **no** chatType. |
| `simulator-access` | Access rules in general — the `execute`/`sign`/`ds`/`reactionOrders` model lives there. |
| `simulator-reactions` | The `done`/`sign`/`reject` reaction mechanics used to complete and approve a task. |
| `simulator-actors` | The Events actor's own `data` fields and the actor `data` value protocol. |
| `simulator-init` | Login + workspace selection (needed before any of this). |

## Reference Documents

| Path | When to read |
|---|---|
| `$PLUGIN_ROOT/docs/entities/tasks.md` | The task model: Events actor + execute/sign/ds roles + done/sign/reject lifecycle |
| `$PLUGIN_ROOT/docs/entities/chats.md` | The shared Events-form field list (startDate/endDate as unix seconds) |
| `$PLUGIN_ROOT/docs/entities/reactions.md` | Reaction types (`done`, `sign`, `reject`), threading, AI-agent reactions |

## Tips

- `accId`/workspace defaults to the active one — make sure `simulator-init` ran first.
- The Events form id is **per workspace** — resolve it (`getForms`/`formName`), never hardcode.
- A task = `createActor(formName="Events", description="<body>", data={startDate,endDate})` **without** `chatType`.
- **The task body is the actor's `description`** (`title` = short label); comments are for follow-up only.
- Roles = one `saveAccessRules` call: `execute` (doer), `sign` (approver), `ds` (legal signature).
- Order sequential approvals with `reactionOrders.sign` / `.ds` (positive ints, 1 = first).
- Completion/approval are `done` / `sign` / `reject` reactions posted by the assignees.
