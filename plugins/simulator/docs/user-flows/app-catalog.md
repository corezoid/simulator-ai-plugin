# App discovery — choosing which Smart Form to run

This document defines how the Smart Form runtime answers one question: *given a user intent
(e.g. "create a smart contract"), which Smart Form do I run, and in which environment?*

Companion docs:
- [`smart-forms.md`](smart-forms.md) — what a Smart Form is (the `scripts` actor + envs +
  Corezoid binding) and its lifecycle.
- [`cdu-page-protocol.md`](cdu-page-protocol.md) — the `get`/`send` page protocol the runtime
  *drives* once an app is chosen (tools `appGetPage` / `appSendForm`).

The runtime skill that uses all three is **`simulator-smart-forms-runtime`**.

> **Design principle.** A mini-app is *data*, not code — and so is everything needed to find
> it. There is **no custom catalog format**: discovery rides entirely on metadata every Smart
> Form already has (title, description, tags, links, environments). Adding an app = adding a
> Smart Form; it becomes discoverable with no extra convention to maintain.

---

## 1. Discovery rides on native metadata

A runnable Smart Form is an actor in the **`scripts`** system form. Everything discovery needs
is already on it:

| Native signal | Where | Used for |
|---|---|---|
| **`title`** | the actor | the app's name — primary intent signal |
| **`description`** | the actor | the author's prose describing what the app does / when to use it — the richest intent signal; match the user's request against it semantically |
| **`tags`** | form/actor metadata (`tags` array, "categorization") | filter/scope candidates by domain (e.g. `finance`, `contracts`) |
| **linked actors** | graph edges → `getRelatedActors(type="linked", actorId)` | related context — e.g. the account templates / forms / process an app works with |
| **environments** | `getApplicationEnvs` | which env to drive (`production` vs `develop`); versions/releases are already tracked per env |
| **`data.sharedWith` / `appSettings`** | the actor | access — see §3 |

There is intentionally **no `intents[]` list and no AppSpec blob**: the description IS the
intent description. If an author wants an app to be easier to route to, they write a clearer
`title`/`description` (and add `tags`) — the same metadata a human reads in the UI.

---

## 2. Discovery — how the runtime finds an app

```
1. List candidate Smart Forms: filterActors over the `scripts` system form
   (optionally narrow by tags, or by an "App Catalog" layer if the workspace curates one).
2. Match the user's intent SEMANTICALLY against each candidate's title + description.
   Use tags to disambiguate domain; use getRelatedActors for related context when useful.
3. Choose the best match → its ref (the actor's own ref) + the target env (default
   `production`) → hand to appGetPage(accId, ref, env, "index").
```

- **Named app** — if the user names the app, resolve it directly: `getActorByRef` in the
  `scripts` form (or `getApplication`), skip matching.
- **Disambiguation** — if several apps match comparably, ask the user to pick by `title`
  rather than guessing.
- **Optional App Catalog layer** — a workspace may place Smart Form actors on a dedicated graph
  layer (a curated, browsable view). Discovery can scope to that layer instead of the whole
  `scripts` form. This is a nice-to-have, **not required**.

---

## 3. Access — who can run an app

Access is **already governed by the platform**; the runtime adds nothing. A Smart Form's
runtime reachability is gated exactly as the human UI (see [`smart-forms.md` §5](smart-forms.md)):

- **`data.sharedWith`** on the `scripts` actor — `userList` / `allWorkspaceUsers` /
  `allRegisteredUsers` / `anyone`.
- **`appSettings.{users,groups,expired}`** — per-context allow-lists and a deadline,
  enforced by `checkContextRestrictions`.

So **"let all users create X"** = add them to the Smart Form's `appSettings.groups` (a single
**capability group**). The same grant covers both clicking through the UI and driving the app
via the runtime — both go through the same public `pages` routes under the user's identity.
Granting access to a *created* record (e.g. a new contract) is the responsibility of the app's
own flow (its Corezoid process / a `saveAccessRules` step), not of discovery.

---

## 4. Resolution summary

```
user intent
  → list scripts-form actors  (optionally narrow by tags / App Catalog layer)
  → match intent against title + description  (disambiguate by title if >1)
  → (accId, ref = actor.ref, env = production unless told otherwise, page = "index")
  → appGetPage(...)  → then the get/send drive loop  (see cdu-page-protocol.md / the runtime skill)
```

The runtime is identity-agnostic: in the reaction→AI-agent context it carries the reaction
author's identity, so access (`sharedWith` / `appSettings.groups`) self-scopes with no extra
wiring.
