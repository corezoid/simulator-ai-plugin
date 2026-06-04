# Simulator.Company MCP Operation Reference

Each Swagger operation is exposed as an MCP tool. The tool name is the operation's `operationId` from the bundled `sim-public-swagger.full.json` (e.g. `getForm`, `createActor`, `searchActors`, `createTransfer`). The `METHOD:/path` column below identifies the underlying REST endpoint so you can cross-reference it with the swagger spec; it is **not** the tool name.

Path parameters (e.g. `{accId}`, `{formId}`) become named string arguments. Request bodies are passed via a single `body` argument as a JSON string.

## Actor Operations

| Operation ID | Summary |
|---|---|
| `POST:/actors/actor/formId` | Create actor (formId = form template ID) |
| `GET:/actors/actorId` | Get actor by ID |
| `PUT:/actors/actor/formId/actorId` | Update actor by ID |
| `DELETE:/actors/actorId` | Remove actor by ID |
| `GET:/actors/ref/formId/ref` | Get actor by external ref |
| `PUT:/actors/actor/ref/formId/ref` | Update actor by ref |
| `DELETE:/actors/ref/formId/ref` | Remove actor by ref |
| `PUT:/actors/status/actorId` | Set actor status (active/removed) |
| `DELETE:/actors` | Remove multiple actors and links |
| `GET:/actors/system/accId/objType/objId` | Get actor by system object ID |

## Form Operations

| Operation ID | Summary |
|---|---|
| `POST:/forms/accId/isTemplate` | Create form (isTemplate=true for templates) |
| `GET:/forms/formId` | Get form by ID |
| `PUT:/forms/formId` | Update form |
| `DELETE:/forms/formId` | Remove form |
| `GET:/forms/templates/accId` | List all workspace forms |
| `GET:/forms/templates/system/accId?formTypes=system` | List system forms (built-in) |
| `GET:/forms/ref/ref` | Get form by ref string |
| `PUT:/forms/status/formId` | Set form status |
| `DELETE:/forms/item_cache/formId/itemId` | Clear item options cache |

## Link Operations

| Operation ID | Summary |
|---|---|
| `POST:/actors/link/accId` | Create link between two actors |
| `PUT:/actors/link/edgeId` | Update link properties |
| `DELETE:/actors/link/edgeId` | Remove link |
| `POST:/actors/exist_link` | Check if link exists |
| `POST:/actors/mass_links/accId` | Create multiple links at once |
| `DELETE:/actors/bulk/actors_link` | Mass remove links |
| `GET:/graph/actor_links/actorId` | Get all links of an actor |
| `GET:/graph/linked_actors/actorId` | Get actors linked to this actor (with link info) |
| `GET:/graph/type/actorId` | Get linked actors by type |
| `GET:/edge_types/accId` | List available link type definitions |

## Layer Operations

| Operation ID | Summary |
|---|---|
| `GET:/graph_layers/layerId` | Get layer details |
| `POST:/graph_layers/actors/layerId` | Add/remove actors on layer |
| `PUT:/graph_layers/actors/layerId` | Save actor positions on layer |
| `DELETE:/graph_layers/clean/layerId` | Remove all actors from layer |
| `POST:/graph_layers/exist/layerId` | Check if actors exist on layer |
| `POST:/graph_layers/move/sourceLayerId/targetLayerId` | Move actors to another layer |
| `GET:/layer_actors_filters/layerId/formId` | Get layer actors by form type |
| `GET:/layer_actors_filters/search/layerId/query` | Search actors on layer by text |
| `GET:/layers_links/actor_global/actorId` | Get all layers containing an actor |

## Account & Financial Operations

| Operation ID | Summary |
|---|---|
| `POST:/accounts/actorId` | Create account(s) for an actor |
| `GET:/accounts/actorId` | Get all accounts for an actor |
| `GET:/accounts/single/accountId` | Get single account by ID |
| `DELETE:/accounts/actorId/currencyId/nameId/accountType` | Remove specific account |
| `PUT:/accounts/amount/accountId` | Set account balance directly |
| `PUT:/accounts/block/actorId` | Block/unblock actor accounts |
| `POST:/accounts/formula/accountId` | Set formula for calculated account |
| `GET:/accounts/formula_info/accountId` | Get account formula |
| `GET:/accounts/bulk` | Get multiple accounts by IDs array |
| `GET:/accounts/children/actorId` | Get accounts of child actors |
| `GET:/accounts/ref/formId/ref` | Get accounts by actor ref |
| `POST:/accounts/ref/formId/ref` | Create accounts by actor ref |
| `GET:/account_names/accId` | List account name definitions |
| `POST:/account_names/accId` | Create account name definition |
| `GET:/currencies/accId` | List currencies in workspace |
| `POST:/currencies/accId` | Create currency |
| `POST:/accounts/pair/accId` | Create account name + currency pair |

## Transaction Operations

| Operation ID | Summary |
|---|---|
| `POST:/transactions/accountId` | Create transaction on account |
| `GET:/transactions/list/accountId` | List transactions by account |
| `GET:/transactions/actorId` | List transactions by actor |
| `GET:/transactions/actor_ref/formId/actorRef` | List transactions by actor ref |
| `GET:/transactions/ref/accountId/ref` | Get transaction by ref |
| `GET:/transactions/children/transactionId` | Get child transactions |
| `POST:/transactions/accountId/authorized` | Authorize (hold) transaction |
| `POST:/transactions/accountId/canceled` | Cancel authorized transaction |
| `POST:/transactions/accountId/completed` | Complete authorized transaction |
| `POST:/transactions/atom/accId` | Create atomic multi-account transactions |

## Transfer Operations

| Operation ID | Summary |
|---|---|
| `POST:/transfers/accId` | Create transfer between accounts |
| `POST:/transfers/accId/authorized` | Create transfer holding |
| `GET:/transfers/transferId` | Get transfer by ID |
| `POST:/transfers/filter/accId` | Filter/search transfers |

## Search & Filter Operations

| Operation ID | Summary |
|---|---|
| `GET:/actors_filters/search/accId/query` | Full-text search actors in workspace |
| `GET:/actors_filters/formId` | Filter actors by form type |

## Reaction Operations

| Operation ID | Summary |
|---|---|
| `POST:/reactions/type/actorId` | Create reaction (comment, approval, etc.) |
| `GET:/reactions/list/actorId` | Get all reactions on actor |
| `GET:/reactions/stats/actorId` | Get reaction statistics |
| `PUT:/reactions/rootActorId` | Update reaction |
| `DELETE:/reactions/actorId` | Remove reaction |
| `POST:/reactions/read/actorId` | Mark reactions as read |

## Attachment Operations

| Operation ID | Summary |
|---|---|
| `POST:/upload/accId` | Upload file |
| `POST:/upload/base64/accId` | Upload file as base64 |
| `POST:/attachments/accId` | Attach uploaded file to actor |
| `GET:/attachments/accId` | Get all attachments |
| `DELETE:/attachments/accId` | Remove attachment |
| `GET:/download/fileName` | Download file |
| `POST:/download/zip/accId` | Download files as zip |

## Counter Operations

| Operation ID | Summary |
|---|---|
| `POST:/counters/accId` | Create counter entries |
| `POST:/counters/list/accId` | Get list of counters |
| `POST:/counters/uniq_values/accId` | Get unique counter values |
| `POST:/counters/list/uniq_values/accId` | Get list of unique counter values |

## Access Control

| Operation ID | Summary |
|---|---|
| `GET:/access_rules/objType/objId` | Get access rules for object |
| `POST:/access_rules/objType/objId` | Set/update access rules |

## System Forms (key IDs to look up)

Use `GET:/forms/templates/system/accId?formTypes=system` to get system form IDs. Key system forms:
- **Graph** - Container for a business process graph
- **Layer** - Visual layer for organizing actors
- **Script/CDU** - Custom data unit / smart form template
- **Event** - Calendar/scheduling entity
- **Account** - Financial account definition
- **Currency** - Unit of value
- **Transaction** - Financial transaction record
- **Transfer** - Fund transfer between accounts
- **Reaction** - Comment/approval/interaction
- **Stream** - Real-time data stream
