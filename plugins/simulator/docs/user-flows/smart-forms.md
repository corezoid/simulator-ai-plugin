# Smart Forms (Applications / CDU) — Lifecycle & Behaviour

This document describes, in depth, how **Smart Forms** work on the Simulator.Company
platform: what they are, the data model behind them, every stage of their lifecycle, and
the request/response flows that drive them. It is written to be useful both to a human
engineer reading it cold and to an AI agent reasoning about how to operate the platform.

> **Source of truth.** This document is derived from the backend implementation in
> `pong-server`, under
> `src/packages/controlMain/api/applications/` (HTTP layer),
> `.../data/applications/` (DB layer), `.../models/applications/` (ORM models),
> `.../helpers/applications/`, `.../middleware/checkAppAccess.ts`, and
> `.../cache/applications/getSmartForms.js`. File/line references are given throughout so
> the behaviour can be re-verified against code.

---

## 1. What a Smart Form is

A **Smart Form** is an interactive web application that runs on the Simulator.Company
platform. In code and UI you will also see it called a **Script**, **CDU** ("Custom
Development Unit"), or **application** — these are the same thing viewed from different
angles:

| Name you'll see | Where it appears | What it emphasises |
|---|---|---|
| **Smart Form** | UI / product, `smart_forms` API | The installable, end-user-facing app |
| **CDU** | `applications` / `app_content` API, code comments | The editable project (folders + files) |
| **Script** | system form name `scripts`, `pages` API (`getScriptPage`) | The runtime that serves pages |
| **Application** | `applications.js`, access object type `application` | The actor + its environments + releases |

Conceptually a Smart Form is **three things bundled together**:

1. **An actor** in the system form `SCRIPTS` — this gives it an identity (`id`, `ref`,
   `title`, `description`, `picture`) inside a workspace and ties it into the platform's
   access-control and graph model. See `applications.js:197-218` (`createActorReq` into
   `SYSTEM_FORMS.SCRIPTS`).
2. **A versioned project** — a tree of folders and files (pages, styles, locale,
   view-model, widget config, definitions) stored per **environment**.
3. **A backend binding** — Corezoid credentials per environment. When a page is requested,
   the platform calls a Corezoid process to fetch dynamic data, then renders the page from
   the project files.

A Smart Form is therefore **not** a regular Simulator "form template". A regular form
defines the *data schema* of actors created from it (see
[`custom-car-form.md`](custom-car-form.md) and [Forms](../entities/forms.md)). A Smart
Form *is itself an actor* (in the `scripts` system form) that carries a whole editable
application alongside it.

### Relationship to the rest of the platform

```
Workspace (accId)
   │
   ├── system form "scripts"  ──►  Smart Form actor (id, ref, title, data.sharedWith)
   │                                   │
   │                                   ├── access rules (objType: "actor"/"application")
   │                                   └── appSettings { expired, users[], groups[] }  (context gating)
   │
   ├── system form "graphs"   ──►  companion graph actor  "smartForm_graph_actor@<actorId>"
   │                                (optional, created when withGraph=true)
   │
   └── application backing tables (keyed by actor_id):
           app_envs ──< app_folders ──< app_files ──< app_file_versions
                   └─< app_releases ──< app_release_objects
```

---

## 2. Data model

Six PostgreSQL tables back a Smart Form. All are keyed (directly or transitively) by the
Smart Form's **`actor_id`** (the id of the actor in the `scripts` form).

### 2.1 `app_envs` — environments
ORM: `models/applications/AppEnvs.js`. Every Smart Form gets **exactly two** environments
on creation: `develop` (writable) and `production` (read-only). See
`applications.js:71-99` (`createAppEnvs`).

| Column | Type | Meaning |
|---|---|---|
| `id` | INTEGER PK | Environment id |
| `actor_id` | VARCHAR | The Smart Form actor it belongs to |
| `title` | VARCHAR | `'develop'` or `'production'` |
| `readonly` | BOOLEAN | `develop`→`false`, `production`→`true` |
| `is_system` | BOOLEAN | Both default envs are system envs |
| `corezoid_credentials` | JSONB | `{ procId, companyId, apiLogin, apiSecret }` — the Corezoid process this env talks to |
| `owner_id` | INTEGER | Creator |
| `created_at` / `updated_at` | INTEGER (unixtime) | |

The shape of `corezoid_credentials` is `CorezoidCredentials` in
`types/applications-types.ts:144-149`.

### 2.2 `app_folders` / `app_files` — the editable project tree
ORM: `AppFolders.js`, `AppFiles.js`. A recursive folder tree (via `parent_id`) holds files.
The project's default skeleton is defined in
`helpers/applications/getProjectStructure.js`:

```
root/                         (isSystem)
├── definitions/              reusable component fragments referenced via "$ref": "#/button"
│   └── button                (application/json)
├── pages/                    one sub-folder per page
│   └── index/                the default landing page
│       ├── config            (application/json) — grid + forms layout of the page
│       └── locale            (application/json) — page-level i18n strings
├── locale                    (application/json) — app-wide i18n strings
├── viewModel                 (application/json) — default view-model values
├── widgets                   (application/json) — widget / control settings (ctrlSettings)
└── styles/
    └── index                 (text/css) — entry stylesheet
```

Key columns:

| `app_folders` | | `app_files` | |
|---|---|---|---|
| `id` PK | | `id` PK | |
| `env_id` | which env | `folder_id` | parent folder |
| `parent_id` | tree link (NULL = root) | `title` | file name (e.g. `config`, `index`) |
| `title` | folder name | `type` | MIME (`application/json`, `text/css`, …) |
| `is_system` | system asset, protected | `source` | TEXT — the file contents |
| `is_static` | static page (skip Corezoid call) | `is_system` | protected from edit/delete |
| `deleted_at` | soft-delete timestamp | `deleted_at` | soft-delete timestamp |

Notes:
- **System objects** (`is_system=true`) cannot be renamed, edited (beyond title), or
  deleted by users — `appContent.js:155-157`, `:281-285`. Recursive folder deletes stop at
  any system folder/file so the CSS compiler's assets are never stripped
  (`foldersFilesDB.js:499-509`).
- **Static pages** (`is_static=true`) are served without calling Corezoid — `pages.ts:293`.
- Deletes are **soft** (`deleted_at` set), so history and restore keep working
  (`appContent.js:263-266`).

### 2.3 `app_file_versions` — per-file history
ORM: `AppFileVersions.js`. A *before-state* snapshot is written on every mutation. The
operation type is auto-detected on update (`foldersFilesDB.js:435-465`):

```js
let operation = 'update';
if (before.folderId !== folderId) operation = 'move';
else if (before.title !== title)  operation = 'rename';
```

Plus `'create'` (on file creation) and `'delete'` (on soft-delete,
`foldersFilesDB.js:482-494`). Columns: `file_id`, `operation`, `title`, `type`, `source`,
`source_hash` (SHA-256, CHAR(64)), `folder_id`, `user_id`, `created_at`.

**Retention:** the newest **50** versions per file are kept
(`DEFAULT_PER_FILE_VERSION_LIMIT = 50`, `foldersFilesDB.js:18`; override via config
`applications.history.perFileVersionLimit`). Trimming runs after every write
(`trimFileVersions`).

### 2.4 `app_releases` / `app_release_objects` — immutable deploy snapshots
ORM: `AppReleases.js`, `AppReleaseObjects.js`. A **release** is a frozen copy of an
environment's whole tree, created when you deploy. The manifest (`app_releases`) records
status and lineage; `app_release_objects` holds the snapshotted folders/files.

`app_releases` columns:

| Column | Meaning |
|---|---|
| `id` BIGINT PK | Release id |
| `actor_id` | Smart Form |
| `env_id` | The **target** env this release belongs to |
| `release_number` | Linear, per-env (unique `(env_id, release_number)`) |
| `source_env_id` | Where the snapshot was taken from |
| `parent_release_id` | Set when this release was produced by a *rollback* |
| `title` / `description` | e.g. `"Deploy from develop"`, `"Rolled back to v3"` |
| `status` | `'active'` \| `'archived'` \| `'rolled_back'` (default `'active'`) |
| `user_id` | Who triggered it |
| `created_at` / `archived_at` | unixtime |

`app_release_objects` snapshots each node by a **stable `original_id`** (so renames can be
diffed across releases), with `parent_original_id`, `title`, `type`, `source`, `source_hash`,
`is_system`, `is_static`.

**Retention:** the newest **5** releases' *objects* are kept per env
(`DEFAULT_RELEASE_LIMIT = 5`, `releasesDB.js:8`; override via
`applications.history.releaseLimit`). Older release **objects** are pruned by
`pruneOldReleaseObjects`, but the manifest rows remain for audit. A release whose objects
were pruned can no longer be rolled back (`releases.js:137-145`).

---

## 3. Lifecycle overview

A Smart Form moves through these stages. Each is detailed in §4.

```
   ┌─────────────┐   install    ┌────────────┐   edit files   ┌─────────────┐
   │  CATALOG    │ ───────────► │  CREATED   │ ─────────────► │  DEVELOPING │
   │ (templates) │  (async task)│ (actor +   │  app_content   │ (develop env│
   └─────────────┘   or create  │  2 envs)   │  + history     │  changes)   │
                                 └────────────┘                └──────┬──────┘
                                                                       │ deploy develop → production
                                                                       ▼
   ┌─────────────┐   serve pages   ┌──────────────┐   new release   ┌─────────────┐
   │  END USERS  │ ◄────────────── │  RELEASED    │ ◄────────────── │  DEPLOYING  │
   │ (pages API) │  Corezoid+CSS   │ (prod active │  T1 snapshot    │ (T1 + T2)   │
   └─────────────┘                 │   release)   │  T2 materialize └─────────────┘
                                    └──────┬───────┘
                                           │ rollback (creates new forward release)
                                           ▼
                                    previous release re-materialised; old active → archived
```

Plus two cross-cutting capabilities available throughout the DEVELOPING/RELEASED stages:
**file history / restore** (§4.4) and **trash** for soft-deleted objects.

---

## 4. Lifecycle in detail

> **Auth.** Every endpoint below exists on two surfaces (see §5.0): the **internal** API
> (`/api/1.0/...`, browser session via `checkAuth`) and the **public** API
> (`/papi/1.0/...`, OAuth2 bearer via `checkPublicApiAuth([scope])`). The public scope for
> each is listed in §7. The paths and handlers are identical on both; only the auth
> middleware differs. The `pages` endpoints additionally apply `checkAppAccess` /
> `sharedWith` rules. The descriptions below use the bare path (e.g. `/applications/:accId`)
> — prefix with `/api/1.0` or `/papi/1.0` as appropriate.

### 4.0 The catalog (discovering installable Smart Forms)

Before creating from scratch, a workspace can browse a **catalog** of pre-built Smart
Forms.

- `GET /api/1.0/smart_forms/list/:accId` → `getSmartFormsListReq` (`smartForms.js:104-111`).
  Returns the catalog. Implementation (`cache/applications/getSmartForms.js`):
  - fetches templates via `graphInstallerApi('files.get', { group: 'smartform' })`,
  - for each catalog entry with `data.unique === true && data.ref`, checks whether an actor
    with that `ref` already exists in the **`graphs`** system form, and if so marks it
    `installed: true`,
  - caches the result in Redis under `smartFormsLists_<accId>` for **60 s** (`TTL = 60`).
- The optional `?filter=` query is **field selection** (projection), applied via
  `filterData` — it trims which fields are returned, it does not filter rows.

### 4.1 Installing a Smart Form (from the catalog)

`POST /api/1.0/smart_forms/:accId` → `installSmartFormReq`
(`smartForms.js:116-173`). Requires permission `SCRIPTS_MANAGEMENT` **or**
`ACTORS_MANAGEMENT`.

Body: `{ title, ref, description, fileUrl, smartFormRef }`.

What it does:
1. Verifies no actor with `ref` already exists in the `scripts` form
   (`ERR_DUPLICATE_REF` otherwise).
2. Creates a **scoped API key** in the workspace (via SuperAdmin / `getSingleOAuth2Obj`),
   granting all `__SCOPES__`. A backing API user is created in `usersDB`.
3. Launches an **async import task** (`ASYNC_TASKS_NAMES.IMPORT`) that imports the package
   from `fileUrl`, using a `dataReplace` rule to rename the imported actor
   (`from: {type: ACTOR, ref: smartFormRef}` → `to: {title, description, ref}`) and a
   `postInstall` task that writes runtime config (base URLs, token, workspace id).
   Import strategy for actors/forms/transfers/transactions is `'reuse'` (`importOps`).
4. Invalidates the catalog cache (`cleanSmartFormsCache`).
5. Returns `{ data: { task } }` — installation is **asynchronous**.

**Polling for completion:**
`GET /api/1.0/smart_forms/check_install/:taskId` → `checkSmartFormInstallReq`
(`smartForms.js:178-198`). Only the task owner may check (`ERR_OWNER`). When the task
status is `COMPLETED`, it resolves the imported actor by its `ref` and returns
`{ data: { task, actor } }` (actor with privileges). Until then, just `{ data: { task } }`.

> **AI prompt helper.** `POST /api/1.0/smart_forms/prompt_node/:accId` →
> `promptNodeReq` (`smartForms.js:203-259`) forwards a prompt (file or free text) to an
> external **API Gateway** (`SETTINGS.API_GW_HOST` + `SETTINGS.PROMPT_NODE_ENDPOINT`) with a
> signed JWT, used to AI-generate parts of a Smart Form. Requires the gateway to be
> configured (`ERR_PROMPT_APIGW`).

### 4.2 Creating a Smart Form from scratch

`POST /api/1.0/applications/:accId` → `createAppReq` (`applications.js:180-285`). Requires
`SCRIPTS_MANAGEMENT` **or** `ACTORS_MANAGEMENT`.

Body: `{ title, picture, description, ref, sharedWith?, corezoidCredentials, devScriptStuct?, prodScriptStuct? }`.
Query: `?withGraph=true` (default).

Steps (note the ordering — the actors are created first, then a single DB transaction wraps
the rest):
1. Reject `ref` that collides with the configured API-gateway ref (`ERR_API_GW`).
2. Create the **Smart Form actor** in the `scripts` form, with `data: { sharedWith }`
   (default `'userList'`).
3. If `withGraph`, create a companion actor in the `graphs` form with
   `ref = "smartForm_graph_actor@<actorId>"` (lets the app appear/behave on graphs).
4. **In one transaction:** create the two environments (`createAppEnvs`), populate each
   env's project tree (`createProjectStruct`) using `devScriptStuct`/`prodScriptStuct` if
   supplied, else the default skeleton from `getProjectStructure()`, and grant the creator
   `{view, modify, remove}` access rules. Commit.
5. Return the assembled app model (`makeAppModel`): the actor + its `envs`. If the caller
   lacks `modify`, `corezoidCredentials` are stripped from the returned envs
   (`applications.js:62-64`).

**Reading an app:**
- `GET /api/1.0/applications/:actorId` → `geAppReq` — app/actor details (passes through
  `getAppParams` + `checkAppAccess(false)`).
- `GET /api/1.0/applications/envs/:actorId` → `getAppEnvsReq` — the env list (needs
  `view` on the actor).

**Updating environment backend binding:**
`PUT /api/1.0/applications/env/:actorId/:envId` → `updateEnvCredReq`
(`applications.js:290-302`, needs `modify`). Replaces an env's `corezoid_credentials`.

### 4.3 Developing (editing the project)

All editing happens against an environment's folder/file tree — in practice the `develop`
env, because `production` is `readonly` and writes are rejected (`validate` in
`appContent.js:24-43`). Base prefix `/api/1.0/app_content`, all need `modify` on the actor.

| Endpoint | Handler | Purpose |
|---|---|---|
| `POST /:actorId` | `createObjsReq` | Create folders/files (batch). Files get a `create` history row. |
| `PUT /:actorId` | `updateObjsReq` | Update folders/files (batch). Files get a before-state history row with auto-detected `update`/`move`/`rename`. System objects can only change non-title fields. |
| `GET /:actorId/:folderId` | `getFolderContentReq` | List the immediate children of one folder. |
| `GET /struct/:actorId/:envId` | `getEnvStructReq` | The **entire** env tree, built recursively by `getDBProjectStruct` from the env's `rootFolderId`. |
| `DELETE /:actorId` | `removeObjsReq` | **Soft-delete** folders/files (cannot delete system objects; readonly envs rejected). |

Side effects of every successful mutation:
- **Non-production CSS caches are invalidated** (`invalidateNonProductionCssCaches`).
- A **real-time event** is published (`publishCDUContentChanges`, action
  `create`/`update`/`delete`) so collaborators' editors update live.

> **Validation note.** The save endpoint validates only the envelope (`objType` enum, required
> `title`/`type`, folder ownership, readonly/`isSystem` rules) — the file `source` (page
> `config` JSON, `viewModel`, `locale`, CSS) is stored **opaque, with no structural check**
> against the protocol. Authoring tooling should validate page JSON against the protocol
> *before* saving. Details in **[CDU Page Protocol §10](cdu-page-protocol.md#10-server-side-validation-of-saved-pages)**.

### 4.4 File history, rollback & trash

Base prefix `/api/1.0/file_history`.

| Endpoint | Handler | Auth | Purpose |
|---|---|---|---|
| `GET /:actorId/:fileId` | `listFileVersionsReq` | `view` | Paged version list for a file (`limit`/`offset`, default 50/0). |
| `GET /:actorId/:fileId/:versionId` | `getFileVersionReq` | `view` | One specific version (full `source`). |
| `POST /:actorId/:fileId/rollback` | `rollbackFileReq` | `modify` | Restore a file to a prior version (writes a new history row; readonly env rejected). |
| `GET /trash/:actorId/:envId` | `listTrashReq` | `view` | List soft-deleted (`deleted_at` set) objects in an env. |
| `POST /trash/:actorId/restore` | `restoreReq` | `modify` | Restore soft-deleted objects by `{ ids: [{id, objType}] }`. |

Rollback and restore also invalidate non-production CSS caches and publish CDU content
events.

### 4.5 Deploying (develop → production) and releases

Deploying snapshots one environment into another (typically `develop` → `production`) as an
immutable **release**, then makes that release the live content of the target env.

`POST /api/1.0/applications/deploy/:actorId` → `deployAppReq` (`applications.js:310-419`).
Requires **`execute`** on the actor. Body: `{ sourceEnvId, targetEnvId }`
(must differ).

It runs as **two short transactions** so a long write lock is never held across both the
source read and the target rewrite:

**T1 — snapshot into a release** (`applications.js:330-359`):
1. `prepareEnvTx({ envId: target, snapshot: true })` takes a `FOR UPDATE` lock on the
   target env row. This **serialises concurrent deploys/rollbacks** on the same env so two
   flows' T1/T2 never interleave.
2. `archivePreviousActive` flips the currently-`active` release on the target env to
   `archived`.
3. Allocate the next linear `release_number`, `createRelease` (status `active`,
   `source_env_id = source`).
4. `snapshotEnvToRelease` copies the source env's whole tree into `app_release_objects`.
5. Commit (lock released).

**T2 — materialise into the target env** (`applications.js:362-391`):
1. Re-lock the target env.
2. `removeEnvObjs(target)` wipes the target env's current folders/files.
3. `materializeReleaseToEnv` rebuilds the tree from the release objects (fresh folder/file
   ids).
4. Commit.

**Failure handling:** if T2 fails, `revertFailedRelease` reconciles the manifest with the
(rolled-back) env contents so the live release always matches what's actually in the env.

**Post-commit side effects** (`applications.js:393-415`):
- Invalidate the target env's compiled CSS (`invalidateCssCacheByEnvId`).
- Background **pre-warm** the new CSS (`preWarmEnvCss`).
- Publish a `deployed` real-time event (`publishCDUReleaseEvent`).
- Background **GC** of old release objects (`pruneOldReleaseObjects`).

Returns `{ data: { ok: true, release } }`.

**Inspecting releases** — base prefix `/api/1.0/releases` (all need `view`, except rollback
needs `execute`):

| Endpoint | Handler | Purpose |
|---|---|---|
| `GET /:actorId` | `listReleasesReq` | List releases (`?envId`, `?status`, `limit`/`offset`). |
| `GET /:actorId/:releaseId` | `getReleaseReq` | One release manifest. |
| `GET /:actorId/:releaseId/struct` | `releaseStructReq` | The release manifest **plus** its snapshotted objects. |
| `GET /:actorId/:releaseId/diff?vs=<otherId>` | `releaseDiffReq` | Diff two releases as `{ added, removed, modified }`. Compares by stable `original_id`; equality uses `source_hash`, so **no file bytes leave the DB** for the diff. |
| `POST /:actorId/:releaseId/rollback` | `rollbackReleaseReq` | Roll the env back to that release's state. |

### 4.6 Rollback

`POST /api/1.0/releases/:actorId/:releaseId/rollback` → `rollbackReleaseReq`
(`releases.js:116-229`). Requires `execute`.

Rollback **does not rewind history** — it moves forward by creating a *new* release whose
content equals the target release, keeping history linear:

**T1** (`releases.js:130-172`): lock the env; **re-check inside the lock** that the target
release still `hasObjects` (guards against concurrent GC pruning them — else
`HTTP_BAD_REQUEST` "Release content was archived by retention policy"); archive the current
active release; create a new `active` release with `parent_release_id = target.id` and title
`"Rolled back to v<n>"`; `copyObjectsBetweenReleases(target → new)`; mark the **target**
release `rolled_back` (audit marker); commit.

**T2** (`releases.js:180-203`): lock, `removeEnvObjs`, `materializeReleaseToEnv(new)`,
commit. Same `revertFailedRelease` compensation on failure.

Post-commit: invalidate + pre-warm CSS, publish a `rolledback` event, background prune.
Returns `{ data: release }` (the new release).

### 4.7 Serving pages to end users

This is the runtime path that turns a Smart Form into a live web app. Base prefix
`/api/1.0/pages`. These endpoints are gated by **`checkAppAccess(false)`** (see §5), not by
a session check — they can serve anonymous users when `sharedWith === 'anyone'`.

> **The page JSON contract these endpoints exchange** — the `Page` / `grid` / `forms` /
> `sections` / component structure, the `get`/`send` request-response codes (200 / 205 / 302),
> the `changes` patch protocol, `viewModel` / `locale` / `definitions` / `contentLoop` / `bbcode`
> templating, the full component catalogue, and **what the save endpoint does and does not
> validate** — is specified in **[CDU Page Protocol](cdu-page-protocol.md)**.

| Endpoint | Handler | Purpose |
|---|---|---|
| `GET /:accId/:ref/:envTitle` | `getScriptConfig` | App-level config: compiled `style` (CSS) + `widgets`. |
| `GET /:accId/:ref/:envTitle/:page` | `getScriptPage` | Render one page. |
| `POST /:accId/:ref/:envTitle/:page` | `sendScriptPage` | Submit a form on a page. |
| `POST /realtime/:actorId/:envTitle/:page` | `realtimeChanges` | Push live page changes to receivers (needs session + `modify`). |

**Rendering a page (`getScriptPage`, `pages.ts:273-370`):**
1. Resolve the app (`getApp` by `accId` + `ref`) and the env (`getEnvByAppIdAndTitle`).
2. Load the env's project tree (`getAppEnv` → `getDBProjectStruct`).
3. Unless the page is `is_static` or `?__debug=true`, build the **context** from request
   headers (`parseContextHeader` → `{ accId, appId, actorId, rootActorId, country, city,
   language, browser, os, ip }`) and enforce **context restrictions**
   (`checkContextRestrictions`): if the context actor's `appSettings.expired` has passed →
   `SCRIPT_EXPIRED`; if `appSettings.users`/`groups` are set and the caller is in neither →
   `SCRIPT_CONTEXT_ACCESS`.
4. Call the env's **Corezoid process** via `corezoidSyncApi` with `path: '/get'`, passing
   `{ page, query, context }` and `sessionData`. Corezoid returns `{ code, data,
   viewModel, sessionData, language }`.
5. Branch on `code`:
   - `HTTP_OK` → merge the page's `config` + app/page `locale` + `viewModel` into a
     `PageData`, resolve any `$ref` `definitions` by xpath, `renderPage(config, data,
     getDefinition)`, and return `{ code, data: page, sessionData }`.
   - `HTTP_REDIRECT` → return the redirect target.
   - anything else → return a page carrying an error notification.

**Submitting a form (`sendScriptPage`, `pages.ts:375-492`):** same context checks, strips
HTML from submitted values (`TextUtils.stripObjectHtml`), calls Corezoid with `path:
'/send'`, then:
- `HTTP_OK` → `renderResponse` of the returned changes (optionally expanding
  `contentLoop` sections via `replaceContentLoopWithContent`).
- `HTTP_RESET_CONTENT` → re-render a (possibly different) page fresh.
- `HTTP_REDIRECT` → pass the redirect through.

**Session data (`getSessionData`, `pages.ts:52-105`):** for `sharedWith === 'anyone'`
without a session, the user is represented by a `control-events-fingerprint` header
(`{ id: fingerprint, unauthorized: true }`). For authenticated users, a signed JWT is
attached when the user is an *authorized actor* of the app, and sensitive token fields are
stripped before the data is handed to the page.

**Styles (`getScriptConfig` → `compileAppStyles`):** the `styles/` folder is compiled from
Less to CSS by the **virtual file manager** in `cssCompiler.ts`, which resolves
`@import "styles/…"`/`@import "pages/<page>/style"` against the in-DB project, rewrites
`attachments/…` URLs to download URLs, and wraps output in `.cdu-page` scope. Results are
cached in Redis (`appStyle_<envId>`) with a lock to avoid stampedes, invalidated on every
content change / deploy, and pre-warmed after deploys.

### 4.8 Archival & deletion

- **Releases** are never hard-deleted as manifests; superseded ones become `archived`, and
  their bulky **objects** are GC'd once they fall outside the retention window (5 per env).
- **Files/folders** are soft-deleted (`deleted_at`) and recoverable from **trash** (§4.4).
- The Smart Form **actor** itself follows normal actor deletion (via the graph/actors API);
  deleting it removes the application identity. The companion `graphs` actor and the
  `app_*` rows are keyed off `actor_id`.

---

## 5. Access & sharing model

### 5.0 Internal API vs Public API

Every Smart Form / application endpoint is registered **twice** in `pong-server`
(`apiRegister.js`), under two prefixes built as `/{type}/{apiVersion}/{url}`:

| Surface | Prefix | Auth mechanism | Who calls it |
|---|---|---|---|
| **Internal** | `/api/1.0/...` | Browser **session** (`checkAuth`) | The Simulator web UI / builder |
| **Public** | `/papi/1.0/...` | **OAuth2 bearer / API key** (`checkPublicApiAuth([scope])`) | External integrations, **the simulator MCP server** |

The internal handlers and the public handlers are the **same functions** — the public
modules (`publicApi/applications/*`) just re-export the internal request handlers behind
`checkPublicApiAuth`. Only `/papi/` paths appear in the generated public OpenAPI
(`swaggerRegister.js` filters everything else out in non-dev), which is what the MCP
server's drift gate (`testdata/papi-openapi.json`) validates against.

**Public scopes** reuse the platform-wide `actors.*` scopes (consistent with the rest of
the apps surface):
- **`actors.readonly`** — all read endpoints (get app/envs, list/get releases, diff,
  release struct, list/get file versions, list trash, env struct, folder content, catalog
  list, check install).
- **`actors.management`** — all write endpoints (create app, update env creds, deploy,
  create/update/delete content, rollback release, rollback file, restore trash, install
  smart form, prompt node).
- Page-serving endpoints (`pages`) use `checkAppAccess(true)` publicly, which requires
  `actors.readonly` and then applies the `sharedWith` rules.

> **As of this revision, the public API has full parity with the internal API** for
> `applications`, `app_content`, `releases`, and `file_history` — previously `releases` and
> `file_history` were internal-only, and `applications`/`app_content` exposed only a subset
> publicly. See the scope column in §7.

### 5.1 Sharing & per-context gating

Two further layers gate *who can use a Smart Form*, on top of the API auth above.

**Builder/management endpoints** (`applications`, `app_content`, `releases`,
`file_history`) use the standard actor access checks (the same on both surfaces):
- `checkAccess('view'|'modify'|'execute', 'actor')`, or
  `checkPermissions([SCRIPTS_MANAGEMENT, ACTORS_MANAGEMENT])` for creation/installation.
- `view` = read, `modify` = edit content/envs/restore, `execute` = deploy/rollback.

**Runtime page endpoints** (`pages`) use `checkAppAccess(isPublicApi)`
(`middleware/checkAppAccess.ts`), driven by the actor's `data.sharedWith`
(`AppSharedWith`, `types/applications-types.ts:136-141`):

| `sharedWith` | Who can open the app |
|---|---|
| `userList` | Only users with an explicit `view` access rule on the actor. |
| `allWorkspaceUsers` | Any member of the app's workspace (`checkWorkspaceUsers`). |
| `allRegisteredUsers` | Any authenticated platform user. |
| `anyone` | Public — no session required (identified by fingerprint). |

On top of *who can open the app*, **per-context gating** via the context actor's
`appSettings` restricts individual runtime contexts: `expired` (a deadline), and
`users`/`groups` allow-lists (`checkContextRestrictions`, `pages.ts:222-243`).

---

## 6. Release state machine

```
                    deploy / rollback creates a new release (status = active)
                                         │
        ┌────────────────────────────────▼────────────────────────────┐
        │                            active                            │
        │            (the live content of its target env)              │
        └───────┬─────────────────────────────────────┬───────────────┘
                │ a newer release is created           │ this release is
                │ (deploy or rollback) for the env     │ chosen as a rollback target
                ▼                                       ▼
            archived                               rolled_back
   (superseded; objects GC'd                (audit marker; a new forward
    once outside the 5-release               release carries its content,
    retention window)                        parent_release_id = this.id)

   failed:  transient — if T2 (materialise) fails, revertFailedRelease reconciles
            the manifest with the rolled-back env so the env's live release always
            matches its actual contents.
```

Invariants:
- At most one `active` release per env at any time (`archivePreviousActive` runs inside the
  serialising `FOR UPDATE` lock before a new active release is created).
- `release_number` is strictly increasing per env (unique `(env_id, release_number)`).
- History is **append-only / forward-only**: rollback never deletes or rewinds; it appends
  a new release.

---

## 7. API quick reference

Every row is available on **both** surfaces (see §5.0): internal `/api/1.0/<path>`
(session) and public `/papi/1.0/<path>` (OAuth2). The **Public scope** column is the
`checkPublicApiAuth` scope required on `/papi/1.0`; the **Actor access** column is the
`checkAccess` rule enforced on both surfaces. `operationId` is the name under which the
endpoint appears in the public OpenAPI.

| Method & path | Handler | Public scope (`/papi/1.0`) | Actor access | operationId | Notes |
|---|---|---|---|---|---|
| GET `/smart_forms/list/:accId` | `getSmartFormsListReq` | `actors.readonly` | acc access | `listSmartForms` | Catalog (cached 60 s); `?filter` = field selection |
| POST `/smart_forms/:accId` | `installSmartFormReq` | `actors.management` | — | `createSmartForm` | Async import task |
| GET `/smart_forms/check_install/:taskId` | `checkSmartFormInstallReq` | `actors.readonly` | owner only | — | Poll install; returns actor when done |
| POST `/smart_forms/prompt_node/:accId` | `promptNodeReq` | `actors.management` | acc access | — | Forward AI prompt to API gateway |
| GET `/applications/:actorId` | `geAppReq` | `actors.readonly` | `view` | `getApplication` | App/actor details |
| GET `/applications/envs/:actorId` | `getAppEnvsReq` | `actors.readonly` | `view` | `getApplicationEnvs` | Env list |
| POST `/applications/:accId` | `createAppReq` | `actors.management` | — | `createApplication` | Create app + 2 envs + tree |
| PUT `/applications/env/:actorId/:envId` | `updateEnvCredReq` | `actors.management` | `modify` | `updateApplicationEnv` | Update Corezoid creds |
| POST `/applications/deploy/:actorId` | `deployAppReq` | `actors.management` | `execute` | — | T1 snapshot + T2 materialise |
| POST `/app_content/:actorId` | `createObjsReq` | `actors.management` | `modify` | `createAppContent` | Create files/folders |
| PUT `/app_content/:actorId` | `updateObjsReq` | `actors.management` | `modify` | `manageAppContent` | Update (with history) |
| GET `/app_content/:actorId/:folderId` | `getFolderContentReq` | `actors.management` | `modify` | `getAppContentFolder` | List folder children |
| GET `/app_content/struct/:actorId/:envId` | `getEnvStructReq` | `actors.management` | `modify` | — | Whole env tree |
| DELETE `/app_content/:actorId` | `removeObjsReq` | `actors.management` | `modify` | `deleteAppContent` | Soft-delete |
| GET `/releases/:actorId` | `listReleasesReq` | `actors.readonly` | `view` | `listReleases` | List releases |
| GET `/releases/:actorId/:releaseId` | `getReleaseReq` | `actors.readonly` | `view` | `getRelease` | Manifest |
| GET `/releases/:actorId/:releaseId/struct` | `releaseStructReq` | `actors.readonly` | `view` | `getReleaseStruct` | Manifest + objects |
| GET `/releases/:actorId/:releaseId/diff?vs=` | `releaseDiffReq` | `actors.readonly` | `view` | `diffReleases` | added/removed/modified |
| POST `/releases/:actorId/:releaseId/rollback` | `rollbackReleaseReq` | `actors.management` | `execute` | `rollbackRelease` | New forward release |
| GET `/file_history/:actorId/:fileId` | `listFileVersionsReq` | `actors.readonly` | `view` | `listFileVersions` | Version list |
| GET `/file_history/:actorId/:fileId/:versionId` | `getFileVersionReq` | `actors.readonly` | `view` | `getFileVersion` | One version |
| POST `/file_history/:actorId/:fileId/rollback` | `rollbackFileReq` | `actors.management` | `modify` | `rollbackFile` | Restore file version |
| GET `/file_history/trash/:actorId/:envId` | `listTrashReq` | `actors.readonly` | `view` | `listAppTrash` | Soft-deleted objects |
| POST `/file_history/trash/:actorId/restore` | `restoreReq` | `actors.management` | `modify` | `restoreAppContent` | Restore from trash |
| GET `/pages/:accId/:ref/:envTitle` | `getScriptConfig` | `actors.readonly`† | `checkAppAccess` | — | CSS + widgets |
| GET `/pages/:accId/:ref/:envTitle/:page` | `getScriptPage` | `actors.readonly`† | `checkAppAccess` | — | Render page (calls Corezoid `/get`) |
| POST `/pages/:accId/:ref/:envTitle/:page` | `sendScriptPage` | `actors.readonly`† | `checkAppAccess` | — | Submit form (Corezoid `/send`) |
| POST `/pages/realtime/:actorId/:envTitle/:page` | `realtimeChanges` | `actors.readonly` | `modify` | — | Push live changes |

† `pages` GET/POST use `checkAppAccess(true)`, which requires `actors.readonly` **and** then
applies the app's `sharedWith` rule (so an `anyone` app can also be served without a token).

---

## 8. Constants & enums

- **System forms:** `SYSTEM_FORMS.SCRIPTS` (`'scripts'`) holds Smart Form actors;
  `SYSTEM_FORMS.GRAPHS` (`'graphs'`) holds the companion graph actor.
- **Permissions:** `SCRIPTS_MANAGEMENT`, `ACTORS_MANAGEMENT`, `SCRIPTS_READONLY`
  (`constants/sa.js`). API-key installs are granted all `__SCOPES__`.
- **Access verbs (per actor):** `view`, `modify`, `execute` (deploy/rollback), `remove`.
- **`AppSharedWith`:** `userList` | `allWorkspaceUsers` | `allRegisteredUsers` | `anyone`.
- **Release `status`:** `active` | `archived` | `rolled_back` (+ transient failed handling).
- **File-version `operation`:** `create` | `update` | `move` | `rename` | `delete`.
- **Retention:** 50 versions/file (`applications.history.perFileVersionLimit`); 5
  releases/env objects (`applications.history.releaseLimit`).
- **Env titles:** `develop` (writable), `production` (readonly).

---

## 9. Notes for AI agents

- A Smart Form is addressed at runtime by **`(accId, ref, envTitle)`** and at build time by
  its **`actorId`**. Resolve `ref → actorId` via `getApp`/`getActorByRef` in the `scripts`
  form.
- **Never write to `production` directly** — it is `readonly`. Edit `develop`, then
  `deploy` `develop → production`. Promotion is always via a release, never a raw copy.
- **Rollback is forward-only.** To "undo", roll back to the desired release id; this appends
  a new active release rather than mutating history. A release whose objects were pruned
  (outside the 5-release window) cannot be rolled back.
- **System objects are protected.** Don't attempt to rename/delete `is_system` files/folders
  (e.g. `styles/index`, `pages/index/config`); such writes are rejected.
- **Dynamic data comes from Corezoid**, not from the Simulator DB: page rendering calls the
  env's Corezoid process (`/get`, `/send`). The Smart Form project files define *layout,
  styles, i18n, view-model defaults*; Corezoid supplies *runtime data and control flow*.
- **`filter` on the catalog is field projection**, not row filtering — consistent with the
  rest of the platform's `filter` semantics.
- **Use the public API (`/papi/1.0`).** The simulator MCP server targets `/papi/1.0` with an
  OAuth2 bearer token. The whole Smart Form surface (`applications`, `app_content`,
  `releases`, `file_history`, `smart_forms`, `pages`) is available there with full
  parity — reads need the `actors.readonly` scope, writes need `actors.management` (§7).
- **Runtime page tools are wrapped as MCP tools.** The two page-serving operations are exposed
  as the Smart Form **runtime** tools in `internal/tools/smartforms.go` (`smartFormOps`): **`appGetPage`**
  (render a page) and **`appSendForm`** (submit a form) — the `get`/`send` protocol of
  [CDU Page Protocol](cdu-page-protocol.md). They are the universal primitives a runtime agent
  uses to drive *any* Smart Form (the `simulator-smart-forms-runtime` skill). The remaining
  build-time endpoints (`applications`, `app_content`, `releases`, `file_history`,
  `smart_forms`) are available on `/papi/1.0` but not yet wrapped as tools — they can be added
  without backend changes.
- **Editing pages?** The page JSON contract and the (lack of) server-side validation when
  saving page files are documented in **[CDU Page Protocol](cdu-page-protocol.md)** — validate
  page `config` against the protocol before saving, since the backend stores it opaque.

---

## Related documentation

- [CDU Page Protocol](cdu-page-protocol.md) — the runtime page JSON contract: `get`/`send`
  protocol, `Page`/`grid`/`forms`/`sections`/component model, the `changes` patch protocol,
  templating, the component catalogue, and server-side save validation.
- [Actors](../entities/actors.md) — a Smart Form is an actor in the `scripts` system form.
- [Forms](../entities/forms.md) / [System Forms](../entities/system-forms.md) — the
  `scripts` and `graphs` system forms that host Smart Forms.
- [Custom Car Form](custom-car-form.md) — contrast: a *data* form vs. a Smart Form *app*.
- [Simulator.Company API Documentation](https://doc.simulator.company) — full API reference.
