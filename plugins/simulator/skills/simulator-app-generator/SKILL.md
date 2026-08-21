---
name: simulator-app-generator
description: >
  Generates a complete, tested multi-page web application as a Simulator.Company Smart Form
  from a set of EXISTING Corezoid processes plus a description of the desired product. Use
  when the user has a working Corezoid backend (a folder or list of process ids) and wants it
  turned into a usable web app / site / portal / mini-app — not when they want to edit one
  page or one process. This skill owns the end-to-end pipeline: pull every process, derive
  its real input/output contract from its `api_rpc_reply` nodes, design a page map that uses
  ALL of the processes, build the Smart Form, generate the Corezoid middleware process that
  bridges the form to those processes via `api_rpc`, bind it, then verify the whole thing
  end-to-end and self-repair. It delegates page authoring to `simulator-smart-forms`, backend
  briefs to `simulator-smart-forms-logic`, styling to `simulator-styles`, and page-driving to
  `simulator-smart-forms-runtime`. Activate on: "build an app from these processes", "make a
  web app out of these corezoid processes", "generate a site from process ids", "turn this
  API into a smart form", "build a portal on top of these processes", "app from corezoid
  processes", "generate smart form from processes", "middleware process for a smart form",
  "згенеруй застосунок з цих процесів", "зроби сайт на основі процесів корезоїд", "побудуй
  веб-додаток з процесів", "згенеруй смартформу з процесів", "сгенерируй приложение из этих
  процессов", "сделай сайт на базе процессов корезоид", "построй веб-приложение из процессов",
  "сгенерируй смарт форму из процессов", "мидлвара для смарт формы".
---

# Simulator.Company App Generator

You turn **a set of existing Corezoid processes + a product description** into a
**deployed, tested, multi-page Smart Form**.

You are an **orchestrator**. You own three things nobody else does:

1. **Contract extraction** — deriving what each process really consumes and produces.
3. **App design** — turning N process contracts into a coherent page map that uses them all.
4. **Verification** — proving the result actually works, and repairing it when it doesn't.

Everything else you delegate:

| Concern | Delegate to |
|---|---|
| Page layout, locale, viewModel, push/deploy | `simulator-smart-forms` |
| The Corezoid backend contract and brief format | `simulator-smart-forms-logic` |
| Authoring / editing the actual process JSON | `corezoid:corezoid-create` / `corezoid:corezoid-edit` |
| Less/CSS theme | `simulator-styles` |
| Driving pages during the E2E test | `simulator-smart-forms-runtime` |

Reply to the user in **their own language**; keep this document's logic intact.

---

## 0. Preflight

Confirm before doing anything else:

1. **Corezoid plugin present** — you need **both** halves:
   - the MCP tools `pull-process`, `push-process`, `lint-process`, `layout-process`,
     `create-process`, `run-task`, `create-api-key`, `find-principal`, `share-object`;
   - the skills **`corezoid:corezoid-create`** and **`corezoid:corezoid-edit`** in the
     available-skills list — they author the middleware in §7, and there is no fallback.

   If either is missing, stop and tell the user to install
   [`github.com/corezoid/corezoid-ai-plugin`](https://github.com/corezoid/corezoid-ai-plugin)
   and run `/corezoid-init`.
3. **Simulator authenticated** — a workspace is selected (`/simulator-init` otherwise).
4. **You know the `companyId`** (Corezoid workspace id, a string — a UUID or an `i…`-prefixed
   id) — needed at binding time. Ask if unknown.

Do **not** hand-author `.conv.json` files yourself when the Corezoid plugin is absent.

---

## 1. What this skill accepts

| Input | Form |
|---|---|
| **Process ids** (primary) | a list of numeric Corezoid process ids |
| Folder id | one Corezoid folder id — pull it and use every process inside |
| Local paths | already-pulled `<ID>_<Title>.conv.json` files |
| **Product description** (required) | free text: who the users are, what the app is for |

If the description is missing or vague, ask **once** for: who uses the app, what they should
be able to do, and whether it needs a login. Do not start designing without it.

---

## 2. Phase 1 — Acquire

Run `pull-process(process_id=<id>)` for every id. It writes
`<ID>_<Title>.conv.json` into a directory mirroring the process's location in Corezoid
(resolved from `parent_id`).

> **Never rename these files.** The `<ID>_` prefix is load-bearing — `push-process` and
> `lint-process` recover the process id from it and fail with a format error otherwise.

Record the local path for each process; you will re-read them throughout.

---

## 3. Phase 2 — Contract extraction

This is the analytical heart of the skill. Produce one **contract manifest**
(`app-contract.json`) describing every process. Work from the file, not from assumptions.

### 3.1 The trap: declared `params` drift

Each `.conv.json` has a root `params[]`:

```jsonc
"params": [
  {"name":"phone",       "type":"string", "flags":["required","input"], "descr":"phone"},
  {"name":"bonusAmount", "type":"string", "flags":["output"],           "descr":"code :200"}
]
```

This is the **declared** contract and it **drifts**. In the reference 12-process set, five
processes declare *zero* outputs yet clearly reply with data, and one declares an output no
reply node ever sets. Treat `params` as a **hint and a cross-check, never the source of
truth.**

### 3.2 Outputs — derive them from `api_rpc_reply` nodes

Walk `scheme.nodes[].condition.logics[]` and collect every logic with
`type == "api_rpc_reply"`:

```jsonc
{
  "type": "api_rpc_reply",
  "mode": "key_value",
  "res_data":      {"code":"200","result":"success","transactionList":"{{transactionList}}"},
  "res_data_type": {"code":"string","result":"string","transactionList":"array"},
  "throw_exception": false
}
```

- `mode` is `"key_value"` in practice (a `"keys"` array form exists — handle it if seen).
- **Every value in `res_data` is a string**, even when `res_data_type` says `array`/`object`.
  A `"{{var}}"` value means "produced upstream"; a bare literal is a constant.

### 3.3 The envelope convention — group replies into three sets

Every process in the reference set replies `{result, code, ...payload}` with
`result ∈ success | error | recovery | registration` and `code ∈ 200 | 400 | 401 | 402 | 403`.
Group the reply nodes:

| Set | Match | Meaning for the app |
|---|---|---|
| **success** | `result:"success"` or 2xx `code` | The union of its payload keys (minus `code`/`result`) **is the real output schema** |
| **alternate** | any other non-error `result` (`recovery`, `registration`, …) | A **control-flow outcome**, not a failure → becomes a `302` navigation edge |
| **error** | `result:"error"` | Boilerplate → becomes an error notification |

> **Alternate outcomes are the most commonly missed signal.** The reference auth process has
> 8 reply nodes: `200/success` (with `token`, `cardCode`, `bonusAmount`, `QR`),
> `402/registration`, `401/recovery`, and several `403/error`. A design that only reads the
> 200 branch silently loses the entire registration and password-recovery flows.

### 3.4 Types

Take types from `res_data_type`. They are **inconsistent between sibling reply nodes** in the
same process (`code` typed `"number"` in one node and `"string"` in five others; `bonusAmount`
typed both ways). **Majority-vote** and move on; do not treat a conflict as an error.

### 3.5 Array element shape

To render an array as a `table` you need its *element* structure, which `res_data_type: "array"`
does not give you. Three tiers, in order:

1. **Reply-node `description`** — often holds a pretty-printed example response, including
   nested arrays. This is the richest static source. Measured availability in the reference
   set: **18 of 36** reply nodes — roughly half, so treat it as opportunistic.
3. **Upstream trace** — follow the `{{var}}` back to the `api_code` / `set_param` / `api_rpc`
   that produced it; a Code node often reveals the shape.
4. **Live probe** — Phase 2b below.

If all three fail, record `"elementShape": "unknown"` and design that page with a
`contentLoop` of labels driven by whatever keys appear at runtime, or ask the user.

### 3.6 Inputs

Real inputs = `params[flags ∋ input]` **∪** `{{placeholders}}` that are *consumed before being
produced*. **Payloads live in three different fields — scan all of them**, or you will report a
process as input-free when it is not:

| Node | Payload field |
|---|---|
| `api_rpc`, `set_param` | `extra` / `extra_type` |
| `api_copy` | **`data` / `data_type`** — there is no `extra` field at all |
| `api` with `format:"raw"` | **`raw_body`** — a JSON *string*; `extra` is `{}` |

```jsonc
{"type":"api_rpc","conv_id":1760254,
 "extra":{"lat":"{{location_lat}}","lon":"{{location_lon}}","radius":"200000"}}
```

`location_lat` / `location_lon` are inputs; `radius` is a constant. Carry the `required` flag
from `params` where present.

Two traps worth checking before you design a field for an input:

- **A declared input can be dead.** If `{{thatParam}}` appears in no node, the value comes from
  somewhere else — typically a state-diagram read like
  `{{conv[<id>].ref[SessionData].Session}}`, which is one *workspace-wide* session rather than a
  per-user token. Drop it from the manifest and record why.
- **`{{proc[<id>].ref[<key>].<field>}}` is data, not input** — except for a nested inner
  placeholder (`…ref[Cities].{{cityCode}}`), where the inner one *is* an input.

### 3.7 Side-effect classification

**Side effects are not statically obvious.** In the reference set `Send complain` and
`Send calback mailing` look inert at this layer but fan out via `api_copy` to a mailing
process, and `Cashback Categories` sends Telegram/Viber messages. Classify each process
`likely | unlikely | unknown` from `api_copy` presence, non-GET `api` nodes, and
title/description keywords (send, mail, register, recovery, complain, notify, create).

**Score only the nodes reachable from Start.** Corezoid never prunes orphans, so score the
subgraph reached by BFS over `to_node_id` + `err_node_id` + `semaphors[].to_node_id`. In the
reference set `Cashback Categories` has **126 nodes and 6 reachable ones** — the other 120 are dead
chatbot senders. Over the whole bag it scores `likely` and gets excluded from probing; over the
reachable set it is `unlikely` and probes safely. The same walk stops you deriving outcomes from
unreachable reply nodes (`Authorization + Bonus amount` has a duplicate `200/success` reply that
nothing reaches). Record `"nodes": "<reachable> of <total> reachable"` in the manifest, and flag a
lopsided ratio to the user — it usually means the process was repurposed and its declared `params`
describe the old job.

**This classification gates two things: probing (§4) and where the process may be wired (§5.1a).**
Never treat `unlikely` as proof of safety.

### 3.8 The manifest

```jsonc
{
  "processes": [{
    "id": 1760349,
    "title": "Authorization + Bonus amount",
    "path": "689413_API2/1760349_Authorization_+_Bonus_amount.conv.json",
    "inputs":  [{"name":"phone","type":"string","required":true},
                {"name":"cardPassword","type":"string","required":true}],
    "outcomes": {
      "success":   {"code":"200","keys":{"token":"string","cardCode":"string",
                                         "bonusAmount":"number","QR":"string"}},
      "alternate": [{"code":"402","result":"registration"},
                    {"code":"401","result":"recovery"}],
      "error":     [{"code":"403"}]
    },
    "arrays": {},
    "nodes": "23 of 25 reachable",
    "sideEffects": "unknown",
    "declaredParamsMatch": false,
    "deadDeclaredInputs": []
  }]
}
```

### 3.9 Report to the user

Show a contract table before designing, and **explicitly flag**:

- processes with **no derivable outputs** (nothing to render),
- arrays with **unknown element shape**,
- the **side-effect classification**, and the **reachable/total node ratio** wherever it is
  lopsided — that is the signal the process was repurposed,
- any place `params` disagreed with the reply nodes (`declaredParamsMatch: false`), **including
  declared inputs that no node consumes** (`deadDeclaredInputs`).

**State the evidence for any claim you make about the backend.** Saying "this process is broken"
is a finding about someone else's system, and the payload-carrier traps in §3.6 make it easy to
get wrong — an `api_copy` read through `extra` looks like it sends nothing. Before reporting a
defect, confirm you read the right field (`data` for `api_copy`, `raw_body` for a raw `api`), and
name the field you read.

---

## 4. Phase 2b — Ground-truth probing (opt-in, user-gated)

`run-task(process_path, data)` runs a task on the deployed process and waits for a final
node, so it returns the process's **real** reply — the only reliable way to resolve an
unknown array shape.

**Never probe automatically.** Present the side-effect classification, let the user choose
which processes are safe to call with test data, and probe only those. Calling a process
blind can send a real SMS, email, or Telegram message to a real customer.

Feed anything learned back into the manifest before designing.

---

## 5. Phase 3 — App design

Turn N contracts into a page map. Produce `app-plan.md` and get it approved before building.

### 5.1 Clustering heuristics

| Contract shape | Becomes |
|---|---|
| Emits a token / session key, takes credentials | **Entry / login page** (the app's `index`) |
| Chained by a shared key (process B's input = process A's output, e.g. `cardCode`) | A **wizard**: one page per step |
| Array output, few or no inputs | **Content page** — a `table` or a `contentLoop` list |
| Input-heavy, output-poor | **Form page** with a submit button |
| An **alternate outcome** of another process (`401 recovery`, `402 registration`) | **Its own page**, reached by `302 nextPage` |
| No inputs, static-ish output (FAQ) | **Content page**; consider `is_static` only if it needs no Corezoid call at all |

### 5.1a A side-effectful process never goes on a page's `/get`

`/get` is not a one-off call. It fires on **every** page open, on every re-render, for every
visitor, unattended — and a crawler, a refresh or a shared link fires it again. So wiring a
process to `/get` is *repeated automatic probing*, which is exactly what §4 makes you ask the
user about before doing **once**.

> **Rule: a process classified `likely` or `unknown` (§3.7) may only be reached from a `/send`
> button. Only `unlikely` may sit on a `/get`.**

This bites hardest on a wizard whose first step *looks* like a read. In the reference set
`Registration : 1 step - Get fields` reads like a getter and was classified `unknown`; it turned
out to **allocate a real loyalty card number** from a shared backend session on every call.
Wired to the `reg` page's `/get`, it burned a live card number every time anyone opened the page —
including each L3 test render.

The fix is cheap when you design for it and annoying afterwards: give the wizard an explicit
**start button** (`<verb>_start_btn`) whose `/send` branch makes the call and returns the result
as `changes[]`. The page's `/get` then renders step 1 statically. You get a real three-step
wizard instead of a page that mutates on sight.

When a `likely`/`unknown` process genuinely has no button to hang it on, say so and ask — do not
quietly put it on `/get`.

### 5.2 The coverage rule (load-bearing)

**Every input process must appear in the map.** Emit an explicit coverage table:

| Process | Page | Trigger |
|---|---|---|
| 1760349 Authorization + Bonus amount | `index` | button `login_btn` |
| 1760347 Transactions history | `history` | page `/get` |
| … | … | … |

If a process genuinely does not fit the product description, **say so and ask** — offer to
drop it, give it a plain page, or reshape the app. Never silently omit one; silent omission
is the main way this skill can produce a wrong result that still looks finished.

### 5.3 Per-page specification

For each page define: `id`, title, backing process(es), fields (`id` / `class` / `type` /
`required`), buttons (`id` → action), viewModel keys, which hidden session carriers it
needs, and its outgoing navigation edges.

Naming conventions that keep the layers in sync:

- viewModel key ↔ `{{key}}` in `pages/<id>/config` — same name, no aliasing.
- A table's body binds as `"body": "{{<page>_<entity>_body}}"`.
- Buttons: `<verb>_btn` (`login_btn`, `submit_complaint_btn`).
- Hidden session carriers: `__`-prefixed (`__token`, `__cardCode`).

### 5.4 Approval gate

Show the page map, the coverage table, and a navigation diagram. Get an explicit **yes**
before building anything. This is the last cheap moment to change the design.

---

## 6. Phase 4 — Build the Smart Form shell

```
createSmartForm(title="…", ref="<slug>")      // credentials omitted ON PURPOSE — see §8
pullSmartForm(actorId="<uuid>")               // writes .manifest.json that push needs
… author develop/pages/<id>/{config,locale}, develop/locale, develop/viewModel …
pushSmartForm(actorId="<uuid>")               // fix validationErrors, repeat until clean
```

Edit **`develop` only** — `production` is readonly. Follow `simulator-smart-forms` for the
page-config grammar (grid → forms → sections → items).

**Seed every `{{key}}` in `develop/viewModel`** with a sane default. A page whose viewModel
key is never defaulted renders a literal `{{key}}` if the backend call fails — and the L3
test below checks for exactly that.

**One exclusion: placeholders inside a `contentLoop` section's `content` are loop-scoped, not
viewModel keys.** They are substituted from the loop entries the backend returns, so seeding them
is wrong — a default would shadow nothing and a "missing default" report on them is a false
positive. In the reference `promo` page, `{{img}}` / `{{text}}` / `{{url}}` live only inside the
loop template; `{{promos_loop}}` (the array binding itself) is the viewModel key that needs the
default (`[]`). The same exclusion applies to anything you write that audits these tokens.

**Decide the app's language here, once — before you write a single Code node.** You author *two*
sources of user-visible text and only one of them is translated:

| Source | Localised? |
|---|---|
| `[[key]]` resolved from `locale` / `pages/<id>/locale` | yes — per the viewer's language |
| string literals inside the middleware's Code nodes (`changes[].value`, `notifications[].title`, every empty state) | **no** — they bypass the locale layer entirely |

So a `locale` carrying `en` + `uk` plus Code nodes hardcoded in one language renders a mixed page
for anyone whose language resolves to the other: in the reference build the `home` card showed
*"Your bonuses"* directly above *"Сесія не знайдена"*. Nothing flags it — both layers are
individually valid, `pushSmartForm` is clean, and `appGetPage` returns exactly what you asked for.

Pick one and write it into the brief:

- **single-language app** — give every locale key the same text under every language code, so the
  viewer's locale cannot pull the page away from the Code nodes. Right answer when the product has
  one audience;
- **genuinely multilingual** — read `body.context.language` in the `/get` prep node and branch every
  literal, or return locale *keys* from the backend and let the page resolve them.

Emitting `en` + `uk` locale files and then hardcoding one language in the backend is the option that
looks finished and is not.

Three renderer constraints shape every page you author. Design around them from the start;
discovering them mid-build forces a redesign of every empty state:

| Constraint | Consequence |
|---|---|
| `visibility` cannot be a `{{placeholder}}` — it is validated against the literal enum at push time | **A page cannot vary visibility on `/get` at all.** Dynamic visibility exists only via `changes[]` on `/send`. Do not emit `*_visibility` viewModel keys for `/get`; they do nothing. |
| A `label` value may never be `""`, nor an `image` value an empty src | Express "nothing to show" as a non-breaking space (`\u00a0`) for labels. For an image use a **fetchable** `http(s)` placeholder URL — **not** a `data:` URI: the renderer proxies every `image.value` through `/api/1.0/image?src=`, which answers `400 "URL is not allowed"` for that scheme |
| A `changes[]` patch cannot touch `extra` | Anything configured through `extra` is fixed for the life of the rendered page; pick components whose state lives in `value` |

Together the first two mean **empty states are driven by text, not by visibility**: bind a
`label` to a viewModel key and let the backend decide whether it says anything.

### 6.1 Write the baseline stylesheet NOW, not in Phase 8

**Two platform defaults make an unstyled generated app look broken rather than plain.** They are
not cosmetic and they are not deferrable — a page authored without knowing about them gets a layout
you have to redo:

| Default | What you actually see | Fix |
|---|---|---|
| Every `.section__content` ships its **own** grey background + `padding: 20px 16px 0` + `margin-bottom: 20px` (some rules theme-scoped → `0,2,0`) | a grey padded box *inside* every card you author, with doubled padding | neutralize once, then let your own card supply padding |
| `[data-class="grid-one-column"]` is **capped and centred** | every page squeezed into the platform's narrow card, no app shell | unlock it and cap the width yourself |

So Phase 4 ships `develop/styles/` from the start. The minimum that makes a generated app read as
an app rather than a broken form:

```less
// styles/index — imports only, in cascade order
@import "colors_fonts";   // tokens first: init_styles AND every pages/<id>/style inherit them
@import "init_styles";
```

```less
// styles/init_styles — the mandatory part
*, *::before, *::after { box-sizing: border-box; }
& { background: @bg; min-height: 100vh; color: @ink; font-family: @font_main; }

// 1. the grey box inside every card
.section .section__content {
  background: transparent !important; margin-bottom: 0 !important; padding: 0 !important;
}
.label, .button, .form { margin: 0 !important; }
.button-wrapper { width: auto !important; display: block; }

// 2. the app shell
.content__main { padding: 0 !important; }
[data-class="grid-one-column"] { max-width: none !important; }
.pg { max-width: 980px; margin: 0 auto; padding: 20px 20px 40px; }   // grid.styleClass = "pg pg-<id>"
.pg.pg-auth { max-width: 428px; }                                    // login / recovery / register

// 3. the utility the hidden session carriers of §7.4 depend on
.visually_hidden { position:absolute; width:1px; height:1px; margin:-1px; padding:0;
  border:0; white-space:nowrap; clip-path:inset(100%); clip:rect(0 0 0 0); overflow:hidden; }
```

Give **every page's `grid` a `styleClass`** (`"pg pg-<pageId>"`) while authoring the config — it is
the only hook that lets the stylesheet treat auth screens differently from app screens, and adding
it later means touching all nine configs again.

Two authoring notes that only matter if you know them up front:

- **A `styleClass` on a `row` is dropped.** The `row` value is space-separated, so `row: "geo geo_row"`
  puts a literal `.geo_row` on the row wrapper — that is the *only* way to style a multi-column row
  (e.g. to align a button with the inputs beside it, or to stack them under a breakpoint).
- **A bare `contentLoop` renders its iterations flat, with no per-iteration box.** Setting the
  section's `sortable: true` makes the server wrap each iteration in a `draggable` — see §6.2 — which
  is how you get a card grid. Decide this while authoring, not after.

### 6.2 `contentLoop` bound from the viewModel — verified shapes

Both of these are confirmed against a live render, so design on them freely:

- **`"contentLoop": "{{promos_loop}}"`** works exactly like a table's `"body": "{{rows}}"` — the
  backend returns an array of plain variable bags and the server expands the section's `content`
  template once per entry.
- **`sortable: true` + `contentLoop`** makes the server expand each iteration into a `draggable`
  wrapper instead of flat items:

  ```jsonc
  // served config, per iteration
  {"id": "<sectionId>-cl-0", "class": "draggable", "items": [ /* the template, substituted */ ]}
  ```

  That wrapper is what you style as a card (`.draggable__content`). Hide `.draggable__handle` to
  drop the drag affordance — it is the only activator, so there is no reorder submit to handle:

  ```less
  .promoloop .section__content { display: grid !important;
    grid-template-columns: repeat(auto-fill, minmax(258px, 1fr)); gap: 20px !important; }
  .promoloop .draggable__handle { display: none !important; }
  .promoloop .draggable { border:0 !important; background:transparent !important;
    padding:0 !important; box-shadow:none !important; cursor:default !important; }
  .promoloop .draggable__content { background:@surface; border:1px solid @line;
    border-radius:18px; overflow:hidden; display:flex; flex-direction:column; height:100%; }
  ```

Hand the rest — palette, typography scale, component re-skinning — to `simulator-styles` in Phase 8.
That phase is for *making it branded*; this one is for *not shipping something broken*.

---

## 7. Phase 5 — Build the middleware process

One Corezoid process is bound to the Smart Form env (`procId`) and is the **only** bridge
between the form and the domain processes.

> ### You do NOT author this process yourself
>
> **Never hand-write the middleware `.conv.json`.** Your job is to produce a precise brief
> and hand it to the Corezoid plugin's authoring skill, which owns `create-process` →
> node authoring → `layout-process` → `lint-process` → `push-process`:
>
> ```
> Skill(skill="corezoid:corezoid-create", args="<the brief from §7.8, verbatim>")
> ```
>
> Use the plugin-qualified name `corezoid:corezoid-create`. If the host reports it as
> unknown, retry with the bare `corezoid-create` (older hosts resolve unqualified names).
> For **changes** to a middleware that already exists — including every fix in the repair
> loop of §9.4 — use `corezoid:corezoid-edit` the same way, never a hand edit.

Read `$CLAUDE_PLUGIN_ROOT/skills/simulator-smart-forms-logic/SKILL.md` §1–§2 first: it
defines the `/get` and `/send` contract and the reusable node fragments (§2.1–§2.9) that
your brief should reference by number instead of restating.

### 7.1 Topology

**Default: one bound middleware process** that owns both paths and calls each domain process
via `api_rpc`. The domain processes already carry the real work — the middleware is a
router, so keeping it in one graph keeps it debuggable.

Split into per-page sub-processes (called with `api_copy` / `api_rpc`, each aliased) only
when the single graph exceeds roughly **60 nodes** or **8 pages**. Say which you chose and
why.

**Count business nodes against that budget, not error nodes.** Escalations and Error finals scale
mechanically with the number of fallible nodes — `layout-process` measured **32%** of both handlers
in the reference build as error nodes (77 nodes → ~52 business; 74 → ~50). Counting them makes you
split a graph that is comfortably inside the threshold. Count Start, dispatches, domain calls,
builders and callbacks; ignore the error clusters.

Two levers keep a graph small without splitting it — both in §4.1a of
`docs/user-flows/app-generation.md`: **one Code node for all navigation** (a lookup table instead of
a dispatch arm per `nav_*` button), and **one Code node per outcome mapping** instead of a
`go_if_const` tree per domain call. The second also removes the unbracketed-nested-`param` silent
fallthrough entirely, at the cost of hiding that branching from the Corezoid canvas — so keep the
`path` / `page` / `buttonId` **dispatches** as real condition nodes and use Code nodes only for the
mapping inside a branch.

### 7.2 The shape of the graph

```
Start
 └─ Condition on `path`                              (go_if_const)
     ├─ /get  → Condition on `body.page`
     │           └─ per page: api_rpc(domain) → set_param(namespace) → api_code(build viewModel)
     │                                                              └─→ Callback GET
     └─ /send → api_code(extract body.buttonId)
                 └─ Condition on `body.buttonId`
                     ├─ <button ids>          → api_rpc(domain) → branch on result/code
                     │                            ├─ success   → build changes/notifications
                     │                            ├─ alternate → build 302 nextPage
                     │                            └─ error     → build error notification
                     └─ <submitOnChange ids>  → cascade update or empty ack
                                                                  └─→ Callback SEND
```

Both callback nodes are `api` nodes POSTing to `{{__callback_url}}`:

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
```

> ⚠️ **`format`, `send_sys`, `debug_info`, `cert_pem`, `max_threads` are required — do not trim
> them.** Omitting them makes `lint-process` fail the file twice: `max_threads` is a JSON-schema
> required property (`missing property 'max_threads'`), and the rest trip
> `UNDERSPECIFIED API CALL NODES`, whose real-world symptom is a server commit that hangs
> ~15–20 s and then reports `no response from server` instead of a useful error.

`/send` is identical with `extra:{"code":"200","data":"{{responseData}}"}` and
`extra_type:{"code":"number","data":"object"}`.

> `customize_response:false` is mandatory — cb-apigw acks with an empty body, and response
> casting then throws `api_wrong_convert_param` even though delivery succeeded.

### 7.3 Namespace isolation (the subtle one)

An `api_rpc` callee's `res_data` **merges into the caller's task data at top level**. Because
every domain process replies with the same `result` / `code` envelope, calling two of them in
one task means the second silently overwrites the first's verdict.

**Rule: in a task that calls more than one domain process, every `api_rpc` is immediately followed
by a `set_param` that copies the results into namespaced keys and clears the shared envelope.**

```jsonc
{"type":"set_param",
 "extra":{"auth_result":"{{result}}","auth_code":"{{code}}",
          "auth_token":"{{token}}","auth_cardCode":"{{cardCode}}",
          "result":"","code":""},
 "extra_type":{"auth_result":"string","auth_code":"string",
               "auth_token":"string","auth_cardCode":"string",
               "result":"string","code":"string"},
 "err_node_id":"<id>"}
```

Branch on `{{auth_result}}` / `{{auth_code}}`, never on the bare `{{result}}`.

**Scope the rule to where the collision can actually happen.** The hazard is *two* callees writing
the same envelope into one task. So:

- a task that calls **two or more** domain processes → namespace after **every** call, no exceptions;
- a branch that calls **exactly one** domain process and then reads its reply → there is nothing to
  overwrite. Read `{{result}}` / `{{code}}` / the payload keys directly in the builder Code node and
  skip the `set_param`.

Applying it unconditionally is not "safe by default" — it is a real cost. On a 9-page `/get` router
where each page branch makes a single call, the mandatory version adds a `set_param` **plus its own
error cluster** per branch: ~18 nodes of pure ceremony. Decide per branch, and if you skip it, say so
in the brief so the next editor knows to add it the moment a second call appears.

**If a Code node reads the values, author the node as `api_code` rather than `set_param`.**
`lint-process` cannot parse JavaScript, so it resolves a `set_param`'s outputs by scanning
downstream node *configuration*; a `data.result` read inside an `api_code` is invisible to it and
the file fails with `UNUSED SET_PARAM … but no downstream node references them`. This bites exactly
when you combine this section with the "one Code node per outcome mapping" pattern (§4.1a of
`app-generation.md`) — which is the recommended shape. Same assignments, different node type:
`data.auth_result = data.result; data.result = "";`

### 7.4 Hidden session carriers

The Smart Form protocol is stateless: `/get` and `/send` carry no server session. Carry state
in **hidden form fields**.

- Put a CSS-hidden `edit` item in every **form** that needs it, with `visibility: "visible"` —
  a field hidden by CSS but `visible` in the protocol **is still submitted**. Give them
  `styleClass: "visually_hidden"`.
  ```jsonc
  {"id":"__token","class":"edit","type":"text","value":"{{__token}}",
   "visibility":"visible","styleClass":"visually_hidden"}
  ```
- `/get` seeds them from the viewModel it returns.
- `/send` reads them from `body.data.__token` — always from `body.data`, never from
  `buttonData`.
- The login handler populates them by returning `changes[]` entries for those ids before
  navigating.

> ⚠️ **A submit sends exactly ONE form, so a carrier only arrives if it sits in the SAME form as
> the button.** "Per page" is the wrong unit and it fails silently. The shape that bites is the
> obvious one: an `appbar` form holding the `nav_*` buttons, and the carriers over in the content
> form. Every nav click then submits a form with no value-bearing items at all, `body.data` is
> `{}`, and the handler writes an empty session into its `302 query` — the user is quietly logged
> out on the first click. Nothing catches it: `pushSmartForm` is happy, `appGetPage` renders the
> page correctly, and only the *navigation* is broken.
>
> **So read the session defensively instead of relying on placement:**
> ```javascript
> var d = (data.body && data.body.data)  || {};
> var q = (data.body && data.body.query) || {};
> data.sess_token = String(d.__token || q.token || "");
> ```
> `body.query` is present on `/send` too (the page URL holds it), so this works no matter which
> form the button ended up in — and it keeps working when someone later moves a button. Duplicating
> the hidden fields into every form also works, but it rots the moment a form is added.

Never put a password or a payment token in a carrier — carriers reach the browser. If a
contract forces one, tell the user and propose an actor-backed session instead.

### 7.5 The `submitOnChange` fork — do not skip it

`/send` fires for button clicks **and** for every element with `submitOnChange: true`.
`body.buttonId` is the triggering element's id in both cases, and `buttonData` is `{}` for
buttons *and* for `radio` / `check` / `toggle` / `edit` — only `select` populates it.

So: **dispatch on `body.buttonId`, and enumerate every `submitOnChange` field id**, including
the last one in a chain (give it a no-op ack branch). Any id you don't route falls through
to the submit path and the app "jumps a step". Do **not** dispatch on `buttonData.action`.

Read a changed value from `body.data.<fieldId>`.

**Two structural defences, so this stops depending on a complete list.** Enumerating ids is
necessary but it is a list a human has to keep in sync, which is exactly the kind of thing that rots:

- **Prefer zero `submitOnChange` fields when the UX allows it.** A field only needs it to drive
  *other* fields on the same page (a cascade, a reveal). If the page has no cascade, leaving it off
  removes the whole failure class rather than handling it. The reference nine-page app shipped with
  none.
- **Make the dispatch's default branch a no-op ack, never a submit.** Route it into the navigation
  lookup Code node (§4.1a of `app-generation.md`): a `buttonId` that matches no nav target and no
  button sets `changes: []` and returns `200`. Then an id you forgot to enumerate acknowledges
  harmlessly instead of navigating — verified live by posting an unrouted id and getting
  `{"code":200,"data":{"changes":[],"notifications":[]}}`.

### 7.6 Outcome → UI mapping

| Domain reply | Middleware response |
|---|---|
| `success` | `code:200`, `data:{changes:[…], notifications:[…]}` |
| alternate (`registration`, `recovery`, …) | `code:302`, `data:{nextPage:"<page>"}` |
| `error` | `code:200` with an error `notification`, or `4xx` for a hard failure |

### 7.7 Invariants your brief must state

`corezoid:corezoid-create` runs `layout-process` → `lint-process` (until clean) →
`push-process` itself. What it cannot infer, and your brief must therefore spell out:

- Every fallible node (`api`, `api_rpc`, `api_code`, `set_param`) needs its **own** error target
  — never one shared between two nodes. **The target's `obj_type` depends on whether the error
  path does anything:**
  - it **has** logic (build a fallback viewModel, then rejoin the flow) → `obj_type: 3`
    escalation, whose `go` leads on and whose own `err_node_id` ends at a dedicated
    `obj_type: 2` Error final;
  - it has **no** logic (just stop) → point `err_node_id` **straight at an `obj_type: 2` Error
    final**. Do *not* wrap it in an escalation node that only holds a `go` — `lint-process`
    flags that as a passthrough escalation, and padding it with a throwaway `set_param` to
    "give it logic" earns you `UNUSED SET_PARAM` plus `SHARED ERROR CLUSTERS` instead.
- Every `api_rpc` needs a `time` semaphor, or a hung callee hangs the page. **30 sec is the
  server minimum** — lint rejects anything lower (`below the server minimum of 30 sec; the
  deploy is rejected`), so 30 s is also the floor on how fast a page can fail. Design the
  empty state to read sensibly after a half-minute pause, not as a flicker.
- Both callback nodes need `customize_response: false`, `extra_type.code: "number"`, and the five
  canonical fields of §7.2 (`format`, `send_sys`, `debug_info`, `cert_pem`, `max_threads`).
- Every `api_rpc` is followed by the namespacing `set_param` of §7.3 — **when the task makes more
  than one domain call.** See §7.3 for when it is pure overhead, and for why the node must be an
  `api_code` (not a `set_param`) whenever a Code node is what reads the values.

After the skill reports success, record the returned **numeric process id** — it becomes
`procId` in §8.

### 7.8 Brief template

Fill this in and pass it verbatim as `args`. Reference §2.x fragments of
`simulator-smart-forms-logic` rather than restating their JSON, but **write out every Code
Node body in full** — do not leave it to chance.

> **Process purpose:** Middleware bound to Smart Form `<ref>`. Receives `/get` and `/send`
> from the Smart Form runtime and bridges them to the domain processes `<ids>` via
> `api_rpc`.
>
> **Input parameters:** `path` (`/get` | `/send`), `body` (`body.page`, `body.buttonId`,
> `body.buttonData`, `body.data.*`, `body.context.*`), `sessionData`, `__callback_url`.
>
> **Expected output:** POST to `{{__callback_url}}` — `/get` → `{code:200, viewModel:{…}}`;
> `/send` → `{code:200, data:{changes,notifications}}` or `{code:302, data:{nextPage}}`.
>
> **Process type:** Business logic (orchestrator).
>
> **Folder path:** `<folder>`   **Process name:** `<app> Middleware`
>
> **Alias:** none — the bound process is referenced by numeric `procId`. (Aliases are
> needed only for sub-processes you fan out to.)
>
> **Pages:** `<page ids>`   **Buttons:** `<button ids>`
> **submitOnChange field ids:** `<ids>` ← every one of these needs its own dispatch arm
>
> **Domain processes to call:** for each — `conv_id`, the `extra`/`extra_type` map built
> from its inputs (§3.6), and its success / alternate / error outcomes (§3.3).
>
> **Required nodes:** the node table of §4.1 in
> `$CLAUDE_PLUGIN_ROOT/docs/user-flows/app-generation.md`, instantiated per page/button.
>
> **Language of every string literal below:** <single language, or driven by `body.context.language`
> — see §6; these literals are NOT localised>
>
> **Code Node bodies:** <written out in full — viewModel builders, changes/notifications
> builders, array→table transforms per §4.3 of that doc>
>
> **Variables to create:** <any constant that should land in `_ENV_VARS_.json`>

---

## 8. Phase 6 — Wire the middleware to the form

The Smart Form runtime calls Corezoid **as an API key**. That key needs at least `create`
privilege on the bound process, or every `/get` and `/send` fails at the edge before reaching
node 1.

```
create-api-key(title="Smart Form <ref>")           // → obj_id; creds in ~/.corezoid/api-keys/
   … or find-principal(name="<apiLogin>", kind="api_key") for an existing key

share-object(obj="conv", obj_id=<middlewareProcId>,
             obj_to="user", obj_to_id=<apiKeyObjId>, privs="create")
```

API keys are addressed as `obj_to="user"` — that is how Corezoid models them.

Then bind **both** environments — each env has its own independent binding, and a release
ships *files only*, never credentials. Binding only `develop` leaves production 5xx-ing:

```
updateSmartFormEnv(actorId="…", env="develop",    apiLogin=…, apiSecret=…,
                   procId="<middlewareProcId>", companyId="<companyId>")
updateSmartFormEnv(actorId="…", env="production", apiLogin=…, apiSecret=…,
                   procId="<middlewareProcId>", companyId="<companyId>")
```

> This is why §6 created the form **without** credentials: the form must exist before the
> middleware can be designed against its pages, and the middleware must exist before its
> `procId` can be bound. Create form → build middleware → bind.

---

## 9. Phase 7 — Test end-to-end, and repair

Three layers. Run them in order; each is cheap relative to the next.

### 9.1 L1 — Static

- `lint-process(process_path=<middleware>)` → must be clean.
- `pushSmartForm(actorId)` → must report no `validationErrors`, **and read its `warnings`**.
  The push runs two passes: the per-file component-schema check, and a **cross-file token audit**
  (`cduschema.ValidateTree`) that the per-file pass structurally cannot do — it resolves every
  `[[key]]` against the merged app+page locale and every `{{key}}` against the viewModel defaults.
  The split follows what the runtime can still rescue:
  - a missing **locale** key is an **error** — locale comes only from the static files, so an
    unresolved `[[key]]` always reaches the browser as literal text;
  - a missing **viewModel** default is a **warning** — the bound process may fill it per request;
    it only shows as a literal `{{key}}` when the backend call fails.

  Warnings do not block the push, so they are easy to skip past. Read them: an undefaulted key is
  precisely the L3 failure you would otherwise find by hand.
- **Compile the Less** before or with the push — a compile error never surfaces as an error
  (`/* Less Error … */` comment, page renders unstyled). Recipe in `simulator-styles` →
  "Verifying a styling result".

### 9.2 L2 — Backend, via `run-task`

Run synthetic tasks straight at the middleware: one `/get` per page, and one `/send` per
button **and per `submitOnChange` field**.

```jsonc
// /get
{"path":"/get","body":{"page":"index","context":{"appId":"<id>"},"query":{}},"sessionData":{}}

// /send — button
{"path":"/send","body":{"page":"index","buttonId":"login_btn","buttonData":{},
 "formId":"login","sectionId":"body",
 "data":{"phone":"380000000000","cardPassword":"0000","__token":""}},"sessionData":{}}
```

> **Expect the final callback node to fail here, by design.** A synthetic task has no real
> `{{__callback_url}}`, so the callback `api` node errors. That is not a test failure.
> Assert instead that the task **reached** the callback node with a correctly built
> `viewModel` / `responseData` in its data — inspect with `list-task-history`. Treat the
> callback POST error as expected and say so in the report, so a reader doesn't mistake it
> for a real defect.

What to assert: the task routed to the right branch for the given `path`/`page`/`buttonId`;
the namespaced keys from §7.3 are populated; `viewModel` contains every key the page's config
references.

### 9.3 L3 — Real end-to-end

Now drive the live `develop` environment the way a user would (see
`simulator-smart-forms-runtime`):

- `appGetPage(accId, ref, "develop", <page>)` for **every** page.
  - Assert **no unresolved `{{` or `[[` remains** — `{{` catches a missing viewModel key, a failed
    backend call and a typo'd placeholder; `[[` catches a locale key absent for the active
    language, which is a different defect with the same symptom. L1's token audit should have
    caught both statically, so anything surfacing here means the page and the tree disagree.
  - Assert **no value resolved to `""`** on a `label` or `image` — the renderer rejects an empty
    value, and an empty string contains no `{{`, so the check above sails past it.
  - Assert the items the design promised are present and `required` where specified.

> ⚠️ **`appGetPage` does not run the client validator.** It returns the *server-resolved*
> config, so it proves templating and the backend, not validity. No component schema sets
> `additionalProperties: false`, so an unknown key (`head[].id`, `image.extra.height`, a typo'd
> `visibilty`) is stored, served and reported clean here — then rejected in the browser
> console. The real structural gate is `pushSmartForm`'s `validationErrors`; treat a clean push
> as the precondition and `appGetPage` / `appSendForm` as the behavioural check on top. Plan on
> one human browser pass too: nothing else exercises the renderer, and CSS and BBCode are never
> compiled by either tool.
- `appSendForm(...)` for **every** button, with representative data.
  - Assert the returned `code` (200 / 205 / 302), and that `changes[]` ids / `nextPage`
    match the design.
- Walk the full navigation graph, including the alternate-outcome pages (`registration`,
  `recovery`) — those are the ones a happy-path-only test misses.

### 9.4 The repair loop

For each defect, classify it and fix it in the **owning layer**:

| Symptom | Layer at fault |
|---|---|
| Literal `{{key}}` in the rendered page | viewModel builder (middleware) or missing default (`develop/viewModel`) |
| A field change navigates instead of updating | missing `submitOnChange` id in the `buttonId` dispatch (§7.5) |
| Wrong branch taken after a domain call | shared `result`/`code` not namespaced (§7.3) |
| `api_wrong_convert_param` on the callback | `customize_response` not `false` (§7.2) |
| Page renders but the button does nothing | `changes[].id` doesn't match a real item id |
| Backend returns a shape the page can't bind | contract misread — go back to §3 |

Fix middleware defects by handing an edit brief to
`Skill(skill="corezoid:corezoid-edit", args="…")` — process identifier, the user-visible
change, and the exact node shapes to add/modify. Never patch the pushed `.conv.json` by
hand. Page-config defects you fix directly, then `pushSmartForm`.

Re-run from L1 after every fix. **Cap the loop** (about three passes per defect). If it is
not converging, stop and report precisely what fails, what you tried, and what you think the
cause is. A truthful "these 9 of 11 flows pass, this one doesn't and here's why" is worth far
more than a loop that quietly gives up.

---

## 10. Phase 8 — Brand the theme

The **mandatory** resets, app shell and `visually_hidden` utility already shipped in §6.1 — the app
is not broken at this point, just unbranded. This phase makes it look designed.

Hand `simulator-styles` a brief covering:

- `styles/colors_fonts` — brand/ink/surface/status tokens, radii, a spacing scale, the font;
- `styles/init_styles` — one base look per component (buttons by `.button__<type>`, inputs via
  `.<sc> .field`, tables via `.<sc> td.table-cell`, toasts via `[class*="notifyItem"]`);
- `pages/<id>/style` — per-page exceptions only (auto-appended after the main sheet, and it
  inherits the tokens with no `@import`).

Two cascade facts to put in the brief, because they decide whether overrides land at all:

- Your rules are auto-wrapped in `.cdu-page`, so a bare single-class rule is already `0,2,0` and
  beats a bare renderer default for free — **but** the renderer theme-scopes roughly a third of its
  rules to the same `0,2,0` and loads *after* you, so those ties go to the default. Component
  overrides therefore carry `!important` deliberately; that is normal here, not a smell.
- Pick class chains over ids: a `styleClass` that must beat `.button.button__text` (`0,3,0`) has to
  be written as e.g. `.appbar .button.navlink` (`0,4,0`), not as a bare `.navlink`.

**Verify the Less actually compiles before you push.** A compile error is not an error at serve
time — it is emitted as a `/* Less Error … */` comment and the page silently renders unstyled, so
neither `pushSmartForm` nor `appGetPage` will tell you. Compile locally against the same
`.cdu-page { … }` wrapper the server applies — the exact recipe (and the three traps that make the
obvious command fail: partials have no `.less` extension, `lessc` can't read `<(…)`, and the npm
package is `less` not `lessc`) is in **`simulator-styles` → "Verifying a styling result"**.

`@font-face` and `@media` correctly bubble out of the wrapper — verified — so they are safe to keep
in the imported partials.

Re-run L1 afterwards — styles don't change page config, but the push must stay clean.

---

## 11. Phase 9 — Deploy and report

`deploySmartForm(actorId)` **only after the user confirms** — it publishes to production.

Final report:

- Smart Form `ref` + `actorId`, and a link to open it.
- Middleware process id (and any sub-process aliases).
- The **coverage table** — every input process and where it ended up.
- Test results per layer, including anything that is expected-to-fail (§9.2).
- Known gaps: unknown array shapes, unprobed processes, anything the user chose to drop.

---

## 12. Rules and pitfalls

1. **Never hand-author the middleware `.conv.json`.** Produce a brief and hand it to
   `corezoid:corezoid-create` (or `corezoid:corezoid-edit` for changes). That skill owns
   `create-process` → `layout-process` → `lint-process` → `push-process`.
2. **Derive outputs from `api_rpc_reply`, not from `params`.** Declared params drift.
3. **Alternate outcomes are features, not errors.** `402/registration` and `401/recovery` are
   whole user journeys; a 200-only reading loses them.
4. **Use every process, or say which one you didn't and why.** Silent omission is the
   failure mode that looks like success.
5. **Namespace every `api_rpc` result** before the next call (§7.3).
6. **Enumerate every `submitOnChange` id** in the dispatch (§7.5).
7. **`customize_response:false`** on both callback nodes; `extra_type.code` is `"number"`.
8. **Bind both envs**; releases never carry credentials.
9. **Share the bound process to the API key**, or everything fails before node 1.
10. **Never probe a process with side effects** without explicit user consent (§4) — and never
    wire a `likely`/`unknown` one to a page's `/get`, which probes it on every view (§5.1a).
11. **Never put secrets in hidden carriers** — they reach the browser. And remember a submit
    sends **one form**: a carrier only reaches the handler from the same form as the button, so
    read the session as `body.data.X || body.query.x` rather than trusting placement (§7.4).
12. **Edit `develop` only**; `pullSmartForm` before editing, always.
13. **Don't rename pulled `.conv.json` files** — the `<ID>_` prefix is load-bearing.
14. **Report failures honestly.** If L3 doesn't go green, say exactly what fails.

---

## 13. When to use this skill vs. its neighbours

| The user wants… | Skill |
|---|---|
| A whole app generated from existing processes | **this skill** |
| To edit a page / layout / viewModel of an existing form | `simulator-smart-forms` |
| To change the backend logic of an existing form | `simulator-smart-forms-logic` |
| To run / fill in an existing form | `simulator-smart-forms-runtime` |
| To restyle a form | `simulator-styles` |
| To author one Corezoid process, no Smart Form | `corezoid:corezoid-create` |

If the user asks to add one page or one button to an app this skill already built, that is
`simulator-smart-forms` + `simulator-smart-forms-logic`, not a regeneration.

---

## 14. Reference documents

| Path | When to read |
|---|---|
| `$CLAUDE_PLUGIN_ROOT/docs/user-flows/app-generation.md` | The full contract-extraction algorithm, the annotated middleware skeleton, and the synthetic test-payload catalogue |
| `$CLAUDE_PLUGIN_ROOT/docs/user-flows/cdu-page-protocol.md` | Component catalogue, templating, `changes[]` protocol, response codes |
| `$CLAUDE_PLUGIN_ROOT/docs/user-flows/smart-forms.md` | Smart Form lifecycle, env binding, releases |
| `$CLAUDE_PLUGIN_ROOT/skills/simulator-smart-forms/SKILL.md` | Page-config grammar and the pull/push/deploy cycle |
| `$CLAUDE_PLUGIN_ROOT/skills/simulator-smart-forms-logic/SKILL.md` | The `/get` `/send` contract, node fragments, brief format |
| `$CLAUDE_PLUGIN_ROOT/skills/simulator-styles/SKILL.md` | The Less/`styles/` layer |
| Corezoid plugin: `docs/node-structures.md` | Exact JSON schemas for `api_rpc`, `api_rpc_reply`, `api`, `api_code`, `set_param`, `go_if_const` |
