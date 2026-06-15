# Users & their digital-twin actors

Simulator.Company has a first-class **`user`** entity (a workspace member) **and**, for
every user, a 1:1 **digital-twin actor** — a graph node that represents that user. The two
are used for **different** jobs, and picking the wrong one is the most common modelling
mistake:

| You want to… | Work with… | How |
|---|---|---|
| **Share / grant access** to an actor, account, form, layer, … | the **`user`** (by `userId`) | `saveAccessRules(... data:{userId})` |
| **Add a participant** to a chat / task / approval | the **`user`** (by `userId`) | access-rule member `userId` (chat participants, task `execute`/`sign`/`ds`) |
| **Run a transaction / transfer**, read or create **accounts**, place the person **on the graph** | the user's **twin actor** (by `actorId`) | `getSystemActor` → then `getAccounts` / `createTransfer` / `manageLayerActors` |

**Rule of thumb:** *who may see/do something* → the **user**. *Where value or a graph node
lives* → the **twin actor**. Accounts, transactions and graph placement are properties of an
**actor**, so they always go through the user's twin actor — never the bare `userId`.

---

## The `user` entity

A workspace member. Key fields the tools surface:

- **`userId`** — the per-workspace user id; the grantee in access rules and the member id in
  chats/tasks. This is what you pass to `saveAccessRules`, `filterActors(members=…)`, etc.
- **`saId`** — the SuperAdmin (global) account id behind the user; `nick` — display name.
- **`type`** — `user` or `api`.

Find a user with **`searchUsers(query=…)`** (by name/email) or **`getUsers()`** (list all);
resolve a group with `getUser(type="group")`. Access-rule grantees are identified by **exactly
one** of `userId` | `saId` | `groupId`.

> **There is no PAPI "current user" endpoint.** The auth token carries the caller's
> **`saId` + `nick`**, *not* the per-workspace `userId`. To get the caller's own `userId`,
> match the token's `saId`/`nick` against `getUsers()`, or ask the user. (Relevant for p2p
> chat refs and self-assignment — see [`chats.md`](chats.md).)

## The digital-twin actor

When a user first needs to be represented as a graph node (e.g. to hold accounts), the
platform creates a **system actor** that mirrors them, **one-to-one**:

- `isSystem: true`, `systemObjType: "user"`, `systemObjId: <userId>` — the back-link to the
  user.
- Lives on the **`System`** system form; `title` = the user's `nick`.
- Is **get-or-created idempotently**: the first `getSystemActor(objType="user", objId=<userId>)`
  creates it (with its **default accounts**) and every later call returns the same actor.
- Carries the user's **accounts** (created via the form's default accounts), so it is the
  endpoint for transactions/transfers and the node you place on a graph/layer.

Resolve it with the MCP tool:

```
getSystemActor(objType="user", objId="<userId>")   → the twin actor (id, title, formId, …)
```

### Worked examples

**Transfer money between two people** (value lives on accounts → twin actors):
```
searchUsers("Lugovoy")            → userId
getSystemActor(objType="user", objId=<userId>)   → twin actorId
getAccounts(actorId=<twin actorId>)               → accountId (pick the right currency/name)
createTransfer(... from/to legs by accountId ...)  # or createTransaction for one account
```

**Place a person on a graph layer** (a graph node is an actor → the twin):
```
searchUsers("Lugovoy") → userId → getSystemActor(objType="user", objId=<userId>) → twin actorId
manageLayerActors(actorId="<layerActorId>", items=[{action:"create", data:{id:"<twin actorId>", type:"actor", position:{x,y}}}])
```

**Share an actor with a person** (access is granted to the *user*, not the twin):
```
searchUsers("Lugovoy") → userId
saveAccessRules(objType="actor", objId="<actorUUID>",
  rules=[{action:"create", data:{userId:<userId>, privs:{view:true, modify:true, remove:false}}}])
```

## Twins of other entities

The same mechanism exists for several entity kinds — `SYS_ACTORS_OBJ_TYPES` =
`user`, `workspace`, `aiAgent`, `actorBagGraph`, `actorBagLayer` — plus a **form-template**
twin (an actor on the `AccountTemplates` form, found by `ref = "accountTemplate_<formId>"`).
These are created automatically by the platform (workspace bootstrap, form creation, etc.).

> **Only the `user` twin is exposed through `getSystemActor`** — its `objType` accepts
> `user` only. The other twins are internal; the form-template twin is reachable with
> `getActorByRef(formId=<AccountTemplates form id>, ref="accountTemplate_<formId>")`. Don't
> try `getSystemActor(objType="workspace"|"form")` — it returns nothing.

## Related

- [`accounts.md`](accounts.md) — accounts hang off actors (incl. the user twin).
- [`actors.md`](actors.md) — the actor model and the `isSystem` / `systemObjType` fields.
- The `simulator-access` skill (sharing → users), `simulator-finance` (transactions →
  twins), `simulator-chat` and the task model in [`tasks.md`](tasks.md) (members → users).
