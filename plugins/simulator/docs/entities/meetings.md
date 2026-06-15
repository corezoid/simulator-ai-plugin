# Meetings (Events-form actors)

A **meeting** in Simulator.Company is an **actor of the `Events` system form** with
**`data.scheduleMeeting = true`** (and **no `chatType`** — a `chatType` would make it a
chat). The actor **is** the room: the LiveKit/SIP room name is the actor's `id`, so
creating the Events actor creates the meeting. There is no separate "create meeting"
endpoint — compose it from `createActor` + the link tools.

> Like chats and tasks, a meeting is one of the `Events`-form use-cases (see
> [`chats.md`](chats.md), [`tasks.md`](tasks.md)). `chatType` empty + `scheduleMeeting`
> true ⇒ meeting.

## Create a meeting

```
createActor(
  formName="Events",
  title="<meeting name>",
  description="<optional notes>",
  data={
    "scheduleMeeting": true,
    "startDate": <startUnixSec>,        # required by Events — plain unix SECONDS
    "endDate":   <endUnixSec>,          # required
    "moderator": <userId>               # optional, a workspace member (select: workspaceMembers)
  })
```

- `startDate` / `endDate` are **required** and are **plain unix seconds** on Events (not
  the nested calendar object the generic actor `data` protocol uses).
- **No `chatType`.** Leave it empty/absent.
- The Events form **id is per workspace** — resolve via `getForms(formTypes="system")` (title
  == "Events") or pass `formName="Events"`. Never hardcode it.
- **Participants** are the actor's **access-rule members** (keyed by `userId`), exactly like
  chats/tasks — add them with `saveAccessRules` (not a data field). The creator is owner.
  `moderator` is a `data` field naming the meeting host (a `userId`).

## Persistent rooms

A **standing / persistent room** that stays open (doesn't auto-close when empty and is reused
across sessions) is marked with **`data.isPersistentMeeting = true`**:

```
createActor(formName="Events", title="Team standup room",
  data={ "scheduleMeeting": true, "isPersistentMeeting": true,
         "startDate": <now>, "endDate": <now + 1h> })
```

> `data.isActiveMeeting` is a **runtime** flag the platform toggles while a call is live —
> don't set it yourself.

## Recurring meetings

Set **`data.recurrence`** (the platform computes occurrences from it). Schema (from the
backend recurrence calculator):

```
data.recurrence = {
  "freq": "daily" | "weekly" | "monthly" | "yearly",   # required
  "interval": <number>,                # every N freq units (default 1)
  "until": <unixSec>,                  # optional end date, OR
  "count": <number>,                   # optional number of occurrences
  "byDay": [0..6],                     # weekly: days, 0=Sun..6=Sat (e.g. [1,3] = Mon+Wed)
  "byMonthDay": <1..31>,               # monthly by date
  "bySetPos": <1 | -1>,                # monthly positional (1=first, -1=last) …
  "byDayOfWeek": <0..6>,               # …combined with byDayOfWeek (e.g. last Friday)
  "timezone": "<IANA tz>"              # optional
}
```

```
# Weekly on Mon & Wed:
data={ "scheduleMeeting": true, "startDate": <s>, "endDate": <e>,
       "recurrence": { "freq":"weekly", "interval":1, "byDay":[1,3] } }
```

> **Duration must fit the interval.** The single occurrence's `endDate - startDate` must not
> exceed the recurrence interval (a daily series can't have a >24h meeting, a weekly series
> can't exceed `interval` weeks, etc.) — the backend rejects it otherwise.

## Agenda

Set **`data.agenda`** — an ordered list of agenda items the meeting UI renders, e.g.:

```
data={ "scheduleMeeting": true, "startDate": <s>, "endDate": <e>,
       "agenda": [ { "id":"a1", "title":"Intro", "order":1 },
                   { "id":"a2", "title":"Demo",  "order":2 } ] }
```

(Item shape is owned by the meeting UI — `id` + `title` + `order` are the core fields.)

## Joining: links

| Goal | Link | How |
|---|---|---|
| Open the meeting **in-app** (meeting tab of the actor) | `/actors_graph/{acc}/view/{actorId}?tab=meeting` | `buildLink(entity="actor", id=<meetingActorId>)` then append **`?tab=meeting`** |
| Open the dedicated **meeting room** route | `/meetingRoom/{acc}/{actorId}` | `buildLink(entity="meeting", id=<meetingActorId>)` |
| **Public join without a login** (external people / SIP) | `<host>/m/<hash>` | `generatePublicLink(actorId, waitList)` → `{hash, url}`; `getPublicLink` / `revokePublicLink` to read / kill it |

> **`?tab=meeting`** is the key trick: appending it to an actor's view URL opens the **meeting
> tab** directly (verified in the web client). `buildLink` doesn't add query params, so append
> `?tab=meeting` to its `actor` result yourself.

## Transcription (readable)

A meeting's speech transcription is **readable** via **`getTranscription(actorId, limit?,
offset?, orderValue?)`** — the spoken-word log, paged (`total` + `hasMore`), `orderValue`
`ASC`/`DESC`. Use it to summarize a call or extract action items. The backend serves it for a
meeting with a **live/active** room (it errors "active call" otherwise), and the caller needs
**view** access.

## Runtime (not via these tools)

The live-call **mechanics** — joining, real-time participants, mute, SIP dial-in, recording,
producing the transcription, AI meeting agents — are handled by the platform (LiveKit/SIP) and
the web/mobile clients, not by the curated MCP tools. The agent's job here is to **create and
configure** the meeting (Events actor + agenda/recurrence/persistent), manage **participants**
(access rules) and **links** (public + in-app), **read** the transcription, and read/update
the actor — not to drive the live session.

## Related

- [`chats.md`](chats.md) — the shared `Events`-form field list (startDate/endDate as unix seconds).
- [`users.md`](users.md) — moderator/participants are `userId`s (share to the user, not the twin).
- The `generatePublicLink` / `getPublicLink` / `revokePublicLink` tools and `buildLink`.
