# UI context (`control-events-context`)

When the assistant runs as the platform's **AI agent** (an `extra.mcp` reaction — see
[`reactions.md`](reactions.md)), the client passes a **UI-context** object describing
**where the user is in the Simulator interface** at the moment they asked. The platform
forwards it (base64-JSON in the `control-events-context` request header) and injects the
decoded JSON into the agent's system prompt. **Read it to resolve deictic references**
("here", "this actor", "this layer", "open it") and to default ids the user left implicit.

> This context is **input awareness only** — you don't create or send it. It is present in
> the AI-agent flow; an ordinary Claude Code session won't have it. Use it **when present**.

## Example

```json
{
  "actorRef": "ai_console_system",
  "workspaceId": "a58d969b-4b2f-42ce-add5-0972c4f45421",
  "hostOrigin": "https://mw.simulator.company",
  "page": "/actors_graph/a58d969b-4b2f-42ce-add5-0972c4f45421/graph/4c39176b-c3fd-4e24-b63f-c2319e2ceada/layers/21a7c6b6-bba1-4b19-9fcd-7e94f29a9e8a",
  "activeActor": "21a7c6b6-bba1-4b19-9fcd-7e94f29a9e8a",
  "activeReaction": "5f0c8a91-2b14-4f3e-9d77-1a2b3c4d5e6f",
  "activeLayer": "21a7c6b6-bba1-4b19-9fcd-7e94f29a9e8a",
  "activeGraph": "4c39176b-c3fd-4e24-b63f-c2319e2ceada",
  "graphDiscovery": {
    "21a7c6b6-bba1-4b19-9fcd-7e94f29a9e8a": {
      "rectsHistory": [
        { "x": -695.75, "y": -460.56, "width": 663.26, "height": 891.83 },
        { "x": -695.75, "y": -460.56, "width": 851.02, "height": 891.83 }
      ],
      "balanceParams": { "withBalances": true, "withReactions": true, "withNum": true },
      "newNodesCount": 2
    }
  }
}
```

## Fields

| Field | Meaning | How to use it |
|---|---|---|
| `workspaceId` | The active workspace (full UUID). | The workspace the user is in — use it as `accId` if you need to override the configured one. |
| `hostOrigin` | The web-app origin, e.g. `https://mw.simulator.company`. | The web base for deep-links (`hostOrigin + page`, or pair with `buildLink`). |
| `page` | The current route path (everything after the origin). | A ready-made link to *what the user is looking at* — `hostOrigin + page` is the full URL. Also tells you the section (`/actors_graph/…`, `/chats/…`, `/events/…`). |
| `activeActor` | UUID of the actor currently open/focused. | Resolve "this actor" / "the open card" / "it". Pass to `getActor`, `createReaction`, `buildLink(entity="actor")`, etc. For the **actor's own files** ("files on this actor"), `getActorAttachments(activeActor)`. |
| `activeReaction` | UUID of the **reaction that triggered the agent** — i.e. the user's message itself (the `extra.mcp` reaction). May be absent on older platforms. | Resolve "**this message**" / "the file **I sent**". The triggering message is a *reaction* under `activeActor` (a separate actor), so its files are read with `getActorAttachments(activeReaction)` → `readAttachment(...)` — a **different set** from the actor's own attachments. |
| `activeLayer` | UUID of the graph layer currently open. | Resolve "this layer" / "here" for placement. Pass as the `actorId` of layer tools (`getLayerActors`, `manageLayerActors`, …). |
| `activeGraph` | UUID of the graph (graph-folder) currently open. | Resolve "this graph/diagram". The `graphFolderId` in graph routes. |
| `actorRef` | The origin widget/console ref (e.g. `ai_console_system`). | Routing/provenance — where the request came from. Rarely needed for the answer itself. |
| `graphDiscovery` | Per-layer view state, keyed by **layer UUID**. See below. | Tells you *where on the canvas* the user is looking and what the view loaded. |

### `graphDiscovery[<layerId>]`

- **`rectsHistory`** — viewport rectangles (`{x, y, width, height}` in canvas coordinates);
  the **last entry is the current visible region**. Use it to answer "what's on screen" or to
  place new nodes within view (vs. far off-canvas).
- **`balanceParams`** — what the open view loaded: `withBalances` (account balances shown),
  `withReactions` (comments shown), `withNum` (counts shown). Hints at what the user can see.
- **`newNodesCount`** — how many nodes were recently added to this layer in the session.

## Using it

- **Deixis → ids.** "Add a step **here**" → place on `activeLayer`. "Rename **this** actor" →
  `activeActor`. "What's in **this** diagram?" → `activeGraph`. Don't ask the user for an id you
  already have in the context.
- **Default, don't override.** If the user names a specific actor/layer, that wins; the context
  only fills the gaps.
- **`activeActor` is "this/current/open actor" — it outranks the actor you were triggered on.**
  In the AI-agent flow you act on a *root actor* (the one the triggering reaction lives on), but the
  user may be **viewing a different actor** (e.g. from the AI console). When `activeActor` is present
  and differs from that root actor, deictic references — "this actor", "the current actor", "what I'm
  looking at" — mean **`activeActor`**, not the root actor. Resolve them against `activeActor` (e.g.
  `getActor(activeActor)`) unless the user names another.
- **Attachments: two distinct sets, same tool.** `getActorAttachments(<id>)` → `readAttachment` reads
  whichever you point it at — pick the id by what the user means:
  - **Actor's own files** ("files on this actor", the attachments tab) → `getActorAttachments(activeActor)`.
  - **A file the user attached to *their message*** ("do you see the file I sent?") → the triggering
    message is a **reaction** under `activeActor`, so use **`activeReaction`**:
    `getActorAttachments(activeReaction)`. **Fallback** when `activeReaction` is absent (older platform):
    `getReactions(actorId=activeActor, view="flat", orderValue="DESC")`, take the most recent human
    (non-`ai`) reaction (the one with `extra.mcp`), then `getActorAttachments(<that reaction id>)`.

  These are different sets — the message's file is **not** in the actor's own attachments and vice-versa.
  If it's ambiguous which the user wants, check **both**.
- **Stay in place.** Prefer operating on the layer/graph the user is viewing rather than creating
  a new one, unless they ask otherwise.
- **Link to the current view.** `hostOrigin + page` is the exact URL the user is on. The
  **`buildLink` tool auto-consumes this context** when the host forwards it: it defaults the web
  base to `hostOrigin`, the workspace to `workspaceId`, and the primary id to the open
  `activeActor` (for `entity=actor`/`meeting`) or `activeLayer` (for `entity=layer`) — so
  `buildLink(entity="actor")` / `buildLink(entity="layer")` with no id link to what the user has
  open. An explicit `id`/`accId` always overrides the context.

> **Note on the `acc` segment.** In `page`, the workspace segment is the **full** `workspaceId`
> UUID (e.g. `a58d969b-4b2f-42ce-add5-0972c4f45421`), whereas `buildLink` emits the short 8-char
> form. Both resolve the same workspace; if you need to reproduce the user's exact URL, reuse
> `page` verbatim rather than rebuilding it.

## Related

- [`reactions.md`](reactions.md) — the `extra.mcp` AI-agent flow this context arrives with.
- [`users.md`](users.md) — `workspaceId` here is the full workspace UUID; tools use the `accId`.
- The `buildLink` tool — turn `activeActor`/`activeLayer`/`activeGraph` into clickable URLs.
