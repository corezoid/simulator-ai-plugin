---
name: simulator-chat
description: >
  Simulator.Company chat & messaging specialist — sending messages to a user, and
  creating or reusing p2p (1:1) and group chats. In Simulator a chat is an actor of
  the Events system form (data.chatType = "p2p" | "group"), its participants are the
  actor's access-rule members, and its messages are `comment` reactions. Use when the
  user wants to "message", "write to", "DM", "send a message to" someone, "open a chat
  with", "start a conversation", or "reply in the chat". Activate on
  "write a message to user N", "send N a message", "open a chat with", "message someone",
  "напиши повідомлення користувачу", "надішли повідомлення", "відкрий чат з",
  "почни розмову з", "напиши юзеру", "напиши сообщение пользователю",
  "отправь сообщение", "открой чат с", "напиши в чат". For comments/approvals on an
  arbitrary actor (not a chat) use `simulator-reactions`; for files use
  `simulator-attachments`; for sharing/access in general use `simulator-access`.
---

> **Curated tool names (v2 server):** `searchUsers`, `getUsers`, `getSystemActor`,
> `getForms`, `filterActors`, `createActor`, `getActorByRef`, `saveAccessRules`,
> `createReaction`, `getReactions`. Call them by these exact names. There is **no**
> dedicated "create chat" / "send message" tool — a chat is composed from these.

# Simulator.Company Chat Specialist

In Simulator.Company a **chat is an actor of the `Events` system form**:

- `data.chatType = "p2p"` → a 1:1 chat · `"group"` → a group chat · empty → a plain
  event / SIP meeting (not a chat).
- **Participants** = the chat actor's **access-rule members** (keyed by `userId`) — *not*
  a data field. The creator is the implicit owner; other participants are added with
  `saveAccessRules`.
- **Messages** = **`comment` reactions** posted under the chat actor.

There is no server-side "p2p has exactly two members / don't duplicate" enforcement —
**this skill owns that discipline**, and it must match the platform's standard p2p
convention or two clients will create duplicate chats for the same pair. The
convention:

- **Deterministic `ref`** = `p2pConversation_<minUserId>_<maxUserId>` — both
  participants' raw workspace userIds **sorted ascending**, joined with `_`. This is the
  p2p dedup key; the UI finds/reuses a chat *only* by this ref.
- **`title`** = `<nickA>:p2p:<nickB>` — nicks ordered by the same ascending userId sort.
- **`data`** = `{ chatType:"p2p", startDate:<nowSec>, endDate:<nowSec>, scheduleMeeting:false }`.
- **Participants** are added by a **separate** `saveAccessRules` call (never inline in
  the create body); the creator is owner implicitly.

> Read `$PLUGIN_ROOT/docs/entities/chats.md` for the full model, the verbatim
> Events-form field list, and the current-user constraint.

Reply to the user in **their own language**.

---

## Test case: "write a message to user N"

This is the canonical flow. Each step names the exact tool.

### 1 — Resolve user N

```
searchUsers(query="N")          # accId defaults to the active workspace
```

Take N's `userId` from the result (e.g. `4210`). If several match, ask the user which
one (or show name + email). `getUsers` lists everyone if you need to browse.

> `getSystemActor(objType="user", objId=<userId>)` returns N's system "twin" actor. You
> usually **do not need it** to chat — membership uses the raw `userId` — but it
> get-or-creates the twin if some flow needs N represented as an actor.

### 2 — Resolve the Events form id (per workspace)

```
getForms(formTypes="system")    # find the row whose title == "Events" → its id
```

The Events form id differs per workspace, so don't hardcode it. (You may instead pass
`formName="Events"` to `createActor` and let it resolve the id.)

### 3 — Build the canonical ref + title (reuse key)

The p2p dedup key needs **both** userIds. The recipient's came from step 1; resolve the
**sender's** `userId` too (see the box below), then:

```
ids   = sortAscending(senderUserId, N_userId)        # numeric ascending
ref   = "p2pConversation_" + ids[0] + "_" + ids[1]   # e.g. p2pConversation_4210_4310
title = nick(ids[0]) + ":p2p:" + nick(ids[1])        # e.g. olena:p2p:petro
```

> **Getting the sender's userId.** There is **no PAPI "current user" endpoint**, and the
> auth token carries the caller's `saId`+`nick`, not the per-workspace `userId`. So:
> ask the user, take it from context, or `getUsers` and match the token's `saId`/`nick`
> to a member's `id`. **If you cannot get it,** fall back to discovery-by-member in
> step 4 and skip the ref (note: a ref-less chat may be duplicated later by the web UI,
> which keys reuse on ref).

### 4 — Find an existing chat (reuse, don't duplicate)

Canonical — by the deterministic ref, exactly as the web UI does:

```
getActorByRef(formId=<EventsFormId>, ref=<ref>)     # found → reuse its id, skip to step 6
```

Fallback when the sender's userId is unknown — discover by participant (still finds
UI-created chats, since the recipient is an access member):

```
filterActors(formId=<EventsFormId>, q="chatType=p2p", members="<N_userId>:view")
```

### 5 — Create the p2p chat (only if none exists)

```
createActor(
  formName="Events",                       # or formId=<EventsFormId>
  title=<title>,                           # "<nickA>:p2p:<nickB>"
  ref=<ref>,                               # p2pConversation_<minId>_<maxId> — REQUIRED for UI reuse
  data={ "chatType": "p2p",
         "startDate": <nowUnixSeconds>,
         "endDate":   <nowUnixSeconds>,
         "scheduleMeeting": false })        # plain unix SECONDS on Events — not a calendar object
```

`startDate`/`endDate` are **required** by the Events form. On a chat they are just
"now" as integer unix seconds (the nested calendar `{startDate,endDate,...}` object used
elsewhere does **not** apply here).

Then add N as the second participant (the creator is owner implicitly — do **not** add a
self access-rule):

```
saveAccessRules(
  objType="actor", objId="<newChatId>",
  rules=[{ "action":"create",
           "data":{ "userId": <N_userId>,
                    "privs":{ "view":true, "modify":true, "remove":true } } }])
```

`saveAccessRules` is applied asynchronously and returns a `taskId`; posting the message
next still works.

### 6 — Post the message (a `comment` reaction)

```
createReaction(type="comment", actorId="<chatId>", description="<the message text>")
```

That `comment` reaction under the chat actor **is** the chat message. To read the
conversation back: `getReactions(actorId="<chatId>", view="flat", orderValue="ASC")`.

---

## Variations

- **Group chat.** Create with `data.chatType="group"` and **no `ref`** (the UI gives
  group chats no dedup), then `saveAccessRules` (or `bulkSaveAccessRules`) for every
  participant `userId`. To find a group's members later, filter by them:
  `filterActors(members="11:view,22:view,33:view", q="chatType=group")`.
- **Reply in a thread.** Pass `parentId=<reactionId>` to `createReaction` (see
  `simulator-reactions`).
- **Attach a file to a message.** Upload first (`uploadBase64` → `attachId`), then
  `createReaction(..., attachments=[{attachId:<id>}])` — see `simulator-attachments`.
- **Post quietly (no notification).** `createReaction(..., notify=false)`.
- **Hand the message to the AI agent.** `createReaction(..., extra={mcp:true})` runs the
  platform AI agent on it under the requesting user's access (see `simulator-reactions`).

## Reuse & safety rules

- **Always look up (step 4) before creating (step 5).** Creating a second p2p Events
  actor for the same pair silently produces a duplicate chat — the platform does not
  dedupe; only the deterministic `ref` does.
- **Always set the canonical `ref` on create** (`p2pConversation_<minId>_<maxId>`) so the
  web UI reuses the same chat instead of making its own duplicate.
- **Participants are access rules, never `data` fields.** Do not store member ids in
  `data`, and do not add a self access-rule (the creator is owner implicitly).
- **Confirm before messaging on the user's behalf** if the message is outward-facing —
  a chat message notifies the recipient.
- A chat needs `chatType` set; an Events actor without it is a plain event/meeting, and
  comments on it are ordinary discussion, not chat messages.

---

## Relationship to the other skills

| Skill | Boundary |
|---|---|
| `simulator-reactions` | Generic comments/approvals/ratings on **any** actor (a chat message is a `comment` reaction — that skill covers the reaction mechanics). |
| `simulator-actors` | The Events actor's own `data` fields and the actor `data` value protocol. |
| `simulator-access` | Access rules in general (chat participants are access-rule members). |
| `simulator-attachments` | Files on a message/actor. |
| `simulator-init` | Login + workspace selection (needed before any of this). |

## Reference Documents

| Path | When to read |
|---|---|
| `$PLUGIN_ROOT/docs/entities/chats.md` | The chat model: Events form, chatType, members-as-access, messages-as-reactions |
| `$PLUGIN_ROOT/docs/entities/reactions.md` | Reaction types, tree/threading, AI-agent reactions |
| `$PLUGIN_ROOT/docs/entities/system-forms.md` | The Events system form among the built-ins |

## Tips

- `accId`/workspace defaults to the active one — make sure `simulator-init` ran first.
- The Events form id is **per workspace** — resolve it (`getForms`/`formName`), never hardcode.
- On Events, `startDate`/`endDate` are plain unix **seconds**, both "now" for a chat.
- Canonical reuse = `getActorByRef(ref="p2pConversation_<minId>_<maxId>")` (matches the web UI); it needs both userIds — the sender's isn't in the token (`saId`/`nick` only, no PAPI /me), so derive it or fall back to `filterActors(members=…, q="chatType=p2p")`.
- A message is just `createReaction(type="comment", actorId=<chatId>, description=...)`.
