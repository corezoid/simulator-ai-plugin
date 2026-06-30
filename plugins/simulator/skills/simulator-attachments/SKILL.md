---
name: simulator-attachments
description: >
  Simulator.Company files & attachments specialist. Use when the user wants to upload a
  file, attach a document/image to an actor or comment, list/rename/detach workspace files.
  Activate when the user says "upload a file", "attach this document", "add an attachment",
  "rename the file", "detach the file", "list workspace files", "завантаж файл",
  "прикріпи файл/документ", "відкріпи вкладення", "загрузи файл", "прикрепи документ".
  For setting an actor's picture/avatar use the `uploadActorPicture` engine tool; for
  comments that carry files use `simulator-reactions`.
---

> **Curated tool names (v2 server):** `uploadBase64`, `getAttachments`, `getActorAttachments`,
> `addAttachments`, `updateAttachment`, `removeAttachments`. Plus the engine tools
> `uploadActorPicture` / `uploadActorPictureBulk` for actor avatars. Call them by these exact names.

# Simulator.Company Files & Attachments Specialist

A **file** is uploaded once into the workspace and becomes an **attachment record** (with an
`attachId`). You then **link** that record to actors or reactions. Listing, renaming and
unlinking operate on the record.

> **Relationship to the other skills**
> - **`simulator-reactions`** — a comment can carry files via its `attachments:[{attachId}]`.
> - **`simulator-actors`** — `uploadActorPicture` sets an actor's avatar (an engine tool, not here).

## The flow: upload → attachId → link

1. **Upload** the bytes → get the attachment record (`attachId`, stored `fileName`).
2. **Link** the `attachId` to an actor/reaction (or pass it in a reaction's `attachments`).
3. **Manage** it later (rename, list, unlink).

## Workspace context

`uploadBase64`, `getAttachments`, `addAttachments`, `removeAttachments` take an `accId`
(workspace id) — it defaults to the configured workspace if omitted. `updateAttachment`
is addressed by `attachId` only.

---

## Upload a file

```
uploadBase64(
  accId="ws_xxx",                  # optional — defaults to the configured workspace
  file="<base64 string>",          # prefer RAW base64; a data:<mime>;base64, prefix is also stripped server-side
  originalName="report.pdf",       # sets the type + title
  ttl=0)                           # seconds; 0 = permanent
# → returns the attachment record incl. attachId + fileName
```

> Prefer raw base64. A `data:` URI prefix is accepted and stripped server-side, but raw base64
> is the safe default. Multipart uploads are not a curated tool — use `uploadActorPicture` for
> actor images, or `uploadBase64` for everything else.

## Link / unlink files

```
addAttachments(accId="ws_xxx", items=[
  { "attachId": 5521, "actorId": "<actor or reaction UUID>" }
])
removeAttachments(accId="ws_xxx", items=[
  { "attachId": 5521, "actorId": "<actor or reaction UUID>" }
])   # unlinks; does NOT delete the stored file
```

## List & rename

```
getAttachments(accId="ws_xxx", limit=50, orderBy="created_at", orderValue="DESC")  # all files in the workspace
getActorAttachments(actorId="<actor UUID>", limit=100)                             # only files linked to one actor
updateAttachment(attachId=5521, title="Q3 report.pdf")
```

> Use `getActorAttachments` when you want a single actor's files; `getAttachments` lists the
> whole workspace. `getActorAttachments` is addressed by `actorId` (no `accId`).

## End-to-end: attach a PDF to an actor

```
# 1. upload
uploadBase64(originalName="contract.pdf", file="<base64>")   → attachId 5521
# 2. link it to the actor
addAttachments(items=[{ "attachId": 5521, "actorId": "<actor UUID>" }])
# (or attach to a comment instead — see simulator-reactions: createReaction(..., attachments=[{attachId:5521}]))
```

---

## Reference Documents

| Path | When to read |
|---|---|
| `$CLAUDE_PLUGIN_ROOT/docs/entities/attachments.md` | Attachment model, statuses, storage, virus-scan flow |
| `$CLAUDE_PLUGIN_ROOT/docs/entities/reactions.md` | Attaching files to comments |

## Tips

- Upload first to get an `attachId`, then `addAttachments` (or a reaction's `attachments`).
- `removeAttachments` only **unlinks** — it does not delete the stored bytes.
- `uploadBase64` prefers raw base64; a `data:<mime>;base64,` prefix is stripped server-side.
- For an actor's avatar/picture use `uploadActorPicture` (handles URL / local path / base64, auto-rasterises SVG).
- `addAttachments`/`removeAttachments` `items` need BOTH `attachId` and `actorId` per entry.
