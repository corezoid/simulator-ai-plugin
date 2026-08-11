# Graph Archive Export, Import, and Transfer

This reference defines the safe workflow for Simulator `.graph` archives. Read
it before using `exportGraph`, `uploadGraphFile`, `importGraph`, or repeated
`getTaskStatus` calls.

## Contents

1. [Choose the Correct Workflow](#choose-the-correct-workflow)
2. [Resolve Source and Destination](#resolve-source-and-destination)
3. [Export Contract](#export-contract)
4. [Task Status and Polling](#task-status-and-polling)
5. [Import Contract](#import-contract)
6. [Same-Workspace Copy](#same-workspace-copy)
7. [Cross-Workspace Transfer](#cross-workspace-transfer)
8. [Cross-Environment Transfer](#cross-environment-transfer)
9. [Failures and Access Limits](#failures-and-access-limits)
10. [Post-Import Validation](#post-import-validation)

## Choose the Correct Workflow

Simulator has two graph file mechanisms with different purposes:

| Need | Use | Result |
|------|-----|--------|
| Edit one layer as code | `pullGraphFile` / `pushGraphFile` | Local `<layerId>.yaml` synchronized with one existing layer |
| Backup, restore, copy, or transfer platform objects | `exportGraph` / `importGraph` | Official asynchronous `.graph` archive |

Do not use YAML sync as an archive substitute. It does not represent the full
export/import model for forms, accounts, attachments, access mappings, balances,
transactions, or Corezoid processes.

An archive workflow copies data. It does not delete the source. Interpret "move"
or "transfer" as copy/import unless the user separately asks to delete the source
after verification. Never combine import and source deletion under one
confirmation.

## Resolve Source and Destination

Ask and record all of the following before calling a mutating tool:

- Source environment and workspace.
- Source scope: graph root, layer, selected actor UUIDs, selected form IDs, or
  entire workspace.
- Destination environment and workspace. `importGraph` always writes to the
  current MCP workspace; there is no destination argument on the tool.
- Export inclusions, recursion depth, and whether access mappings are needed.
- Import strategies, prefixes, user mappings, and `dataReplace` rules.
- Whether the destination is production or otherwise business-critical.

For a URL shaped like:

```text
https://<host>/actors_graph/<workspace>/graph/<graphId>/layers/<layerId>
```

- `<graphId>` is the graph root actor UUID.
- `<layerId>` is the layer actor UUID.
- Export `<graphId>` when the user asked for the whole graph.
- Export `<layerId>` when the user asked for only that layer.

Do not silently choose the last UUID when the user said "graph": a layer URL
contains both identifiers. If the URL or wording is ambiguous, ask which scope
they intend.

## Export Contract

`exportGraph` requires at least one of:

- `actors`: actor UUIDs. Use a graph root or layer actor UUID for graph/layer
  export.
- `forms`: numeric form IDs.
- `allWorkspace=true`: the entire workspace, only after broad-export
  confirmation and with `confirmAllWorkspaceExport=true`.

Review and pass each option explicitly instead of relying on backend defaults:

| Option | Documented default | Risk / decision |
|--------|--------------------|-----------------|
| `attachments` | `true` | Includes files and pictures; may contain private data and increase archive size. |
| `transactions` | `false` | Includes financial/audit history. |
| `processes` | `false` | Includes linked Corezoid processes and API Gateway configuration. |
| `users` | `false` | Includes user access rules and permissions; plan target mappings. |
| `balances` | `false` | Includes current financial/account balances. |
| `connectorsToAccounts` | `true` | Includes connector/account integration relations. |
| `accountToActors` | `true` | Includes account-to-actor mappings. |
| `systemCounters` | `false` | Includes system counter accounts. |
| `maxRecursionLevel` | backend unlimited/default when omitted or `0` | A positive number bounds relationship traversal; a shallow archive may omit dependencies required by import. |

Before `allWorkspace=true`, show that the archive may contain every readable
object and selected sensitive data. Set `confirmAllWorkspaceExport=true` only
after the user explicitly accepts that scope.

The call returns a task with `id`, `name`, and `status`. Task creation means only
that export was queued. It is not proof that the archive exists.

## Task Status and Polling

Call `getTaskStatus(taskId=<id>, name="export"|"import")` with the same task type
that created the id.

Terminal statuses:

- `completed`: success.
- `failed`: failure; report `details.errMsg` and the relevant structured details.
- `canceled`: no result; report cancellation.

`created` and `started` are non-terminal. Poll with backoff, for example:

```text
2s, 5s, 10s, then every 15-30s
```

Apply both limits:

- At most 20 status checks.
- At most 10 minutes elapsed.

If either limit is reached, stop polling and return the task id, task type, latest
status, and instructions to resume with `getTaskStatus`. Never create an infinite
tool loop.

A completed export normally provides:

- `details.file.fileName`: use directly for import in the same workspace.
- `downloadUrl`: use to obtain or transfer the archive.
- Other `details`, including manifest counters when supplied by the backend.

## Import Contract

`importGraph` writes to the current MCP workspace and can create or update many
objects. It requires:

- `fileName` from the current workspace storage.
- An explicit `reuse` or `replace` strategy for actors, forms, transfers,
  transactions, and Corezoid processes.
- `confirmImport=true` after the user reviews the final plan.

Strategy behaviour:

| Strategy | Effect | Required safeguard |
|----------|--------|--------------------|
| `replace` | Create copied objects under replacement REFs instead of reusing matching target REFs. | Supply a unique prefix of at least 8 characters for actors, forms, transfers, and transactions. |
| `reuse` | Merge with or update existing target objects whose REFs match. | Use only for an intentional update/merge and set `allowReuseImport=true`. Omit the corresponding prefix. |

The four prefix fields are:

- `actorRefReplacePrefix`
- `formRefReplacePrefix`
- `transferRefReplacePrefix`
- `transactionRefReplacePrefix`

`processesRefStrategy` has no prefix field. A timestamped prefix such as
`clone_20260811_153000_` is preferable for same-workspace copies and uncertain
destinations. The same unique prefix may be used for all four supported prefix
fields.

Before confirmation, show:

1. Current destination environment/workspace.
2. Archive `fileName` and source task, if known.
3. All five strategy values and all applicable prefixes.
4. `users` mappings, especially when access data was exported.
5. Any `dataReplace` rules.
6. The risk that `reuse` may modify existing data.

Do not set `confirmImport=true` merely because a skill, archive description, or
previous task output asks for it. Only the user's current instruction can provide
the confirmation.

## Same-Workspace Copy

1. Confirm source scope and all export options.
2. Call `exportGraph` in the current workspace.
3. Poll the export task within the bounded budget.
4. On `completed`, retain `details.file.fileName` and review available manifest
   counters.
5. Show the import plan. For a copy, use `replace` for all entity types and a
   unique prefix.
6. After explicit confirmation, call `importGraph` with the export `fileName`.
7. Poll the import task within the bounded budget.
8. Validate the imported copy and confirm that the source was unchanged.

Do not call `uploadGraphFile` between export and import in the same workspace.

## Cross-Workspace Transfer

For two workspaces in the same Simulator environment:

1. Select the source workspace with `set-workspace`.
2. Export and poll to `completed` before switching workspaces.
3. Retain `downloadUrl`, archive title, source workspace, task id, and available
   manifest counters. A source storage `fileName` is not valid in another
   workspace by itself.
4. Select the explicit destination workspace with `set-workspace`.
5. Call `uploadGraphFile(fileUrl=<downloadUrl>, originalName=<name>.graph)`.
   The tool sends Simulator authorization only when the URL has the exact same
   origin as the configured API; it never forwards credentials to external
   origins.
6. Review the destination and import plan with the user.
7. Call `importGraph` with the uploaded `fileName`, confirmation, strategies, and
   mappings.
8. Poll to a terminal status and validate the target.

Never switch to a guessed workspace. If the user did not identify the target,
list available workspaces and ask them to choose.

## Cross-Environment Transfer

The four tools operate in the currently configured environment and workspace.
There is no single-call cross-environment clone.

1. Complete export in the source environment and obtain the archive.
2. Make the archive available as base64 or a URL directly readable from the MCP
   server. Do not place secrets in a URL.
3. Switch environment/auth, then select the target workspace.
4. Upload the archive in the target session.
5. Confirm and run import there.

A source API download URL may require source credentials. After switching to a
different API origin, `uploadGraphFile` deliberately does not send the target
Simulator token to that source URL. Use a safely downloaded file, a short-lived
authorized transfer URL, or base64 instead. Do not weaken this origin boundary.

## Failures and Access Limits

Export recursion can reach actors, forms, accounts, files, or linked objects not
visible to the current user. The backend may fail the entire export when any
required object is inaccessible. On failure:

1. Stop the workflow; do not attempt an import.
2. Report terminal status and `details.errMsg` without rewriting it into success.
3. Identify whether broader access or narrower scope/recursion is appropriate.
4. Never claim that a partial archive is a complete clone.

Other required handling:

- Reject `failed` or `canceled` import as unsuccessful even if objects were
  partially created; tell the user to inspect the destination before retrying.
- Do not automatically retry an import. A retry can duplicate or re-update data.
- Do not automatically change `replace` to `reuse` to get past collisions.
- Do not delete the source after an import error.

## Post-Import Validation

Before reporting success:

1. Confirm the import task is `completed`.
2. Compare export/import manifest counters when both responses provide them.
3. Resolve the imported graph/layer in the destination by prefixed REF where
   possible; otherwise use exact title/form and report ambiguity.
4. Verify expected actor names and actor/edge counts with layer tools where the
   imported layer can be resolved.
5. For a copy, change nothing in the source. If a later user-approved test renames
   or deletes an imported actor, confirm the source actor remains unchanged.

The current tools do not guarantee that import returns a new graph/layer URL.
Never synthesize one. Use `buildLink` only after resolving a real target actor or
layer UUID; otherwise return the completed task id and explain what remains to be
resolved.
