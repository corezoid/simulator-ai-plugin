---
name: simulator-smart-forms
description: >
  Simulator.Company Smart Form (CDU / Script / Application) specialist.
  Use when the user wants to create, edit, inspect, or deploy a Smart Form;
  work with its pages, locale, viewModel, styles, definitions, or widgets;
  understand the CDU page protocol; pull or push form files; manage releases;
  or ask questions about the app_content / applications / releases / file_history
  APIs. Activate on: "smart form", "CDU", "script application", "pullSmartForm",
  "pushSmartForm", "page config", "viewModel", "locale", "deploy smart form",
  "edit page layout", "add component to CDU", "rollback release".
---

# Simulator.Company Smart Form Author

You are a specialist in creating and editing **Smart Forms** (also called CDU,
Script, or Application) on the Simulator.Company platform, using the `simulator`
MCP server plus the `pullSmartForm` / `pushSmartForm` engine tools.

---

## Core Concepts

A **Smart Form** is three things in one:

| Layer | What it is |
|---|---|
| **Actor** | An actor in the `scripts` system form — has `id`, `ref`, `title`, `data.sharedWith` |
| **Versioned project** | A folder/file tree per environment: pages, locale, viewModel, styles, definitions, widgets |
| **Backend binding** | Corezoid credentials per env — dynamic data and control flow come from Corezoid at runtime |

A Smart Form is **not** a data-schema form. Compare:

```
Regular Form (template)        Smart Form (actor + app)
──────────────────────         ───────────────────────
defines field schema           is itself an actor in "scripts" form
actors are instances of it     carries pages/styles/i18n as versioned files
account definitions            Corezoid process supplies runtime data
```

---

## Environments

Every Smart Form always has exactly **two environments**:

| Env | `readonly` | Purpose |
|---|---|---|
| `develop` | `false` | Active editing target |
| `production` | `true` | Live, served to end users |

**Rule:** never write directly to `production`. Edit `develop`, then deploy
`develop → production` via a release. The server rejects writes to readonly envs.

---

## Project File Structure

After `pullSmartForm` the local directory looks like:

```
<actorId>/
  develop/
    .manifest.json              ← file IDs + SHA-256 hashes (used by pushSmartForm)
    pages/
      index/
        config                  ← Page layout (JSON): { grid, forms[] }
        locale                  ← Page-scoped i18n (JSON): { key: { en: "…", uk: "…" } }
    locale                      ← App-wide i18n (JSON)
    viewModel                   ← Default view-model values (JSON)
    widgets                     ← Widget/control settings (JSON, ctrlSettings)
    definitions/
      button                    ← Reusable component fragment (JSON), used via "$ref": "#/button"
    styles/
      index                     ← Less stylesheet (text/css); compiled → scoped CSS at serve time
  production/
    .manifest.json
    … (same tree, read-only)
```

### File roles

| File | MIME | Purpose |
|---|---|---|
| `pages/<id>/config` | `application/json` | Page layout: grid + forms + sections + items |
| `pages/<id>/locale` | `application/json` | Page-level i18n strings |
| `locale` | `application/json` | App-wide i18n strings; merged with page locale at serve time |
| `viewModel` | `application/json` | Default values; merged with Corezoid-supplied viewModel at serve time |
| `definitions/<name>` | `application/json` | Reusable component fragments; inlined by `$ref` at serve time |
| `styles/index` | `text/css` | Less source; compiled to scoped CSS (`.cdu-page` scope); `@import "pages/<id>/style"` works |
| `widgets` | `application/json` | ctrlSettings for embedded third-party widgets |

---

## Standard Workflow

### Creating a new Smart Form

```
createSmartForm(title="My App", ref="my-app")
→ { actorId: "...", envs: [{id: 1, title: "develop"}, {id: 2, title: "production"}] }
```

`corezoidCredentials` is optional — omit it for static/design-only forms and configure the Corezoid binding later.

Then immediately pull the default skeleton:

```
pullSmartForm(actorId="<uuid>")
→ downloads all envs to <actorId>/develop/ and <actorId>/production/
```

### Edit cycle (existing or newly created form)

```
1.  pullSmartForm(actorId="<uuid>")
    → downloads all envs to <actorId>/develop/ and <actorId>/production/
    → writes .manifest.json (file IDs + hashes) in each env dir

2.  Edit files under <actorId>/develop/
    (page config, locale, viewModel, styles, definitions)

3.  pushSmartForm(actorId="<uuid>")
    → walks <actorId>/develop/, diffs every file/folder against .manifest.json
    → validates new + changed files against the CDU page protocol schema
    → if errors: aborts with { validationErrors: [...] } — fix and retry
    → POSTs new folders (parents first) and new files, then PUTs modified files
    → updates .manifest.json with returned ids + content hashes
    → returns { created: { folders, files }, updated, unchanged, orphanFiles }

4.  Deploy (when ready to publish):
    deploySmartForm(actorId="<uuid>")
    → deploys develop → production; returns { releaseId, releaseNumber, status }
```

---

## Page Config Format (`pages/<id>/config`)

The page `config` is the layout template. Structure: **Page → Grid → Form → Section → Item**.

### Minimal page

```json
{
  "grid": {
    "type": "one_column",
    "components": {
      "center": ["main"]
    }
  },
  "forms": [
    {
      "id": "main",
      "title": "My Form",
      "sections": [
        {
          "id": "body",
          "type": "body",
          "content": [
            {
              "id": "greeting",
              "class": "label",
              "value": "[[hello]]"
            },
            {
              "id": "name",
              "class": "edit",
              "value": "{{defaultName}}",
              "type": "text",
              "placeholder": "Enter your name",
              "required": true
            },
            {
              "id": "submit",
              "class": "button",
              "title": "Submit",
              "type": "default"
            }
          ]
        }
      ]
    }
  ]
}
```

### Grid

```jsonc
{
  "type": "one_column" | "two_column",
  "header": {
    "class": "default" | "steps",
    "extra": { "steps": ["Step 1", "Step 2"], "active": 1 }
  },
  "components": {
    "header":  ["<formId>"],
    "left":    ["<formId>"],
    "center":  ["<formId>"],
    "right":   ["<formId>"],
    "footer":  ["<formId>"],
    "sidebar": ["<formId>"]
  },
  "styleClass": "custom-grid"
}
```

### Form

```jsonc
{
  "id": "info",
  "title": "Details",
  "styleClass": "card",
  "visibility": "visible",    // "visible" | "disabled" | "hidden"
  "sections": [ /* Section[] */ ]
}
```

### Section

```jsonc
{
  "id": "s1",
  "type": "body",             // "body" | "block" | "modal" | "float"
  "visibility": "visible",
  "header":  [ /* Item[] — Label | Button only */ ],
  "content": [ /* Item[] — the main items */ ]
  // modal-only: "modalHeader": [ /* Item[] */ ],
  //             "modalSize": "small"|"medium"|"large"|"xlarge",
  //             "modalCloseConfirmText": "…"
}
```

`block` renders as a grouped card; `modal`/`float` are overlays.

> ⚠️ **A section has no `footer`** — only `header` and `content` (plus `modalHeader` for modals).
> Put bottom-of-form actions as the last items in `content`, or in a separate form bound to the
> grid's `footer` region. (The grid's `components.footer` above holds **form ids**, not items.)

---

## Component Catalogue

Every item has `class` + base fields (`id`, `value`, `visibility`, `required`, `error`,
`errorMsg`, `styleClass`, `row`, `w`). Below are the most common components:

> **Layout/rendering gotchas** (verified live — full list in
> `docs/user-flows/cdu-page-protocol.md` §12): `row` renders as a CSS **table** so its items
> **do not wrap** (use `display:inline-block` items directly in `content` for a wrapping grid);
> `styleClass` on a `row` is dropped from the DOM (style leaf items, not the `row`); a `radio`/`select`
> option dot is a JSS-hashed `<i class="i-icon-…">` (not the `<input>`) — restylable via
> `[class*="icon"]`/`[class*="content"]` (match by substring; the `i-` prefix isn't stable across builds) but that couples to renderer internals, so `button`s are
> the version-proof alternative for card pickers; and a field with `visibility:"visible"` but hidden via
> CSS **still submits** (the idiomatic hidden value carrier).

### Input components

| `class` | `value` type | Key `extra` / options |
|---|---|---|
| `edit` | string | `type`: `text` `email` `int` `float` `phone` `multiline` `date` `password` `colorPicker`; `placeholder`, `regexp`, `mask`, `submitOnEnter` |
| `select` | string | `options: [{title, value, visibility, icon, tooltip, avatar, badge, styleClass}]`; `type`: `default` `autocomplete`; `submitOnChange` |
| `multiselect` | string[] | `options: [{title, value, visibility, tooltip}]`; `extra.length` (max) |
| `radio` | string | `options: [{title, value, visibility}]`; `extra.direction`: `row`\|`column` |
| `check` | boolean | checkbox |
| `toggle` | boolean | `title` label on the switch |
| `slider` | number | `extra: { min, max, step }` |
| `phone` | `{countryCode, number}` | `options` (country codes), `regexp` |
| `otp` | object `otp-0…otp-N` | `extra.length` (2–20); `type`: `text`\|`int` |

### Display components

| `class` | Notes |
|---|---|
| `label` | Static text; supports `[[locale]]` and `{{viewModel}}` tokens; BBCode rendered; `align`: `left`\|`center`\|`right` |
| `divider` | Visual separator; no value |
| `image` | `value` = src URL; `extra: {alt, height, width}` |
| `carousel` | `items[]` (slides); `extra: {autoplay, interval}` |
| `timer` | `value` = remaining ms; `extra.duration` |
| `comments` | Comment thread widget; `title` |

### Action components

| `class` | Notes |
|---|---|
| `button` | `title` (bbcode), `type`: `default` `secondary` `tertiary` `text` `error`; submits its form. `extra.url` opens the URL **instead of submitting** (`extra.target` `_self`\|`_blank` — newer renderers only; older open same-tab); `extra.action:'logout'`; `extra.request` (bare fetch first, submits only if it resolves); `extra.autoSubmit {interval,maxCount}` (interval clamped 5–60s); `extra.options[]` (menu — bypasses `url`/`request`/`action`/submit); `extra.icon`, `rounded`, `mobileVisible` |
| `copy` | `value` = text to copy; `title` = button label |

### Data & navigation components

| `class` | Notes |
|---|---|
| `table` | `head: [{id,title}]`, `body: [{<id>: value}]`; `type`: `default` `radio` `check`; `submitOnChange`, `submitOnScroll` |
| `tab` | `options: [{id,title}]`; `value` = selected id; `submitOnChange` |
| `stepper` | `options: [{id,title}]`; `value` = step; `extra.direction` |
| `mainMenu` | Nested navigation; `options` tree |

### File components

| `class` | Notes |
|---|---|
| `file` | Preview/download; `value: FileProps`; `extra: {downloadUrl, auth}` |
| `upload` | File upload; `type`: `default`\|`webcam`; `extra: {accept, minSize, maxSize}` |
| `attachment` | Multi-file viewer; `value: FileProps[]`; `extra.downloadUrl` |
| `signature` | Canvas signature → base64; `extra: {strokeStyle, saveButtonTitle}` |

### Layout

**Do not hand-author a `row` component.** Lay items out with the **base `row` / `w` fields** that
every component carries: give sibling items the same `row` string to place them on one line, and
set `w` (relative **weight** — rendered width is `w / Σw` of the row) on each. The renderer
synthesizes a `row` component from them. For drag-reorder use a **section** with `sortable: true` +
`contentLoop` (there is also a standalone `draggable` component, but the section route is the usual
one). `row` and `draggable` are absent from the Scripts swagger, so prefer these field-based paths.

```jsonc
// two items on one line, 50% each (equal weights → each gets half the row):
{ "id": "first", "class": "edit", "type": "text", "row": "1", "w": "50" }
{ "id": "last",  "class": "edit", "type": "text", "row": "1", "w": "50" }
```

### Embedded widgets

`class: "widget"` — `type`: `iframe` `onfido` `twilio` `amazonConnect` `webComments`. Each type has its own `extra` schema.

---

## Templating

All substitution is **server-side** — the renderer receives concrete values.

| Syntax | Source | Example |
|---|---|---|
| `[[key]]` | `locale` (app + page merged) | `"[[hello]]"` → `"Hello"` in en |
| `{{key}}` | `viewModel` (default + Corezoid-supplied merged) | `"{{userName}}"` → `"Alice"` |
| `"$ref": "#/button"` | `definitions/button` file | inlined at serve time |
| `contentLoop` | section array expansion | one template → N rows |
| BBCode | `label`/`button`/`edit`/`check` titles | `[b] [i] [u] [color=#f00] [size=N] [br]`, and `[url=https://…]text[/url]` → clickable `<a target="_blank">` (inline link; `[iurl=…]` opens same-tab; renderer supports more, e.g. `[bg]`). Raw `<a>` HTML is escaped — use `[url]`. |

### locale file format

```json
{
  "hello": { "en": "Hello", "uk": "Привіт" },
  "submit": { "en": "Submit", "uk": "Надіслати" }
}
```

### viewModel file format

```json
{
  "defaultName": "Anonymous",
  "maxItems": 10
}
```

### definitions fragment format

```json
{
  "class": "button",
  "title": "[[submit]]",
  "type": "default"
}
```

Used in config as `{ "$ref": "#/button" }` — the whole object is replaced.

---

## Change Protocol (POST 200 response)

When Corezoid returns `code: 200`, the server sends `changes[]` — surgical patches
to the live page without a full re-render:

```jsonc
[
  {
    "id": "name",             // item / section / form id
    "class": "edit",          // component class (for item changes)
    "value": "Alice",
    "visibility": "visible",
    "error": false,
    "required": true
  },
  {
    "id": "status",
    "options": [{"id":"a","title":"Active"},{"id":"i","title":"Inactive"}],
    "changeRules": {
      "options": { "action": "replace" }   // concat | unshift | delete | replace | merge
    }
  }
]
```

`code: 205` → re-render a whole page (can switch to a different `pageId`).
`code: 302` → redirect to `nextPage`.

---

## Creating a Smart Form from Scratch

Use the `createSmartForm` MCP tool — it calls `POST /papi/1.0/applications/<accId>` internally
and returns the actor ID plus both env IDs ready for use.

```
createSmartForm(
  title="My App",
  ref="my-app",
  description="Optional",          // optional
  sharedWith="userList",            // optional, default userList
  apiLogin="...",                   // optional — set Corezoid binding later if omitted
  apiSecret="...",
  procId="...",
  companyId="..."
)
→ { actorId: "...", ref: "my-app", title: "My App",
    envs: [{ id: 12, title: "develop", readonly: false },
           { id: 13, title: "production", readonly: true }],
    next: "run pullSmartForm(actorId=...) to download the initial file tree" }
```

`sharedWith` values: `userList` | `allWorkspaceUsers` | `allRegisteredUsers` | `anyone`.

Corezoid credentials are optional at creation time and can be configured later.

After creation always run `pullSmartForm` to download the default file skeleton before editing.

---

## Working with Releases

### Deploy develop → production

```
deploySmartForm(actorId="<uuid>")
→ { actorId, sourceEnv: "develop", targetEnv: "production",
    releaseId, releaseNumber, status: "active" }
```

`sourceEnv` and `targetEnv` default to `develop` and `production`; pass them explicitly
to deploy between non-standard envs.

### List releases

```
listReleases(actorId="<uuid>")           // production releases (default)
listReleases(actorId="<uuid>", env="develop")
→ { actorId, env, releases: [{ id, release_number, status, created_at, … }] }
```

### Diff two releases

```
diffReleases(actorId="<uuid>", releaseId="5", vsReleaseId="3")
→ { added[], removed[], modified[] }
```

Compared by `source_hash` — no file bytes transferred. Use before a rollback to preview
what will change.

### Rollback to a previous release

```
rollbackRelease(actorId="<uuid>", releaseId="3")
→ { actorId, rolledBackTo: "3", newReleaseId, releaseNumber, status: "active" }
```

Rollback is **forward-only**: a new `active` release is created whose content equals
the target release. History is never rewritten.

**Retention:** 5 releases per env (objects). Older release manifests remain for audit;
only objects are GC'd. A release outside the 5-release window cannot be rolled back.

---

## File History

```
// List versions of a file (fileId from .manifest.json)
getFileHistory(actorId="<uuid>", fileId=12345)
getFileHistory(actorId="<uuid>", fileId=12345, limit=20, offset=0)
→ list of { versionId, operation, createdAt, … }

// Fetch source of a specific version
getFileVersion(actorId="<uuid>", fileId=12345, versionId="<id>")
→ { source: "…full file content…" }

// Restore file to a prior version (creates a new version; run pullSmartForm to refresh local)
rollbackFile(actorId="<uuid>", fileId=12345, versionId="<id>")

// List soft-deleted objects in an env
listTrash(actorId="<uuid>")              // develop (default)
listTrash(actorId="<uuid>", env="production")
→ list of { objectId, title, objType, deletedAt, … }

// Restore a deleted object
restoreFromTrash(actorId="<uuid>", objectId="<id>")
```

All writes create a before-state history row (`operation`: `create`|`update`|`move`|`rename`|`delete`).
Retention: 50 versions per file.

---

## Key Rules for Authoring

1. **Edit only `develop`** — the server rejects writes to `production` (readonly env).
2. **Run `pullSmartForm` before editing** — establishes `.manifest.json` needed by `pushSmartForm`.
3. **Server stores `source` opaque** — no structural validation of `config`, `viewModel`, or `locale` JSON at save time. Validate page JSON against the CDU protocol before pushing.
4. **Missing `$ref` resolves to `{}`** — a `definitions/button` reference to a non-existent fragment silently produces an empty object at serve time. Always verify definition names.
5. **System files are protected** — `is_system` files (`styles/index`, `pages/index/config`, etc.) cannot be renamed or deleted. Edit their content freely; rename/delete will fail.
6. **CSS is Less** — `styles/index` is compiled with Less at serve time, wrapped in `.cdu-page {}`. Use Less syntax; `@import "pages/<id>/style"` imports page-level stylesheets.
7. **Dynamic data comes from Corezoid** — the Smart Form files define layout and defaults; Corezoid processes supply runtime `viewModel` values and control flow (`code` 200/205/302).
8. **Deploy is a two-phase snapshot** — T1 locks the target env and takes a snapshot; T2 materialises it. A failed T2 is compensated automatically.

---

## Typical Session Example

```
// 1. Pull all files to develop/
pullSmartForm(actorId="69bbd03e-0d4c-4122-9234-e06ffe9ca1eb")
→ { envs: [{ env: "develop", dir: "…/develop", files: 11 }, { env: "production", … }] }

// 2. Edit pages/index/config — add a new label item to the body section

// 3. Push changes (also creates new pages/folders if you added them locally)
pushSmartForm(actorId="69bbd03e-0d4c-4122-9234-e06ffe9ca1eb")
→ { created: { folders: 0, files: 0 }, updated: 1, unchanged: 10, orphanFiles: [] }

// Adding a new page? Just create the files locally and push:
//   develop/pages/survey/config   (page layout JSON)
//   develop/pages/survey/locale   (page i18n JSON)
// → pushSmartForm POSTs the "survey" folder, then both files inside it,
//   then writes their ids back into .manifest.json. No PUT/PATCH needed.

// 4. Deploy to production
deploySmartForm(actorId="69bbd03e-0d4c-4122-9234-e06ffe9ca1eb")
→ { releaseId: 5, releaseNumber: 3, status: "active" }
```

---

## Embedding a Smart Form into an actor or a reaction

A Smart Form is an actor — but you can also **embed and run it inside another actor's card or
inside a reaction**, so the smart form shows up where the user is working:

- **In a regular actor:** set **`appId`** = the Smart Form actor id on `createActor` /
  `updateActor`; the actor's card then renders/runs that smart form. **`appSettings`** tunes it:
  `{ autorun:boolean, expired:int (unix s), users:int[], groups:int[], fullWidth:boolean }` (or null).
  ```
  updateActor(formId=<f>, actorId="<actorId>",
              appId="<smartFormActorId>", appSettings={ "autorun": true })
  ```
- **In a reaction:** same `appId` / `appSettings` on `createReaction` / `updateReaction` — the
  reaction renders the smart form (e.g. drop an interactive form into a discussion thread).
  ```
  createReaction(type="comment", actorId="<actor>", appId="<smartFormActorId>",
                 appSettings={ "fullWidth": true })
  ```
- The embedded form's pages/logic come from the Smart Form's **production** env by default;
  edit them with the pull/push cycle above.

> Also see `simulator-reactions` (the `appId` + `extra.linkedActorId` + `[application=…]` chip)
> and `docs/entities/reactions.md` → "Embedding".

## Access & Scopes (Public API)

All Smart Form operations use the public API (`/papi/1.0/...`) with an OAuth2 bearer token.

| Operations | Required scope |
|---|---|
| Read (get app, envs, releases, file history, env struct, folder content) | `actors.readonly` |
| Write (create app, edit content, deploy, rollback, restore trash) | `actors.management` |
| Page serving (render/submit pages) | `actors.readonly` + `sharedWith` rules |

`pullSmartForm` and `pushSmartForm` both require `actors.management` scope.

---

## Styling (hand-off)

This skill covers **page structure** — pages, locale, viewModel defaults, and the layout JSON
(grid/forms/sections/items) plus the `styleClass` **hooks** you attach to them. It does **not**
author the CSS/Less itself. When the user wants to **style / restyle / theme** a Smart Form —
write the `style` / `styles/` layer, re-skin a component, build a design system, fix
spacing/colors/fonts — switch to the **`simulator-styles`** skill. It reuses the same
pull/push/deploy cycle and consumes the `styleClass` hooks defined here.

## Adding Backend Logic

For the backend that produces dynamic `viewModel` values, handles `/send` submits, and
mutates the page via `changes[]`, switch to the **`simulator-smart-forms-logic`**
skill. It is a brief generator: it translates the user's intent into prompts for
the Corezoid plugin's `corezoid-create` / `corezoid-edit` skills, and binds the
resulting bound process to the Smart Form env via `corezoidCredentials` / `procId`.

---

## Reference Documents

| Path | When to read |
|---|---|
| `$CLAUDE_PLUGIN_ROOT/docs/user-flows/smart-forms.md` | Full lifecycle, data model, deploy/release internals, access model, API reference table |
| `$CLAUDE_PLUGIN_ROOT/docs/user-flows/cdu-page-protocol.md` | Complete component catalogue, templating, change protocol, server-side save validation |
| `$CLAUDE_PLUGIN_ROOT/skills/simulator-styles/SKILL.md` | Style/restyle the form: the `style` / `styles/` (Less) layer, theming, component re-skinning |
| `$CLAUDE_PLUGIN_ROOT/skills/simulator-smart-forms-logic/SKILL.md` | Author + bind the Corezoid backend processes for this Smart Form |
