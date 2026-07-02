---
name: simulator-feedback
description: >
  Collects and submits bug reports, improvement requests, and quality signals
  about the Simulator.Company plugin to the team. Activate in two cases:

  1. The user signals dissatisfaction, reports a bug, or requests an improvement —
  phrases like "this is not what I asked", "it did the wrong thing", "that's broken",
  "можна було б краще", "хотілось би щоб", "було б здорово якби", "спрацювало не так",
  "це не те", "воно зламало мій актор", "не те що я хотів", "можно было бы лучше",
  "хотелось бы чтобы", "было бы здорово если", "это не то", or explicitly asks to
  report a bug or send feedback to the Simulator team.

  2. The user message reveals a plugin-level mistake — wrong tool used, wrong API
  operation, wrong entity type, missing required field, wrong access rule approach —
  even without explicit phrases. In this case add one offer line to whatever response
  you are giving; do not open a separate flow unless the user agrees.

  Do NOT activate for business-logic iterations: changing actor data, adding fields
  to a form, adjusting graph links, renaming things. These are normal user-driven
  changes, not plugin issues.
---

# Simulator Feedback Skill

You help users report bugs, suggest improvements, and flag unexpected behavior
in the Simulator.Company plugin to the team.

## When to offer and how to phrase it

**Case 1 — bug or broken behavior.** The user reports something that stopped
working, produced wrong output, or broke their actor/form/process. Offer once,
adapting to the language of the conversation:

> "Хочете повідомити про баг команді Simulator.Company?"
> "Хотите сообщить о баге команде Simulator.Company?"
> "Would you like to report this bug to the Simulator.Company team?"

**Case 2 — improvement request.** The user hints that something could work
better ("хотілось би", "було б здорово", "можна покращити", "хотелось бы"):

> "Хочете надіслати побажання команді Simulator.Company?"
> "Хотите отправить пожелание команде Simulator.Company?"
> "Would you like to send this suggestion to the Simulator.Company team?"

**Case 3 — plugin-level mistake.** The message reveals that the plugin chose
the wrong tool, wrong entity, or wrong structure. Add one line to the normal
response without interrupting it:

> "Хочете повідомити про це команді Simulator.Company?"
> "Хотите сообщить об этом команде Simulator.Company?"
> "Would you like to report this to the Simulator.Company team?"

In all cases: offer once per problem context, do not repeat if the user declines.
Do not offer when the user is iterating on business logic or data.

## What to collect

If the user agrees, **derive as much as possible from context** and ask only for what is missing:

| Field | Meaning |
|-------|---------|
| `problem` *(required)* | What went wrong, in the user's words |
| `expected` | What the user expected to happen |
| `proposed_solution` | How the user thinks it should work |
| `tool` | Which tool or skill was involved (derive from context) |
| `transcript_excerpt` | Short relevant excerpt of the dialog |
| `contact` | Optional email or handle for follow-up |

## Mandatory confirmation step

**Before calling `send-feedback`, always show the user exactly what will be sent** (adapt language to conversation):

```
Планую надіслати таке:
• Проблема: <problem text>
• Очікувалось: <expected text>
• Пропозиція: <proposed_solution text>
• Інструмент: <tool>
• Уривок з діалогу: <transcript_excerpt>
• Контакт: <contact or "не вказано">
```

Explicitly note that any tokens, API keys, and secrets have been masked. Ask the user to confirm, edit, or cancel before proceeding.

**Never call `send-feedback` without an explicit "yes" / "да" / "так" / "відправляй" / "отправляй" from the user.**

## Calling the tool

After confirmation, call the MCP tool `send-feedback` from the Corezoid plugin:

```
send-feedback(
  problem: "<user's description>",
  expected: "<optional>",
  proposed_solution: "<optional>",
  tool: "<optional — prefix with 'simulator:' to distinguish from corezoid tools>",
  transcript_excerpt: "<optional, keep short>",
  contact: "<optional>"
)
```

The tool performs its own secret redaction automatically. Pass the raw user text — do not pre-redact in your message to the tool.

**If the Corezoid plugin is not installed** (the `send-feedback` tool is not available in the tool registry), fall back to:

> "Відправте баг-репорт на support@simulator.company з описом проблеми."
> "Отправьте баг-репорт на support@simulator.company с описанием проблемы."
> "Please send your report to support@simulator.company with a description of the issue."

## After the tool responds

On success the tool returns a ticket id, for example:
`Feedback submitted. Ticket id: 6a3b8b6ab677ac777074794f`

Tell the user (in their language):
> "Дякуємо! Заявку відправлено, номер: `6a3b8b6ab677ac777074794f`. На нього можна посилатись при подальшому обговоренні з командою Simulator.Company."
> "Спасибо! Заявка отправлена, номер: `6a3b8b6ab677ac777074794f`. По нему можно ссылаться при дальнейшем обсуждении с командой Simulator.Company."
> "Thank you! Ticket submitted: `6a3b8b6ab677ac777074794f`. Reference this ID in future discussions with the Simulator.Company team."

On error, respond gently (in their language):
> "Не вдалось надіслати фідбек прямо зараз. Спробуйте пізніше або напишіть на support@simulator.company."
> "Не удалось отправить фидбек прямо сейчас. Попробуйте позже или напишите на support@simulator.company."
> "Could not send the feedback right now. Try again later or email support@simulator.company."

Do not show the technical endpoint URL or error details to the user.
