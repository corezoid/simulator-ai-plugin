# App Generation — from Corezoid processes to a Smart Form

Deep reference for the **`simulator-app-generator`** skill. The skill's `SKILL.md` carries
the pipeline; this document carries the parts that are too long to keep in front of the
model at all times: the contract-extraction algorithm, the annotated middleware skeleton,
and the test-payload catalogue.

Everything below was derived from a real 12-process Corezoid API (a retail loyalty backend:
authorization + bonus balance, three-step card registration, transaction history, promo
list, cashback categories, FAQ, store locator, complaint submission, callback request,
password recovery).

---

## 1. The contract-extraction algorithm

### 1.1 Why `params` is not enough

A `.conv.json` declares its interface at the root:

```jsonc
"params": [
  {"name":"phone","type":"string","flags":["required","input"],"descr":"phone",
   "regex":"","regex_error_text":""},
  {"name":"bonusAmount","type":"string","flags":["output"],"descr":"code :200",
   "regex":"","regex_error_text":""}
]
```

`flags` carries the direction (`input` / `output`) and `required` orthogonally.

Measured drift across the 12 reference processes:

| Drift | Count |
|---|---|
| Declares **zero** outputs but replies with data | 5 of 12 |
| Declares an output no reply node ever sets | 1 |
| Reply emits keys absent from `params` | common |

Conclusion: **`params` is a hint. The reply nodes are the truth.**

### 1.2 Walking the graph

Nodes live at `scheme.nodes[]`. Each has `condition.logics[]`, an array that holds the
node's action logic plus its routing `go`. So:

```
for node in doc.scheme.nodes:
    for logic in node.condition.logics:
        if logic.type == "api_rpc_reply": → an output branch
        if logic.type == "api_rpc":       → a downstream call (inputs live in .extra)
        if logic.type == "set_param":     → produced values (.extra keys)
        if logic.type == "api_code":      → produced values (parse .src)
```

`node.obj_type`: `1` Start, `2` Final, `0` normal, `3` escalation/error. `node.extra` is a
UI-chrome **string** (`"{\"modeForm\":\"collapse\",\"icon\":\"\"}"`) — never data.

Note the Start node carries **no** parameter information; it is always a bare `go`.

### 1.3 Reply-node shape

```jsonc
{
  "type": "api_rpc_reply",
  "mode": "key_value",
  "res_data":      {"code":"200","result":"success","transactionList":"{{transactionList}}"},
  "res_data_type": {"code":"string","result":"string","transactionList":"array"},
  "throw_exception": false
}
```

- `mode`: `"key_value"` (object) in all 91 reply nodes across the wider 63-process backend this
  set belongs to; a `"keys"` array form exists in the schema — handle it defensively.
- **Every `res_data` value is a string.** A literal `[]`, `{}`, `0` or `true` passes schema
  validation but hangs the server-side commit, so authors always route arrays through a
  `{{var}}`. When generating replies yourself, obey this.
- `throw_exception: true` (plus `exception_reason`) routes the **caller** to its
  `err_node_id`. Rare — 3 of those 91.
- **A reply value can be a literal JSON *string* rather than a `{{var}}`** — a whole array
  serialized inline. That makes the process a constant source: its output does not depend on the
  task at all, so `run-task` returns empty task data and tells you nothing. Read the schema off the
  literal in `res_data`, not off a probe.
- **Corezoid translation refs inside such a literal resolve to the EMPTY STRING, not to
  themselves.** A reply carrying `"[{\"text\":\"{{t'LoyaltyAnswers}}\",\"url\":\"…\"}]"` arrives as
  four entries whose `text` is `""` — the `t'` ref is substituted with nothing, and the literal
  `{{t'…}}` never reaches you. This matters because the obvious defence is wrong: a regex looking
  for `t'([A-Za-z]+)` in the value can never match, so a per-key label map keyed on the ref is dead
  code. Fall back on a **sibling field that survives** — in the reference FAQ the `url` was intact
  and distinct per row, so the labels were recovered from it. Check for empty strings in a
  constant reply before you design around its contents.

### 1.4 Grouping replies into outcomes

```
success   := result == "success"  or  code starts with "2"
error     := result == "error"
alternate := everything else (recovery, registration, …)
```

The **union of the success set's keys, minus `code` and `result`, is the output schema.**

Worked example — process 1760349 "Authorization + Bonus amount" has 8 reply nodes:

| code | result | payload keys |
|---|---|---|
| 200 | success | `QR`, `bonusAmount`, `cardCode`, `token` |
| 200 | success | (duplicate branch, same keys) |
| 402 | registration | — |
| 401 | recovery | — |
| 403 | error | — |
| 403 | error | — |
| 403 | error | — |
| — | error | — |

So: outputs = `{QR, bonusAmount, cardCode, token}`; **and two alternate outcomes that are
entire user journeys** — `402` must route the user to the registration wizard, `401` to
password recovery. A reading that only looks at the 200 branch loses both.

### 1.5 Type conflicts

`res_data_type` disagrees between sibling nodes in the same process. Observed: `code` typed
`"number"` in one node and `"string"` in five; `bonusAmount` typed `"number"` in one success
node and `"string"` in its duplicate. **Majority-vote; never treat this as an error.**

Type frequency over those same 91 nodes: `string` 159, `array` 9, `object` 4, `number` 2 — the
surface is overwhelmingly strings, so prefer string handling and cast where a number is genuinely
needed.

### 1.6 Recovering array element shape

`res_data_type: "array"` tells you nothing about elements, but you need them to build a
`table`. In priority order:

**Tier 1 — the reply node's `description`.** Authors frequently paste an example response:

```
{ "result": "success", "code": "200",
  "transactionList": [ { "date": "29.10.2025 10:59", "location": "м. Златопіль, Єдності, 7",
    "cardCode": "5550001785598", "type": "Продажа", "discount": "105.95", "amount": "429.00",
    "payForm": "Нал", "skuList": [ { "id": 0, "price": 4.5, "quantity": 1, … } ] } ] }
```

That single field yields the full column list for the history table. **Availability: 18 of the
36 reply nodes in the 12-process set** — about half. Parse leniently: the JSON is often preceded
by prose or whitespace, so scan for the first `{` and attempt a parse, rather than requiring
the description to start with one.

**Tier 2 — trace the `{{var}}` upstream** to the `api_code` / `set_param` / `api_rpc` that
produced it.

> **Watch for `__conveyor_api_array__`.** When an upstream HTTP API replies with a *bare* JSON
> array, Corezoid cannot bind it to an object response and wraps it: `body` becomes
> `{"__conveyor_api_array__": [ … ]}`. A process that replies `{"items": "{{body}}"}` therefore
> hands you a wrapper, not a list, and the array you want is one level down. Unwrap defensively
> and treat "not an array" as an error rather than mapping it — the same endpoint may answer
> `{"data": null, "error": "…"}` on a bad token, and blindly mapping an object's keys produces
> convincing rows of garbage.

**Tier 3 — probe with `run-task`** (user-gated; see §3).

If all three fail, record `elementShape: "unknown"` and render with a `contentLoop` of
generic label rows rather than inventing columns.

### 1.7 Recovering inputs

Real inputs = declared `input` params **∪** placeholders consumed before being produced. The
richest source is `api_rpc.extra`:

```jsonc
{"type":"api_rpc","conv_id":1760254,"group":"",
 "extra":{"lat":"{{location_lat}}","lon":"{{location_lon}}","radius":"200000"},
 "extra_type":{"lat":"string","lon":"string","radius":"string"},
 "err_node_id":"…","user_id":81558}
```

`{{location_lat}}` / `{{location_lon}}` are inputs; `"200000"` is a constant.

#### `extra` is only one of three payload carriers — check all three

A scan that reads `extra` alone will report processes as input-free when they are not. **This is the
single most likely way to produce a confidently wrong contract**, so make the scan cover:

| Node | Payload lives in | Trap |
|---|---|---|
| `api_rpc` | `extra` / `extra_type` | — |
| `api_copy` | **`data` / `data_type`** | it has **no** `extra` field at all. Reading `extra` yields `null` and the call looks payload-less |
| `api` with `format: "raw"` | **`raw_body`** (a JSON string) | `extra` / `extra_type` are `{}`. The real inputs are template placeholders *inside the string* |

```jsonc
// api_copy — the payload is in `data`, and `extra` does not exist
{"type":"api_copy","conv_id":1760199,"mode":"create","group":"","ref":"{{root.ref}}",
 "data":{"htmlText":"{{type}} : {{comment}} у {{city}}","subject":"Обращение клиента"},
 "data_type":{"htmlText":"string","subject":"string"},"err_node_id":"…"}

// api with format:"raw" — extra is empty; raw_body carries token/cardCode
{"type":"api","method":"POST","url":"{{ApiURL}}/chatbot/getTransactions","format":"raw",
 "extra":{},"extra_type":{},
 "raw_body":"{\n  \"token\": \"{{token}}\",\n  \"cardCode\": \"{{cardCode}}\",\n  \"locId\": 0\n}"}
```

So: extract placeholders from `extra` **∪** `data` **∪** `raw_body` (and `set_param.extra`), then
subtract whatever is produced upstream. Measured on the reference set, `raw_body` was the *only*
source of the real inputs for two of the twelve API-connector processes, and the `api_copy` `data`
map was the only place the outbound email body was assembled — a scan that missed it reported a
non-existent "sends an empty email" defect in a perfectly correct process.

#### Reference forms that are NOT task inputs

- `{{proc[<convId>].ref[<key>].<field>}}` — a read from a **state diagram**, e.g.
  `{{proc[1768467].ref[SalesCatalog].promos}}`. Data, not input.
- Nested interpolation: `{{proc[1769197].ref[Cities].{{cityCode}}}}` — the *inner*
  `{{cityCode}}` **is** an input.
- `{{conv[<convId>].ref[<key>].<field>}}` — same thing in the other syntax. Watch for it
  supplying something that *looks* per-user: in the reference set every API connector resolved its
  `sessionId` from `{{conv[1760289].ref[SessionData].Session}}`, i.e. **one workspace-wide session**,
  not a per-caller token. Two processes declared an `authToken` / session input that no node ever
  reads because of it — see §1.7a.

### 1.7a A declared input can be dead

§1.1 covers `params` drifting on the **output** side. The mirror case is just as common and more
expensive, because it makes you design a form field for data the backend never consumes:

> A process declares `{"name":"authToken","flags":["input"]}`, and **no node references
> `{{authToken}}`** — the value it stands for actually arrives from a state-diagram read inside the
> callee.

Before you put an input on a page, grep the process (and, when the call is `extra:{}` / `group:"all"`,
the callee too) for the placeholder. If it appears nowhere, the input is dead: drop it from the
manifest and record *why*, because the declared param will keep suggesting otherwise. In the
reference set `Registration : 1 step` declared `authToken`, consumed nothing, and was in truth
**input-free** — it allocates a fresh card code against a shared backend session.

### 1.8 Call-target forms

`api_rpc.conv_id` is either a **numeric id** (`1760254`) or an **alias string**
(`"@getcashback"`, `"@setclientfields"`). Aliases are always lowercase and survive stage
moves; numeric ids do not. When the middleware you generate calls a domain process, prefer
whichever form the process set already uses consistently.

**Naming, when you create one:** `create-alias` accepts **only `a-z`, `0-9` and `-`**, under 127
characters. An underscore is rejected outright — `chudo_get` fails with *"Only letters (a-z),
numbers (0-9), and dashes are allowed for short name"*. Since middleware sub-process names tend to
be written with underscores, pick the dashed form up front (`chudo-get`, `chudo-send`) and use that
same string in the caller's `conv_id` (`"@chudo-get"`).

---

## 2. Side-effect classification

Static classification is **unreliable**. It gates two decisions and nothing else: whether to
probe (§3), and whether the process may be reached from a page's `/get` (§2.2).

In the reference set, `Send complain` and `Send calback mailing` look inert at the API layer
— their only outbound node is `api_copy` to process 1760199 — but that process sends email
via eSputnik. `Cashback Categories` fans out to Telegram and Viber senders. None of this is
visible without following the call graph into another folder.

| Signal | Weight |
|---|---|
| `api_copy` to anything | strong — fire-and-forget usually means "go do something" |
| `api` / `api_rpc` whose **URL path or callee name** carries a mutating verb (`create`, `set`, `send`, `register`, `update`, `delete`, `add`, `pay`, `issue`) | strong |
| Title/description verbs: send, mail, notify, register, recovery, complain, create, push | strong |
| An outbound call whose reply is **discarded** — nothing downstream reads its `body` | strong: the process called it *for the effect* |
| `api` node with a non-GET method **and nothing else** | **weak — do not classify on this alone** |
| `api_rpc` to an unknown process | unknown — could be either |
| No outbound calls at all (FAQ, Promo List) | genuinely inert |

> **`POST` is not a side-effect signal in this codebase.** These backends POST to read: the
> reference `Transactions history` fetches its rows with
> `api POST {{ApiURL}}/chatbot/getTransactions` and mutates nothing. Weighting "non-GET" as strong
> marks essentially every reader `likely`, which then collides with §2.2 and bans content pages from
> `/get` — so the rule gets ignored wholesale, including where it matters. Read the **path and the
> payload**, not the method: `getTransactions` reading `token`/`cardCode` is a read;
> `setClientFields` or `sendMail` is not.

Classify `likely | unlikely | unknown` and **always ask before probing**.

### 2.1 Score only the nodes reachable from Start

**`scheme.nodes[]` is a bag, not a graph — much of it can be dead.** Corezoid never prunes
orphaned nodes, so a process that used to be a chatbot flow keeps every sender it ever had. Run the
heuristics above over the **reachable** subgraph or you will classify safe processes as dangerous:

```
reachable = BFS from the obj_type:1 Start node, following
              logic.to_node_id  (go / go_if_const)
            + logic.err_node_id
            + condition.semaphors[].to_node_id
```

In the reference set `Cashback Categories` has **126 nodes, of which 6 are reachable**: Start →
`api_rpc @getcashback` → reply. The other 120 — every Telegram and Viber `api_copy`, all the
`api_callback` parking nodes — are unreachable leftovers. Scored over the whole bag it is
`likely` and gets excluded from probing; scored over the reachable set it is `unlikely` and probes
cleanly. That one distinction was the difference between resolving its output shape and having to
guess it.

The same walk pays for itself twice more:

- **Orphaned reply nodes inflate the outcome set.** `Authorization + Bonus amount` carries two
  identical `200/success` reply nodes; only one is reachable. Deriving outcomes (§1.4) from
  unreachable replies invents branches the process cannot take.
- **Unreachable nodes are where stale contracts hide.** If a payload key only ever appears in a
  dead node, it is not part of the contract no matter how plausible it looks.

Report the reachable/total count in the manifest (`"nodes": "6 of 126 reachable"`). It is the
cheapest signal there is that a process is not what its node count suggests — and if the count is
lopsided, say so to the user: it usually means the process was repurposed and its declared `params`
describe the *old* job.

### 2.2 `/get` is repeated probing — keep side effects off it

The classification is usually read as a probing gate and then forgotten at design time. That
loses its second, larger use: **`/get` fires on every page open, re-render and shared link,
unattended.** Putting a process there is not one probe, it is an unbounded number of them.

> A process classified `likely` or `unknown` may only be reached from a `/send` button.
> Only `unlikely` belongs on a `/get`.

The trap is the wizard step that reads like a getter. `Registration : 1 step - Get fields`
declares one dead input, calls what looks like a GET, and was classified `unknown` — and it
**allocates a real card number** from a shared backend session on every call. On the `reg`
page's `/get` it minted a live card each time the page was rendered, test renders included.

Design around it with a start button: `<verb>_start_btn` makes the call on `/send` and returns
the allocated values as `changes[]`; `/get` renders step 1 statically. Same three steps, no
mutation on sight.

---

## 3. Probing with `run-task`

`run-task(process_path, data, wait_sec)` starts a task on the **deployed** process and polls
until it reaches a final node, so it returns the real reply — including real array elements.
It crosses async nodes (`api`, `api_rpc`, `db_call`, delays) within `wait_sec`; on timeout it
reports the node the task is parked at plus a TaskRef for `list-task-history`.

Rules:

- Only probe processes the **user explicitly approved**.
- Use obviously-fake input data (`380000000000`, `test`), never a real customer identifier.
- Probe read-shaped processes first; they resolve most unknown shapes.
- Record the observed reply in the manifest so it doesn't need re-probing.

---

## 4. The middleware skeleton

This section describes **what to put in the brief**, not what to type into a file. The
middleware process is always authored by the Corezoid plugin's skill:

```
Skill(skill="corezoid:corezoid-create", args="<brief>")   # new process
Skill(skill="corezoid:corezoid-edit",   args="<brief>")   # any later change
```

That skill owns `create-process`, node authoring, `layout-process`, `lint-process` and
`push-process`. Hand-writing or hand-patching the `.conv.json` bypasses the lint gate and
the canonical node-id reassignment that happens on push.

### 4.1 Node table

**Read this as a spine plus a template, not as a node list to copy once.** The spine exists exactly
once per process; the branch template is instantiated **once per page** (`/get`) and **once per
button** (`/send`) — callback node and error targets included. Instantiating it once and letting
every branch converge on a shared callback is the single most common way to earn
`SHARED ERROR CLUSTERS` from `lint-process`; see the three shapes below.

**Spine — one of each, for the whole process:**

| # | Node | obj_type | logic | Routes to |
|---|---|---|---|---|
| 1 | Start | 1 | `go` | 2 |
| 2 | Dispatch by `path` | 0 | `go_if_const` | `/get`→3, `/send`→10, default→ its own Error final |
| 3 | Dispatch by `body.page` (GET) | 0 | `go_if_const` | one **GET branch** per page |
| 10 | Extract `body.buttonId` | 0 | `api_code` | 11, err→ its own Error final |
| 11 | Dispatch by `buttonId` | 0 | `go_if_const` | one **SEND branch** per button / `submitOnChange` id; default→ the nav lookup Code node of §4.1a |
| S | Success | 2 | — | — |

**GET branch — instantiate once per page.** Every `G*` below is a *fresh* node in each instance:

| # | Node | obj_type | logic | Routes to |
|---|---|---|---|---|
| G1 | Call domain process | 0 | `api_rpc` | G2, err→ own target |
| G2 | Namespace results — **only if this branch calls 2+ domain processes** (§4.2) | 0 | `api_code` | G3, err→ own target |
| G3 | Branch on `result` / `<ns>_result` | 0 | `go_if_const` | G4 / G5 / G6 |
| G4 · G5 · G6 | Build viewModel — success · alternate · error | 0 | `api_code` | G7, err→ own target |
| G7 | **Callback GET** — this branch's own `api` node | 0 | `api` | S, err→ **this branch's own** Error final |
| GE… | This branch's error targets (escalation and/or Error final, per the shapes below) | 3 / 2 | — | — |

**SEND branch — instantiate once per button.** A `submitOnChange` id or a `nav_*` button skips
D1–D3 and goes straight to its builder:

| # | Node | obj_type | logic | Routes to |
|---|---|---|---|---|
| D1 | Call domain process | 0 | `api_rpc` | D2, err→ own target |
| D2 | Namespace results — same condition as G2 | 0 | `api_code` | D3, err→ own target |
| D3 | Map the outcome — one Code node, or a `go_if_const` tree (§4.1a weighs both) | 0 | `api_code` / `go_if_const` | D4 |
| D4 | Build `responseData` + `respCode` (200 / 205 / 302) | 0 | `api_code` | D5, err→ own target |
| D5 | **Callback SEND** — this branch's own `api` node, `extra.code` templated as `{{respCode}}` (§4.4) | 0 | `api` | S, err→ **this branch's own** Error final |
| DE… | This branch's error targets | 3 / 2 | — | — |

The Success final `S` **is** shared by every branch — it is a plain terminal with no logic, and
nothing lints it. Only *error* terminals must stay per-branch.

Every fallible node gets its **own** error target — never one shared with a neighbour. Two shapes,
picked by whether the error path does any work:

```
[fallible node] --err--> [obj_type: 2 Error final]                       ← nothing to do: stop
[fallible node] --err--> [obj_type: 3 escalation] --go--> [back to the flow]
                                    └--err--> [obj_type: 2 Error final]  ← its own terminal
```

Three things about that second shape are worth stating outright, because each one cost a lint round
in practice:

- **Do not invent an escalation when there is nothing for it to do.** An `obj_type: 3` node holding
  only a `go` is the *passthrough escalation* anti-pattern (`lint-process` flags it), and giving it
  a token `set_param` to look busy earns `UNUSED SET_PARAM` **and** `SHARED ERROR CLUSTERS` — the
  terminal is then fed by both the escalation and the node it protects. Point `err_node_id` straight
  at the final instead. In a Smart-Form handler the node this bites is always the **callback `api`
  node**: if the callback itself fails there is genuinely nothing left to do.
- **An escalation's `go` may rejoin the happy path.** That is what makes "every branch still answers
  the runtime" achievable: the escalation builds a degraded `viewModel` and continues into the
  branch's callback node. Only the *error terminals* must stay unshared — rejoining the main flow is
  ordinary routing, not a shared cluster.
- **Give each branch its own callback node.** Nine page branches converging on one shared callback
  means that callback's error target is fed by nine escalations — a shared cluster by construction.
  Duplicating the (identical) callback `api` node per branch is the shape that both lints clean and
  reads linearly; the node count is worth it.

Each `api_rpc` also needs a `time` semaphor. **30 sec is the server minimum** — lint rejects a
lower value outright, so 30 s is simultaneously the floor on how fast a page can fail when a callee
hangs:

```jsonc
"semaphors": [{"type":"time","value":30,"dimension":"sec","to_node_id":"<timeout_node>"}]
```

### 4.1a Four patterns that keep the graph small

> **Budget business nodes, not error nodes.** The threshold below is about the graph you *designed*.
> Error clusters scale mechanically with the number of fallible nodes — `layout-process` measured
> **32%** of both handlers in the reference build as error nodes (77 nodes → ~52 business, 74 → ~50).
> Counting them against the budget makes you split a graph that is actually well inside it. Count
> Start, dispatches, domain calls, builders and callbacks; ignore escalations and Error finals.

**Delegating to sub-processes.** When the single graph outgrows the threshold (roughly 60 nodes
or 8 pages), split it — but note who owns the callback. The router calls a sub-process with
`api_copy` (fire-and-forget), so the **sub-process** receives a full copy of the task data,
including `{{__callback_url}}`, and answers the runtime itself. The router then has nothing left
to do and goes to a plain success final:

```
Router:  … dispatch → api_copy(@sub) → [Final: "Delegated To Sub-Process"]
Sub:     Start → … → Callback 200/302 → [Final: "Response delivered"]
```

Because the sub answers directly, it has **no** `api_rpc_reply` anywhere and its finals are
reachable without one — which is correct here and is why `lint-process` does not complain
(it only flags that shape in a process that replies elsewhere).

**One Code node for all navigation.** A `nav_*` button needs no domain call, only a redirect. Do
not give each one its own dispatch arm and node chain; route the `buttonId` dispatch's
**default** branch into a single Code node holding a lookup table:

```javascript
var b = (data.body && data.body.buttonId) || "";
var d = (data.body && data.body.data) || {};
var map = { nav_home_btn: "home", nav_history_btn: "history", nav_index_btn: "index" };
var target = map[b] || "";
if (!target) {
  data.navFound = "no";                       // unrouted id: acknowledge, never navigate
  data.responseData = { changes: [], notifications: [] };
} else {
  data.navFound = "yes";
  data.responseData = { nextPage: target, query: { token: d.__token || "" } };
}
```

Then one condition on `navFound` picks the `302` or the `200` callback. On a nine-page app this
replaced about 24 nodes with one, and it also fixes the `simulator-app-generator` §7.5 failure
mode structurally: an
unrouted `submitOnChange` id lands on a **no-op ack** instead of falling through to a submit
path and jumping the user a page.

**Mapping an outcome in one Code node, not a condition tree.** After a domain `api_rpc`, the
reply's `result` / `code` has to become a 200 patch, a 302 redirect, or an error toast. The obvious
shape is a `go_if_const` tree: a condition node plus one builder per outcome, each builder carrying
its own error cluster. One `api_code` does all three:

```javascript
var res  = String(data.result || "");
var code = String(data.code   || "");
if (res === "success") {
  data.respCode = "302";
  data.responseData = { nextPage: "home", query: { token: String(data.token || "") } };
} else if (res === "registration" || code === "402") {
  data.respCode = "302";
  data.responseData = { nextPage: "reg", query: {} };
} else {
  data.respCode = "200";
  data.responseData = {
    changes: [{ id: "index_msg", class: "label", value: "Wrong phone or card password." }],
    notifications: [{ title: "Could not sign in", type: "error" }]
  };
}
```

On the reference 7-button `/send` handler that is 7 mapper nodes (plus 7 error clusters) instead of
7 condition nodes plus ~20 builders and *their* clusters. Two things to weigh, and they pull in
opposite directions:

- **It buys correctness.** JS reads `data.body.buttonId` directly, so there is no bracketing rule to
  get wrong. A `go_if_const` on a nested path must be written `"param": "{{body.buttonId}}"` —
  unbracketed it **silently never matches**, the condition falls through to its default, and the task
  completes "successfully" having skipped the domain call. That failure has no error and no warning
  (see `simulator-smart-forms-logic` §2.1).
- **It costs visibility.** The branching vanishes from the Corezoid canvas: a human debugging in the
  UI sees one opaque node instead of labelled arms. So keep the **dispatches** (`path`, `body.page`,
  `body.buttonId`) as real condition nodes — they are what makes the graph legible as an app — and
  use a Code node only for the outcome mapping *inside* a branch. State which you did in the brief.

**Carrying a session across pages.** Hidden carrier fields survive a submit but not a
navigation. A `302` response takes `{nextPage, target?, close?, query?}` — and that `query`
arrives at the next page's `/get` as `body.query`, which is the mechanism that carries state across
a navigation.

> ⚠️ **The `query` is the page URL. Treat everything you put in it as public.** It lands in browser
> history, in the `Referer` header of every outbound link and image, in proxy and access logs, and —
> the one that actually bites — in any link the user copies and shares, which hands the recipient the
> session. So:
>
> - **Never** put a credential in it: a password, a payment token, or anything that authenticates
>   the bearer on its own.
> - **A domain session token is in that class by default.** Do not decide per app whether "this
>   token is sensitive"; assume it is. Keep the session in a state process (or an actor) keyed by an
>   opaque, short-lived id, and carry only that id — plus non-secret state like `page`, a filter, or
>   a display-only `cardCode`.
> - If the backend you were given leaves no alternative — the domain processes take a bearer token
>   and there is nowhere to park it — **say so to the user and get a decision** before shipping it in
>   a URL. Do not make that trade silently.

The reference build carried `{token, cardCode}` in the query because its domain processes took a
bearer token and nothing else; that is the constrained shape, not the recommended one. Where you
must use it, every page's `/get` reads `body.query.*` into its viewModel and each `nav_*` redirect
passes the same query along.

**Read the session from `body.data` OR `body.query` — never `body.data` alone.** A submit sends
exactly one form, so a hidden carrier reaches the handler only when it lives in the *same form* as
the button that was clicked. Nav buttons almost always sit in their own `appbar` form, which has no
value-bearing items, so `body.data` arrives as `{}` and a handler reading only `body.data.__token`
writes an **empty** session into its own `302 query` — logging the user out on their first click.
`body.query` is sent on `/send` as well, so one fallback in the `/send` prep node immunises every
button at once:

```javascript
var d = (data.body && data.body.data)  || {};
var q = (data.body && data.body.query) || {};
data.sess_token = String(d.__token    || q.token    || "");
data.sess_card  = String(d.__cardCode || q.cardCode || "");
```

This is invisible to every automated check — the push validates, the page renders, and only the
navigation loses state — so prefer the fallback to auditing which form each button landed in.

### 4.2 Namespacing after every call

```jsonc
{"type":"set_param",
 "extra":{"auth_result":"{{result}}","auth_code":"{{code}}","auth_token":"{{token}}",
          "auth_cardCode":"{{cardCode}}","result":"","code":""},
 "extra_type":{"auth_result":"string","auth_code":"string","auth_token":"string",
               "auth_cardCode":"string","result":"string","code":"string"},
 "err_node_id":"<id>"}
```

Without this, a second `api_rpc` in the same task overwrites `result`/`code` and every
downstream condition reads the wrong verdict. This is the single most likely silent bug in a
generated middleware.

The converse matters too, because the ceremony is not free: **a branch that calls exactly one domain
process has nothing to collide with.** Read `{{result}}` / `{{code}}` / the payload keys straight in
the builder Code node and skip the `set_param` — on a 9-page `/get` router where every branch makes a
single call, applying the rule unconditionally adds a `set_param` *plus its own error cluster* per
branch, roughly 18 nodes of pure ceremony. Decide per branch, and note the decision in the brief so
the next editor adds it the moment a second call appears.

**When a Code node is the consumer, write the node as `api_code`, not `set_param`.** `lint-process`
resolves a `set_param`'s outputs by scanning downstream *node configuration* — it does not parse
JavaScript, so a `data.result` read inside an `api_code` is invisible to it and the whole node is
reported dead:

```
=== UNUSED SET_PARAM ===
  Degrade · history
  Issue: set_param sets [result transactionList] but no downstream node references them
```

This is not a lint quirk you work around — it is the collision between the two recommendations on
this page. The canonical example above is consumed by a `go_if_const` (`Branch on {{auth_result}}`),
which lint reads fine. But §4.1a tells you to map outcomes in **one Code node instead of a condition
tree**, and the moment you follow both, every namespacing/clearing `set_param` feeds a Code node and
lint fails the file. Author those nodes as `api_code` doing the same assignments (`data.auth_result =
data.result;` / `data.result = "";`) and both recommendations hold at once.

### 4.3 Building a table body

A `table` binds as `"body": "{{history_tx_body}}"`. A row is **not** an object keyed by column
id — it carries its own `value` (the row id) and puts its cells in `options[]`:

```javascript
const rows = data.tx_transactionList || [];
data.viewModel = data.viewModel || {};
data.viewModel.history_tx_body = rows.map(function (r, i) {
  return {
    value: "row_" + i,                       // the ROW's id
    options: [                               // the row's CELLS
      { value: "date",     title: r.date || "" },
      { value: "location", title: r.location || "" },
      { value: "type",     title: r.type || "" },
      { value: "amount",   title: (r.amount || "0") + " \u20b4" },
      { value: "discount", title: r.discount || "\u2014" }
    ]
  };
});
// A label value may never be the empty string, so "nothing to say" is a non-breaking space.
data.viewModel.history_tx_empty = rows.length ? "\u00a0" : "No purchases found.";
```

The page's `head[]` keys the same columns by **`value`**, not `id`:

```jsonc
"head": [{ "value": "date", "title": "[[date]]" }, … ]
```

Both mistakes — cells as direct row properties, and `head[].id` — pass the server and fail
only in the browser (`"body[0].date" is not allowed`, `"head[0].id" is not allowed`). See
`cdu-page-protocol.md` §5.1 for the full shape and §10.1 for why the server does not catch it.

**Empty states are driven by text, not by visibility.** A table bound to `[]` renders as a
bare header and looks broken, so pair every list with a `label` whose value you set — and
note the two constraints that force this design:

- `visibility` **cannot** be a `{{placeholder}}`; it is validated against the literal enum at
  push time, so a page cannot vary an item's visibility on `/get` at all. Only `changes[]` on
  `/send` can.
- a `label` value may **never** be an empty string, so "nothing to show" is a non-breaking
  space (`\u00a0`), not `""`.

Emitting `*_visibility` keys into the viewModel therefore does nothing on `/get`. It is worth
being blunt about the cost: an app generated that way accumulates dead keys that read as
though dynamic `/get` visibility exists — one real 9-page build finished with 25 of them.

### 4.4 Callback nodes

```jsonc
// GET
{"type":"api","method":"POST","url":"{{__callback_url}}","rfc_format":true,
 "content_type":"application/json",
 "extra":{"code":"200","viewModel":"{{viewModel}}"},
 "extra_type":{"code":"number","viewModel":"object"},
 "extra_headers":{"content-type":"application/json; charset=utf-8"},
 "response":{"header":"{{header}}","body":"{{body}}"},
 "response_type":{"header":"object","body":"object"},
 "customize_response":false,
 "format":"","send_sys":true,"debug_info":false,"cert_pem":"","max_threads":5,
 "is_migrate":true,"err_node_id":"<id>","version":2}

// SEND
 "extra":{"code":"200","data":"{{responseData}}"},
 "extra_type":{"code":"number","data":"object"}
```

**The five fields authors drop, and what dropping them costs.** `format`, `send_sys`,
`debug_info`, `cert_pem` and `max_threads` look like noise; they are not. `max_threads` is a
**JSON-schema required** property of the `api` logic, so a file without it fails
`lint-process`'s schema gate outright (`missing property 'max_threads'`). The other four trip the
`UNDERSPECIFIED API CALL NODES` check, which exists precisely because the failure is otherwise
undiagnosable: the server commit hangs ~15–20 s, then answers `no response from server`.
`response` / `response_type` are inert while `customize_response` is `false`, but ship them —
they are part of the canonical shape and a later flip of `customize_response` needs them.

**`extra.code` may be templated — one callback node per path is enough.** Writing
`"extra":{"code":"{{respCode}}","data":"{{responseData}}"}` with `extra_type.code:"number"`
casts a `"302"` string to `302` correctly (verified live). Each `/send` branch then sets
`data.respCode` (`200` / `205` / `302`) next to `data.responseData` and they all converge on one
node, instead of a separate callback per response code.

`customize_response:false` is mandatory: cb-apigw acks with an empty non-JSON body, and
response casting then throws `api_wrong_convert_param: "Param: body, Value: , Try convert to:
object"` even though the payload was delivered.

For a `302`, set `extra.code` to `"302"` and `responseData` to `{"nextPage":"<pageId>"}`.

---

## 5. Test-payload catalogue

### 5.1 `/get`, one per page

```jsonc
{"path":"/get",
 "body":{"page":"index","query":{},
         "context":{"appId":"<smartFormActorId>","language":"en","timeZoneOffset":0}},
 "sessionData":{"userInfo":{"id":1,"login":"test@example.com"}}}
```

### 5.2 `/send`, button click

```jsonc
{"path":"/send",
 "body":{"page":"index","formId":"login","sectionId":"body",
         "buttonId":"login_btn","buttonData":{},
         "data":{"phone":"380000000000","cardPassword":"0000","__token":""},
         "query":{},"context":{"appId":"<smartFormActorId>"}},
 "sessionData":{}}
```

### 5.3 `/send`, `submitOnChange` field

A `select` populates `buttonData`; everything else sends `{}`. Test **both** forms:

```jsonc
// select
{"path":"/send","body":{"page":"form","buttonId":"city","buttonData":{"action":"select","value":"kyiv"},
 "data":{"city":"kyiv"},"formId":"f","sectionId":"s","query":{}},"sessionData":{}}

// radio / check / toggle / edit — buttonData is EMPTY, exactly like a button click
{"path":"/send","body":{"page":"form","buttonId":"channel","buttonData":{},
 "data":{"channel":"sms"},"formId":"f","sectionId":"s","query":{}},"sessionData":{}}
```

The second form is the one that catches the "wizard jumps a step" bug: if `channel` is not
enumerated in the `buttonId` dispatch, it falls through to the submit path.

### 5.4 Interpreting a synthetic run

A `run-task` payload has no real `{{__callback_url}}`, so the callback `api` node **will
fail**. That is expected, not a defect. Assert on the state *at* the callback node via
`list-task-history`:

- the task took the branch matching `path` / `page` / `buttonId`;
- the namespaced keys (`auth_result`, …) are populated;
- `viewModel` / `responseData` contains every key the page config references.

Report this expected failure explicitly so a reader doesn't mistake it for a real one.

### 5.5 L3 assertions via `appGetPage` / `appSendForm`

| Assertion | Catches |
|---|---|
| No literal `{{` in the returned config | missing viewModel key, failed backend call, typo'd placeholder |
| No component value that resolved to `""` | a `label`/`image` the renderer will reject outright |
| Every designed item id is present | page config drift |
| `required` matches the design | validation gaps |
| `appSendForm` returns the designed `code` | wrong branch, wrong response shape |
| `changes[].id` all match real item ids | silently-dropped patches |
| Alternate-outcome pages reachable | happy-path-only coverage |

`appGetPage` returns the **server-resolved** config (locale and viewModel expanded) but does
**not** compile CSS or expand BBCode — visual results still need a human.

> ⚠️ **`appGetPage` is not a validity check, and "no literal `{{`" is not the strong signal it
> looks like.** It proves templating resolved and the backend answered; it does **not** run the
> client validator. Two whole classes of defect sail straight through it:
>
> - **a value that resolved to `""`** — a `label` or `image` with an empty value is rejected by
>   the renderer, and an empty string contains no `{{`, so the check passes;
> - **an unknown key** — `head[].id`, `image.extra.height`, a typo'd `visibilty`. No component
>   schema sets `additionalProperties: false`, so the server stores and serves it happily while
>   `control-cdu` rejects it (`cdu-page-protocol.md` §10.1).
>
> Both surface only as browser console errors. So L3 needs a third leg beside `appGetPage` and
> `appSendForm`: **validate the page files against the swagger before pushing.** `pushSmartForm`
> does this (`mcp-server/internal/cduschema`) — treat its `validationErrors` as the real gate and
> `appGetPage` as the behavioural check on top. Budget for one browser pass regardless; that is
> the only thing that exercises the renderer.

---

## 6. Related documents

| Path | Contents |
|---|---|
| `cdu-page-protocol.md` | Component catalogue, templating, `changes[]`, response codes |
| `smart-forms.md` | Smart Form lifecycle, env binding, releases, serving |
| `cdu-dom-tree-reference.md` | Rendered DOM map, for styling hooks |
| `app-catalog.md` | Discovering an existing app to run |
