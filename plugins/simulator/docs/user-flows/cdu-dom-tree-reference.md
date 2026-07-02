# CDU Rendered-DOM Reference (styling hooks)

A map of **"page-config JSON element → the actual rendered tag tree and its classes"** for
styling Smart Forms (CDU / Script apps). Where [`cdu-page-protocol.md`](cdu-page-protocol.md)
specifies the page *on the wire* (the JSON contract), this document specifies the page *in the
DOM* (what `control-cdu` renders) — i.e. the elements and class names your CSS/Less actually
has to target.

> **Source.** Built by inspecting a live `control-cdu` (corezoid-driven-ui) render of a
> kitchen-sink page (`all-elements`) that exercises every documented component. It mirrors the
> component catalogue in [`cdu-page-protocol.md` §5](cdu-page-protocol.md) and the canonical
> "Simulator.Company Scripts" OpenAPI. Hashed class suffixes (`__12ehI`, `-0-2-50`) are
> **build-pinned** — they change between renderer releases; treat every hash shown here as an
> illustration of node *position*, not a selector. If a restyle suddenly breaks, re-dump the
> rendered DOM (DevTools → copy `outerHTML` of `#page`) and re-verify the hooks.

---

## 0. How to read this — and how to select

Every component renders with **three layers of classes**:

| Layer | Example | Stability | Use for styling? |
|---|---|---|---|
| Semantic | `label`, `edit`, `button`, `table` | stable | ✅ yes, but broad |
| Hashed (CSS-modules) | `label__12ehI`, `edit__pIifb`, `Component-field-0-2-50` | **changes per build** | ⚠️ only via `[class*="…"]` on the prefix |
| Your `styleClass` | `ks-edit-text`, `ks-btn-default` | you control it | ✅✅ primary hook |

**Stable hooks to rely on** (do not change between builds):

- **Semantic root class** — `edit`, `select`, `multiselect`, `radio`, `check`, `toggle`,
  `slider`, `otp`, `phone`, `label`, `divider`, `image`, `timer`, `comments`, `button`, `copy`,
  `tab`, `stepper`, `mainMenu`, `upload`, `signature`, `table`, `widget`, `form`, `section`,
  `draggable`. Broad but stable — narrow it with your own `styleClass`.
- **`data-class`** — on the root of a structure/component: `grid`, `grid-two-column`,
  `grid-two-column-left/right`, `form`, `section`, `label`, `image`, `phone`, `timer`,
  `comments`, `copy`, `upload`, `stepper`, `table`, `draggable`, `widget`.
- **Your `styleClass`** — lands on the component **root** (see the `button` caveat).
- **`data-wrapper-for="<id>"`** — on the button wrapper (stable; see `button`).
- **`id`** — your `id` lands on the root (and on the `<input>` for edit/select/phone parts).

> **Rule of thumb:** select by `styleClass` (your `ks-*`), the semantic class, or
> `[data-class="…"]`; reach internal hashed nodes via `[class*="prefix__"]` /
> `[class*="Component-field-"]`; and never hardcode a full hash suffix (`__12ehI`, `-0-2-50`).
> Expect to need `!important` or a doubled selector (`.x.x`) to beat the renderer's base/inline
> styles.

---

## 1. Page skeleton

```
#mainRoot.theme-light.theme                          ← theme: theme-light | theme-dark
└ #cdu                                                (flex, height:100vh)
  └ #page.cdu-page.cdu-page-all-elements.page__1M7x5  ← .cdu-page = scope of ALL your styles
    │                                                   .cdu-page-<pageId> = per-page hook
    ├ .i-notify-…-6.notify__…                          ← toast container (top)
    └ #pageWrap.page__wrap__…
      └ .content__…
        ├ .header__….steps__…                         ← grid header (see §3)
        ├ .header__bottom__…
        └ #contentWrap.content__wrap__…
          └ .content__main.content__main__…
            └ [data-class=grid].<gridStyleClass>.grid__…   ← grid.styleClass here
              └ … (see §2)
        (at the end of the page, outside contentWrap:)
        .footer__… > a   ← "Powered by Simulator.Company"
└ .i-notify-…-1.notify__…                             ← second toast container (bottom)
```

- **All your styles are wrapped under `.cdu-page`** at serve time — `&` = the page root.
- `cdu-page-<pageId>` (e.g. `cdu-page-all-elements`) is a convenient per-page hook.
- **Dark mode is a class, not a media query**: `.theme-light` / `.theme-dark` on `#mainRoot`.
  Prefer `.theme-dark .… {}` over `@media (prefers-color-scheme)` for theme-following styles.
- **Toasts** render into two containers (`[class*="notify"]`), top and bottom — see §6 → toast.

---

## 2. Grid

### two_column

```
[data-class=grid].<gridStyleClass>.grid__…
└ .gridtwo__… [data-class=grid-two-column]
  ├ .gridtwo__header__…                       ← forms from components.header
  ├ .gridtwo__row__…
  │ ├ .gridtwo__left__…  [data-class=grid-two-column-left]
  │ └ .gridtwo__right__… [data-class=grid-two-column-right]
  └ .gridtwo__footer__…                        ← forms from components.footer
```

`components.left/right/footer/header` are lists of **form ids** laid out into these regions.
`one_column` yields an analogous tree with a single central region.

### sideBar (persistent app rail — `grid.sideBar`)

Config lives on the page grid: `grid.sideBar.components.{header,center,footer}` (each = a list of
**form ids**) + `grid.sideBar.styleClass`. If `header` is omitted the platform renders a default
logo. This is a **separate always-visible rail**, distinct from the column regions above
(`components.left/right/center/footer`).

**Rendered (verified live capture).** Hooks below are hash-free **base classes** — the slots
(`.sidebar`, `.sidebar__header/content/footer`, `.page__sidebar`) each ship a stable base class
alongside their hashed twin, so target the base class directly. The slideout (mobile) nodes ship
**only** hashed classes → reach them with substring selectors `[class*="slideout__…"]`.

```
#page .cdu-page.cdu-page-<pageId> .sidebar__…        ← #page GAINS a hashed .sidebar__… modifier when a sideBar exists
└ #pageWrap
  ├ .page__sidebar                                   ← the persistent desktop rail wrapper (base class)
  │ └ .sidebar                                        ← rail root (base class; no data-testid needed)
  │   ├ .sidebar__header    ← forms from sideBar.components.header (or default logo)
  │   ├ .sidebar__content   ← forms from sideBar.components.center
  │   ├ .sidebar__footer    ← forms from sideBar.components.footer
  │   └ [class*="slideout__bottom"] > [class*="slideout__btn"]   ← collapse / expand toggle
  └ #contentWrap → .content__main → [data-class=grid] …          ← the column grid (see §2 → two_column / one_column)

(elsewhere on the page — a MOBILE DRAWER duplicate of the whole rail:)
[class*="slideout__bg"]                              ← backdrop
└ [class*="slideout__menu"]
  └ .sidebar …                                        ← the SAME forms, rendered a 2nd time
```

Each form assigned to the rail keeps the normal `[data-class=form].<styleClass>` → `[data-class=section]`
→ `.section__content` chain (§4/§5); e.g. a nav form's content holds a `mainMenu` (§6) + logo/user `label`s.

> ⚠️ **Gotchas (verified):**
> - **`grid.sideBar.styleClass` is NOT emitted onto the rail** in the captured build. Style the rail
>   via the structural base classes instead: `.page__sidebar`, `.sidebar`,
>   `.sidebar__header` / `.sidebar__content` / `.sidebar__footer`.
> - **The rail renders TWICE** — the desktop `.page__sidebar` rail **and** a mobile slideout drawer
>   (`[class*="slideout__menu"]`). So every form/item **`id` inside the sidebar appears twice** in the
>   DOM. Prefer `styleClass`/class hooks over `#id` selectors here (an `#id` rule matches both copies).
> - Collapse toggle → `[class*="slideout__bottom"]`; drawer backdrop → `[class*="slideout__bg"]`.

**Manual alternative (no `grid.sideBar`):** a sidebar can also be composed inside an ordinary grid
region — a `form` (with a `styleClass`) whose section `content[]` holds a `mainMenu` (+ logo/user
`label`s). Then the forms are direct children of `[data-class=grid-one-column]` (or a column region),
with the usual form → section → `.section__content` → items chain.

---

## 3. Header (grid header, `class: "steps"`)

```
.header__….steps__…                       ← .steps__ = header class "steps"
└ .header__step__…
  ├ .header__step__item__… .active__…      ← active step (extra.active)
  │ ├ .header__step__item__circle__…
  │ │ └ .header__step__item__num__…   → "1"
  │ └ .header__step__item__label__…   → "Inputs"
  ├ .header__step__item__… .disabled__…    ← the other steps
  └ .header__step__line__…                 ← connector line
```

Step state: active → `.active__…`, the rest → `.disabled__…`.

---

## 4. Form

```
#<formId> [data-class=form].form.<styleClass>.form__…
├ .form__title__… > span   → form.title (node absent if title is empty)
└ … sections …
```

---

## 5. Section

```
#<sectionId> [data-class=section] .section .<styleClass> .section__…  [.block__…]
└ .section__wrap.section__wrap__…
  ├ .section__header.section__header__…   ← items from header[]
  └ .section__content.section__content__… ← items from content[]
```

- `type:"block"` → adds `.block__…` to the section root (card look).
- `type:"body"` → no `.block__…`.
- The `section__header` wrapper is absent when `header[]` is empty.
- `type:"modal"` / `type:"float"` render as overlays — **see §8 → Overlay sections**.

---

## 6. Components

### label
```
#<id> [data-class=label].label.<styleClass>.label__… [.left__ | .center__ | .right__]
└ <span> …text…
```
- `align` → suffix class `left__`/`center__`/`right__` (each with its own hash).
- **BBCode renders to real tags** inside `<span>`: `[b]`→`<b>`, `[i]`→`<i>`, `[u]`→`<u>`,
  `[color=#x]`→`<span style="color:#x">`, `[br]`→`<br>`.

### divider
```
#<id> .divider .<styleClass> .divider__…     (empty)
```

### edit (all types)
```
#<id> .edit [.edit__<type>] .<styleClass> .edit__… .Component-txt-… .Component-medium-…
      [state: selected | error | bordered | hasLeftIcon]
├ .Component-label-….label  (aria-hidden)        → title
├ .Component-field-….field
│ ├ [.Component-leftIcon-….leftIcon > i > svg]   (type date/phone — left icon)
│ ├ [.Component-colorBadge-….badge style=bg]     (type colorPicker — color chip)
│ ├ <input id type inputmode placeholder value>   OR  <textarea> (multiline)
│ └ [.endAdornment… > [role=button] > svg(×)]     (resettable / when value present)
└ [.Component-helperText-… [.error]]              → helpMsg / errorMsg
```
- `edit__<type>`: `text/date/colorPicker/multiline/masked/phone(edit)` share `edit__text`;
  distinct ones — `edit__email`, `edit__password`, `edit__int`, `edit__float`, `edit__phone`.
- Root state: `bordered` (always — draws the full border box), `selected` (has value),
  `error` (error=true, also duplicated on the helperText).
- Kill the default box: `.<sc> .field { border:none }`.

### select
```
#<id> .select .<styleClass> .select__… .i-select-…
└ #<id> .i-edit-… .Component-txt-… .Component-medium-… .bordered
  ├ .Component-label-….label  → title
  └ .Component-field-….field
    ├ <input readonly inputmode=none>   (type=default)  |  <input inputmode=text> (autocomplete)
    └ .endAdornment… > [role=button] > svg(▾ caret)
```
`styleClass` is on the outer `.select`. There is **no native `<select>`** — target `.<sc> input`.
Options (icon/badge/avatar) only exist in the open dropdown (closed in a static render).

### multiselect
```
#<id> .multiselect .<styleClass> .clickOutside(i)-autocomplete-… .clickOutside(i)-medium-… [selected] bordered
├ .clickOutside(i)-label-….label   → title
└ .clickOutside(i)-field-….field
  └ .clickOutside(i)-fieldInput-… .clickOutside(i)-paddings-… bordered
    └ <input placeholder value>          (selected chips appear here)
```
> Renders as a **chip/autocomplete field, not checkbox rows**. The class literally contains
> `clickOutside(i)-…` with parentheses — select via `[class*="clickOutside(i)-field"]`.

### radio
```
(radio container)                  ← parent wrapper
├ .f-label-…                       → title
└ #<id> .radio .<styleClass> .radio__… [.row__….horizontal] .f-radio-… 
  └ .f-item-….i-radioItem-… [.checked | .disabled]
    └ .i-content-…
      ├ <input.i-input-… type=radio>
      ├ <i.i-icon-…> svg            (visible control is the svg circle; native input is hidden)
      └ <label.i-label-…> <span> title
```
- `extra.direction:"row"` → adds `.row__….horizontal` to the radio root.
- Selected item → `.checked`; disabled → `.disabled`.
- To build pills/scales: hide `[class*="i-icon"]`, style `[class*="radioItem"]` + `&.checked`.

### check
```
#<id> .check .<styleClass> .f-checkbox-… [.checked | .error] [required]
├ .f-content-…
│ ├ <input.f-input-… [.error] type=checkbox>
│ ├ <i.f-icon-… [.error]> svg
│ └ <label.f-label-…> <span> title
└ [<span.f-helperText-….error>]    → errorMsg
```

### toggle
```
#<id> .toggle .<styleClass> .toggle__… [.left__ | .right__]
├ <span.toggle__title.toggle__title__…>  → title
└ #<id> > .i-row-….xsmall
        └ .toggle__button.toggle__button__….i-switchWrap-….i-medium-… [.active]
          └ .i-switch-… [.active]
```
`align` → `left__`/`right__` on the root. On → `.active` on both button and switch.

### slider
```
#<id> .slider .<styleClass> .slider__… [.skillBar__…]
├ .slider__header__…
│ ├ <span.slider__header__title…> title
│ └ <span.slider__header__value…> value
├ .slider__wrap__…  (rc-slider-wrap)
│ └ .rc-slider.rc-slider-horizontal
│   ├ .rc-slider-rail
│   ├ .rc-slider-track (style: width)
│   ├ .rc-slider-step  [<span.rc-slider-dot[.rc-slider-dot-active]>… if extra.dots]
│   └ .rc-slider-handle [role=slider]
└ .slider__footer…
  ├ <span.slider__min>  → min (+ extra.measure, e.g. "0 USD")
  └ <span.slider__max>  → max
```
`type:"skillBar"` → `.skillBar__…`. The track is third-party `rc-slider` — `.rc-slider-*` are stable.

### otp
```
#<id> .otp .<styleClass> .otp__…
├ <span.otp__title.otp__title__…>   → title
├ .otp__row__…
│ └ (× extra.length) .otp__edit.otp__edit__….Component-txt-… .bordered
│   └ .Component-field-….field > <input autocomplete=one-time-code [type=number if type=int]>
└ <span.otp__helperText.otp__helperText__…>
```

### phone
```
#<id> [data-class=phone].phone .<styleClass> .phone__…
├ .phone__label__…                    → title
├ .phone__items__…
│ ├ #countryCode .select .i-select-… > … (select-like, value "+380")
│ └ #number .edit .Component-txt-… > .field > <input type=number>
└ [data-class-prop=errorMsg] .phone__helperText__…
```

### image
```
#<id> [data-class=image].image .<styleClass> .image__… [.center__]
└ <img alt="…" src="/api/1.0/image?src=<URL-encoded>">
```
> External `src` is **proxied** through `/api/1.0/image?src=…`. `align` → `center__`/… on the root.

### carousel
```
#<id> .carousel .<styleClass> .carousel__… .Component-stack-…       (type=preview)
├ .carousel__preview.carousel__preview__…                          ← big preview pane (type=preview only)
│ ├ .carousel__preview__header__… > zoom btns [.carousel__preview__header__zoomIn / __zoomOut .g-button-…]
│ ├ #<id> .carousel__preview__item.carousel__preview__item__… > <img alt=<title>> + <span.carousel__preview__item__title__…>
│ └ [role=button].carousel__preview__nav .right .arrowDown …       (prev/next)
└ .carousel__content.carousel__content__… [.preview__…]            ← thumbnail strip
  └ (per option) .carousel__content__item.carousel__content__item__… [.active__…]
    ├ .file.carousel__content__item__file__… > .file__item__… > <img>
    └ <span.carousel__content__item__title__…> → option.title
```
> `type:"default"` shows only the `.carousel__content` strip (no preview pane). Needs a complete
> File `value` to render (see §9).

### timer
```
#<id> [data-class=timer].timer .<styleClass> .timer__…
└ <span> "02:17"    (format via extra.format; counts up/down via extra.mode)
```

### comments
```
#<id> [data-class=comments].comments .<styleClass> .comments__…
└ .comments__list.comments__list__…
  └ (per message) .mes__hwrap.mes__hwrap__… > div
    ├ .mes__wrap.mes__wrap__…
    │ └ .mes__dscr.mes__dscr__…
    │   ├ .mes__avatar.mes__avatar__… > .avatar.i-avatar-… > span(initials)
    │   └ div
    │     ├ .mes__name.mes__name__… > span.mes__name__sb(name) + span(description)
    │     └ <span.mes__date.mes__date__…> + <span.mes__location.mes__location__…>
    └ .mes__content.mes__content__… > div   → value
```

### button
```
.button__….button-wrapper [data-wrapper-for=<id>]
└ #<id> [role=button] .button .button__<type> .<styleClass> .button__inner__…
        .g-button-… .g-<type>-… .g-medium-… [hideIcon] [hideIconAfter]
  ├ [<i.i-wrap-…> svg]              (extra.icon)
  └ <span.button__label.button__label__…> → title
```
> ⚠️ **`styleClass` lands on the INNER `#<id>` button, not the wrapper.** The wrapper carries
> `data-wrapper-for="<id>"` — use it to position/align the button (e.g. inside a row/w cell).
- `type` → `button__<type>` + `g-<type>-…` (`default/secondary/tertiary/quaternary/quinary/text/error`).

### copy
```
#<id> [data-class=copy].copy .<styleClass> .copy__… [.left__]
└ .copy__container.copy__button__….i-chip-… .i-rectangular-… .i-small-…
  ├ <i.i-icon-…> svg (copy icon)
  └ <span title="<tooltip>" .i-label-…> → title
```

### tab
```
.tab__…                                          ← outer container
└ #<id> .tab .<styleClass> .tab__comp__… .i-tabContainer-…
  └ .i-tabs-…
    └ (per option) .tab__item .i-tab-… .i-fixed-… [.active] [.error] .tabItem
      └ <span.i-label-…> → title
```
An option with `visibility:"hidden"` is **absent** from the DOM. Active → `.active`; `error:true` → `.error`.

### stepper (component)
```
#<id> [data-class=stepper].stepper .<styleClass> .stepper__… [.center__]
└ .items.stepper__items__…
  └ (per option) .stepperItem__… [.completed__ | .active__]
    ├ .stepperItem__icon__…
    │ ├ [<i> svg ✓]                         (completed:true)
    │ └ <input type=radio value=<optValue>>
    └ <label.stepperItem__label__…> <div> → title
```
`align` → `center__`/… on root. States: `.completed__…`, `.active__…`.

### mainMenu
```
<nav> #<id> .mainMenu .<styleClass> .mainMenu__…
└ .mainMenuListContainer [data-depth=0] style="--depth:0"
  ├ (leaf) [name=<itemId>] .mainMenuItem.inline.mainMenuItem__….inline__….i-menuItem-…
  │ ├ <i.i-icon-….i-iconLeft-…>     (icon)
  │ ├ .i-labelContainer-… > span.i-labelContainerContent-… > span → title
  │ └ [.i-badge-….badge → badge]
  └ (branch) <details.mainMenuItemGroup.mainMenuItemGroup__…>
    ├ <summary.mainMenuItemGroup__summary__…>
    │ ├ .mainMenuItemGroup__dropdown__… > i svg(▾)
    │ └ [name=<id>].mainMenuItem… [.active]      (parent item, +badge)
    └ .mainMenuListContainer [data-depth=1]       → child leaves
```
Nesting depth → `data-depth` + the `--depth` CSS variable. Active path → `.active`.

**Verified (live capture — nav sidebar):**
- The `<details.mainMenuItemGroup>` carries the `open` attribute while expanded (driven by
  `value.expandedIds` / `extra.autoExpandActive`). Collapsing removes `open`.
- The group's **own row lives INSIDE `<summary>`**, right after the dropdown toggle
  (`[class*="mainMenuItemGroup__dropdown"]`, `role="menuitemcheckbox"` + `aria-checked`).
- Every node — leaf **and** group row — is `<div [name="<itemId>"] .mainMenuItem.inline [.active]>`.
  **Select a specific node by `[name="<itemId>"]`** (stable; mirrors `items[].id`). Title text is
  `.mainMenuItem span span`.
- Badge renders as `<div [class*="i-badge"].badge>…</div>` **inside the leaf** (both `i-badge` and
  `.badge` hooks are present).
- The config `structure` map (`$root` + parentId→childIds) is what expands into the nested
  `.mainMenuListContainer[data-depth=N]` levels (0-based).
- Stable hooks (hash-free): `nav.mainMenu` · `.mainMenuListContainer[data-depth]` ·
  `[class*="mainMenuItemGroup__summary"]` · `[class*="mainMenuItemGroup__dropdown"]` ·
  `[class*="mainMenuItem"]`(+`.active`) · `[name="<itemId>"]` · `[class*="i-badge"]`/`.badge`.

### file
```
#<id> .file .<styleClass> .file__… .file__3P8YO [.left__]
├ (image)  .file__item__… [role=button] > <img src="/api/1.0/image?src=…" alt=<title>>
└ (pdf/doc) .file__preview__… > .pg-viewer-wrapper > #pg-viewer.pg-viewer > .pdf-viewer-container
            > .pdf-viewer > .pdf-controlls-container (.view-control > i.zoom-in/.zoom-reset/.zoom-out) + .pdf-loading
```
> Render path is chosen by mime `value.type`: images → `.file__item__ img`; PDFs/docs →
> `.pg-viewer`/`.pdf-viewer`. Needs a complete File `value` (see §9).

### upload (default / webcam)
```
#<id> [data-class=upload].upload .<styleClass> .upload__…
└ .upload__wrap__…
  ├ .upload__box__…
  │ ├ .upload__icon.upload__box__icon__… > svg
  │ └ .upload__box__title__…       → title
  ├ (×4) .upload__corner.upload__trl__… .{leftTop|leftBottom|rightBottom|rightTop}__…   (frame corners)
  └ <input.upload__file__… type=file accept="…">     (type=default)
```
> `type:"webcam"` renders the same shell, but the drop `<input type=file>` is replaced by a
> capture trigger `<div.upload__file__… role="button">`, and the component needs `extra.accept`.

### attachment
```
#<id> [data-class=attachment].attachment .<styleClass> .attachment__…
├ #<id> .i-upload-…                                ← upload control
│ └ .i-loadBtn-… > <i.i-iconUpload-…>svg + <span> label + .i-browseLinkBox-… (span.i-browseLink-… + <input type=file accept>)
└ .attachment__list__…                              ← file chips
  └ (per file) #<fileName> .e-fileItemChip-….fileItemChip
    ├ <img.e-chipImg-…> (image)  |  <i> svg (non-image icon)
    ├ .e-chipTextBox-… > span.e-chipText-…(title) + span.e-chipHelperText-…(size)
    └ <i.e-delFileIcon-…> svg (remove ✕)
```
> Each `value[]` item is a full File object (see §9).

### signature
```
#<id> .signature__wrap .<styleClass> .signature__wrap__… .Component-stack-…
├ <span.signature__title.signature__title__…>  → title
└ .Component-space-…
  └ .signature.signature__…
    ├ .signature__content.signature__content__… #<id>-content
    │ └ <canvas width height style="cursor:url(<pen>)">
    └ .signature__toolbar…
      ├ [role=button].signature__toolbar__clear.g-button-… [.disabled] > span → clearButtonTitle
      └ .signature__toolbar__container…
        └ [role=button].signature__toolbar__save.g-button-… [.disabled] > span → saveButtonTitle
```

---

## 7. Tables

Common wrapper (all `type`):
```
#<id> [data-class=table].table .table__<type> .<styleClass> .table__…
└ .table__wrap__… > <table> > <thead> / <tbody>
```
- `type` → `.table__check` / `.table__radio` / `.table__group` (default has no suffix).

### thead
```
<thead><tr>
  ├ <th .table__head__… [.sortable__…]>
  │   └ .table__head__div__…
  │     ├ .table__head__text__…   → column.title
  │     └ .table__head__icon__… [.arrowUp__…]   ← sort arrow (when extra.sort)
  └ (sticky col) <th|td class="<headStyleClass> sticky-col table__head__…" style="left:0;right:0">
```
- `head[].isSticky:true` → `td/th.sticky-col` + inline `left/right`.
- `head[].styleClass` → class straight on the header `<td>` (e.g. `ks-col-check`).
- Sortable column → `<th.sortable__…>` + arrow `.arrowUp__…`.

### tbody — row
```
<tr .table-row .table__row__… [.active__… when selected in radio mode]>
  └ <td.table-cell> → cell (plain title)
```

### type=check
- `genericCheckColumn` header → master checkbox:
  `<td.sticky-col> #<id> .partiallySelected .table__check__… .f-checkbox-… > input+i(svg)+label`
- First cell in each row:
  `<td.sticky-col> #<rowValue> .table__check__….f-checkbox-… [.checked] > input(checkbox)`

### type=radio
- `genericRadioColumn` header → `<th.table__head__…>&nbsp;</th>` (blank).
- First cell: `<td> .i-radioItem-… [.checked] > .i-content-… > input(radio)+i(svg)+label`.
- Selected row → `<tr … .active__…>`.

### type=group
- Group title row:
  `<tr.table__row__group__…> <th scope=rowgroup colspan=N .sticky-left-content.sticky-left-content__…> <div> → group.title`
- Then the usual `<tr.table-row.table__row__…>` rows of that group.

### type=default — cell mini-components
Each cell renders inside `<td.table-cell [+ cell.styleClass] [.sticky-col]>`; the cell type
decides its contents:
- **plain** (`value`+`title`): text directly in the `<td>`.
- **file**: `.table__img__… > .file.file__… > .file__item__… > <img>` + `.table__img__label__…` (the cell `file.label`).
- **copy**: `#<tableId>--copy [data-class=copy].copy … > .copy__container…` (identical to the `copy` component).
- **check**: `#<tableId>--check .table__check__….f-checkbox-… [.checked] > input+i(svg)+label`.
- **button**: `.Component-popoverParent-… > #<tableId>--button .table__button.table__button__… .g-button-… .g-secondary-… > i(svg icon) + span.table__button__label__…`.

> Cell-component ids are `<tableId>--copy` / `--check` / `--button`. The button cell uses
> `.table__button`, **not** the standard `.button`. With `extra.sort` set, **every** head column
> gets `.sortable__…`.

---

## 8. Layout & wrappers

### row / w (horizontal layout)
```
.row .row__<rowName> .row__…
└ (per item) .row__item__… style="width:<w>%"
  └ <the component>
```
`row:"name"` → wrapper `.row__name`; `w:"50"` → inline `width:50%` on `.row__item__…`.
The reliable group hook is `.row__<rowName>` (flex container); items via `[class*="row__item"]`.

### sortable section + contentLoop
```
#<sectionId> .section … (type=body, no .block__…)
└ .section__content…
  └ #<contentLoopId> [data-class=draggable].draggable .handle__left .draggable__… [role=button]
    ├ .draggable__handle.draggable__handle__… > i svg (grip)
    └ .draggable__content.draggable__content__…
      └ … items …
```
The wrapper `id` comes from the `contentLoop` template; the loop only multiplies with backend data.

### widget (type=iframe)
```
#<id> .widget .widget__iframe .<styleClass> .widget__…
└ [data-class=widget] .widget__inner__… .hidden__…   ← .hidden__ until the iframe loads
  └ <iframe id=iframe title=<id> .iframe__… allowfullscreen allow="…">
```

### Overlay sections (modal / float)
A `type:"modal"` section renders inside a backdrop, portaled near the page root:
```
.i-bg-….visible                                   ← backdrop (greys out the page)
└ .modal.i-<size>-….i-modal-….visible             ← box; i-small/medium/large/xlarge ← modalSize
  └ .Component-contentWrapper-… > .Component-wrap-…
    └ #<sectionId> [data-class=section].section .<sc> .section__… .modal__…
      └ .section__wrap
        ├ .section__modal__header.section__modal__header__…   ← modalHeader[]
        └ .section__content.section__content__…               ← content[]
```
A `type:"float"` section renders in a fixed, draggable (and, with `isResizable`, resizable) wrapper:
```
.Component-wrapper-… [style="position:fixed; width; height; z-index; left; top"]
├ .Component-inner-… > .i-float-….visible > .Component-contentWrapper-… > .Component-wrap-…
│ └ #<sectionId> [data-class=section].section .<sc> .section__… .float__…
│   └ .section__wrap
│     ├ .section__header__dragable__… [data-drag-handle=true]      ← drag bar
│     │   ├ <i.section__drag__icon__…> svg (grip)
│     │   └ .section__close__… > [role=button][aria-label=close] > i.close_btn > svg
│     ├ .section__header.section__header__…   ← header[]
│     └ .section__content…                    ← content[]
└ (×8) .Component-resizeHandle-… .Component-resize_{n,ne,e,se,s,sw,w,nw}-…   ← resize handles (isResizable)
```
> Both render only when the section is present and visible (`visibility:"visible"`, or opened by
> the backend). A hard render error in an earlier sibling (e.g. a crashing table cell) can suppress
> later overlays — fix that error first.

### Toasts / notifications
Two containers: `[class*="notify"]` at the top (inside `#page`) and at the bottom (end of
`#mainRoot`). Toasts appear as their children (empty in a static dump). There is **no
`styleClass`** — reach them structurally:
```
[class*="notify"] > div > [class*="notifyItem"] (+ severity [class*="success"] | [class*="error"] | [class*="info"])
  └ [class*="titleContainer"] > span[class*="i-title"] (text) + i[class*="closeIcon"] > svg .fill (the ✕)
```
> Two traps: (1) `[class*="i-icon"]` inside a toast matches the **close ✕** — do not blanket-hide
> it. (2) `[class*="i-title"]` also matches `i-titleContainer`; qualify as `span[class*="i-title"]`.

---

## 9. Gotcha: file-bearing components need a complete `value`

`carousel`, `file`, `attachment`, and a `default` table with a `file` cell read the mime via
`value.type.indexOf('image')` to choose a preview path. If the File object is incomplete (missing
`type`), the renderer throws and emits an **error stub** instead of the component:

`<div class="item__error__…">Error: '<message>' in {"id":"<id>","class":"<class>"}</div>`

Fix: give every File `value` the full shape `{ fileName, fileSrc, title, type (mime), size }`.
With that, all of these render in a **static** dump — no backend needed. Related notes:
`upload[type="webcam"]` needs `extra.accept`; `modal`/`float` sections render as overlays
(see §8) once present and visible.

> A crashing item can also take out **later siblings** in the same render pass — e.g. while a
> `file`-cell error broke `table[type=default]`, the `modal`/`float` sections after it failed to
> appear too. Once the File values were completed, the table **and** the overlays rendered. So an
> "absent" component downstream is often a symptom of an error stub upstream — fix the stub first.
> (Earlier drafts of this doc listed these as "needs backend"; the real cause was the missing
> `value.type`, not the backend.)

---

## 10. Stable root-selector cheat sheet

| Component | Reliable root selector |
|---|---|
| grid | `[data-class="grid"]` (+ your styleClass) |
| form | `[data-class="form"]` / `#<formId>` |
| section | `[data-class="section"]`; block via `[class*="block__"]`; modal `[class*="modal__"]`, float `[class*="float__"]` |
| label | `[data-class="label"]` |
| edit | `.edit` or your `ks-*` |
| select | `.select` (outer) |
| multiselect | `.multiselect` |
| radio | `.radio` (narrow with your `styleClass`) |
| check | `.check` |
| toggle | `.toggle` |
| slider | `.slider` (track: `.rc-slider`) |
| otp | `.otp` |
| phone | `[data-class="phone"]` |
| image | `[data-class="image"]` |
| timer | `[data-class="timer"]` |
| comments | `[data-class="comments"]` |
| carousel | `.carousel` (+ your styleClass) |
| button | wrapper `[data-wrapper-for="<id>"]`, button `#<id>.button` (styleClass is here) |
| copy | `[data-class="copy"]` |
| tab | `.tab` / `#<id>.tab` (+ your styleClass) |
| stepper | `[data-class="stepper"]` |
| mainMenu | `nav.mainMenu` (+ your styleClass) |
| upload | `[data-class="upload"]` |
| file | `.file` (+ your styleClass) |
| attachment | `[data-class="attachment"]` |
| signature | `.signature__wrap` (+ your styleClass) |
| table | `[data-class="table"]` (+ `.table__check/radio/group`) |
| row/w | `.row__<rowName>` / `[class*="row__item"]` |
| draggable | `[data-class="draggable"]` |
| widget | `[data-class="widget"]` |

---

## Related documentation

- [CDU Page Protocol](cdu-page-protocol.md) — the page JSON contract (the source-side counterpart
  to this DOM-side reference): grid/forms/sections/component model, templating, change protocol.
- [Smart Forms](smart-forms.md) — project file structure, the `styles/` layer and CSS
  compilation, deploy/release, and the page-serving pipeline.
- `$CLAUDE_PLUGIN_ROOT/skills/simulator-styles/SKILL.md` — the styling skill that consumes these
  hooks (patterns, starter kit, design-system approach).
