---
name: simulator-reactions
description: >
  Simulator.Company reactions specialist — comments, events, approvals, ratings and
  other feedback attached under an actor. Use when the user wants to comment on an
  actor, reply in a thread, read/like/pin the discussion, mark comments read, or get
  reaction stats. Activate when the user says "comment on this actor", "leave a note",
  "reply to", "show the comments/discussion", "pin this comment", "mark as read",
  "прокоментуй актор", "залиш коментар", "відповісти на коментар", "закріпи коментар",
  "оставь комментарий", "ответь в треде", "закрепи комментарий". For attaching FILES to a
  reaction use `simulator-attachments`; for the actor's own fields use `simulator-actors`.
---

> **Curated tool names (v2 server):** `createReaction`, `updateReaction`, `deleteReaction`,
> `getReactions`, `getReactionsStats`, `markReactionsRead`, `getPinnedReactions`,
> `togglePinnedReaction`. Call them by these exact names.

# Simulator.Company Reactions Specialist

**Reactions** are the comments / events / approvals / ratings that live **under an actor**.
They are themselves actors, organised as a hierarchical (threaded) tree, so a reaction can
reply to another reaction.

> **Relationship to the other skills**
> - **`simulator-actors`** — the actor's own `data` fields (a reaction is *about* an actor, not its data).
> - **`simulator-attachments`** — files attached to a reaction or actor (a reaction can carry `attachments`).
> - **`simulator-graph`** — graph structure (links/layers), unrelated to reactions.

## The one rule that trips everyone up: `actorId` vs `reactionId`

Every reaction path is addressed by the **parent (root) actor**, while the reaction's **own
id** travels separately:

- **`actorId`** (path) = the **parent actor** whose reaction tree you are working in.
- **`reactionId`** (body, on update/delete/pin) = the **specific reaction** to act on.
- **`parentId`** (on create) = the reaction you are **replying to** (omit for a top-level comment).

The tools expose these as distinct arguments, so you never collide the two.

## Workspace context

Reactions are addressed by `actorId` (a UUID), so no `accId` is needed. Make sure you have the
target actor's UUID (use `simulator-actors` / `searchActors` to find it) before commenting.

---

## Comment on an actor

```
createReaction(
  type="comment",                 # everyday note; other types: view, rating, sign, ds, done, reject, freeze; ai = agent reply (see below)
  actorId="<parent actor UUID>",
  description="Looks good to me",  # the comment text
  notify=true)                     # default true; set false to post quietly
```

Reply in a thread — pass the parent reaction's id as `parentId`:

```
createReaction(type="comment", actorId="<parent actor UUID>",
  description="Agreed", parentId="<reaction UUID being replied to>")
```

Attach files by passing `attachments` (ids from `uploadBase64` / `getAttachments`, see
`simulator-attachments`):

```
createReaction(type="comment", actorId="<actor UUID>", description="see attached",
  attachments=[{ "attachId": 5521 }])
```

## Edit / delete a reaction

```
updateReaction(actorId="<parent actor UUID>", reactionId="<reaction UUID>",
  description="Updated note")
deleteReaction(actorId="<parent actor UUID>", reactionId="<reaction UUID>")   # irreversible — confirm first
```

## Read reactions

```
getReactions(actorId="<parent actor UUID>", view="tree", limit=30)   # view: tree | flat | thread
getReactionsStats(actorId="<parent actor UUID>")                     # counts by type, etc.
```

- `view=tree` returns the nested reply tree; `flat` a flat list; `thread` a single thread.
- `parentId` narrows to the replies under one reaction; `from`/`to` filter by created time.

## AI agent reactions (`extra.mcp`)

A reaction can be handed to the platform's **AI agent** by creating it with
`extra.mcp = true`. The platform then runs the agent on that reaction **under the
requesting user's access** (the agent uses this same MCP server, so it can only
read/write what the user can) and posts the answer back as a child **`ai`** reaction.

```
createReaction(type="comment", actorId="<actor UUID>",
  description="Summarize this actor and flag anything unusual",
  extra={ "mcp": true })            # → triggers the AI agent; its reply appears as a child `ai` reaction
```

The `ai` reply carries a `reasoning` object that the agent streams while it works:

- `reasoning.inProgress` — `true` while the agent is still producing the answer.
- `reasoning.thoughts` — `[{ id, text, createdAt }]`, a step log (e.g. tools it used).
- `description` — fills with the answer text (streamed live over WebSocket, then finalized).

So when reading a discussion (`getReactions`), an `ai` reaction with
`reasoning.inProgress = true` is still being written; treat its `description` as partial.

> **Don't loop:** the agent already replies on its own. Only set `extra.mcp` on a
> *human* request you want the agent to handle — not on the agent's own output. The
> platform bounds runaway chains, but creating `extra.mcp` reactions from agent
> output wastes turns.

When the agent runs, the client also passes a **UI-context** object (`control-events-context`)
that says **where the user is** — `activeActor`, `activeLayer`, `activeGraph`, `page`,
`hostOrigin`, `workspaceId`, `graphDiscovery`. Read it to resolve "here" / "this actor" /
"this layer" and to default ids the user left implicit. See
`$CLAUDE_PLUGIN_ROOT/docs/entities/ui-context.md`.

## Embed a smart form, an actor card, or chips

A reaction can carry more than text:

- **Run a Smart Form (script) in the reaction** — set `appId` to a Smart Form (CDU/Script app)
  actor id (+ optional `appSettings` `{autorun, expired, users, groups, fullWidth}`):
  `createReaction(type="comment", actorId="<actor>", appId="<smartFormActorId>", appSettings={autorun:true})`.
  (The same `appId`/`appSettings` embed a smart form into a regular actor — see `simulator-smart-forms`.)
- **Nested actor card** — `extra.linkedActorId` embeds another actor as a preview card:
  `createReaction(..., extra={ "linkedActorId": "<otherActorId>" })`.
- **Inline chips / rich text** — `description` renders **BBCode**: chips `[actor=<id>]Label[/actor]`,
  `[application=<smartFormId>]Label[/application]`, `[user=…]`, `[event=…]`; formatting `[b]`,
  `[color=…]`, `[h2]`, `[ul][*]…[/ul]`, `[url=…]`; and `[md]…[/md]` for markdown. Fetch the
  environment's exact tag set with **`getBbcodeTags`**. **BBCode works only OUTSIDE `[md]`
  blocks** — keep chips/BBCode out of any `[md]…[/md]` section. See `docs/entities/reactions.md`.

## Pin & read state

```
getPinnedReactions(actorId="<parent actor UUID>")
togglePinnedReaction(actorId="<parent actor UUID>", reactionId="<reaction UUID>", pinned=true)
markReactionsRead(actorId="<parent actor UUID>", count=12)   # clears the unread badge
```

---

## Reference Documents

| Path | When to read |
|---|---|
| `$CLAUDE_PLUGIN_ROOT/docs/entities/reactions.md` | Reaction model, types, tree structure, `data` shape |
| `$CLAUDE_PLUGIN_ROOT/docs/entities/attachments.md` | Attaching files to a reaction |

## Tips

- `actorId` is always the **parent** actor; the reaction's own id is `reactionId` (update/delete/pin).
- `type="comment"` is the default note; the full set is `view`/`comment`/`ai`/`rating`/`sign`/`ds`/`done`/`reject`/`freeze` — reserve `sign`/`ds`/`done`/`reject` for real approval / sign-off / completion flows (there is no `approve` type).
- Reply by setting `parentId`; omit it for a top-level comment.
- `notify=false` posts without sending notifications (it is honoured — sent explicitly).
- `deleteReaction` is irreversible — confirm with the user first.
- To attach a file, upload it first (`uploadBase64` → `attachId`) then pass `attachments:[{attachId}]` — see `simulator-attachments`.
