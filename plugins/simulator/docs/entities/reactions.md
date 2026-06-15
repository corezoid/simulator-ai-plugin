# Reactions

Reactions in the Simulator.Company platform enable users to interact with actors through comments, approvals, ratings, and other feedback mechanisms.

## Overview

Reactions provide a way for users to collaborate and interact with actors in the system. They are implemented as specialized actors organized in a hierarchical tree structure, allowing for threaded comments, nested replies, and structured feedback.

## Properties

Reactions are stored as actors with the following properties:

| Property | Type | Description |
|----------|------|-------------|
| id | String | Unique identifier for the reaction |
| acc_id | String | Workspace ID the reaction belongs to |
| user_id | Integer | ID of the user who created the reaction |
| form_id | Integer | ID of the reaction form template (system form) |
| title | Text | Display title of the reaction |
| data | JSON | Reaction data (comment text, rating value, etc.) |
| created_at | Integer | Unix timestamp of creation time |
| updated_at | Integer | Unix timestamp of last update |

## Reaction Tree Structure

Reactions are organized in a hierarchical tree structure using the ActorsTreeEdges model:

| Property | Type | Description |
|----------|------|-------------|
| id | String | Unique identifier for the tree edge |
| root_actor_id | String | ID of the root actor (the actor being reacted to) |
| actor_id | String | ID of the reaction actor |
| parent_id | String | ID of the parent reaction (for nested replies) |
| branch_id | String | ID of the branch (for organizing reactions) |
| edge_type_id | Integer | ID of the edge type (defining the reaction type) |
| level | Integer | Nesting level in the reaction tree |
| path | Text | Path string representing the position in the tree |
| extra | JSON | Presentation/flags: `commentStyleType`, `linkedActorId`, `layerPosition`, and `mcp` (see [AI Reaction Agent](#ai-reaction-agent-mcp)) |
| reasoning | JSON | AI-agent trace on `ai` reactions: `{ inProgress, thoughts:[{id,text,createdAt}] }` (see below) |
| created_at | Integer | Unix timestamp of creation time |
| updated_at | Integer | Unix timestamp of last update |

## Reaction Types

The platform supports various reaction types:

- **Comments** - Text-based feedback or notes
- **Approvals** - Acceptance or rejection decisions
- **Ratings** - Numerical or star-based evaluations
- **Reactions** - Emoji-based quick reactions (like, love, etc.)
- **Mentions** - References to other users
- **Tasks** - Assigned work items
- **AI (`ai`)** - The AI agent's reply, produced by the platform's agent in
  response to a reaction flagged `extra.mcp` (see [AI Reaction Agent](#ai-reaction-agent-mcp))

## AI Reaction Agent (MCP)

Any reaction created with **`extra.mcp = true`** is handed to the platform's AI
agent. The agent is a Claude run (hosted by the claude-code-api gateway) that uses
**this MCP server** to read and act on the platform — so it operates strictly under
the **requesting user's** access rules (PAPI authorizes every call with a delegated,
short-lived token minted for that user). Its answer is posted back as a child
**`ai`** reaction under the same actor.

### Interaction scheme

```
User (or automation) creates a reaction with extra.mcp=true on actor A
        │
        ▼
Platform dispatches the AI agent for actor A   (per-actor lock → turns are sequential)
        │   • runs as the requesting user (delegated Simulator token)
        │   • the agent calls THIS MCP server (scoped to that user's access)
        ▼
Agent reads/acts via MCP tools (actors, forms, graph, finance, reactions, …)
        │
        ▼
Platform posts a child `ai` reaction and streams the result:
        • description   ← the answer text (streamed live, then finalized)
        • reasoning.inProgress = true while running, false when done
        • reasoning.thoughts  ← step log (e.g. which tools were used)
```

### Fields

| Field | Where | Meaning |
|-------|-------|---------|
| `extra.mcp` | on the reaction you create | `true` ⇒ hand this reaction to the AI agent |
| `ai` (reaction type) | the agent's reply | child reaction holding the agent's answer |
| `reasoning.inProgress` | on the `ai` reply | `true` while the answer is still being produced |
| `reasoning.thoughts` | on the `ai` reply | `[{id,text,createdAt}]` — the agent's step log |
| `description` | on the `ai` reply | the answer text (partial while `inProgress`) |

### Notes

- **One shared thread per actor.** Turns on a given actor are serialized; the agent
  keeps context across turns, so a follow-up `extra.mcp` reaction continues the same
  discussion.
- **Reading mid-stream.** An `ai` reaction with `reasoning.inProgress = true` is still
  being written — treat its `description` as partial.
- **Avoid loops.** Set `extra.mcp` only on genuine (human) requests, not on the agent's
  own output. The platform bounds runaway chains, but it still wastes turns.
- **UI context.** When the agent runs, the client passes a `control-events-context` object
  (where the user is in the UI — `activeActor` / `activeLayer` / `activeGraph` / `page` / …),
  injected into the agent's prompt. See [`ui-context.md`](ui-context.md).

## Embedding: smart forms, nested actor cards, chips

A reaction (and an actor) can embed richer content than plain text:

- **Run a Smart Form (script) in a reaction.** Set **`appId`** to a Smart Form
  (CDU/Script application) actor id when creating/updating the reaction — the reaction then
  renders and runs that smart form. **`appSettings`** configures it:
  `{autorun, expired (unix s), users:int[], groups:int[], fullWidth}` (or null).
  ```
  createReaction(type="comment", actorId="<actor>", appId="<smartFormActorId>",
                 appSettings={ "autorun": true, "fullWidth": true })
  ```
  The **same `appId` / `appSettings`** fields exist on **actors** (`createActor` /
  `updateActor`) — that's how a regular actor embeds/runs a smart form in its card.
  Create the smart form with `createSmartForm` (see the `simulator-smart-forms` skill).
- **Nested actor card.** `extra.linkedActorId` on a reaction embeds **another actor as a
  card/preview** inside the reaction:
  `createReaction(type="comment", actorId="<actor>", extra={ "linkedActorId": "<otherActorId>" })`.
- **Inline chips & formatting (in `description` / comment text).** Both reaction bodies and
  actor `description`s render **BBCode** — chips and rich formatting:
  - Chips: `[actor=<id>]Label[/actor]` (nested actor card), `[application=<smartFormId>]Label[/application]`
    (smart-form chip), plus `[event=…]`, `[graph=…]`, `[graphLayer=…]`, `[user=…]`, `[chat=…]`,
    `[file=…]`, `[quote=…]`, `[smarttag=…]`.
  - Formatting: `[b]` `[i]` `[u]`, `[h1]`..`[h6]`, `[color=…]` `[size=…]` `[bg=…]` `[span …]`,
    `[ul][*]…[/ul]` / `[ol][*]…[/ol]`, `[url=href]…[/url]`, `[img=<id>]` / `[imgSrc=…]`, `[br]`,
    and `[md]…[/md]` for a markdown block.
  - **The exact tag set is per-environment** — fetch it with the **`getBbcodeTags`** tool
    (reads `<web-host>/bbcode-tags.json`: each tag's attributes + an example) before composing
    a rich description.
  - **BBCode is processed only OUTSIDE `[md]` blocks.** Inside `[md]…[/md]` the content is
    markdown, so a chip/BBCode placed there is **not** rendered — keep chips/BBCode outside the
    `[md]` section (use one or the other in a given span).

## API Endpoints

For detailed API documentation on reactions, including request parameters, response formats, and authentication requirements, please refer to the official API documentation:

[Reactions API Documentation](https://doc.simulator.company/#tag/reactions)

The API provides endpoints for:

- Getting all reactions for a specific actor
- Creating new reactions
- Retrieving specific reaction details
- Updating existing reactions
- Deleting reactions
- Replying to existing reactions
- Getting reaction statistics

All API requests require appropriate OAuth2 scopes (`control.events:actors.readonly` for read operations and `control.events:actors.management` for write operations).

## Database Structure

Reactions use multiple tables to implement their functionality:

- Reactions are stored as actors in the `actors` table
- The hierarchical structure is stored in the `actors_tree_edges` table
- Reaction types are defined as edge types in the `edges_types` table

## Example

### Comment Reaction

```json
{
  "id": "reaction_123456",
  "title": "Comment",
  "data": {
    "text": "This looks good, but we should consider adding more details to the customer profile.",
    "mentions": ["user_789"]
  },
  "user_id": 42,
  "created_at": 1621459200,
  "updated_at": 1621459200
}
```

### Reaction Tree

```json
{
  "root_actor_id": "actor_789012",
  "reactions": [
    {
      "id": "reaction_123456",
      "title": "Comment",
      "data": {
        "text": "This looks good, but we should consider adding more details."
      },
      "user_id": 42,
      "created_at": 1621459200,
      "replies": [
        {
          "id": "reaction_234567",
          "title": "Reply",
          "data": {
            "text": "I agree, I'll update the profile with additional information."
          },
          "user_id": 56,
          "created_at": 1621545600
        }
      ]
    },
    {
      "id": "reaction_345678",
      "title": "Approval",
      "data": {
        "status": "approved",
        "notes": "Approved with minor changes requested."
      },
      "user_id": 78,
      "created_at": 1621632000
    }
  ]
}
```

## Real-time Updates

Reactions support real-time updates through:

- WebSocket notifications for new reactions
- Real-time updates when reactions are modified
- Notification system for mentions and replies

## Usage in the Platform

Reactions are used throughout the platform for various purposes:

- **Collaboration** - Enabling team discussions on business processes
- **Approvals** - Supporting approval workflows and sign-offs
- **Feedback** - Collecting user feedback on processes and documents
- **Task Management** - Assigning and tracking tasks related to actors
- **Notifications** - Alerting users to important updates and mentions

Reactions form a key part of the platform's collaboration features, enabling users to interact with business processes and with each other in a structured way.
