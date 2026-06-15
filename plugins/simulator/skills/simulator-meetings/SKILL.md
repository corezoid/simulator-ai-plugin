---
name: simulator-meetings
description: >
  Simulator.Company meeting specialist — scheduling and configuring meetings / video / SIP
  rooms. In Simulator a meeting is an actor of the Events system form with
  data.scheduleMeeting=true (no chatType); the actor IS the room. Use when the user wants to
  "schedule a meeting", "create a call/video room", "set up a recurring meeting", "make a
  persistent meeting room", "add an agenda", "set a moderator", "get a join link", "invite
  people to a meeting", or work with SIP/video rooms. Activate on "schedule a meeting",
  "create a meeting", "set up a call", "recurring meeting", "weekly meeting", "standing room",
  "meeting agenda", "join link", "invite to the meeting", "запланувати зустріч", "створи
  мітинг", "повторювана зустріч", "щотижнева зустріч", "постійна кімната", "агенда зустрічі",
  "посилання на зустріч", "запроси на мітинг", "запланируй встречу", "создай митинг",
  "повторяющаяся встреча", "постоянная комната", "агенда встречи", "ссылка на встречу". Also
  for reading a call's transcription — "transcription", "what was said", "summarize the call",
  "meeting notes from the call", "транскрипція дзвінка", "що казали на дзвінку", "підсумуй
  дзвінок", "транскрипция звонка", "итоги звонка". For a 1:1/group chat use `simulator-chat`;
  for a task use `simulator-tasks`; for general access use `simulator-access`.
---

> **Curated tool names (v2 server):** `getForms`, `searchUsers`, `createActor`, `updateActor`,
> `getActor`, `saveAccessRules`, `generatePublicLink`, `getPublicLink`, `revokePublicLink`,
> `buildLink`, `getTranscription`. Call them by these exact names. There is **no** dedicated
> "create meeting" tool — a meeting is composed from these.

# Simulator.Company Meeting Specialist

A **meeting is an actor of the `Events` system form** with **`data.scheduleMeeting = true`**
(and **no `chatType`** — that would make it a chat). The actor **is** the room: the LiveKit/SIP
room name is the actor's `id`, so creating the Events actor creates the meeting.

- **Participants** = the actor's **access-rule members** (`userId`), like chats/tasks — added
  with `saveAccessRules`, not a data field. `data.moderator` (a `userId`) names the host.
- `startDate` / `endDate` are **required** and are **plain unix seconds** on Events.
- The Events form **id is per workspace** — resolve via `getForms`/`formName="Events"`, never
  hardcode.

> Read `$CLAUDE_PLUGIN_ROOT/docs/entities/meetings.md` for the full model (recurrence schema,
> agenda, persistent rooms, link variants) and `chats.md` for the shared Events fields.

Reply to the user in **their own language**.

---

## Schedule a meeting

```
# 1 — resolve people (moderator / invitees)
searchUsers(query="<name>")              # → userId(s)

# 2 — create the meeting (Events actor, scheduleMeeting=true, NO chatType)
createActor(
  formName="Events",
  title="<meeting name>",
  data={ "scheduleMeeting": true,
         "startDate": <startUnixSec>,    # required, plain unix SECONDS
         "endDate":   <endUnixSec>,      # required
         "moderator": <hostUserId> })    # optional

# 3 — invite participants (access-rule members, by userId — creator is owner)
saveAccessRules(objType="actor", objId="<meetingId>", rules=[
  { "action":"create", "data":{ "userId": <inviteeId>, "privs":{ "view":true } } } ])
```

## Recurring meetings

Set `data.recurrence` (the platform expands occurrences from it):

```
createActor(formName="Events", title="Weekly sync", data={
  "scheduleMeeting": true, "startDate": <s>, "endDate": <e>,
  "recurrence": { "freq":"weekly", "interval":1, "byDay":[1,3] } })  # Mon & Wed
```

- `freq` (**required**): `daily` | `weekly` | `monthly` | `yearly`; `interval` (every N, default 1).
- End it with `until` (unix seconds) **or** `count` (number of occurrences).
- `weekly` → `byDay:[0..6]` (0=Sun..6=Sat). `monthly` → `byMonthDay:1..31`, or positional
  `bySetPos:1|-1` + `byDayOfWeek:0..6` (e.g. last Friday). `timezone` optional (IANA).
- **The meeting's duration (`endDate-startDate`) must fit within one `interval`** — the backend
  rejects e.g. a 2-day meeting on a daily series.

## Persistent (standing) room

A room that stays open and is reused across sessions:

```
createActor(formName="Events", title="War room", data={
  "scheduleMeeting": true, "isPersistentMeeting": true,
  "startDate": <now>, "endDate": <now + 3600> })
```

> Don't set `data.isActiveMeeting` — that's a runtime flag the platform toggles while a call
> is live.

## Agenda

`data.agenda` is an ordered list of items (the meeting UI renders it):

```
data={ "scheduleMeeting": true, "startDate": <s>, "endDate": <e>,
       "agenda": [ { "id":"a1", "title":"Intro", "order":1 },
                   { "id":"a2", "title":"Demo",  "order":2 } ] }
```

To change agenda/time/moderator later: `updateActor(formId=<Events>, actorId=<meetingId>, data={…})`.

## Join links

| Want | Tool / URL |
|---|---|
| Open the meeting **in-app** (meeting tab) | `buildLink(entity="actor", id="<meetingId>")` → **append `?tab=meeting`** to the result → `/actors_graph/{acc}/view/{meetingId}?tab=meeting` |
| Dedicated **meeting-room** route | `buildLink(entity="meeting", id="<meetingId>")` → `/meetingRoom/{acc}/{meetingId}` |
| **Public join, no login** (external / SIP) | `generatePublicLink(actorId="<meetingId>", waitList=<true/false>)` → `{hash, url}` (`<host>/m/<hash>`); `getPublicLink` to re-share, `revokePublicLink` to kill it |

- **`?tab=meeting`** is the key trick — appending it to the actor view URL opens the meeting
  tab directly. `buildLink` doesn't add query params, so append it yourself to the `actor` link.
- `generatePublicLink` `waitList=true` puts joiners in a waiting room (admit manually);
  `false` = join directly. Optional `ttl` (60–86400s) / `dueDate` (unix s) bound the link's life;
  the link is temporary — generate again to refresh.

## Read the call transcription

You **can read** a meeting's speech transcription — useful to summarize a call, pull action
items, or answer "what was said about X":

```
getTranscription(actorId="<meetingId>", orderValue="ASC")   # oldest-first = natural reading order
getTranscription(actorId="<meetingId>", limit=50, offset=0) # page; response carries total + hasMore
```

- `orderValue` = `ASC` (oldest first, reads naturally) or `DESC` (newest first, default); page with
  `limit` (1–100) / `offset`. Requires **view** access to the meeting actor.
- The backend serves this for a meeting with a **live / active** room — it returns an
  "active call" error when there is no current call for the actor. So read it during (or right
  around) the call, not for a long-finished one.

## What this skill does NOT do

The **live call mechanics** — joining, real-time participants, mute, SIP dial-in, recording,
producing the transcription, AI meeting agents — are handled by the platform (LiveKit/SIP) and
the web/mobile clients, **not** by these tools. Your job is to **create and configure** the
meeting, manage **invitees** (access rules) and **links**, **read** the transcription, and
update the meeting actor.

## Safety & correctness rules

- **No `chatType`** on a meeting — it would become a chat (see `simulator-chat`).
- **Invitees are access rules, not data** — `saveAccessRules` with `userId`; don't store them in `data`.
- **Resolve every `userId` first** (`searchUsers`) — moderator and invitees must be real members.
- `startDate`/`endDate` are **required**, **plain unix seconds**.
- **Confirm before inviting** on the user's behalf — `saveAccessRules` notifies people.
- Recurrence: `freq` required; the meeting's duration must fit one `interval`.

## Relationship to the other skills

| Skill | Boundary |
|---|---|
| `simulator-chat` | The *chat* use of Events (`chatType` set); a meeting has **no** chatType. |
| `simulator-tasks` | The *task* use of Events; a meeting uses `scheduleMeeting`. |
| `simulator-access` | Access rules in general (invitees/moderator visibility are access rules). |
| `simulator-actors` | The Events actor's own `data` fields and the actor `data` protocol. |
| `simulator-init` | Login + workspace selection (needed first). |

## Reference Documents

| Path | When to read |
|---|---|
| `$CLAUDE_PLUGIN_ROOT/docs/entities/meetings.md` | Full meeting model: recurrence schema, agenda, persistent rooms, link variants |
| `$CLAUDE_PLUGIN_ROOT/docs/entities/chats.md` | Shared Events-form field list (startDate/endDate as unix seconds) |
| `$CLAUDE_PLUGIN_ROOT/docs/entities/users.md` | moderator/invitees are userIds (share to the user, not the twin) |

## Tips

- A meeting = `createActor(formName="Events", data={scheduleMeeting:true, startDate, endDate})` **without** `chatType`.
- Recurring → `data.recurrence={freq, interval, …}`; persistent room → `data.isPersistentMeeting=true`.
- Agenda → `data.agenda=[{id,title,order}]`; moderator → `data.moderator=<userId>`.
- In-app link = `buildLink(entity="actor")` + `?tab=meeting`; public link = `generatePublicLink`.
- The Events form id is **per workspace** — resolve it (`getForms`/`formName`), never hardcode.
- Summarize a call → `getTranscription(actorId, orderValue="ASC")` (needs a live/active room; view access).
