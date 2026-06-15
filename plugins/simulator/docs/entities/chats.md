# Chats (Events-form actors)

A **chat** in Simulator.Company is not a separate entity — it is an **actor of the
`Events` system form** whose `data.chatType` marks it as a conversation. Chat
**messages** are **`comment` reactions** posted under that actor. Chat
**participants** are the actor's **access-rule members** (keyed by `userId`), not a
data field.

This is the model behind the test case *"write a message to user N"*: resolve N's
user → find or create the p2p-chat Events actor → post a `comment` reaction.

---

## The `Events` system form

`Events` is one combined system form used for calendar events, scheduled SIP
meetings **and** chats. Its form **id is per-workspace** (system templates are
copied into each workspace with a fresh auto-increment id), so resolve it at
runtime — never hardcode it:

```
getForms(formTypes="system")        # find the entry whose title == "Events" → its id
# or pass formName="Events" to createActor (it resolves the title to the id)
```

Fields (system forms use **semantic field ids**, not `item_<digits>`):

| Field id | Class | Meaning |
|---|---|---|
| `startDate` | calendar | **Required.** On chats just a unix-seconds timestamp (see note). |
| `endDate` | calendar | **Required.** Same. |
| `chatType` | select | `""` = plain event / meeting · `"p2p"` = 1:1 chat · `"group"` = group chat |
| `channel` | edit | Optional channel label |
| `smartTags` | multiSelect | Smart tags |
| `moderator`, `disableCallbackSoundUsers` | select (workspaceMembers) | Meeting roles |
| `scheduleMeeting`, `isActiveMeeting`, `isPersistentMeeting`, `autoCollapseOnEntry` | check | SIP-meeting flags |
| `agenda`, `recurrence` | edit (JSON) | Meeting agenda / recurrence rules |

> **`startDate` / `endDate` value shape on Events.** Production code creates chats
> with **plain unix-second integers** — `data: { startDate: <unixSec>, endDate: <unixSec> }`
> — *not* the nested `{startDate,endDate,timeZoneOffset}` calendar object the generic
> actor `data` protocol describes. Use plain seconds here. For a chat, set both to
> "now".

## The p2p convention (the standard p2p contract)

The platform's web client does **not** invent its own scheme — and neither should the
MCP, or the two will create duplicate chats for the same pair. The standard p2p
contract:

- **Deterministic `ref`** = `p2pConversation_<minUserId>_<maxUserId>` — the two
  participants' **raw workspace userIds sorted ascending**, joined with `_`
  (prefix `p2pConversation_`). This is the p2p **uniqueness/dedup key**.
- **`title`** = `<nickA>:p2p:<nickB>` — the two nicks ordered by the same ascending
  userId sort (smaller-id user's nick first).
- **`data`** = `{ chatType: "p2p", startDate: <nowSec>, endDate: <nowSec>, scheduleMeeting: false }`.
- **Participant** = a **separate** `saveAccessRules` call after create granting the
  *other* user `{view:true, modify:true, remove:true}`. The creator is owner
  **implicitly** (no self access-rule). Members are **never** passed inline in the
  create body.
- **Reuse** = `getActorByRef(formId=<Events>, ref=<the deterministic ref>)`; on 403/404
  → create. The UI keys reuse on `ref` only — *not* a members filter.

> **Group chats** use the same Events form with `data.chatType="group"` but set **no
> `ref`** (so there is no reuse/dedup) and add every participant through access rules.

## How an actor becomes a chat

`const isChat = !!rootActor.data?.chatType;` — any Events actor with a non-empty
`chatType` is a chat:

- **`chatType="p2p"`** — peer-to-peer (1:1) chat.
- **`chatType="group"`** — group chat.
- empty / absent — a plain event or SIP meeting (these are separated from chats by a
  `chatType=` empty filter in the live-stream counter).

There is **no dedicated "create chat" endpoint** and **no server-side enforcement of
the two-participant p2p contract** — chats are created via the generic `createActor`
on the Events form, and the "exactly two members, reuse instead of duplicate"
discipline is owned by the caller (this plugin / the `simulator-chat` skill).

## Participants = access-rule members

The two people in a p2p chat are expressed as **access rules** on the chat actor:

- The **creator** becomes the owner automatically (gets access on create).
- The **other participant** is added with `saveAccessRules(objType="actor", objId=<chatId>,
  rules=[{action:"create", data:{userId:<N>, privs:{view:true, modify:true, remove:true}}}])`.

The unread-badge logic iterates `rootActor.access` (`{userId, userType}` rows) to
decide who is notified, which confirms participation lives in the access list.

## Messages = `comment` reactions

A chat message is a `comment` reaction under the chat actor:

```
createReaction(type="comment", actorId="<chatActorId>", description="<message text>")
```

Reactions are themselves actors (on the `Reactions` system form) linked to the chat
via the reactions tree edge; the reaction's `treeInfo` carries the chat's `chatType`.
Replies thread via `parentId`; files attach via `attachments` (see
`attachments.md` / the `simulator-attachments` skill).

## Finding an existing p2p chat (reuse, don't duplicate)

**Canonical (UI-interoperable):** look it up by the deterministic ref, exactly as the
web UI does:

```
getActorByRef(formId=<EventsFormId>, ref="p2pConversation_<minUserId>_<maxUserId>")
```

If it returns an actor, reuse it; on 403/404, create. This is the only lookup that is
guaranteed to match a chat the web UI created (and vice-versa), because both sides key
on this ref.

**The catch — you need the *sender's* userId too.** The ref is built from *both*
participants' workspace userIds. The recipient's comes from `searchUsers`. The MCP's
auth token, however, is a JWT carrying the caller's **`saId` + `nick`**, **not** the
per-workspace `userId`, and there is **no PAPI "current user" endpoint**. So obtain the
sender's `userId` by other means — ask the user, take it from context, or list
`getUsers` and match the JWT's `saId`/`nick` to a member's `id`.

**Fallback when the sender's userId is unknown** — discover by participant instead of
ref (still finds UI-created chats, since the recipient is an access member):

```
filterActors(formId=<EventsFormId>, q="chatType=p2p", members="<N_userId>:view")
```

But if you then *create* without the canonical ref, the web UI (which keys on ref)
won't find your chat and may make a duplicate — so set the ref whenever both userIds
are known.

## End-to-end: "write a message to user N"

1. `searchUsers(query="N")` → N's `userId` + `nick`.
2. `getForms(formTypes="system")` → the `Events` form id (or pass `formName="Events"`).
3. Resolve the **sender's** `userId` (see the catch above) → build
   `ref = "p2pConversation_<minId>_<maxId>"` and `title = "<nickA>:p2p:<nickB>"`
   (both sorted by ascending userId).
4. `getActorByRef(formId=<Events>, ref=<ref>)` → reuse if found. *(No sender id? use the
   `filterActors(members=…, q="chatType=p2p")` fallback.)*
5. If none: `createActor(formName="Events", title=<title>, ref=<ref>,
   data={chatType:"p2p", startDate:<now>, endDate:<now>, scheduleMeeting:false})`, then
   `saveAccessRules(objType="actor", objId=<chatId>, rules=[{action:"create",
   data:{userId:<N_userId>, privs:{view:true,modify:true,remove:true}}}])`.
6. `createReaction(type="comment", actorId="<chatId>", description="<message>")`.

> `getSystemActor(objType="user", objId=<userId>)` returns a user's twin actor; chat
> membership uses the raw `userId`, so you usually don't need it for messaging.

## Scopes

Chat operations use the same scopes as the underlying entities:
`control.events:actors.*` (the Events actor), access-rule management, and
reaction creation. See `reactions.md`, `actors.md`, and the access-rules docs.
