---
name: simulator-styles
description: >
  Simulator.Company Smart Form (CDU) STYLING specialist — authoring complex Less/CSS for
  Smart Forms: theme tokens, page/form/section layout, component re-skinning, reusable style
  patterns, responsive and design-system approaches. Use when the user wants to STYLE or
  RESTYLE an existing Smart Form / CDU app — change its look, build a theme, style a table /
  sidebar / modal / form, add a design system, fix spacing/colors/fonts, or apply a complex
  visual design. This skill owns the `style` / `styles/` layer; it reuses the Smart Form tools
  (pullSmartForm / pushSmartForm / deploySmartForm) but does NOT create form templates — for the
  data-schema form template use `simulator-forms`, and for page layout / viewModel / backend
  logic use `simulator-smart-forms` / `simulator-smart-forms-logic`. Activate on: "style a smart
  form", "CDU styles", "theme the form", "restyle", "custom CSS/Less for the app", "style the
  table/sidebar/modal/button", "design system for the smart form", "make it look like …",
  "застилізувати смартформу", "стилі CDU", "тема для форми", "кастомний CSS", "оформити таблицю/
  сайдбар/модалку", "застилизовать смартформу", "стили CDU", "тема формы", "кастомный CSS",
  "оформить таблицу/сайдбар/модалку".
---

# Simulator.Company Smart Form STYLING specialist

You author and apply **CSS/Less styles** for Smart Forms (CDU / Script apps) on
Simulator.Company. Your domain is the **`style` / `styles/` layer** — themes, layout, component
re-skinning, reusable patterns, responsive and design-system approaches.

This skill is built on patterns reverse-engineered from real production Smart Forms; the recipes
below are taken from live Less, not invented.

---

## Scope — what this skill owns (and what it does NOT)

| Concern | Skill |
|---|---|
| **Styling**: `style` / `styles/*`, `pages/<id>/style`, `styleClass`, themes, Less | **this skill** |
| Form template (data fields / Account Template) | `simulator-forms` |
| Page layout JSON (`pages/<id>/config`: grid/forms/sections/items), viewModel, locale | `simulator-smart-forms` |
| Backend logic (Corezoid `/get` `/send`, dynamic viewModel, `changes[]`) | `simulator-smart-forms-logic` |

You **reuse** the Smart Form engine tools (`pullSmartForm`, `pushSmartForm`, `deploySmartForm`,
file-history/rollback) — you do not introduce new platform behaviour. When the user needs a *new
form/page* first, defer to `simulator-smart-forms`; you come in to make it look right. To attach a
`styleClass` to a component you may need a one-line edit to `pages/<id>/config` — that's in scope
(it's the binding), but designing the layout itself belongs to `simulator-smart-forms`.

---

## How Smart Form styling works (the model)

1. **One Less stylesheet, compiled per save, scoped to `.cdu-page`.** Whatever you write is wrapped
   in `.cdu-page { … }` at serve time, so `&` = the page root and your styles can't leak out. Less
   syntax (variables, mixins, `@import`, functions, maps, guards, `each()`) is fully supported.
2. **File organization — and where each rule goes.** Two layouts compile identically; **prefer the
   modular `styles/` folder** for anything non-trivial:
   - **Legacy single file** — a root `style` file holds everything (big forms: *Admin Panel*, *CMS*, *LMS*).
   - **Modular `styles/`** — `styles/index` is the entry point and mainly just `@import`s partials,
     **in cascade order**:
     ```less
     // styles/index — the entry/manifest; imports in cascade order
     @import "colors_fonts";   // tokens FIRST (everything below + page styles inherit them)
     @import "init_styles";    // then platform resets + project-wide component defaults
     // … any other shared partials (mixins, shared components) …
     ```
   - **Page styles** — `pages/<id>/style` is **auto-appended after** the main stylesheet (so it
     **wins the cascade**) and **inherits all root variables/mixins** (no `@import` needed). The
     platform saves it as `text/css` automatically. It is for **that page's exceptions only**.

   | File | What belongs here |
   |---|---|
   | `styles/index` | Just `@import`s (the entry/manifest), in cascade order. |
   | `colors_fonts` | Color + font **tokens** (`@color_*`, `@font_*`, `@font-face`). Define them here because everything else — including page styles — inherits them. |
   | `init_styles` | **(a)** neutralize the platform's default styling (resets); **(b)** project-wide **component defaults** — one base look reused on every page (e.g. a `.button` skin). |
   | `pages/<id>/style` | **Only the page's differences** from the shared styles — page layout + one-off component tweaks. Never define tokens here (they live in `colors_fonts`). |
3. **`styleClass` is the binding contract** between layout JSON and CSS. Every grid / form /
   section / item in `pages/<id>/config` may carry a `styleClass`; your CSS targets that class.
   - **Static** for structure: `"styleClass":"main_table"`.
   - **Dynamic** for backend-driven state/theme: `"styleClass":"{{settings_page_text_align}}"` —
     Corezoid pushes the value (e.g. `text_align_right`, `active_sidebar_btn`) to switch styling.
4. **Validation**: CSS is **not** validated on save; a Less compile error is emitted as a
   `/* Less Error … */` comment rather than breaking the page. `styleClass` values are never
   validated — a class with no matching rule is harmless (but dead; clean it up).
5. **Reaching renderer internals**: the public knobs are `styleClass` + documented component
   classes (`.button`, `.edit`, `.select`, `.table`, `.check`, …). For deeper structure use
   substring/attribute selectors — `[class*="table__wrap"]`,
   `[data-class="grid-one-column"]`.
   **Winning the cascade** — the renderer ships base/inline styles AND per-component CSS-module rules
   that load *after* your scoped `styles/index`, so an equal-specificity rule of yours **loses the
   tie by source order**. Beat it by **raising specificity with more stable classes, not ids**: chain
   your `styleClass` + an ancestor `[class*="…"]` + the element — e.g.
   `.book-table td.bc-cover-cell .file img` (0,3,2) beats the component's `.file__item__hash img`
   (0,2,1). Prefer this class chain over `#id` (brittle — it pins the rule to a config `id`, and
   classes are the house rule) and over a lone `!important` (it only beats non-`!important`; in an
   `!important` vs `!important` fight specificity still decides). A doubled class (`.x.x`) is a last
   resort. **See the verified per-component DOM map below for exact hooks (`edit`, `select`,
   `multiselect`, `radio`, `button`, `row__<name>`).**

   **The cascade math, with real numbers** (measured against a live renderer build — dump the
   defaults yourself to confirm, see Workflow §"dump the defaults"):
   - Your stylesheet is auto-wrapped in `.cdu-page`, so a **bare single-class rule you write is
     already `0,2,0`** (`.cdu-page .myClass`) and **beats a bare renderer default `.hashed` (`0,1,0`)
     for free** — most overrides need nothing more.
   - **BUT the renderer theme-scopes ~⅓ of its rules** (`.theme-light .x` / `.theme-dark .x`, on
     `#mainRoot`), and those are **also `0,2,0`** — a *tie* with your wrapped single-class rule, and
     since defaults load **after** you, **the theme default wins**. Symptom: a `background`/`color`
     override "does nothing" even though your selector clearly matches. Beat it with an extra class
     (`0,3,0`) or `!important`.
   - **~1 in 5 default rules already use `!important`** (heavy on `active`/`selected`/`checked`
     states and theme colors). Matching them with your own `!important` is **normal here, not a smell**
     — a lone `!important` still loses to a default `!important` of higher specificity, so pair it
     with a solid class chain.
   - **Slots own their own background/padding.** Containers like `.section__content`, the sidebar
     slots (`.sidebar__header/__content/__footer`), and `.page__sidebar` each ship their own bg/padding
     — painting the *parent* won't show through. Override the **slot**, not its ancestor.

> Authoritative component-class + CSS reference: the **"CSS styling"** tag in the CDU swagger and
> `$CLAUDE_PLUGIN_ROOT/docs/user-flows/cdu-page-protocol.md`.

---

## Rendered DOM map (verified component internals)

Verified against a live `control-cdu` render. The summary below is enough for most work; for the
**full per-component tag tree of every component** (incl. all 4 table types, page skeleton,
overlays) see `$CLAUDE_PLUGIN_ROOT/docs/user-flows/cdu-dom-tree-reference.md`. Use this to target
the right element instead of guessing. **Three rules first:**

1. **`styleClass` lands on the component ROOT.** E.g. `class="label hd-meta label__12ehI"`,
   `class="edit edit__text txt-input … bordered"`, `class="radio pills … horizontal"`. So a root
   hook (`.hd-meta`, `.txt-input`, `.pills`) always hits — but only the **outer** element.
2. **Internals use hashed CSS-module classes** (`label__12ehI`, `f-item-0-2-64`, `i-label-0-2-70`,
   `clickOutside(i)-field-0-2-97`) that change between renderer builds. **Never target the hash.**
   Reach inside via substring selectors (`[class*="radioItem"]`, `[class*="i-icon"]`,
   `[class*="chip"]`, `[class*="i-edit"]`), stable state classes (`.checked` `.disabled`
   `.bordered`), `data-class` attributes, or element selectors (`input`, `textarea`, `label`,
   `svg`).
3. **`row` / `w` grouping yields a STABLE group class `.row__<rowName>`.** An item with `"row":"act"`
   renders inside `<div class="row row__act row__hash">` whose children are
   `<div class="row__item__…" style="width:…%">`. Note `w` is a **relative weight**, not a raw
   percentage: the rendered width is `w / Σw` across the row (two items at `w:50` each get 50%; at
   `w:50` + `w:100` they get 33% / 67%). Style the group via `.row__act` (flex container) and items
   via `[class*="row__item"]` — your reliable hook for multi-column rows, progress bars, etc.

**Page skeleton & theme** (stable structural hooks):

- **Scope:** all your CSS is wrapped in `.cdu-page` (`&` = page root); per-page hook
  `.cdu-page-<pageId>`.
- **Dark mode is a CLASS, not a media query:** `.theme-light` / `.theme-dark`. In a live render the
  class sits on the host wrapper `#mainRoot` (an **ancestor** of `.cdu-page`), which is why
  `.theme-dark .… {}` works from inside your scoped stylesheet. `control-cdu` itself also stamps the
  theme class onto `#page` (the `.cdu-page` node) — so **verify the placement on your build**: if the
  class is only on the `.cdu-page` node (not an ancestor), a wrapped `.theme-dark .foo` won't match
  and you'd need `&.theme-dark .foo`. Prefer this class approach over `@media (prefers-color-scheme)`.
  ⚠️ Caveat: the renderer's `light`/`dark` token maps are currently **identical** — the mechanism is
  wired but dark mode has no distinct palette yet, so a `.theme-dark` override is the only way to make
  dark actually differ today.
- **Grid regions:** `[data-class="grid-one-column"]` / `[data-class="grid-two-column"]`
  (+`-left`/`-right`); header/footer regions via `[class*="gridtwo__header"]` /
  `[class*="gridtwo__footer"]`.
- **Section slots:** `[data-class="section"]` (+ `[class*="block__"]` for `type:"block"` cards);
  inner `[class*="section__header"]` / `[class*="section__content"]`.
- **Toasts:** two `[class*="notify"]` containers (top inside `#page`, bottom at `#mainRoot` end).

**Per-component root + key inner hooks** (`<sc>` = your `styleClass`):

| Component | Root selector (styleClass here) | Key inner hooks / notes |
|---|---|---|
| `label` | `.label` / `[data-class="label"]` | `<span>` text; BBCode → real tags; `align`→ `.left__`/`.center__`/`.right__` |
| `divider` | `.divider` | empty |
| `edit` (all types) | `.edit` (+ `.edit__<type>`) | `.field > input` / `textarea`; states `bordered`(box)/`selected`/`error`; kill box `.field{border:none}`; help/err `[class*="Component-helperText"]` |
| `select` | `.select` (outer) | readonly `<input>` in `[class*="i-edit"]` + caret `[class*="endAdornment"]`; **no native `<select>`** |
| `multiselect` | `.multiselect` | chip field: `[class*="chip"]` + search `<input>` via `[class*="clickOutside(i)-field"]`; **not** checkbox rows |
| `radio` | `.radio` (+ `.horizontal` for row) | options `[class*="radioItem"]`(+`.checked`/`.disabled`); hide svg `[class*="i-icon"]`, style label `[class*="i-label"]` → pills/scales |
| `check` | `.check` | `[class*="f-icon"]` svg + `<label>`; states `.checked`/`.error`; native `input{appearance:none}` |
| `toggle` | `.toggle` (+ `.left__`/`.right__`) | `[class*="toggle__button"]`(+`.active`) / `[class*="i-switch"]` |
| `slider` | `.slider` (+ `.skillBar__`) | `rc-slider`: `.rc-slider-rail`/`-track`/`-handle`/`-dot`; `[class*="slider__header"]`, `[class*="slider__min"]`/`max` |
| `otp` | `.otp` | boxes `[class*="otp__edit"] input` |
| `phone` | `[data-class="phone"]` | `#countryCode .select` + `#number .edit input`; `[class*="phone__items"]` |
| `image` | `[data-class="image"]` | `<img>` (src **proxied** via `/api/1.0/image`); `align`→ `.center__` |
| `timer` | `[data-class="timer"]` | `<span>` text |
| `comments` | `[data-class="comments"]` | `[class*="mes__wrap"]`, `[class*="mes__name"]`, `[class*="mes__content"]`, avatar `[class*="i-avatar"]` |
| `carousel` | `.carousel` | preview pane `[class*="carousel__preview"]` (zoom/nav btns), thumb strip `[class*="carousel__content__item"]`(+`.active__`); give it a full File `value` for a correct preview (tolerates a missing `type` — won't error-stub) |
| `button` | wrapper `[data-wrapper-for="<id>"]` → inner `#<id>.button.button__<type>` | **`<sc>` is on the INNER button**; text `[class*="button__label"]`; align via wrapper/`.row__<name>`; 7 types via `.button__<type>` |
| `copy` | `[data-class="copy"]` | `[class*="copy__container"]`, icon `[class*="i-icon"]`, label `[class*="i-label"]` |
| `tab` | `.tab` (outer `.tab__…`) | items `[class*="tab__item"]`(+`.active`/`.error`); `hidden` option is absent from DOM |
| `stepper` | `[data-class="stepper"]` | items `[class*="stepperItem"]`(+`.completed__`/`.active__`); label `[class*="stepperItem__label"]` |
| `mainMenu` | `nav.mainMenu` | items `[class*="mainMenuItem"]`(+`.active`); groups `<details>`/`[class*="mainMenuItemGroup"]`; `data-depth` + `--depth`; badge `[class*="i-badge"]` |
| `upload` | `[data-class="upload"]` | `[class*="upload__box"]`, corners `[class*="upload__corner"]`, `input[type=file]` (webcam: trigger `div.upload__file__…[role=button]`, needs `extra.accept`) |
| `file` | `.file` | image → `[class*="file__item"] img`; pdf/doc → `.pg-viewer`/`.pdf-viewer`; path chosen by mime `value.type` |
| `attachment` | `[data-class="attachment"]` | upload ctrl `[class*="i-upload"]`; chips `[class*="fileItemChip"]` (text `[class*="e-chipText"]`, remove `[class*="e-delFileIcon"]`) |
| `signature` | `.signature__wrap` | `<canvas>`; toolbar `[class*="signature__toolbar__clear"]`/`__save` |
| `table` | `[data-class="table"]` (+ `.table__check`/`__radio`/`__group`) | see **Tables** below |
| `widget` | `[data-class="widget"]` | `iframe[class*="iframe__"]`; `[class*="widget__inner"]`(+`.hidden__` until load) |
| row/w group | `.row__<rowName>` | flex container; items `[class*="row__item"]` (inline `width`) |
| `draggable` (sortable/contentLoop) | `[data-class="draggable"]` | grip `[class*="draggable__handle"]`, body `[class*="draggable__content"]` |
| `notification` (toast) | **none** — page root `[class*="notify"]` | `[class*="notifyItem"]` (+ severity `[class*="success"]`/`[class*="error"]`/`[class*="info"]`) → text `span[class*="i-title"]` + close `i[class*="closeIcon"] > svg .fill` |

**Tables** (DOM is a real `<table>`): head cells `[class*="table__head"]` (sortable col
`[class*="sortable__"]`, sort arrow `[class*="table__head__icon"]`); sticky col → `td.sticky-col`
+ the head cell's own `styleClass`; body rows `.table-row` / `[class*="table__row__"]`
(selected radio row → `.active__…`); first column for `check`/`radio` → `[class*="table__check"]`
/ `.i-radioItem-…`; group title row → `[class*="table__row__group"]`. **Default-table cell
mini-components** inside `<td.table-cell>`: file → `[class*="table__img"]`, copy →
`#<id>--copy[data-class="copy"]`, check → `#<id>--check[class*="table__check"]`, button →
`[class*="table__button"]` (id `<id>--button`, **not** `.button`).

> **Table cells are NOT text-only.** A `default`-table cell can be plain text, an **image** (`file`
> cell → real `<img>`), a **button**, a `copy`, or a `check` — so covers/actions live inside a table
> (cell JSON shape belongs to `simulator-smart-forms`; you style via each cell's own `styleClass`,
> which lands on its `<td>`). And **plain + head cells render BBCode** (verified — see the BBCode
> matrix below): the text goes into a plain `<td>`/`<div>`, so `[b]/[color]/[div]…` expand there.
> This is why you can build a status pill in a cell **without** a button cell: put `[div]Label[/div]`
> in the plain cell and style `.<cellClass> div`.

**Overlay sections:** `modal` → backdrop `[class*="i-bg"]` > box `[class*="modal__"]` (+ size
`i-small`/`i-medium`/`i-large`/`i-xlarge` ← `modalSize`), header `[class*="section__modal__header"]`.
`float` → `[class*="float__"]` inside `[class*="Component-wrapper"]` with drag bar
`[class*="section__header__dragable"]` + 8 `[class*="Component-resizeHandle"]`.

> ⚠️ **File-bearing components need a complete `value`.** The **`file` component and a `default`
> table's `file` cell** read mime via `value.type` **with no guard** — a File missing `type` throws
> and the item renders as a `[class*="item__error"]` stub. (`carousel` and `attachment` also read
> `value.type` but tolerate it missing — `carousel` defaults to `''` and uses optional chaining — so
> they won't stub out; still give them the full shape for a correct preview.) Give File values the
> full shape `{fileName, fileSrc, title, type, size}`. A stub can also **suppress later siblings** (a
> crashing table cell hid the `modal`/`float` sections after it). `upload[webcam]` needs
> `extra.accept`. With complete values everything renders statically — no backend needed. Full
> per-component trees: `$CLAUDE_PLUGIN_ROOT/docs/user-flows/cdu-dom-tree-reference.md`.

**Worked example — radio rendered as pills** (the native input is hidden; the svg is the control):

```less
.pills { display:flex; flex-wrap:wrap; gap:8px; }
.pills [class*="i-icon"] { display:none; }                 // hide the svg circle
.pills [class*="radioItem"] { border:1px solid @line; border-radius:100px; }
.pills [class*="radioItem"] label { padding:7px 16px; }    // make the whole pill the label
.pills [class*="radioItem"].checked { background:@ink; }
.pills [class*="radioItem"].checked label { color:#fff; }
```

**Worked example — toast notification.** Toasts render at the page root (inside `.cdu-page`, which
scopes your CSS), so you reach them structurally — there is no `styleClass`. For the DOM nesting +
the two traps (`[class*="i-icon"]` matches the close ✕, not a severity icon — don't blanket-hide it;
`[class*="i-title"]` also matches the wrapper `i-titleContainer` — qualify as `span[class*="i-title"]`)
see the **Toasts** entry in `cdu-dom-tree-reference.md`. A reusable skin:

```less
[class*="notify"] [class*="notifyItem"] {
  background:@white; color:@ink; border:1px solid @line; border-radius:@r;
  box-shadow:0 8px 28px rgba(0,0,0,.12); padding:14px 16px;
  display:flex; align-items:center; gap:10px;
}
[class*="notify"] [class*="notifyItem"]::before {        // severity dot
  content:""; width:8px; height:8px; border-radius:50%; background:@ink-3; flex-shrink:0;
}
[class*="notify"] [class*="notifyItem"][class*="success"]::before { background:#16a34a; }
[class*="notify"] [class*="notifyItem"][class*="error"]::before   { background:#dc2626; }
[class*="notify"] [class*="titleContainer"] { flex:1; min-width:0; }
[class*="notify"] span[class*="i-title"] { font-size:14px; font-weight:500; color:inherit; }
[class*="notify"] [class*="closeIcon"] { opacity:.45; cursor:pointer; }   // keep the ✕
[class*="notify"] [class*="closeIcon"]:hover { opacity:1; }
[class*="notify"] [class*="closeIcon"] svg .fill { fill:currentColor; }
```

> Hashes (`-0-2-64`) are version-pinned to the renderer build — substring selectors survive a bump,
> exact-hash selectors don't. If a restyle suddenly breaks, re-dump the rendered DOM (browser
> DevTools → copy `outerHTML` of `#page`) and re-verify these hooks.

---

## BBCode — where it renders (empirically verified)

A component's text is expanded to HTML by the client (`Utils.bbCodeToHtml`) **only in some fields**.
The smart-form BBCode set (swagger `bbcode` tag) is
`[b] [i] [u] [ul][*] [iurl=…] [url=…] [div] [br] [color=…] [size=…] [bg=…]` — note `[div]` (the only
block-level tag) and `[iurl]` (internal link); there is **no `[span style=…]`** here (that exists
only in actor/reaction BBCode). Mapping: `[color]→<span style="color">`,
`[bg]→<span style="background-color">`, `[div]→<div>`.

**Verified support matrix** (built by rendering a live probe string, not read off the swagger):

| Renders BBCode ✅ | Stays literal ❌ |
|---|---|
| `label.value` | `edit` value / placeholder / multiline |
| `button.title` | `select` value · `multiselect` option |
| `check.title` | `radio` option title |
| `stepper` option title | `toggle.title` · `tab` option title |
| `form.title`, `mainMenu` title, `comments`, `carousel` item title, `upload.title` | `copy.title` |
| **`table` head title AND plain cell** | |

Heuristic: it expands where the text lands in a plain **text node** (`<span>/<div>/<td>`) and stays
literal where it lands in a form-control **`value`/`placeholder`** — but it's only a rough heuristic:
it's whether that specific component passes its title through `bbCodeToHtml`/`dangerouslySetInnerHTML`,
not the node type. `check` ✅ but `radio` ❌ even though both render a `<label>`; `stepper` ✅ but
`toggle`/`tab` ❌. **Trust the matrix (built from the renderer source), not the swagger:** the swagger
marks only `label/button/mainMenu` as bbcode fields, yet table cells clearly render it. When a field
isn't in the matrix, confirm it in the live UI before relying on it.

**Styling use — inject a styleable element where there is none.** A plain `table` cell is bare text
in a `<td>` (no inner wrapper to hook). `[div]…[/div]` gives you an inner `<div>` to style as a
pill/badge (`.<cellClass> div { … }`); `[bg]`/`[color]` give inline `<span>`s. (Alternative with no
inner element: absolutely-position the `<td>` so it shrink-wraps its text into a pill.)

---

## Workflow (reuses the Smart Form tools)

```
1. pullSmartForm(actorId)        → downloads <actorId>/develop/ + production/ (incl. style / styles/, pages/<id>/style)
2. Edit ONLY under develop/:
     - styles/index (+ partials)  OR  the root `style` file
     - pages/<id>/style           (page-specific)
     - pages/<id>/config          (only to add/adjust a styleClass hook)
3. pushSmartForm(actorId)        → validates + uploads changed files; style/styles files carry MIME text/css (the push tool sets it; the server stores the client-supplied type — it does not force it by folder)
4. deploySmartForm(actorId)      → publishes develop → production (when approved)
```

Rules: **edit `develop` only** (`production` is readonly); **`pullSmartForm` first** (it writes the
`.manifest.json` push needs). Files under `styles/` are sent as `text/css`; everything else
`application/json`.

**Verifying a styling result.** `appGetPage` returns the **server-resolved config** (locale/viewModel
expanded) — it does **not** run CSS compilation or `bbCodeToHtml`, so you **cannot** confirm a visual
result or whether BBCode expanded from it. Visual/BBCode results are verified in the live UI by the
user.

**When to ask for the rendered HTML.** First fix from the DOM map below; if it doesn't land, correct
once more. **If after the second correction the user still doesn't get the expected result, stop
guessing and ask them to copy the element's HTML** (Inspect → Copy `outerHTML` of the element, or dump
`#page`). The real DOM — hashed classes, wrapper nesting, inline sizes — resolves what the map can't;
most restyle stalls end the moment you read it.

---

## Starter kit — snippets that recur across real forms

These appear near-verbatim in multiple production forms; lift them as a baseline.

**`colors_fonts` — tokens + font (define here; everything, incl. page styles, inherits them).**
```less
// color tokens (flat is fine for small/medium forms; mature toward Less maps for large apps)
@color_primary:#151f6d; @color_white:#fff; @color_black:#0f0f0f; @color_border:#EAECF0;
@color_grey:#b0adb7; @font_main:'Inter';
// status palette as one token per state (drives chips/rows by status)
@color_new:#A7D8F0; @color_in_progress:#A7E3B1; @color_completed:#B6E3C5; @color_pending:#FBE7A1;
// icons/logos as URL tokens (proxied workspace assets or relative attachments/…)
@icon_search: url('https://…/api/1.0/download/…svg?preview=true');
@font-face { font-family:'Inter'; font-weight:100 900; font-display:swap;
  src:url(https://fonts.gstatic.com/…woff2) format('woff2'); unicode-range:U+0000-00FF, …; }
```

**`init_styles` — platform resets + project-wide component defaults.** `@import`ed from
`styles/index` *after* `colors_fonts`. Page-specific exceptions do **not** go here (→ `pages/<id>/style`).
```less
// ── 1. core resets ───────────────────────────────────────────────────────────
*, *::before, *::after { box-sizing: border-box; }            // the single key reset
// Every .section__content ships its OWN grey bg + padding:20px 16px 0 + margin-bottom:20px
// (some rules theme-scoped → 0,2,0), so it paints a grey padded box INSIDE your cards.
// Neutralize it once here; re-add padding on your own card wrapper. !important beats the theme rule.
.section .section__content { background:transparent !important; margin-bottom:0 !important; padding:0 !important; }
.label, .button, .form { margin:0 !important; font-size:16px; line-height:1.5; }
[class*="row__item"] { display: contents; }                   // row/w items flow into the parent
.button-wrapper { width:auto !important; display:block; }
.button-wrapper:has(.hidden) { display:none; }

// ── 2. platform chrome (header/footer) — NOT hidden by default; reachable via:
//   & { #pageWrap > [class*="content"] > [class*="header"],
//       #pageWrap > [class*="content"] > [class*="footer"] { display:none !important; } }

// ── 3. (optional) full-bleed — drop the default centered/capped card ─────────
.content__main { padding:0 !important;
  [data-class="grid"] { display:block; }
  [data-class="grid-one-column"] { max-width:none; } }

// ── 4. project font (token from colors_fonts) ────────────────────────────────
input, textarea, .button > span, .toggle__title,
:not(&) body > .popoverContent, span[class*="label"], & { font-family:@font_main, sans-serif; }

// ── 5. utilities ──────────────────────────────────────────────────────────────
.visually_hidden { position:absolute; width:1px; height:1px; margin:-1px; padding:0; border:0;
  white-space:nowrap; clip-path:inset(100%); clip:rect(0 0 0 0); overflow:hidden; }
.font_12{font-size:12px;} .font_14{font-size:14px;} .mb_10{margin-bottom:10px !important;}

// ── 6. loading overlays (platform spinners) ──────────────────────────────────
[class*="button__spinner__wrap"], [class^="table__spinner__wrap"] {
  position:fixed; inset:0; z-index:10000; display:flex; align-items:center;
  justify-content:center; background:rgba(0,0,0,.1); backdrop-filter:blur(1px); }

// ── 7. project-wide component defaults (customize to brand) ──────────────────
.button {                                   // ONE base button look used on every page
  padding:14px 20px; border:none; border-radius:10px; font-size:15px; font-weight:600;
  cursor:pointer; line-height:1; display:flex; align-items:center; justify-content:center; gap:8px;
  transition:all .3s cubic-bezier(.4,0,.2,1);
  &.primary_btn { background:@color_primary; color:@color_white;
    &:hover { transform:scale(1.02); box-shadow:0 6px 20px fade(@color_primary,40%); } }
  // … add your semantic variants (.submit_btn, .cancel_btn, …) by this pattern …
}
.modal {                                    // default sizing by modalSize
  &[class*="small"]  { width:400px !important; }
  &[class*="medium"], &[class*="large"] { width:auto !important; max-height:90% !important; }
  &[class*="xlarge"] { width:80% !important; max-height:90% !important; }
}
```

---

## Pattern catalogue (reusable recipes)

### Layout
- **Centered card form** — cap and center the grid wrapper:
  `[data-class="grid-one-column"] { max-width:440px; margin:20px auto; padding:20px; background:#fff; border-radius:8px; box-shadow:0 0 10px #0000001a; }`
- **Full-bleed admin** — unlock the cap: `.content__main [data-class="grid-one-column"] { max-width:none; }`
- **Fixed sidebar shell** — `.sidebar { width:250px; grid-template-rows:80px 1fr 120px; }` with header/content/footer panes.
- **Pinned footer + scrolling body** — `.info_form{height:calc(100vh - 50px); display:flex; flex-direction:column;} .content_section{flex:1; max-height:calc(100vh - 100px);}`

### Components — re-skin, don't accept defaults
- **Floating-label input** — `.edit__text .label { top:36px; } .edit__text.selected .label { top:16px; font-size:12px; } .edit__text.focus .field { border-color:@blue; box-shadow:0 0 0 2px #007bff40; }`
- **Custom checkbox** — hide native (`input{-webkit-appearance:none}`, `i{display:none}`), draw tick on `.checked input::after` (rotated border).
- **Card data table** — `.main_table { border-radius:12px; border:1px solid @border; } .main_table [class*="table__wrap"]{ overflow:hidden; border-radius:12px 12px 0 0; }` + custom pagination by swapping `[id*="table-next"] i::after { content:""; background-image:@icon_arrow_right; }`. Use **modifier classes**: `.main_table.thin_table`, `.no_thead`, `.center_cell`.
- **Table/radio → cards** (the e-commerce look): `thead{display:none}`, `tbody{display:flex;flex-direction:column;gap:10px}`, `.table-row{border:1px solid #ddd;border-radius:8px;padding:10px}`, highlight selection with `.table-row:has([class*="radioItem"].checked){border-color:#34303d}`.
- **Status chip** — state-class + token map: `.status_cell.completed-… div{ background:@color_completed; }` (backend sets `styleClass:"status_cell completed-…"`).
- **Button variants** — base `.button` + semantic modifiers (`.blue_btn`, `.red_button`, `.submit_btn`); hover-lift (`transform:scale(1.02); box-shadow:…`) or an animated `::after` fill-sweep (`transform:skewX(-20deg)`).
- **Toast notifications** — centered, severity icon via `[class*="notifyItem"][class*="success"]::before{ background-image:@icon_success; }`.
- **Modal sizing** — base `.modal` + `:has(.<innerSection>)` to size each, or size-class match `&[class*="xlarge"]{ width:80% !important; }`.

### Interactivity (CSS-only, no backend round-trip)
- **Accordion / dropdown / collapsible tree** — a `check`/`toggle` with `.checked`, expanded by parent `:has()`, animated via `grid-template-rows:0fr → 1fr`:
  ```less
  .menu { display:grid; grid-template-rows:0fr; overflow:hidden; transition:grid-template-rows .32s; }
  .sidebar:has(.nav_btn.checked) .menu { grid-template-rows:1fr; }
  ```
- **Slide-in panel / popover** — toggle `opacity`/`transform`/`pointer-events` under `&:has(.trigger.checked)`.

### State without CSS-only tricks
- **Screens-as-sections** — put each step/success/error/loading state as a sibling **section**, the backend flips `visibility` (one stylesheet, many screens).
- **Hidden state carriers** — `edit` items with `styleClass:"visually_hidden"` stash client state (selected id, cart id) for submit.

### Icons
- **Recolorable monochrome** — `mask:url(@icon) no-repeat center; mask-size:contain; background-color:currentColor;` (inherits text color). Bundled asset: relative `attachments/cart.svg`; external: proxied `/api/1.0/download|image`.
- **Full-color** — `background-image:@icon` token.

### RTL / theme toggle from the backend
- Page root `styleClass:"{{settings_page_text_align}}"`; rule `.form.text_align_right { … flex-direction:row-reverse; text-align:right; … }`. The backend pushes one value to mirror the whole UI. Same mechanism toggles nav-active state, light/dark, etc.

---

## Design-system approach (advanced — for large/branded apps)

When a form grows or needs strict brand consistency, build a **token system in Less** (this is the
*LMS* form's approach — Tailwind/Untitled-UI in Less):

```less
// maps for everything, not just color
@breakpoint: { minimum-mobile:375px; mobile-tablet:820px; tablet-desktop:1180px; }
@space:      { 0:2px; 1:4px; 2:8px; 3:12px; 4:16px; 5:20px; 6:24px; 8:32px; /* … */ 64:256px; }
@color: { @gray:{25:#FCFCFD; /* … */ 700:#344054; 900:#101828;} @brand:{…} @error:{…} @success:{…} }

// generate utility classes from a map (each + guard)
.make-space-set(@base,@rule,@min,@max){ each(@space,{ .apply(@k,@v);
  .apply(@k,@v) when (@k<=@max) and (@k>=@min){ .@{base}-@{k}.@{base}-@{k}{ @{rule}:@v; } } }); }
.make-space-set(mt, margin-top, 0, 16);   // → .mt-0 … .mt-16, applied in styleClass

// typography scale via lookup mixin
.text-font(@size; @weight:regular){ @s:@typography[@text][@@size];
  font-size:@s[font-size]; font-weight:@font-weight[@@weight]; line-height:@s[line-height]; }

// responsive tokens: Less map → CSS var overridden per breakpoint
@media (min-width:@breakpoint[tablet-desktop]) { :root { --space-large: 32px; } }
.gap-large { gap: var(--space-large); }
```

Advanced Less available: `each()`, guards (`when`), maps + indirect lookup (`@@key`),
`replace()`, `extract()/if()/isnumber()/isdefined()`, detached-ruleset returns (`mixin()[@result]`).
**Caveat:** keep this in a **modular `styles/`** split (`colors_fonts`, `space`, `typography`,
`shadows`, `mixins`, `index`) — a single 7k-line file is the maintainability ceiling.

---

## Gotchas (learned from real forms)

- **Windows filenames**: a folder/file titled with a reserved char (`\ / : * ? " < > |`) breaks
  `pullSmartForm` on Windows (`mkdir` fails). Don't name definitions like `edit|create_…`. If a
  pull aborts on `mkdir … syntax is incorrect`, rename the offending folder in the editor and
  re-pull (or recover content from a `.graph` export that carries `scripts/<uuid>/…` file trees).
- **Winning the cascade**: component CSS-modules load after you and win equal-specificity ties — beat
  them with a chain of stable classes (`styleClass` + `[class*="…"]` + tag), not `#id` or a lone
  `!important`. Full worked example + cascade math in §"How Smart Form styling works" pt 5.
- **Image/`file` cell renders "wide but a thin strip"**: not a width bug but a **clipping ancestor** —
  a wrapper (`.file` / `.file__item`) keeps a small fixed height + `overflow:hidden`. Force the box at
  **every** wrapper level (`height` + `min/max-height` + `width:100%` + `max-width:none` +
  `overflow`), not only on `<img>`.
- **Dead `styleClass`**: placeholder classes (`my-custom-class`) with no rule are harmless but
  noise — remove them.
- **Stale copies**: forms accumulate `style(copy)` / `style BACKUP` files — they're not wired in;
  don't analyze or ship them.
- **External images are proxied**: `https://…` URLs in CSS get `/api/1.0/image?src=` prepended
  automatically (actor-attached images are internal and not proxied).
- **`//` comments** compile away (prefer over `/* */`); keep nesting **< 3** levels.
- **Edit `develop` only**; `pages/<id>/style` needs no `@import` (auto-included).

---

## Reference documents

| Path | When to read |
|---|---|
| `$CLAUDE_PLUGIN_ROOT/docs/user-flows/cdu-dom-tree-reference.md` | **The rendered-DOM map**: full per-component tag tree + class names, stable hooks vs. volatile hashes, page skeleton & theme classes, all table types, and which components need backend data to render |
| `$CLAUDE_PLUGIN_ROOT/docs/user-flows/cdu-page-protocol.md` | Component class catalogue, `styleClass`, templating, change protocol |
| `$CLAUDE_PLUGIN_ROOT/docs/user-flows/smart-forms.md` | Project file structure, deploy/release, styles compilation |
| `$CLAUDE_PLUGIN_ROOT/skills/simulator-smart-forms/SKILL.md` | The pull/push/deploy cycle and page-config format this skill builds on |
| CDU swagger → **"CSS styling"** tag | Authoritative Less guide: organization, component default classes, best practices, image proxying |
