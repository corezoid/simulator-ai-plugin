---
name: simulator-smart-forms-logic
description: >
  Author and evolve the Corezoid backend that powers a Simulator.Company Smart Form.
  Use whenever the user wants a Smart Form to do anything beyond a static layout —
  load dynamic viewModel data, react to submits, drive page-level changes, trigger
  notifications, write back to actors — OR wants to modify existing backend logic
  (add a button branch, a new page handler, change submit behaviour, fix a bug in
  the viewModel builder). This skill is an orchestrator: it prepares precise briefs
  and hands the actual process authoring/editing off to the Corezoid plugin's
  `corezoid-create` and `corezoid-edit` skills. Activate on: "add logic to a smart
  form", "wire smart form to corezoid", "/get /send process", "smart form backend",
  "viewModel from corezoid", "submit handler", "page initialization process", "back
  end for CDU", "corezoidCredentials", "procId", "bind process to smart form",
  "smart form callback", "callback_url", "edit smart form backend", "add a button
  to smart form", "change submit logic", "add a page to smart form backend", "fix
  viewModel builder", "додати логіку до смарт форми", "редагувати логіку смарт
  форми", "бекенд смарт форми", "процес для смарт форми", "обработчик /send",
  "логика смарт формы", "процесс инициализации страницы", "редактировать процесс
  смарт формы".
---

# Smart Form Backend Logic

You orchestrate the **Corezoid process(es)** that make a Simulator.Company Smart
Form interactive. Layout, i18n, styles, and viewModel defaults live in the Smart
Form actor itself (see the **`simulator-smart-forms`** skill). Everything *dynamic*
— initial viewModel, page transitions, submit handling, notifications, write-backs
to actors — runs in Corezoid.

**This skill does not author processes itself.** Instead it:

1. Confirms the Corezoid plugin is installed and its skills are available.
2. Translates the user's intent into precise **briefs** in the exact format
   `corezoid-create` / `corezoid-edit` expect (purpose · inputs · expected output
   · process type · concrete node skeleton).
3. Invokes the matching Corezoid skill with that brief and lets it run
   `create-process` / `push-process` / `lint-process` / `run-task`.
4. Provisions an API key (`create-api-key` for a new one, or `find-principal`
   for an existing `apiLogin`) and shares the bound process to it
   (`share-object`) so the Smart Form runtime can call it.
5. Finally binds the process to the Smart Form env (`createSmartForm` or
   `updateSmartFormEnv`) via the `simulator` MCP server.

---

## 0. Preflight — confirm the Corezoid plugin is installed

Before producing any brief, verify the Corezoid plugin is available in this
session:

1. Check whether the `create-process`, `push-process`, `create-alias`,
   `create-api-key`, `find-principal`, and `share-object` MCP tools are listed
   in the tool registry. If they are, the `corezoid` MCP server is running.
   (`create-api-key` / `find-principal` / `share-object` are needed by Step E
   below — provisioning the API key the Smart Form runtime authenticates with.)
2. Check whether the `corezoid-create`, `corezoid-edit`, and (optionally)
   `corezoid-access` skills are reachable (they appear in the available-skills
   list when installed).
3. If either is **missing**, stop and instruct the user (reply in the user's
   language):

   > "This workflow needs the Corezoid plugin. Install it from
   > [`github.com/corezoid/corezoid-ai-plugin`](https://github.com/corezoid/corezoid-ai-plugin)
   > and run `/corezoid-init` to authenticate, then ask me to continue."

Do not attempt to author `.conv.json` files yourself or call PAPI directly when
the Corezoid plugin is absent.

---

## 1. The runtime contract

The platform binds **one Corezoid process per env** to a Smart Form via the env's
`procId`. That bound process receives a POST whenever a page is rendered or a
form submission event fires. It must respond to `{{__callback_url}}` with the right shape:

| `path`  | Trigger                               | Required response shape                                    |
|---------|---------------------------------------|------------------------------------------------------------|
| `/get`  | A page is being opened or re-rendered | `{ "code": 200, "viewModel": { … } }`                      |
| `/send` | A button clicked **or** any element with `submitOnChange: true` changed | `{ "code": 200, "data": { "changes": [], "notifications": [] } }` |

**Two distinct `/send` triggers — always distinguish them:**

| Source | `body.buttonId` | `body.buttonData` |
|---|---|---|
| Button click | button's `id` | `{}` (empty object) |
| `submitOnChange` element | element's `id` | `{ "action": "select"/"check"/…, "value": "newValue" }` |

When an element has `submitOnChange: true` (e.g. a select, radio, or checkbox),
the platform fires `/send` **immediately on value change** without waiting for a
button. `body.buttonId` is the element's own `id` and `body.buttonData` carries the
interaction detail (`action` + `value`). `body.data` still contains the full current
snapshot of all field values.

Other status codes: `205` re-render whole page; `302` redirect to another page
(`data.nextPage`); `4xx`/`5xx` surfaces an error toast.

> **Topology is up to the developer.** The bound process may handle both paths
> itself in a single graph, or fan out to sub-processes (one per `path` / `page` /
> `buttonId`) via `api_copy`. The contract is the response shape, not the layout.
> Ask the user which approach fits the form's complexity before generating briefs.

### Sample `/get` request (incoming)

```jsonc
{
  "__callback_url": "https://cb-apigw.corezoid.com/callback/sync_api/…",
  "body": {
    "context": { "appId": "<smartFormActorId>", "rootActorId": "<actorId>",
                 "browser": "Chrome", "language": "en", "timeZoneOffset": -180 },
    "page": "index",
    "query": {}
  },
  "path": "/get",
  "sessionData": { "userInfo": { "id": 52731, "login": "user@x.com", "saId": 5501,
                                 "memberGroups": [79693] } }
}
```

### Sample `/send` request — button click

```jsonc
{
  "__callback_url": "https://cb-apigw.corezoid.com/callback/sync_api/…",
  "body": {
    "buttonId": "submit_btn",
    "buttonData": {},
    "context": { "actorId": "<actorId>", "rootActorId": "<actorId>", "appId": "<smartFormActorId>" },
    "data": { "day_comment": "…", "self_score": "4" },
    "formId": "<formId>",
    "page": "index",
    "sectionId": "<sectionId>",
    "query": {}
  },
  "path": "/send",
  "sessionData": { /* same as /get */ }
}
```

### Sample `/send` request — `submitOnChange` element

`buttonData` is non-empty: `action` describes the interaction type, `value` is the
newly selected value. `buttonId` is the **field's id**, not a button.

```jsonc
{
  "__callback_url": "https://cb-apigw.corezoid.com/callback/sync_api/…",
  "__headers": {},
  "body": {
    "buttonId": "project_name",
    "buttonData": {
      "action": "select",
      "value": "energy_efficiency"
    },
    "context": {
      "appId": "0118a16b-bf08-4e13-b3e7-0f97dfa8b6db",
      "browser": "Chrome",
      "language": "en",
      "timeZoneOffset": -180
    },
    "data": {
      "grant_category": "ecology",
      "project_desc": "",
      "project_name": "energy_efficiency"
    },
    "formId": "project_section",
    "page": "index",
    "sectionId": "project_body",
    "query": {}
  },
  "path": "/send",
  "sessionData": {}
}
```

### Sample `/get` callback (outgoing)

```json
{
  "code": 200,
  "viewModel": {
    "userName": "Alice",
    "submit_btn_visibility": "visible"
  }
}
```

### Sample `/send` callback

```json
{
  "code": 200,
  "data": {
    "changes": [
      { "id": "submit_btn",  "visibility": "hidden" },
      { "id": "day_comment", "visibility": "hidden" }
    ],
    "notifications": [
      { "title": "Thank you, we received your answer", "type": "success" }
    ]
  }
}
```

`changes[]` is a surgical patch — only listed component ids are touched.
`changeRules` (e.g. `{ "options": { "action": "replace" } }`) controls how arrays
merge. Full reference: `$PLUGIN_ROOT/docs/user-flows/cdu-page-protocol.md`.

### Realistic viewModel shape

Every key returned in `viewModel` must match a `{{placeholder}}` somewhere in
`pages/<page>/config` or in `viewModel` defaults. A representative payload:

```json
{
  "userName": "Mykhailo Sydoreiko",
  "metric_total_value": "245 min",
  "metric_total_sub": "4.08 hr · daily activity",
  "items_table_body": [
    { "value": "row_1",
      "options": [
        { "title": "[url=https://…]Some entry[/url]", "value": "item_name" },
        { "title": "16min", "value": "item_duration" }
      ] }
  ],
  "submit_btn_visibility": "hidden",
  "self_score_visibility": "disabled"
}
```

Adding a new metric to the page? Add the matching key here **and** the matching
`{{key}}` in `pages/<page>/config`.

---

## 2. Reusable node fragments

These are the contract-bound JSON shapes any brief should embed verbatim. They
are reusable building blocks — you decide how they compose into a single process
or several.

### 2.1 Condition on `path` (dispatch `/get` vs `/send`)

```jsonc
{
  "type": "go_if_const",
  "to_node_id": "<getBranchNodeId>",
  "conditions": [
    { "param": "path", "const": "/get", "fun": "eq", "cast": "string" }
  ]
}
```

A second `go_if_const` does the same for `/send`; the trailing `go` falls
through to a default branch.

### 2.2 Condition on `body.page` (dispatch per-page)

For a multi-page form, branch on `body.page` after dispatching by `path`:

```jsonc
{
  "type": "go_if_const",
  "to_node_id": "<indexBranchNodeId>",
  "conditions": [
    { "param": "body.page", "const": "index", "fun": "eq", "cast": "string" }
  ]
}
```

### 2.3 Condition on `body.buttonId` (dispatch per-event source for `/send`)

`body.buttonId` identifies the source of every `/send` event — both button clicks
and `submitOnChange` field changes. Use `go_if_const` on it to route each case:

```jsonc
{
  "type": "go_if_const",
  "to_node_id": "<submitBranchNodeId>",
  "conditions": [
    { "param": "body.buttonId", "const": "submit_btn", "fun": "eq", "cast": "string" }
  ]
}
```

### 2.3a Detect `submitOnChange` vs button click

When the same `/send` handler must behave differently depending on whether the
user clicked a button or changed a `submitOnChange` field, branch on
`body.buttonData.action` — it is present and non-empty only for field-change events:

```jsonc
{
  "type": "go_if_const",
  "to_node_id": "<fieldChangeBranchNodeId>",
  "conditions": [
    { "param": "body.buttonData.action", "const": "", "fun": "ne", "cast": "string" }
  ]
}
```

Alternatively, check the specific field id **and** action together:

```jsonc
{
  "type": "go_if_const",
  "to_node_id": "<projectNameChangedNodeId>",
  "conditions": [
    { "param": "body.buttonId",           "const": "project_name", "fun": "eq", "cast": "string" },
    { "param": "body.buttonData.action",  "const": "",             "fun": "ne", "cast": "string" }
  ]
}
```

Read the new value from `body.data.<fieldId>` (the full field snapshot) or from
`body.buttonData.value` (just the newly selected value). Use whichever is cleaner
for your logic; both contain the same information for single-select fields.

### 2.4 Fan-out to a sub-process (`api_copy`, fire-and-forget)

Use when you want the bound process to delegate work to a separate Corezoid
process. The sub-process becomes responsible for the callback.

```jsonc
{
  "type": "api_copy",
  "user_id": <yourUserId>,
  "conv_id": "@<aliased-sub-process>",
  "ref": "",
  "mode": "create",
  "group": "all",
  "data": {},
  "data_type": {},
  "err_node_id": "<errorNodeId>"
}
```

### 2.5 Build viewModel (Code Node, `api_code`)

JavaScript pattern for assembling `viewModel` from an actor lookup or other
sources:

```javascript
const viewModel = data?.viewModel || {};
const eventData = data?.event_actor?.data || {};

viewModel.userName          = eventData.user_name || "";
viewModel.metric_total_value = `${eventData.total_min ?? 0} min`;

if (eventData.status === "complete") {
  viewModel.submit_btn_visibility = "hidden";
  viewModel.self_score_visibility = "disabled";
} else {
  viewModel.submit_btn_visibility = "visible";
  viewModel.self_score_visibility = "visible";
}

data.viewModel = viewModel;
```

### 2.6 Build `changes` + `notifications` (Code Node)

```javascript
data.changes.push(
  { "id": "submit_btn",  "visibility": "hidden" },
  { "id": "day_comment", "visibility": "hidden" }
);
data.notifications.push({
  "title": "Thank you, we received your answer",
  "type": "success"
});
data.responseData = { changes: data.changes, notifications: data.notifications };
```

### 2.7 Final callback API node (`/get`)

```jsonc
{
  "type": "api",
  "method": "POST",
  "url": "{{__callback_url}}",
  "rfc_format": true,
  "content_type": "application/json",
  "extra":      { "code": "200", "viewModel": "{{viewModel}}" },
  "extra_type": { "code": "number", "viewModel": "object" },
  "extra_headers": { "content-type": "application/json; charset=utf-8" },
  "err_node_id": "<errorNodeId>",
  "version": 2
}
```

### 2.8 Final callback API node (`/send`)

Identical shape, with `data` in place of `viewModel`:

```jsonc
{
  "type": "api",
  "method": "POST",
  "url": "{{__callback_url}}",
  "rfc_format": true,
  "content_type": "application/json",
  "extra":      { "code": "200", "data": "{{responseData}}" },
  "extra_type": { "code": "number", "data": "object" },
  "extra_headers": { "content-type": "application/json; charset=utf-8" },
  "err_node_id": "<errorNodeId>",
  "version": 2
}
```

### 2.9 Complete process skeleton (bound, single-process topology)

The fragments above (§2.1–2.8) are pieces; this is **how they wire together** in
the most common topology — a single bound process that owns both `/get` and
`/send`. Use this as the default starting shape and only depart from it when the
form's complexity demands a multi-process layout (see §3).

> **Why this matters.** The single most common failure when writing a Smart-Form
> backend is forgetting that `/send` has **two** sources — a button click *and*
> any `submitOnChange` field change — and processing both as a real submit. The
> form then tries to persist a half-filled record on every field interaction. The
> skeleton below makes that second fork explicit.

#### 2.9.1 Flowchart

```
                          ┌───────┐
                          │ Start │
                          └───┬───┘
                              │
                  ┌───────────▼───────────┐
                  │ Condition on `path`   │   (§2.1)
                  └─┬───────────────────┬─┘
                    │                   │
               path=/get            path=/send
                    │                   │
         ┌──────────▼─────────┐  ┌──────▼──────────────────────┐
         │ Build viewModel    │  │ Extract                     │
         │ (api_code)  §2.5   │  │  data.buttonAction =        │
         └──────────┬─────────┘  │  body.buttonData.action     │
                    │            │  (api_code, safe ?? "")     │
                    │            └──────┬──────────────────────┘
                    │                   │
                    │       ┌───────────▼───────────────────┐
                    │       │ Condition on buttonAction     │   (§2.3a)
                    │       │  ""        ─→ real submit     │
                    │       │  "select"  ─→ submitOnChange  │
                    │       │  "check"   ─→ submitOnChange  │
                    │       │   …                           │
                    │       └─┬───────────────────────┬─────┘
                    │         │                       │
                    │    real submit           submitOnChange
                    │    (buttonId =           (lightweight
                    │     submit_btn,          ack: empty
                    │     dispatch §2.3)        changes[])
                    │         │                       │
                    │   ┌─────▼────────┐       ┌──────▼─────────┐
                    │   │ Persist /    │       │ Build empty    │
                    │   │ call CREATE  │       │ sendResponse   │
                    │   │ ACTOR / api  │       │ Data           │
                    │   │ (api_rpc /   │       │ (api_code §2.6)│
                    │   │  api §2.6)   │       └──────┬─────────┘
                    │   └──────┬───────┘              │
                    │          │ ok                   │
                    │   ┌──────▼─────────┐            │
                    │   │ Build success  │            │
                    │   │ sendResponse   │            │
                    │   │ Data (api_code)│            │
                    │   └──────┬─────────┘            │
                    │          │                      │
                    │          │   ┌── on err ────────┤
                    │          │   │                  │
                    │   ┌──────▼───▼─────┐            │
                    │   │ Build error    │            │
                    │   │ sendResponse   │            │
                    │   │ (notification) │            │
                    │   └──────┬─────────┘            │
                    │          │                      │
         ┌──────────▼───┐   ┌──▼──────────────────────▼──┐
         │ Callback     │   │ Callback to                │
         │ to           │   │ {{__callback_url}}         │
         │ {{__callback_│   │ POST { code:200,           │
         │   url}}      │   │        data:{              │
         │ POST { code: │   │          changes,          │
         │   200,       │   │          notifications     │
         │   viewModel} │   │        } }      §2.8       │
         │     §2.7     │   └────────────┬───────────────┘
         └────────┬─────┘                │
                  └─────────┬────────────┘
                            │
                       ┌────▼────┐
                       │ Success │
                       └─────────┘
```

#### 2.9.2 Node sequence

| #   | Node title                          | obj_type / logic    | Routes to (success / err)                          | See        |
|-----|-------------------------------------|---------------------|----------------------------------------------------|------------|
| 1   | Start                               | 1 / `go`            | → 2                                                | —          |
| 2   | Dispatch by `path`                  | 0 / `go_if_const`   | `/get`→3, `/send`→4, default→Error                 | §2.1       |
| 3   | Build viewModel (GET branch)        | 0 / `api_code`      | → 8 (Callback GET), err→Error                      | §2.5       |
| 4   | Extract `body.buttonData.action`    | 0 / `api_code`      | → 5, err→Error                                     | §2.5       |
| 5   | Dispatch by action                  | 0 / `go_if_const`   | `""`→6 (real submit), `select`/`check`/…→7 (ack)   | §2.3a      |
| 6   | Persist / call CREATE ACTOR / …     | 0 / `api_rpc` or `api` | → 6a (success), err→6b (error)                  | §2.4 / §2.6|
| 6a  | Build success `sendResponseData`    | 0 / `api_code`      | → 9 (Callback SEND)                                | §2.6       |
| 6b  | Build error `sendResponseData`      | 0 / `api_code`      | → 9 (Callback SEND)                                | §2.6       |
| 7   | Build empty-ack `sendResponseData`  | 0 / `api_code`      | → 9 (Callback SEND)                                | §2.6       |
| 8   | Callback GET (viewModel)            | 0 / `api`           | → Success, err→Error                               | §2.7       |
| 9   | Callback SEND (data)                | 0 / `api`           | → Success, err→Error                               | §2.8       |
| 10  | Success                             | 2 (terminal)        | —                                                  | —          |
| 11  | Error                               | 2 (terminal)        | —                                                  | —          |

#### 2.9.3 Key invariants

- **Two forks in `/send`, not one.** Fork-1 (path) separates GET from SEND. Fork-2
  (action) separates a real submit from a `submitOnChange` event. Skipping fork-2
  is the most common Smart-Form-backend bug: the platform fires `/send` *every
  time* a select/check/radio with `submitOnChange:true` changes value, and the
  process will try to persist a half-filled form.
- **Action dispatch rule of thumb.** `body.buttonData.action` is `""` (or
  absent / empty object) ⇔ real button click — branch further on `body.buttonId`
  (§2.3) to pick the persistence action. Non-empty `action` (`"select"`, `"check"`,
  …) ⇔ `submitOnChange` event — usually return an empty `changes:[]` ack, or a
  targeted cascade update if the change should drive other fields.
- **Single shared callback per path.** All `/send` branches (success, error,
  `submitOnChange` ack) converge on **one** `Callback SEND` node — they differ
  only in what they pre-fill into `data.sendResponseData`. Same for `/get`: one
  `Callback GET` reachable from every viewModel-building branch.
- **Response shape is decided by the path branch.** `/get` →
  `{ code: 200, viewModel: {…} }`. `/send` (every subtype) →
  `{ code: 200, data: { changes, notifications } }`. Mixing them silently breaks
  the form — the platform either ignores the payload or shows an error toast.

#### 2.9.4 When to expand this skeleton

- **Multi-page form.** Insert a third fork on `body.page` *between* the path
  dispatch and the GET/SEND handlers — one viewModel builder per page, one
  action dispatcher per page (§2.2).
- **Multiple submit buttons on one page.** Inside the "real submit" branch
  (node #6), fork on `body.buttonId` — one persistence path per button (§2.3).
- **Multiple `submitOnChange` fields with distinct logic.** Replace node #7's
  single ack with a `buttonId` fork — one cascade-update branch per field. Read
  the new value from `body.data.<fieldId>` or `body.buttonData.value` (§2.3a).
- **Heavy logic on either side.** Split the persistence node (#6) — or the whole
  GET branch — into a separate aliased sub-process called via `api_copy`. See §3
  for the multi-process topology.

---

## 3. Creating logic from scratch — brief handoff to `corezoid-create`

### Step A — agree on topology with the user

Ask (or infer from form complexity): does the user want **one process** handling
everything, or **multiple processes** fanned out via `api_copy`? Common shapes:

- **Single bound process** — Start → Condition on `path` → /get branch (build
  viewModel inline → callback API) and /send branch (Condition on `buttonId` →
  Code Node → callback API). Best for small forms with one page and one button.
  Remember: the /send branch must handle **both** button clicks and
  `submitOnChange` elements — branch on `body.buttonId` for each.
- **Bound process + per-path sub-processes** — bound process dispatches `/get`
  and `/send` to two aliased sub-processes via `api_copy`; each sub-process owns
  its own callback. Best when /get and /send logic differs sharply.
- **Bound process + per-page-per-path sub-processes** — adds a `body.page`
  dispatch. Best for multi-page forms.

When designing the `/send` topology, always ask: **which elements on each page
have `submitOnChange: true`?** Each such element is an independent event source
(its `body.buttonId` = the element's `id`; `body.buttonData.action` is non-empty).
If `submitOnChange` events only update UI state (cascade a select choice into
dependent fields) they usually return a lightweight `changes[]` with no persistence.
If they trigger saves or lookups they need their own sub-process or branch.

Whichever shape, the **bound process** (the one whose numeric id goes into
`corezoidCredentials.procId`) does not need an alias. Sub-processes called from
it via `api_copy` **do** need aliases.

### Step B — pick the target folder

Ask the user (or infer from `pull-folder` artifacts) which Corezoid folder should
host the process(es). A common convention is `<smart-form-ref>/` (a dedicated
folder per Smart Form), but it is not enforced.

### Step C — generate one brief per process

Use the template below, once per process you need. Then invoke
`corezoid-create` once per brief — the Corezoid skill will run `create-process`
→ fill the JSON → `lint-process` → `push-process`.

> **Process purpose:** <one paragraph — what task this process performs, who
> calls it, and what it must produce>
>
> **Input parameters:** (top-level fields available on `data`)
> - `path` (string) — `/get` or `/send`, only when this process is the bound one
> - `body` (object) — CDU payload; the inner fields you actually use, e.g.
>   `body.page`, `body.context.rootActorId`, `body.data.<field>`, `body.buttonId`
> - `sessionData` (object) — only if you read userInfo / memberGroups
> - `__callback_url` (string) — only if this process replies to the platform
> - <any other extra fields passed from a calling process>
>
> **Expected output:** <one of>
> - HTTP POST to `{{__callback_url}}` with body `{ code: 200, viewModel: { … } }`
>   (see §1 for shape), OR
> - HTTP POST to `{{__callback_url}}` with body `{ code: 200, data: { changes, notifications } }`, OR
> - No callback (fire-and-forget — for sub-processes whose caller already
>   answered the platform), OR
> - Reply to caller via `api_rpc_reply` (when this process is itself called via
>   `api_rpc` from another).
>
> **Process type:** Business logic (orchestrator) / API connector.
>
> **Folder path:** `<folder>`
>
> **Process name:** `<descriptive name>`
>
> **Alias to register after push:** `<short-name>` — required when other
> processes will call this one via `api_copy` or `api_rpc` using `@<short-name>`.
> Omit for the bound process (it is referenced by numeric `procId`).
>
> **Required nodes (in order, with title + logic type):**
> 1. Start (`obj_type: 1`, `go`)
> 2. <Set Param / Code Node / Condition / Call a Process / API node / Copy Task>
>    — purpose. Payload shape: see §2.<N>.
> 3. …
> N. Final (`obj_type: 2`) + error escalation nodes for every fallible step.
>
> **Specific node-by-node guidance:** reference §2 fragments by number and list
> the exact `extra` keys / Code Node JS the brief should embed. Do not leave the
> Code Node body to chance — write it out inside the brief.
>
> **Variables to create (if any):** list every constant (URL, alias, account id)
> that should land in `_ENV_VARS_.json` as `{{env_var[@…]}}`.

### Step D — invoke `corezoid-create`

For each brief, call:

```
Skill(skill="corezoid-create", args="<the brief above, verbatim>")
```

If your topology includes sub-processes, after each successful push register its
alias (the `corezoid-create` skill calls `create-alias` automatically if you
include the "Alias to register after push" line, but you can also call it
explicitly):

```
create-alias(short_name="<short-name>", process_id=<numericProcessId>)
```

Make sure aliases referenced by the bound process exist before traffic flows —
otherwise `api_copy` to a missing alias will fail.

### Step E — provision the API key that will call the bound process

The Smart Form runtime authenticates with Corezoid as an **API key**
(`apiLogin` + `apiSecret`). That key must have at least `create` privilege on
the bound process — otherwise every `/get` / `/send` fails at the platform edge
before reaching the process graph. Sub-processes called via `api_copy` /
`api_rpc` do **not** need to be shared to the key; the bound process runs them
under its own owner.

Ask the user which path they want:

- **(A) Create a fresh API key for this Smart Form** — best when there is no
  existing automation key for this form.
- **(B) Reuse an existing API key** — the user already has an `apiLogin` /
  `apiSecret` pair and wants to attach the bound process to it.

#### Path A — create the key, then share the process to it

1. `create-api-key(title="Smart Form <smart-form-ref>")` — the Corezoid plugin
   returns the key's numeric `obj_id` and writes the credentials to
   `~/.corezoid/api-keys/<slug>-<obj_id>.json` (mode 0600). Read that file to
   obtain `apiLogin` + `apiSecret`.
2. `share-object(obj="conv", obj_id=<boundProcessId>, obj_to="user", obj_to_id=<apiKeyObjId>, privs="create")`
   — grants the key permission to create tasks in the bound process. **API
   keys are addressed as `obj_to="user"`** (not `"api_key"`) on the share
   endpoint; that's how Corezoid models them internally.

#### Path B — find an existing key by `apiLogin`, then share the process to it

The user knows `apiLogin` + `apiSecret` but not the key's numeric `obj_id`.

1. `find-principal(name="<apiLogin>", kind="api_key")` — resolves the key by
   login and returns its `obj_id`. If multiple keys match (or none do), surface
   the candidates back to the user and ask which to use — do **not** guess.
2. `share-object(obj="conv", obj_id=<boundProcessId>, obj_to="user", obj_to_id=<apiKeyObjId>, privs="create")`
   — same call as Path A.

> **Higher-level alternative.** If the Corezoid plugin's `corezoid-access`
> skill is available in this session, delegate the find-principal → share-object
> step to it instead of calling the MCP tools directly — it wraps the same flow
> with sensible defaults.

After this step you hold a valid `apiLogin` + `apiSecret` pair that can call
the bound process. Carry them forward to Step F.

### Step F — bind the bound process to BOTH Smart Form envs

Every Smart Form has **two envs** (`develop` and `production`), and **each one
has its own independent Corezoid binding**. Setting credentials on one env does
**not** populate the other — without an explicit second call, `production`
stays empty and the live form 5xx's the moment it is opened.

Per env you need: `apiLogin`, `apiSecret`, `procId` (the **bound process's**
numeric id), and `companyId` (workspace identifier — a **string**, e.g. a UUID
`"4ddb8938-65f4-4f83-8208-7ac3faffe671"` or an `"i…"`-prefixed id like
`"i12412424"` — **not** a number; pass it quoted). Source `apiLogin`/`apiSecret`
from Step E, `companyId` from workspace settings.

Confirm with the user whether `develop` and `production` should hit the
**same** Corezoid stage (same `procId` + key) or **different** stages (separate
dev / prod processes, separate keys — each shared per Step E). Default
assumption when the user has not said otherwise: same stage for both, so the
same quadruple goes into both envs.

**Option A — at Smart Form creation (preferred when the form is new).**

The four flat fields are applied to **both** envs in one call:

```
createSmartForm(
  title="…", ref="<smart-form-ref>",
  apiLogin="…", apiSecret="…",
  procId="<boundProcessId>",
  companyId="<companyId>"
)
```

If `develop` and `production` should differ, pass the per-env shape via
`corezoidCredentials` (the flat fields are ignored when this is set):

```
createSmartForm(
  title="…", ref="<smart-form-ref>",
  corezoidCredentials='{
    "develop":    {"apiLogin":"…","apiSecret":"…","procId":"<devProcId>",  "companyId":"<companyId>"},
    "production": {"apiLogin":"…","apiSecret":"…","procId":"<prodProcId>", "companyId":"<companyId>"}
  }'
)
```

**Option B — update an existing Smart Form's bindings.**

`updateSmartFormEnv` writes **one env at a time**. Always call it **twice** —
once for `develop` and once for `production` — unless the user is intentionally
updating only one side:

```
updateSmartFormEnv(
  actorId="<actorId>",
  env="develop",
  apiLogin="…", apiSecret="…",
  procId="<boundProcessId>",
  companyId="<companyId>"
)

updateSmartFormEnv(
  actorId="<actorId>",
  env="production",
  apiLogin="…", apiSecret="…",
  procId="<boundProcessId>",
  companyId="<companyId>"
)
```

The tool resolves the env name to its numeric id internally. Updating `develop`
credentials does **not** create a release. `production` credentials are
updated independently — they are **never** copied from `develop` by a regular
`deploySmartForm` call (releases ship files, not env credentials).

### Step G — smoke-test

1. Open the Smart Form in the platform UI — a `/get` lands in the bound process
   and the page renders with the returned viewModel.
2. Click a submit control — `/send` lands in the bound process and `changes[]`
   mutates the page in place; notifications appear.
3. Inspect failed tasks in the Corezoid UI; use `run-task` (delegated through
   `corezoid-create` / `corezoid-edit`) to replay tasks with tweaked data.

---

## 4. Editing existing logic — brief handoff to `corezoid-edit`

Use this branch when the backend process(es) for a Smart Form **already exist**
and the user wants to modify behaviour (add a button branch, add a page handler,
extend the persisted payload, fix the viewModel builder, etc.).

### Step A — locate the process

The `corezoid-edit` skill's MANDATORY first step is to resolve `PROCESS_PATH`.
Hand it the identifier directly so it does not have to ask:

- If you know the file path locally, pass it.
- Otherwise pass the process name or alias (`@<short-name>`) — `corezoid-edit`
  will resolve it from the pulled tree.

If the form has multiple backing processes and you are not sure which one owns
the behaviour to change, inspect the bound process first (its numeric id is in
`corezoidCredentials.procId` for the relevant env) and follow `api_copy` edges
from there.

### Step B — classify the change

For each user-visible change, decide which logical piece needs editing:

| Change                                              | Where the change lands                                        |
|-----------------------------------------------------|---------------------------------------------------------------|
| New page added to the Smart Form UI                 | Bound process (new `body.page` branch) + new page handler     |
| New submit button on existing page                  | Branch that handles `/send` for that page — new `buttonId` arm |
| New `{{placeholder}}` in the page config            | Code Node that assembles `viewModel` for that page            |
| Persist additional submit fields                    | `/send` branch — extend the data payload sent to `@api-sim-update-actor` (or your equivalent) |
| Different post-submit `changes[]` (keep form editable, redirect, …) | Code Node that builds `data.changes` / `data.notifications` |
| Switch the source actor for viewModel               | `@api-sim-get-actor-by-id` (or equivalent) `extra`            |
| Add server-side validation before save              | Add Condition + Code Node + error `changes[]` ahead of persist |

For each landing spot, identify the *specific* process file (single bound
process or one of the sub-processes in your topology).

### Step C — produce an edit brief and invoke `corezoid-edit`

The `corezoid-edit` skill expects: process identifier, what to change, and the
exact node shapes for additions. Use this template:

> **Process:** `<path / name / alias>`
>
> **Goal:** <one-sentence statement of the user-visible change>
>
> **Nodes to add / remove / modify:**
> - <Action>: <Node title> (`<obj_type>` / `<logic type>`) — connected from
>   `<prevTitle>`, error → `<errNodeTitle>`. Payload:
>   ```jsonc
>   { /* fragment from §2 */ }
>   ```
> - …
>
> **Existing nodes to keep:** every node not mentioned above.
>
> **Callback contract reminder:** the API node that POSTs to `{{__callback_url}}`
> must keep `extra_type.code: "number"` and the viewModel/data payload typed as
> `object`.
>
> **After deploy:** push the process (`corezoid-edit` does this automatically in
> its Step 3) and ask me to re-test in the UI.

Hand the brief over with:

```
Skill(skill="corezoid-edit", args="<the brief above, verbatim>")
```

### Step D — propagate dependent edits

A change is rarely isolated:

- New `{{placeholder}}` in `pages/<page>/config` → matching key set in the
  viewModel-building Code Node.
- New `buttonId` in `pages/<page>/config` → new branch in the `/send` handler.
- New page in `pages/<id>/config` → new `body.page` branch in whatever process
  dispatches per page; possibly a new sub-process.

After editing logic, also drive `simulator-smart-forms` to push the matching UI
changes (`pushSmartForm`) so the contract stays consistent.

### Step E — re-bind credentials only if `procId` changed

If the edit replaces the bound process itself (rare — only when its numeric id
changes), repeat Steps E (share the new process to the API key — `share-object`
with the new `obj_id`) and F (re-bind via `updateSmartFormEnv`) from §3.
Editing nodes inside the bound process does **not** change its `procId` and
does **not** require re-sharing or re-binding.

---

## 5. Conventions, pitfalls, and rules

1. **`{{__callback_url}}` must be exact.** It is provided by the platform on every
   incoming task. Never reconstruct or hardcode it.
2. **Object payloads need `extra_type: "object"`.** `viewModel` and `data` in the
   final API node must be declared as object types so Corezoid serializes them as
   JSON, not stringified strings.
3. **Numeric `code`.** `extra.code = "200"`, `extra_type.code = "number"`. The
   Smart Form serve layer rejects responses where `code` arrives as a string.
4. **Every fallible node needs `err_node_id`.** Code Nodes, API Calls, Call a
   Process, Copy Task, Set Param all need an escalation target. Errors should
   POST `code: 500` (or 4xx for validation) back to `{{__callback_url}}` so the
   user sees a usable error notification instead of a hung page.
5. **Match `id` values to the page config.** Every `changes[].id` must match the
   `id` of a real item in `pages/<page>/config`. Unknown ids are silently dropped.
6. **`buttonId` is the source id for every `/send` event — not only buttons.**
   Both button clicks and `submitOnChange` field changes arrive as `/send` with
   `body.buttonId` set to the triggering element's `id`. Distinguish the two by
   `body.buttonData`: it is `{}` for button clicks and `{ action, value }` for
   field-change events (see §2.3a). Always check which elements on a page have
   `submitOnChange: true` before designing the `/send` dispatch tree.
7. **`memberGroups` for authz.** `sessionData.userInfo.memberGroups` is the source
   of truth for role-based logic — do not call simulator APIs to re-derive group
   membership.
8. **Aliases over numeric ids for sub-process calls.** Wherever the topology uses
   `api_copy` / `api_rpc`, reference the target by `@short-name`, not by numeric
   id — aliases survive stage moves; numeric ids do not.
9. **`api_copy` is fire-and-forget.** When you fan out via `api_copy`, the parent
   exits immediately and the child owns the callback. Use `api_rpc` instead if
   the parent needs the result.
10. **Develop vs production.** A Smart Form release deploys *files only*. The env
    credentials (`procId` etc.) are **not** part of the release snapshot — they
    are set per env via the binding endpoint. When promoting, update both
    bindings explicitly if dev and prod should hit different Corezoid stages.
11. **The API key must be shared to the bound process.** `apiLogin` /
    `apiSecret` alone are not enough — the platform calls the process *as*
    that key, and Corezoid will reject the call unless the key has at least
    `create` privilege on the conv. Symptom of a missing share: every `/get`
    or `/send` errors at the edge before reaching node 1. Use `share-object`
    (or the `corezoid-access` skill) to grant it; see §3 Step E.

---

## 6. Decision tree: when to use this skill vs. neighbours

| User wants…                                            | Skill                                |
|--------------------------------------------------------|--------------------------------------|
| Build / edit page layout, locale, styles, viewModel    | `simulator-smart-forms`              |
| Add interactivity or modify backend logic              | **this skill** → delegates to corezoid-create / corezoid-edit |
| Generic Corezoid process authoring (no Smart Form)     | `corezoid-create` directly           |
| Modify any Corezoid process file directly              | `corezoid-edit` directly             |
| Manage aliases (rename, list, link)                    | `corezoid-alias-manager`             |
| Set up workspace credentials and OAuth                 | `simulator-init` / `corezoid-init`   |

---

## 7. Reference documents

| Path | When to read |
|---|---|
| `$PLUGIN_ROOT/docs/user-flows/smart-forms.md` | Smart Form lifecycle, env binding endpoints, release model |
| `$PLUGIN_ROOT/docs/user-flows/cdu-page-protocol.md` | Component catalogue, `changes[]` shape, change protocol |
| `$PLUGIN_ROOT/skills/simulator-smart-forms/SKILL.md` | UI-authoring side of the same workflow |
| Corezoid plugin: `corezoid-create` SKILL | New-process brief format and expected steps |
| Corezoid plugin: `corezoid-edit` SKILL | Edit brief format and mandatory push step |
| Corezoid plugin: `docs/node-structures.md` | Exact JSON schemas for `api_copy`, `api_rpc`, `api`, `api_code`, `set_param`, `go_if_const` |
