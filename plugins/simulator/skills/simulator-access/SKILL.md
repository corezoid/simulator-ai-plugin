---
name: simulator-access
description: >
  Simulator.Company access-control specialist — who can view/modify/remove/sign/execute an
  object (actor, form, account, template, tree layer). Use when the user wants to share or
  unshare an object, grant or revoke permissions, list who has access, or bulk-share. Activate
  when the user says "share this with", "give access to", "grant permission", "revoke access",
  "who can see/edit this", "set permissions", "make read-only", "поділись з", "надай доступ",
  "забери доступ", "хто має доступ", "права доступу", "дай доступ", "открой доступ",
  "отзови доступ", "кто имеет доступ", "права доступа". For the object's own data use the
  domain skill (`simulator-actors` / `simulator-forms` / `simulator-finance`).
---

> **Curated tool names (v2 server):** `getAccessRules`, `saveAccessRules`,
> `getTemplateActorsAccess`, `saveTemplateActorsAccess`, `getTreeLayerAccess`,
> `saveTreeLayerAccess`, `bulkSaveAccessRules`, `bulkSaveAccountPairsAccessRules`.
> Call them by these exact names.

# Simulator.Company Access-Control Specialist

Access rules say **who** (a user, SA user, or group) can do **what** (view / modify / remove /
sign / ds / execute) to an **object**. Grants are applied **asynchronously** — save calls
return a **`taskId`**.

## The model

- **objType** — one of `actor`, `form`, `account`, `formTemplate`, `templateActors`, `treeLayer`.
- **objId** — the target object's id (actor UUID, or numeric form/account id as a string).
- **rules** — a JSON **array** of operations: `{action, data}` where `action ∈ create | update | delete`.
- **data** — identifies the grantee by **exactly one** of `userId` | `saId` | `groupId`, plus:
  - **privs** — `{view, modify, remove (required booleans), sign?, ds?, execute?}`. `view` is
    implied when any other privilege is set.
  - **reactionOrders?** — `{sign, ds, execute}` positive integers ordering those reactions.
- **recursive** (default **true**) — cascade to child objects. Set **false** to apply to this object only.
- **notify** (default **true**) — send access-change notifications. Set **false** to apply quietly.

> `recursive`/`notify` default to true; the tools send an explicit `false` when you set it, so an
> opt-out is honoured. "Grant but don't cascade to children" ⇒ `recursive=false`.

## Workspace context

Objects are addressed by their own id (`objId`), so most tools need no `accId`. Only
`bulkSaveAccountPairsAccessRules` takes an `accId` (defaults to the configured workspace).

## Finding the grantee (`userId` / `groupId`)

A rule's grantee is identified by a real `userId` (or `saId` / `groupId`) — resolve it first,
never guess:

- **`searchUsers(accId, query)`** — find a workspace member by name/email (the quickest path).
- **`getUsers(accId)`** — list all members.
- **`getUser(accId, userId, type="group")`** — resolve a group id when sharing with a group.

> **Share onto the `user`, never their twin actor.** Each user has a 1:1 digital-twin actor,
> but access is granted to the **`userId`** — pass `userId` in the rule's `data`, not the twin
> actor's id. The twin actor is for *transactions / accounts / graph placement*
> (`getSystemActor`), not for sharing. See `$CLAUDE_PLUGIN_ROOT/docs/entities/users.md`.

---

## Read who has access

```
getAccessRules(objType="actor", objId="<actor UUID>")      # actor | form | account | formTemplate | treeLayer
getTemplateActorsAccess(objId="<form-template id>")        # the actors of a form template
getTreeLayerAccess(objId="<layer / root actor id>")        # the actors on a tree layer
```

## Grant / change / revoke

```
# Give user 4210 view + modify on an actor, but not delete:
saveAccessRules(objType="actor", objId="<actor UUID>", rules=[
  { "action": "create", "data": {
      "userId": 4210,
      "privs": { "view": true, "modify": true, "remove": false } } }
])

# Revoke a group's access to an account entirely:
saveAccessRules(objType="account", objId="<account id>", rules=[
  { "action": "delete", "data": { "groupId": 88 } }
])

# Apply to this object only (don't cascade) and stay quiet:
saveAccessRules(objType="actor", objId="<UUID>", recursive=false, notify=false, rules=[ … ])
```

`saveTemplateActorsAccess(objId=<form-template id>, rules=[…])` and
`saveTreeLayerAccess(objId=<layer id>, rules=[…])` take the same `rules` shape for the
template-actor and tree-layer scopes.

## Bulk

```
# Many objects at once (≤50), each with its own rules:
bulkSaveAccessRules(items=[
  { "objType": "form", "objId": "42", "rules": [ … ] },
  { "objType": "actor", "objId": "<UUID>", "rules": [ … ] }
])

# Share a whole account-name category across the workspace (every account named <prefix>_*):
bulkSaveAccountPairsAccessRules(accId="ws_xxx", items=[
  { "objId": "<account-name id prefix>", "rules": [ … ] }
])
```

---

## Tips

- A save returns a **`taskId`** (applied async); the change may take a moment to fully propagate.
- Identify the grantee by **exactly one** of `userId` / `saId` / `groupId`.
- `privs.view/modify/remove` are required; `sign`/`ds`/`execute` optional. Setting any privilege implies `view`.
- `recursive=false` ⇒ this object only; `notify=false` ⇒ no notifications (both honoured).
- Use `action:"delete"` (grantee id only) to revoke; `create`/`update` to grant or change.
- Resolve user/group ids first (don't guess) — grantees must belong to the workspace or the rule is rejected.
