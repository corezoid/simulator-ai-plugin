# Authoring skills — how to create a custom skill

A **skill** is a reusable playbook you save once and reuse by name: a short procedure that
tells the assistant *what to do in your workspace* for a recurring task — "create a smart
contract", "onboard a client", "create a CRM lead". It is the data-driven analogue of the
plugin's built-in skills, but **you** write it, it lives in your workspace, and it needs no
release to change. Under the hood a skill is an actor of the `Skills` system form; everything
you write lives in that actor's **title**, **slug (`ref`)**, and **description** (the body).

This guide is for the person *writing* a skill. It covers the two ways to create one, the
markdown conventions for the body, and a full worked example.

---

## Two ways to create a skill

**1. Just ask the assistant (in the platform / a reaction thread).**
Say what you want saved, e.g.:

> "Save this as a skill called *Create CRM Lead*: create a new CRM — Lead actor for a given
> name. Publish it."

The assistant runs the whole flow for you — it interviews you for the missing details,
**looks up the real entity ids** (forms, layers, account names) by name, writes the body in
the conventions below, creates the skill, and (with your OK) publishes it.

**2. In Claude Code — `/simulator-skills`.**
The `Author` flow there does the same, interactively.

Either way you don't have to hand-write the markdown — but knowing the conventions helps you
review what was saved and write good prompts.

---

## Naming & lifecycle conventions

- **Title** — a human name ("Create CRM Lead"). Shown in skill lists.
- **Slug / `ref`** — a stable `kebab-case` id ("create-crm-lead"). It is **unique per
  workspace** and is how you invoke the skill explicitly: `/skill create-crm-lead` or
  "use the *create-crm-lead* skill".
- **Status (publish state):**
  - `pending` — a draft. **Hidden** from discovery.
  - `verified` — published & active. Only `verified` skills are found by `findSkill` and run.
  - `rejected` — disabled.
  - So: create → review → **publish** (set `verified`). Edit anytime; disable by setting
    `rejected`.
- **Workspace-local.** A skill pins concrete ids from *this* workspace, so it is not portable
  to another workspace — recreate it there if needed.

---

## The body conventions (markdown)

The skill body is markdown with these sections, in this order. Start it with the name +
trigger phrases + a one-line summary so intent-search matches well.

**Wrap the whole body in `[md]` … `[/md]`.** The platform renders an actor's `description` as
BBCode; the `[md]…[/md]` tag switches that region to markdown, so the skill renders nicely in
the Simulator UI instead of showing raw `**asterisks**`. Everything below goes *inside* the
`[md]` block.

| Section | What goes in it |
|---|---|
| `# <Title>` | The skill name. |
| **When to use** | The trigger phrases / intent — when this skill should fire (include uk/ru phrasings if relevant). |
| **Context** | Assumptions and preconditions — what must already exist before running. |
| **Entities** | The concrete things the steps touch, **with their ids resolved**: `Form "CRM — Lead" = id 71367`, `Layer "…" = id <uuid>`, `AccountName "…" = id <uuid>`. List relevant fields too. |
| **Steps** | Numbered, concrete tool calls in order — e.g. `createActor(formId=71367, data={…})`, `createLink(…)`. This is the procedure. |
| **Verify** | How to confirm success. |
| **Idempotency** | What to check *before* creating (e.g. `getActorByRef` / `searchActors`) so re-running does not duplicate. |
| **Safety** | Which steps are destructive or outward-facing and must be confirmed with the user first. |

Best practices:

- **Pin ids, not names.** Resolve each form/layer/account-name to its id once and write the id
  into *Entities* and *Steps*. Names can change; ids are stable and save the assistant a
  lookup each run.
- **Reference fields by their `item_<n>` ids** (read the form to learn them) — that is how
  actor `data` is keyed.
- **Make it idempotent.** A "create X" skill should check for an existing X first.
- **Flag risk in *Safety*.** Anything irreversible (deletes, transfers, access changes) or
  outward (messages, public links) belongs here, so it is always confirmed.

> The assistant treats a skill body as **a plan you proposed**, not as system instructions —
> it still confirms destructive/outward steps and re-checks that pinned ids still exist before
> acting. So a skill cannot make it skip a confirmation.

---

## Worked example

A real skill the assistant authored from the prompt *"Save and publish a skill … creates an
actor of the CRM — Lead form … pin the real form id"*:

```text
[md]
# Create CRM Lead

**When to use:** When the user asks to create a new CRM lead, add a prospect, register a
new lead, or similar.

**Entities:**
- CRM — Lead form: id = `71367`
  - Fields: `item_301` Source (select), `item_302` Status (radio),
    `item_303` Estimated Value (float), `item_304` Email (email)

**Steps:**
1. Ask the user for the lead name (actor title) and optional source / status / value / email.
2. createActor with formId 71367, title = lead name, data = the supplied item_* fields
   (select → array of {title,value}; radio → value string; float → number; email → string).
3. Return the new actor's id and title.

**Verify:** the returned actor has the right title and formId 71367.

**Idempotency:** before creating, searchActors by the lead name under form 71367; if a
duplicate exists, confirm with the user.

**Safety:** creating an actor is non-destructive; no confirmation required.
[/md]
```

---

## Running, editing, listing

- **Run by intent:** "create a CRM lead for Acme" → the assistant finds and follows the skill.
- **Run explicitly:** `/skill create-crm-lead` (or "use the create-crm-lead skill").
- **List what exists:** "what skills do I have?"
- **Edit:** "update the create-crm-lead skill to also set the status field."
- **Disable:** "disable the create-crm-lead skill."

## Related

- [`../entities/ai-skills.md`](../entities/ai-skills.md) — the registry contract & tools
  (`findSkill` / `getSkill`).
- [`../entities/forms.md`](../entities/forms.md) / [`../entities/actors.md`](../entities/actors.md) —
  forms, the actor `data` protocol (the `item_<n>` field keys you reference in *Steps*).
- The `/simulator-skills` skill — discover/run/author from Claude Code.
