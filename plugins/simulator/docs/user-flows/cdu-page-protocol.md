# CDU Page Protocol — Smart Form Runtime Reference

This document specifies the **page protocol** that powers Smart Form (CDU / Script) runtime:
the JSON contract a Smart Form page exchanges with the backend to render UI and submit data.
It is the runtime counterpart to [`smart-forms.md`](smart-forms.md) — that doc covers the
*lifecycle* (create → edit → deploy → serve); this one covers *what a page actually is on the
wire and on disk*.

> **Canonical spec.** The authoritative, field-level contract is the OpenAPI document
> **"Simulator.Company Scripts"** (served against the Corezoid Sync API,
> `sync-api.corezoid.com`). It defines 58 component/page schemas and 35 protocol tags
> (`get`, `send`, `viewModel`, `locale`, `definitions`, `contentLoop`, `bbcode`, `styles`,
> `realtime`, `page`, `form`, and one per component). This document mirrors that spec and the
> two reference implementations:
> - **Renderer:** `control-cdu` (`corezoid-driven-ui`) — the React client that consumes the
>   protocol. Types live in `src/types/common-types.ts`.
> - **Server:** `pong-server` `controlMain/api/applications/` — serves pages
>   (`pages.ts`), resolves templates (`helpers/applications/pageUtils.ts`), and persists the
>   page source files (`appContent.js`).

---

## 1. Where the protocol lives

A Smart Form page is **two artifacts that meet at runtime**:

```
 BUILD TIME (authoring)                          RUN TIME (serving an end user)
 ─────────────────────                          ──────────────────────────────
 app_files (per env): a page folder holds        GET  /pages/:accId/:ref/:env/:page
   config      (application/json)  ── layout      ↓  pong-server loads the page folder,
   locale      (application/json)  ── i18n         resolves config+viewModel+locale+$ref,
   …shared at root: viewModel, locale,             calls Corezoid (/get) for dynamic data,
   definitions/, styles/, widgets                  templates the result → Page JSON
                                                  ←  { code, data: { grid, forms[] }, … }
 stored as OPAQUE source (no structural           POST /pages/:accId/:ref/:env/:page
 validation server-side — see §10)                ↓  submit form → Corezoid (/send)
                                                  ←  { code, data: { changes[] | page | nextPage } }
```

- **On disk**, a page is the `config` JSON file (plus the page's `locale`) inside the env's
  `pages/<pageId>/` folder, alongside the app-wide `viewModel`, `locale`, `definitions/`,
  `styles/`, `widgets` (see the project skeleton in [`smart-forms.md` §2.2](smart-forms.md)).
- **On the wire**, the page is the fully-resolved `Page` object the renderer receives: the
  `config` template merged with `viewModel`/`locale`, `$ref` definitions inlined, `[[token]]`
  / `{{token}}` interpolated, and dynamic `viewModel`/data supplied by the Corezoid process.

The renderer never sees the raw templates — **template resolution is server-side**
(`renderPage` in `pageUtils.ts`); the client consumes fully-resolved values.

---

## 2. Transport & lifecycle (get / send / realtime)

The runtime uses three operations (swagger tags `get`, `send`, `realtime`). From the
renderer's perspective they are HTTP calls to pong-server's public `pages` API
(`/papi/1.0/pages/...`), which proxies to the env's Corezoid process via the Sync API.

### 2.1 Request envelope (renderer → server)

`control-cdu` (`src/utils/apiRequest.ts`) issues:

- **GET** `/pages/{workspace}/{app}/{stage}/{page}?<query>` — fetch & render a page.
- **POST** `/pages/{workspace}/{app}/{stage}/{pageId}` — submit a form.

Headers on every call:

| Header | Meaning |
|---|---|
| `Authorization` | session / API token (omitted for `anyone` apps) |
| `control-events-fingerprint` | anonymous-user identity for `anyone` apps |
| `Control-Tab-Id` | tab id (for realtime targeting) |
| `control-events-context` | **base64(JSON)** of the `ScriptContext`: `{ rootActorId, actorId, appId, timeZoneOffset }` |

The server parses the context header (`parseContextHeader`), enforces per-context gating
(`checkContextRestrictions` — `appSettings.expired` / `users` / `groups`), then forwards to
Corezoid with `path: '/get'` or `'/send'` plus `{ page, query, context }` and `sessionData`.
(See [`smart-forms.md` §4.7](smart-forms.md).)

### 2.2 GET response — render a page

`code = 200` with `data` shaped as the **`Page`**:

```jsonc
{
  "code": 200,
  "data": {
    "grid":  { /* Grid — layout */ },
    "forms": [ /* Form[] — content */ ],
    "query": { /* echoed/updated query */ },
    "notifications": [ /* toast messages */ ],
    "language": "uk"            // optional; overrides config.language for Intl
  },
  "sessionData": { /* opaque, round-tripped back on submit */ }
}
```

Static pages (`is_static=true` on the page folder) skip the Corezoid call entirely and render
from the stored config alone.

### 2.3 POST (send) responses — three outcomes

Submitting a form yields one of three response **codes** (renderer enum `HttpCode`):

| code | meaning | `data` shape | renderer behaviour |
|---|---|---|---|
| **200 OK** | patch the current page in place | `{ changes[], notifications[], query, ctrl[] }` | apply `changes` (see §7), show notifications, post `ctrl` messages to parent frame |
| **205 RESET_CONTENT** | re-render a page fresh | `{ page, pageId?, query? }` | replace the whole page (possibly a different `pageId`) |
| **302 REDIRECT** | navigate away | `{ nextPage, target?, close?, query? }` | go to `nextPage` (internal page id or external URL; `target` `_blank`/`_self`) |

The POST body the renderer sends:

```jsonc
{
  "page": "<pageId>",
  "formId": "<submitted form id>",
  "buttonId": "<button that triggered submit>",
  "sectionId": "<section id>",
  "buttonData": { "value": "…" },   // extra button payload (menu choice, auto-submit counter)
  "data": { "<itemId>": <value>, … } // collected values of visible, value-bearing items
}
```

Form data is collected by `getFormData(form)` — every visible, non-hidden item that carries a
value, keyed by item `id`. Submitted text values have HTML stripped server-side
(`TextUtils.stripObjectHtml`).

### 2.4 Realtime (`realtime` tag)

`POST /pages/realtime/{scriptActorId}/{env}/{page}` (swagger path
`/v/1.0/pages/realtime/...`) lets the backend push `changes` to connected tabs without a
submit. The renderer listens for a `WS_CDU_CHANGES` window message and applies the payload
through the same change pipeline as a 200 response. Used for live collaboration / server-pushed
updates.

---

## 3. Page model

The render tree is **Page → Grid → Form → Section → Item**:

```
Page
 ├─ grid            layout: column model, header, sidebar, region→form mapping
 └─ forms[]         one or more Form
       ├─ (grid?)   a form may carry its own nested grid
       └─ sections[]    body / block / modal / float
             ├─ header[]   Item[]   (Label | Button)
             └─ content[]  Item[]   ← the main items
                   └─ Item        keyed by `class` → a component
```

### 3.1 Grid (swagger `page`, `form` / `Page-grid-*`, `Form-grid-*`)

```jsonc
{
  "type": "one_column" | "two_column",   // gridType
  "header": {                            // Page-header-default | Page-header-steps
    "class": "default" | "steps",
    "extra": { "steps": ["…"], "active": 1 },
    "components": { /* region → [formId] */ }
  },
  "sideBar": { "components": { /* region → [formId] */ } },
  "components": {                        // region → ordered list of form ids
    "header": ["…"], "left": ["…"], "right": ["…"],
    "center": ["…"], "footer": ["…"], "sidebar": ["…"]
  },
  "styleClass": "custom-grid"
}
```

The `components` map binds layout **regions** to the **form ids** placed in them. Two grid
shapes exist (`one_column`, `two_column`) and two header shapes (`default`, and `steps`
for wizards — swagger `Page-header-steps`). Note: the header `class` is `steps`, not
`stepper`; `stepper` is a separate *component* class (§5) used inside section content.

### 3.2 Form (swagger `form`)

```jsonc
{
  "id": "info",
  "title": "…",
  "styleClass": "…",
  "visibility": "visible" | "disabled" | "hidden",
  "grid": { /* optional nested Grid */ },
  "sections": [ /* Section[] */ ]
}
```

A `Form` is the unit of submission — `formId` on submit identifies which form's data is sent.

### 3.3 Section (swagger `form`; renderer `SectionType`)

```jsonc
{
  "id": "…",
  "type": "body" | "block" | "modal" | "float",
  "visibility": "visible" | "disabled" | "hidden",
  "styleClass": "…",
  "header":  [ /* Item[] — Label | Button only */ ],
  "content": [ /* Item[] — the main items */ ],
  "contentLoop": [ /* see §6 */ ],
  "contentVersion": 3,        // renderer remount key — bump to force a re-mount of section content
  "sortable": true,           // drag-reorder contentLoop items (needs contentLoop)
  "modalHeader": [ /* Item[] */ ],          // for modal: replaces the default header
  "modalSize": "small" | "medium" | "large" | "xlarge",   // for modal (default large)
  "modalCloseConfirmText": "…",             // confirm dialog before closing modal/float
  "isResizable": true         // for float sections
}
```

`body` renders inline; `block` is a grouped card; `modal`/`float` render as overlays.

> ⚠️ **A section has no `footer`.** Its item slots are `header` and `content` (plus `modalHeader`
> for `modal`). The only `footer` keys in the canonical swagger are **grid region** maps
> (`grid.components.footer` / `sideBar.components.footer`), which hold **form ids**, not items —
> see §3.1. To place actions "at the bottom", put them as the last items in `content`, or give them
> their own form bound to the grid's `footer` region. Earlier drafts showed a section-level
> `footer: Item[]` — that slot does not exist. (Note: `contentVersion` **does** exist — it is the
> renderer's content-remount key, bumped by a section-level `change` (§7); it is just not a
> layout/item slot.)

---

## 4. Item — the component envelope

Every entry in a section's `header`/`content` is an **Item**, dispatched by its `class`
(renderer `components[clazz]` registry; swagger has one schema per class). Shared base fields
(renderer `baseSchema`):

| Field | Type | Meaning |
|---|---|---|
| `id` | string | item id; key under which its `value` is submitted |
| `class` | enum (see §5) | which component to render |
| `value` | string \| object \| array | current value (type depends on component) |
| `visibility` | `visible`\|`disabled`\|`hidden` | render state |
| `required` | boolean | required for submit (drives auto-disable of submit buttons) |
| `error` | boolean | error state (server- or client-set) |
| `errorMsg` | string | message shown when `error` |
| `styleClass` | string | extra CSS class (scoped under `.cdu-page`) |
| `row` / `w` | string | **the** horizontal-layout mechanism you author: items sharing the same `row` string render on one line; `w` is each item's **relative weight** (rendered width = `w / Σw` of the row, not a raw %). You do not hand-write a `row` component — the renderer synthesizes one internally from these fields (see §5). |
| `submitOnChange` | boolean | submit the form as soon as the value changes |
| `extra` | object | component-specific options |

`visibility`, `required`, `error`, `value`, and `options` are the fields most commonly mutated
by a 200-response `change` (§7).

---

## 5. Component catalogue

All component `class` values (renderer `ComponentClasses`). Most are content/input components;
`row` and `draggable` are the two layout classes.

> ⚠️ **You don't hand-author `row` — but the class is real.** Horizontal layout is authored with the
> **base `row` / `w` fields** (see §4): give sibling items the same `row` string and they render on
> one line, with `w` setting each one's relative weight. The renderer then **synthesizes** a
> `row` component (`ComponentClasses.row`) from those grouped items — so `row` is a real registered
> class you observe in the DOM, just not one you place directly with an `items[]` array.
> **`draggable` is a real standalone component** (`ComponentClasses.draggable`, dnd-kit based). Note
> it is *distinct* from a section's `sortable: true` + `contentLoop` drag-reorder (see §3.3 / §6) —
> both mechanisms exist. (Caveat: the two layout classes are **absent from the canonical "Simulator.Company
> Scripts" swagger**, so tool/schema validation won't know them — prefer the base-field path for
> `row`; verify `draggable` against a live render before relying on it.)

| `class` | Kind | Key fields (beyond base) | Notes / `type` enum |
|---|---|---|---|
| `button` | action | `title` (bbcode), `type`, `tooltip`, `extra.{url,target,action,icon,rounded,mobileVisible,request,autoSubmit,options}` | `type`: `default` `text` `secondary` `tertiary` `quaternary` `quinary` `error`. Submits its form; **`extra.url` opens that URL instead of submitting** (`extra.target` `_self`(default)\|`_blank` — honoured by newer renderers; older ones ignore `target` and open in the **same tab** via `window.location.assign`); `extra.action:'logout'`; `extra.request` runs a bare `fetch` first and submits **only if it resolves**; `extra.autoSubmit` `{interval,maxCount}` polls (**interval clamped 5–60s**, default 30; `maxCount` 1–500, default 6); `extra.options[]` opens a click menu **and bypasses `url`/`request`/`action`/submit**. `default`/`secondary` show a spinner while submitting; the CDU schema also has `default` reflect required-validation (disabled while a required field is empty) — verify against your renderer version. |
| `edit` | input | `value`, `type`, `placeholder`, `regexp`, `mask`, `errorMsg`, `helpMsg`, `submitOnEnter`, `resettable`, `extra.{length,lineNumbers}` | `type`: `text` `email` `int` `float` `phone` `multiline` `date` `password` `colorPicker` |
| `select` | input | `value`, `options[]`, `type`, `submitOnChange`, `submitOnScroll` | `type`: `default` `autocomplete`; scroll-paginated options |
| `multiselect` | input | `value[]`, `options[]`, `extra.length` | chip multi-select with search |
| `radio` | input | `value`, `options[]`, `extra.direction` | `direction`: `row`\|`column` |
| `check` | input | `value` (bool), `required` | checkbox |
| `toggle` | input | `value` (bool), `title` | switch |
| `slider` | input | `value`, `extra.{min,max,step}` | range |
| `phone` | input | `value` `{countryCode,number}`, `options[]` (country codes), `regexp`, `required` | regex over combined value |
| `otp` | input | `value` (object `otp-0…otp-N`), `type`, `extra.length` (2–20) | one-time-code boxes; `type`: `text`\|`int` |
| `label` | display | `value` (string), `align`, `tooltip` | static text; BBCode-rendered; `align`: `left`\|`center`\|`right` |
| `divider` | display | — | separator |
| `image` | display | `value` (src), `extra.{alt,height,width}` | static image |
| `copy` | action | `value` (text), `title` | copy-to-clipboard |
| `file` | file | `value` (FileProps), `extra.{downloadUrl,uploadUrl,auth}` | preview/download |
| `upload` | file | `value` (FileProps), `type`, `extra.{accept,minSize,maxSize,compression}` | `type`: `default`\|`webcam` |
| `attachment` | file | `value` (FileProps[]), `extra.downloadUrl` | multi-file viewer |
| `signature` | file | `value` (FileProps), `extra.{strokeStyle,saveButtonTitle}` | canvas signature → base64 |
| `carousel` | display | `items[]`, `value` (index), `extra.{autoplay,interval}` | slideshow |
| `table` | data | `head[]`, `body[]`, `value`, `type`, `submitOnChange`, `submitOnScroll` | `type`: `default` `radio` `check`; row groups; cell buttons/checkboxes/copy |
| `tab` | nav | `options[]`, `value` (selected), `submitOnChange` | tab switcher with per-tab error |
| `stepper` | nav | `options[]`, `value` (step), `extra.direction` | step indicator (wizard) |
| `mainMenu` | nav | `options[]`, `value` | nested menu |
| `comments` | display | `value`, `title` | comment thread widget |
| `timer` | display | `value` (remaining ms), `extra.duration` | countdown |
| `widget` | embed | `type`, `extra` | third-party: `iframe` `onfido` `twilio` `amazonConnect` `webComments` (swagger `Widget-*`) |
| `row` | layout | *(synthesized)* | horizontal group; **not hand-authored** — the renderer builds it from sibling items sharing a `row` field (see §4). Real `ComponentClasses.row`, but absent from the swagger. |
| `draggable` | layout | `items[]`, `value` (order) | standalone drag-reorder list (dnd-kit). Real `ComponentClasses.draggable`; distinct from a section's `sortable`+`contentLoop`. Absent from the swagger — verify against a live render. |

> Field-level schemas (exact `extra` keys, examples, per-variant required fields — e.g.
> `Edit-int` vs `Edit-date`, `Table-group`, `Widget-onfido`) are enumerated in the canonical
> swagger ("Simulator.Company Scripts", `components.schemas`). Treat that spec as the source of
> truth for individual component options.

> **`mainMenu` inline-vs-popover depth** — `mainMenu.extra.maxInlineDepth` (default **1**) sets how
> deep branches expand **inline** (accordion, via native `<details>`); levels below it open as a
> **hover popover** instead, so raise it for a full inline accordion. `extra.submitOnBranchToggle`
> (default `false`) toggles a branch client-side with no `/send`; `true` posts `action:expand|collapse`
> on each toggle. `extra.autoExpandActive` auto-opens the ancestors of the active node.

---

## 6. Templating & data binding

Five mechanisms (swagger tags `viewModel`, `locale`, `definitions`, `contentLoop`, `bbcode`)
shape the page **before** it reaches the renderer. All are resolved server-side
(`pageUtils.ts` / `pages.ts`); the client receives concrete values.

- **`viewModel`** — a key/value bag. The app-wide `viewModel` file is merged with the
  per-request `viewModel` returned by the Corezoid process; values fill `{{token}}`
  placeholders in the config.
- **`locale`** — i18n strings keyed by language. The app `locale` and the page `locale` are
  merged and resolved for the active `language`; values fill `[[token]]` placeholders.
  (`createPageData` merges `{ ...appLocale, ...pageLocale }` and `{ ...appViewModel,
  ...viewModel }`.)
- **`definitions`** (`$ref`) — reusable component fragments under the `definitions/` folder.
  An item `{ "$ref": "#/button" }` is replaced server-side by the referenced definition's keys
  (`getDefinitionByXpath` walks the xpath; `replaceRef` inlines it). The default skeleton ships
  a `definitions/button` fragment (see [`smart-forms.md` §2.2](smart-forms.md)).
- **`contentLoop`** — a section can declare a `contentLoop` that the server expands into
  repeated `content` items (one per data row), so a single template renders a list. On submit
  responses, `replaceContentLoopWithContent` re-expands the loop for the affected form.
- **`bbcode`** — `label`/`button`/`edit`/`check` titles support BBCode, rendered to HTML by the
  client (`Utils.bbCodeToHtml`). Supported tags (verified live): `[b]` `[i]` `[u]` `[color=#rgb]`
  `[size=N]` `[br]`, and **`[url=https://…]text[/url]`** which renders a **clickable
  `<a target="_blank">`** — the idiomatic way to put an inline text link in a form. Raw HTML in a
  value (e.g. `<a href>`) is **escaped** and shown as literal text, so use `[url]`, not `<a>`. (`[url]`
  opens a new tab; the renderer also supports `[iurl=…]…[/iurl]` for a same-tab link, plus more tags such
  as `[bg=…]`, `[sup]`, `[ul]`/`[*]` — the list above is the common subset.) For a whole-button
  link/action use `button` `extra.url` (§5). Build entity URLs with the `buildLink` MCP tool.

> **`[[ ]]` is locale, `{{ }}` is view-model.** Both are substituted server-side in
> `renderPage`; the renderer does no token interpolation itself.

---

## 7. The change protocol (200 responses)

A `send` 200 returns `changes[]` — surgical patches applied to the live page without a full
re-render (renderer `handleResponseChanges`, applied at form / section / item granularity).

```jsonc
{
  "id": "<target id>",          // form, section, or item id
  "class": "edit",              // component class (for item changes)
  "value": <new value>,
  "visibility": "visible|disabled|hidden",
  "styleClass": "…",
  "error": true, "required": false,
  "options": [ /* for select/multiselect/radio/tab */ ],
  "changeRules": {              // how to merge arrays instead of replace
    "options":   { "action": "concat|unshift|delete|replace|merge" },
    "visibility":{ "action": "…" }
  },
  "skipSubmitOnChange": true,   // suppress the implicit submitOnChange this change would trigger
  "formId": "…"
}
```

- **Form-level** changes set `visibility`/`styleClass` on a whole form.
- **Section-level** changes replace `content`, toggle `sortable`/`visibility`, bump
  `contentVersion`.
- **Item-level** changes set `value`/`visibility`/`error`/`required`/`options`.
- **`changeRules.action`** (renderer `ArrayAction`) controls array merges for `options`:
  `concat` (append), `unshift` (prepend), `delete`, `replace`, `merge`.
- **`skipSubmitOnChange`** prevents a feedback loop when a change updates a value on a
  `submitOnChange` component.

`ctrl[]` entries in a 200 response are forwarded as `postMessage` to the parent frame
(host-app integration hook).

---

## 8. Client-side validation

The renderer validates inputs before allowing submit; the server is the final authority and
sets `error`/`errorMsg` that the client only displays.

- **required** — a visible, value-bearing, required item with an empty/invalid value
  auto-disables the form's submit buttons (`isSubmitAllowedToAutoToggle`).
- **regexp** — `edit` and `phone` validate against the item's `regexp`.
- **phone** — error if the `regexp` fails on `countryCode+number`, or if required parts are
  empty.
- **otp** — length clamped to 2–20; value compressed to a single string on submit.
- **type coercion** — `edit[int|float]`, `slider` (min/max/step), `upload` (`accept`,
  `minSize`/`maxSize`) enforced client-side; server re-checks.

---

## 9. Page authoring files (recap)

A page's on-disk form (per env, see [`smart-forms.md` §2.2](smart-forms.md)):

```
pages/<pageId>/
  config   (application/json)  ← the Page template: { grid, forms[] }
  locale   (application/json)  ← page-scoped i18n strings
root (app-wide):
  viewModel  (application/json)
  locale     (application/json)
  definitions/  ($ref fragments)
  styles/       (Less → compiled to scoped CSS; styles tag)
  widgets    (application/json — control/widget settings)
```

`config` is the page; `viewModel`/`locale`/`definitions` feed templating (§6); `styles` is
compiled to CSS at serve time (swagger `styles` tag — see
[`smart-forms.md` §4.7](smart-forms.md)).

---

## 10. Server-side validation of saved pages

⚠️ **Important for authoring tools and AI agents:** when a page's files are saved via
`app_content` (`PUT/POST /papi/1.0/app_content/:actorId`), pong-server validates the **envelope**
but **not** the page content. The `source` (the `config` JSON, `viewModel`, `locale`,
`definitions`, CSS) is stored as an **opaque string**.

**What IS validated** (Fastify schema `schemas/applications/appContent.js` + handler
`appContent.js`):

| Check | Where |
|---|---|
| `objType` ∈ {`folder`,`file`} (enum) | `createObj`/`updateObj` schema |
| `title` required string; for files `type` (MIME) required, `source` optional string | schema (`ifThen objType=file`) |
| `folderId` required integer; `id` required integer on update | schema |
| Folder exists **and belongs to this actor** | `validate()` |
| Env is **not readonly** (production rejected) | `validate()` / `removeObjsReq` |
| **System objects**: title immutable, cannot be deleted | `updateObjsReq` / `removeObjsReq` |
| `minItems: 1` on the objects array | schema |

**What is NOT validated** (stored as-is, best-effort at serve time):

- ❌ **Page `config` structure** — grid type, component `class` enums, required keys,
  `forms`/`sections` shape are **not** checked against the protocol. Any JSON (or non-JSON) is
  accepted into `source`.
- ❌ **`viewModel` / `locale` / `definitions` structure** — arbitrary JSON.
- ❌ **`$ref` validity** — a `$ref` to a missing definition resolves to `{}` at serve time
  (`getDefinitionByXpath` → `JSON.parse` in try/catch).
- ❌ **CSS syntax** — not validated on save; compiled with Less at serve time, and a compile
  error is emitted as a **CSS comment** (`/* Less Error … */`) rather than failing the page.
- ❌ **No `maxLength` / size cap** on `source` (bounded only by the PostgreSQL `TEXT` column and
  Fastify's HTTP body limit); no `maxItems` on the batch; no MIME-type allow-list; no tree-depth
  limit.

**Resilience at serve time** (`pages.ts` / `pageUtils.ts`): malformed `config`/`viewModel`/
`locale`/`definition` JSON is caught and defaults to `{}` (`getFileByTitle`,
`getDefinitionByXpath` use `JSON.parse` in try/catch); missing keys can still surface as runtime
gaps in `renderPage`. **The contract is enforced by convention and by the renderer, not by the
save endpoint** — so authoring tooling should validate page JSON against the canonical swagger
schemas *before* saving.

Smart-form **install/import** (`smartForms.js`) likewise validates only the request envelope
(`fileUrl`, `ref`, `title` required; `ref` uniqueness) — the imported package's page content is
not schema-checked at the API layer.

---

## 11. Canonical spec inventory

The "Simulator.Company Scripts" OpenAPI groups the protocol as:

- **Protocol mechanics (tags):** `get`, `send`, `viewModel`, `locale`, `definitions`,
  `contentLoop`, `bbcode`, `styles`, `realtime`.
- **Structure (tags / schemas):** `page` (`Page`, `Page-grid-one-column`,
  `Page-grid-two-column`, `Page-header-default`, `Page-header-steps`), `form` (`Form`,
  `Form-grid-one-column`, `Form-grid-two-column`).
- **Responses (schemas):** `Get-response-200`, `Send-response-200`, `Send-response-205`,
  `Send-response-302`.
- **Components (tags + schemas):** `stepper` `button` `toggle` `slider` `check` `label`
  `divider` `image` `file` `carousel` `copy` `upload` `attachment` `widget` `edit` `select`
  `multiselect` `radio` `phone` `tab` `comments` `table` `otp` `signature` `mainMenu` `timer`
  (with variant schemas such as `Edit-int`/`Edit-date`/`Edit-colorPicker`,
  `Table-default`/`-check`/`-radio`/`-group`, `Widget-onfido`/`-twilio`/`-amazonConnect`/
  `-webComments`).

For exact per-field types, enums, and examples, consult that spec; this document is the
narrative map over it.

---

## 12. Rendering gotchas (hard-won, verified live in control-cdu)

These are non-obvious layout/behaviour facts confirmed by inspecting the live renderer DOM. They
save real debugging time.

### 12.1 `row` is a CSS **table** — it does NOT wrap
The `row` layout component renders as `display:table` with each item a `table-cell`. Items stay on
ONE line and **overflow** the container; `flex-wrap` has no effect. For a responsive grid that wraps
(side-by-side when space allows, stacking when narrow), do **not** use `row`. Put the items directly
in the section `content` and give each a `styleClass` of
`display:inline-block; width:calc(50% - 8px); min-width:190px; vertical-align:top`. inline-block
items flow horizontally and wrap to the next line naturally. (Caveat: on mobile widths `row` already
collapses to a single column via a `respond-to(mobile)` rule — the no-wrap/overflow behaviour above is
desktop-only.)

### 12.2 `styleClass` on a `row` is dropped; on leaf components it applies
A `styleClass` set on a `row` does **not** reach the DOM — there is no first-class "row object": a
`row` is an implicit grouping of items sharing the same `row` value, and the generated `<Row>` wrapper
applies no author class. A `styleClass` on a normal component (`upload`, `label`, `button`,
`select`, `edit`, …) **is** emitted onto that element. Style the leaf elements, never the `row`
wrapper.

> **Workaround — give a row its own class:** the `row` value is space-separated. The first token is
> the row id (→ `.row__<id>`), and every **extra token the renderer emits as a literal class** on the
> generated row wrapper. So `row:"1 my_row"` puts `.my_row` on the wrapper — the one stable hook you
> can style a whole row by (and reuse across rows: `row:"1 my_row"` + `row:"2 my_row"` share `.my_row`).

### 12.3 No client-side conditional visibility — reveal = `submitOnChange` + 200 `changes`
`visibility` is a static enum (`visible|disabled|hidden`); there is no expression/binding language,
so a field cannot show/hide reactively from another field's value on the client. The only way to
"reveal B when A changes" is a server round-trip: set `submitOnChange:true` on A; the `/send` handler
returns **200** with `changes:[{id:"B", visibility:"visible|hidden"}]` (see §7). A `submitOnChange`
event posts with `buttonId = <element id>` (not a button). **Read the new value from
`body.data.<fieldId>`** — that is the reliable source across components. `body.buttonData.value` is
populated **only by `select`** (and a few components); `radio`, `edit`, `check`, `toggle`, etc. send no
`buttonData`, so `buttonData.value` is `undefined` for them.

### 12.4 A `submitOnChange` dispatch must cover EVERY `submitOnChange` field id
A `submitOnChange` event arrives at `/send` exactly like a button click. Any `submitOnChange` field id
your handler does **not** branch on will **fall through to the normal submit path** (page nav, actor
write). In a chain (e.g. progressive upload slots `up_file … up_file5`), the LAST field — which has no
"next" to reveal — still needs an explicit no-op onChange branch, otherwise selecting/finishing it
navigates the wizard. Route on `body.buttonId`; treat all onChange ids as onChange even when there is
nothing to change.

### 12.5 `radio`/`select` option internals — restylable via JSS-prefix selectors, but fragile
Unlike a `row` (§12.2), a `styleClass` **does** reach a `radio`/`select` container. Each option renders
(verified live) as
`<div class="i-content-…"><input type="radio" class="i-input-…"><i class="i-icon-…">…<label class="i-label-…"></div>`
— so the visible "dot" is the `<i class="i-icon-…">`, **not** the `<input>` (which is why
`input{appearance:none}` does nothing). You CAN hide the dot and card-ify the options (confirmed in DOM):

```less
.my-radio [class*="icon"]    { display:none !important; }          // hide the dot
.my-radio [class*="content"] { display:block; border:1px solid #E5E7EB; border-radius:12px; padding:14px; margin:8px 0; }
.my-radio .checked [class*="content"] { border-color:#3B5BDB; background:#EEF2FF; }  // selected option
```

**Selected state — use the `.checked` class, NOT `:has(input:checked)`.** The renderer puts a literal
`checked` class on the option's outer `<div>`; the `<input>` carries no `checked`/`name` attribute, so
`:has(input:checked)` would highlight **nothing** on a server-preselected value and **several** options
after clicks. Target `.checked` (a plain string, more stable than the JSS fragments). The
`content`/`icon`/`label` class fragments are react-jss-generated and the prefix (`i-` in one build) is
**not** stable across builds — match by substring (`[class*="content"]`), not the full `i-content`.
Because this couples to renderer internals, the robust alternative is one `button` per option (fully
styled via `styleClass`, selected highlight from the server via `changes[].styleClass`, chosen value kept
in a hidden `edit` carrier per §12.6). Both work: radio + substring selectors is less markup; the button
approach is version-proof.

### 12.6 A visually-hidden field still submits if `visibility` stays `visible`
Form data is collected from items whose `visibility` is not `hidden` — it does **not** consult CSS.
So an `edit` with `visibility:"visible"` but a `styleClass` that hides it in CSS
(`position:absolute; width:1px; clip:rect(0 0 0 0)`) is invisible yet **still submitted** — the
idiomatic carrier for a value set by `changes[]` (e.g. the selection behind button-cards). A field
with `visibility:"hidden"` is NOT submitted.

### 12.7 `table type:"group"` hides ungrouped rows; its pager sits outside the rows
Numbered table pagination requires **`type:"group"`** (a `default` table has no page controls) with
`extra.page` / `extra.totalPages`. Three traps: **(1)** `type:"group"` **hides every row without a
`groupValue`** — switching a `default` table to `group` without also giving each `body` row a
`groupValue` (and declaring ≥1 group in `extra.groups`) leaves an empty table with only the pager.
**(2)** An empty group still renders a group-title header row (`[class*="table__row__group"]`) — hide
it when the group is a technical, title-less one. **(3)** The pager renders as a **separate block, not
inside the rows**, so it needs CSS to sit under the table (DOM hooks in `cdu-dom-tree-reference.md` §7;
positioning recipe in the `simulator-styles` skill).

---

## Related documentation

- [Smart Forms (Applications / CDU)](smart-forms.md) — lifecycle, data model, deploy/release,
  access, and the `pages` serving pipeline that produces the JSON described here.
- [Custom Car Form](custom-car-form.md) — a *data* form (actor schema), contrasted with a Smart
  Form *app*.
- Renderer: `control-cdu` (`corezoid-driven-ui`). Server: `pong-server`
  `controlMain/api/applications/` (`pages.ts`, `appContent.js`, `cssCompiler.ts`,
  `helpers/applications/pageUtils.ts`).
- Canonical contract: "Simulator.Company Scripts" OpenAPI (Corezoid Sync API).
