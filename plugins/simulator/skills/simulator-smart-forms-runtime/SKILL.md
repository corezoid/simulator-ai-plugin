---
name: simulator-smart-forms-runtime
description: >
  Simulator.Company Smart Form RUNTIME driver — run/execute an existing Smart Form
  (CDU / Script / mini-app) on the user's behalf, conversationally, instead of the
  user clicking through its UI. Use when the user wants to GO THROUGH a smart-form
  flow: create/submit something an app produces (e.g. a smart contract), fill in a
  form, walk a multi-step wizard, or feed a document into an app. This is the
  execution counterpart to the `simulator-smart-forms` author skill (which BUILDS
  forms). Activate on: "run the app", "use the smart form", "fill the form",
  "create a smart contract", "submit", "go through the flow", "complete the wizard",
  "запусти застосунок", "пройди смартформу", "заповни форму", "створи смарт контракт",
  "оформи договір", "запусти приложение", "заполни форму", "создай смарт контракт".
---

# Simulator.Company Smart Form Runtime

You **drive an existing Smart Form to completion on the user's behalf** — gathering inputs
from the conversation (and any attached document), filling each page's forms, and walking the
flow step by step. You do **not** build or edit Smart Forms — that is the
`simulator-smart-forms` author skill.

A Smart Form is a **server-driven UI state machine**. Corezoid (its backend process) supplies
the data, validation, control flow, and side effects (creating actors, running transactions).
Your job is the part Corezoid and the GUI can't do: turn an unstructured request into the
right field values, ask only for what's genuinely missing, confirm before irreversible steps,
and report the result.

**Read these references when driving:**
- `$PLUGIN_ROOT/docs/user-flows/cdu-page-protocol.md` — the `get`/`send` protocol, the
  Page → Grid → Form → Section → Item model, the component catalogue (§5), and the change
  protocol (§7). This is the contract you speak.
- `$PLUGIN_ROOT/docs/user-flows/app-catalog.md` — how to discover which app to run
  (match intent against each Smart Form's native `title` + `description`; tags / linked actors
  for scope). No custom catalog format.

---

## Tools you use

| Step | Tool(s) |
|---|---|
| Discover the app | `filterActors` (over the `scripts` system form) → match intent against each actor's `title` + `description` (tags / `getRelatedActors` to scope); or `getActorByRef` when the user names it |
| Render a page | **`appGetPage`** (accId, ref, envTitle, page) |
| Submit a form | **`appSendForm`** (accId, ref, envTitle, page, formId, buttonId, data) |
| Read an attached document | `readAttachment` |
| Report a result | `buildLink` (deep-link to a created actor/record) |

These two — `appGetPage` / `appSendForm` — are universal: the **same two tools drive every
Smart Form**. Nothing about the app is hard-coded; you interpret what the page returns.

---

## The drive loop

```
1. Discover → choose app → (accId, ref, env, entryPage)        # see app-catalog.md
2. page = appGetPage(accId, ref, env, entryPage)
3. loop until the flow ends:
     a. Read page.forms[].sections[].content[] → the items to fill (see §4–5 of the protocol).
        Read page.notifications[] → messages from the process.
     b. Fill a data{} map: itemId → value, from the conversation / extracted document.
        Only value-bearing, visible items (edit, select, multiselect, radio, check, toggle,
        slider, phone, otp, date, upload, table, …). Respect `required`; honour `options`.
     c. Ask the user ONLY for fields that are missing or ambiguous — never re-ask for what
        you already know. Use the item `title`/locale to know what each field means.
     d. CONFIRMATION GATE — if this submit has side effects (creates actors, moves money,
        sends something), show the user a plain-language summary and get an explicit "yes"
        BEFORE sending. Reads/navigation need no gate.
     e. resp = appSendForm(accId, ref, env, page.id, formId, buttonId, data)
     f. Handle resp by code (protocol §2.3):
          200 → apply the `changes[]` to your view of the page (§7), read notifications,
                continue on the same page (more fields, or a result).
          205 → re-render: page = appGetPage(... resp.pageId ...) — a fresh (maybe different) page.
          302 → navigate to resp.nextPage (internal page) or report resp.nextPage (external URL).
     g. Self-correct: if notifications contain a validation error, read helperText, fix the
        offending field (re-ask the user if needed), and re-submit the SAME formId.
4. Report: summarise success notifications and give a buildLink to what was created.
```

**Detecting the end of a flow:** the flow is done when a submit yields a terminal signal — a
success notification with no further form to fill, a 302 redirect to a result/landing page, or
a page with no submittable form. Don't loop forever; if a page repeats unchanged after a
submit, stop and report what the process said.

---

## Filling forms — reading the components

Each item carries `id` (the key you submit under), `class` (the component), `value`,
`visibility`, `required`, and `options`. Map your data to the **item `id`**. Key components
(full catalogue in protocol §5):

- `edit` (text/email/int/float/phone/multiline/date/password) — a scalar value.
- `select` / `radio` — one value from `options[]` (match by option value/label).
- `multiselect` — an array of option values.
- `check` / `toggle` — boolean.
- `upload` / `attachment` / `signature` / `file` — file values.
- `button` — carries the `buttonId` you submit with (and may carry `buttonData`).
- `label` / `divider` / `image` / `table` (display) — read for context; usually not submitted.

If a field's meaning is unclear from its `title`/locale, ask the user rather than guessing.

---

## Entry modes

- **Direct intent** — "create a smart contract with Acme for $50k": discover the app, prefill
  what the user gave, ask for the rest, run the loop.
- **Named app** — "open the onboarding app": resolve the ref directly and run.
- **From a document** — "make smart contracts from this file": read the attachment, extract the
  structured records, then run the loop per record. (Detailed below.)

---

## Document → many records (batch)

The high-value case: the user drops a document and says "create smart contracts from this".
The document is unstructured; the app's forms are structured. You bridge them.

```
1. Read the document:
     getActorAttachments(<reaction or actor id>)  → fileName(s)
     readAttachment(fileName)                      → the document content
2. Extract structured records (this is YOUR job, not the platform's):
     parse the document into N records, each a {field: value} map aligned to the app's inputs.
     One document may describe several contracts → segment into N records. Keep a per-record
     note of what you could NOT find (missing/uncertain fields).
3. Discover the app ONCE (title/description match), then for EACH record run the drive loop
     above, prefilling the record's extracted values.
4. Gap-fill in BATCH: collect the missing/ambiguous fields across all records and ask the user
     once (e.g. "currency is missing for Acme and Globex — which?"), not record-by-record.
5. BATCH CONFIRMATION GATE: show a compact summary of all N records (the key fields you will
     submit) and get one explicit "yes" before sending the series. This is mandatory — each
     submit creates real actors / transactions.
6. Run the per-record submits. On a per-record error: read its notifications, fix if you can,
     otherwise SKIP that record and continue the rest — never abort the whole batch silently.
7. Aggregated report: "created N (links: …); M skipped (reason …)". Give a buildLink per
     created record.
```

Rules for extraction:
- **Show, don't assume.** Surface the extracted values in the confirmation summary so the user
  can catch a misread before anything is created.
- **Never fabricate a required field.** If you can't find it and can't infer it safely, ask.
- **Respect the form's options.** Map extracted values onto the page's `select`/`radio`
  `options` (e.g. a currency string → the matching option), don't invent option values.

---

## In the reaction → AI-agent context

When triggered from a reaction, you carry the reaction author's identity, so app access
(`sharedWith` / `appSettings.groups`) self-scopes — no extra auth wiring. Conduct the dialog as
reactions: ask clarifying questions and the confirmation gate as `ai` replies; the user's
follow-up reaction continues the same flow. Finish with a clear result + `buildLink`.

---

## Key rules

- **You run apps; you don't edit them.** For building/editing pages, switch to
  `simulator-smart-forms`.
- **Never skip the confirmation gate** before a side-effecting `send` (created actors, money).
  Show a summary; get an explicit yes.
- **Prefer `production`** unless the user is testing `develop`.
- **Ask for the minimum.** Prefill from context/document; only prompt for missing or ambiguous
  fields.
- **Trust the protocol, not assumptions.** The forms a page returns ARE the source of truth for
  what to fill; follow `code`/`nextPage` rather than guessing the flow.
- **Surface what the process says.** Relay `notifications` (success and error) to the user;
  self-correct on validation errors instead of silently retrying.
- **Respond in the user's language**; keep these instructions' logic intact.
