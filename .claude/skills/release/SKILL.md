---
name: release
description: >-
  Project release helper for simulator-ai-plugin. Prepares a new tagged release end-to-end. Use
  this skill whenever the user says "release", "релиз", "новый релиз", "сделай релиз", "выпусти
  версию", "bump version", "обновить версию", "tag a release", "/release", or anything that
  implies cutting a new version of this plugin. Walks the user through seven explicit phases: (1)
  compare `main` with the latest git tag and summarise what changed, (2) ask which new version to
  publish, (3) run the build / vet / discovery gates so the release can't ship broken Go code or
  stale `public/`, (4) draft a CHANGELOG.md entry in the existing Keep-a-Changelog format, (5)
  mint the version with `make release VERSION=x.y.z`, which promotes the CHANGELOG's `##
  [Unreleased]` section into a dated one and bumps all five manifests in lock-step, (6) show the
  user the full proposed change set and wait for explicit confirmation, (7) commit on the current
  branch, then merge into `main` and tag `main` with the matching `vX.Y.Z`. Always use this skill
  instead of running release steps manually — it delegates the bump to `scripts/release.sh` so all
  five manifests stay in lock-step, runs the build gates, formats the changelog consistently, and
  prevents partial releases.
---

# Release skill

This skill prepares a new tagged release of `simulator-ai-plugin`. It runs through seven phases in order. Do not skip phases, and do not collapse them — the user expects to see and approve each one before the next begins.

## Why a dedicated skill

A release of this plugin touches five separate manifest files plus the changelog, **and** the Go MCP server must still build cleanly with no `public/` drift. Forgetting any one of them ships a broken or inconsistent release: marketplace listings disagree about the version, Codex installs the wrong build, the `release.yml` workflow can't extract release notes from `CHANGELOG.md`, or the published binary doesn't match the manifest. The bump itself is already scripted — `make release VERSION=x.y.z` runs `scripts/release.sh`, which promotes the changelog and rewrites all five version fields — so this skill never edits a manifest by hand; it decides *what* to release, runs the gates, and drives the script. The single source of release truth for this repo is [`RELEASE_CHECKLIST.md`](../../../RELEASE_CHECKLIST.md).

## The seven phases

### Phase 1 — Inspect what changed since the last tag

Goal: understand and summarise everything that has landed since the previous release.

Run, in parallel:

```bash
git fetch --tags --quiet
git describe --tags --abbrev=0                         # the last released tag, e.g. v2.0.0
git log <last-tag>..HEAD --pretty=format:'%h %s'       # commit subjects since that tag
git diff --stat <last-tag>..HEAD                       # files touched, magnitude of change
git status --short                                     # uncommitted work that may need to ship in this release
```

If `git describe` fails (no tags reachable yet), fall back to `git log --pretty=format:'%h %s' -50` and treat this as the very first tagged release.

Then read the actual diff for any commit subjects that are vague or for any non-trivial file changes — commit subjects alone often understate what shipped. Pay particular attention to:

- New skills under `plugins/simulator/skills/` (added directories with a new `SKILL.md`).
- MCP server changes in `plugins/simulator/mcp-server/` (new tools, schema changes, breaking behaviour, drift-spec updates).
- Curated tool changes in `plugins/simulator/mcp-server/internal/tools/<domain>.go` and the drift gate at `internal/tools/testdata/papi-openapi.json`.
- Docs changes in `plugins/simulator/docs/` (shipped with the plugin), the repo-root `docs/`, and top-level docs (`README.md`, `AGENTS.md`, `CLAUDE.md`, `SECURITY.md`, `PUBLISHING.md`, `RELEASE_CHECKLIST.md`).
- CI / workflow changes in `.github/workflows/` — especially `release.yml`, since changes there alter how this very release will be published.
- Generated artifacts under `public/` — these must come from `make discovery`, not hand-edits.

If `git status` shows uncommitted changes, ask the user whether those should be part of this release or stay out of it. Don't assume — pending edits are sometimes the whole point of the release and sometimes unrelated work.

Output to the user: a short summary grouped by Keep-a-Changelog category (`Added`, `Changed`, `Fixed`, `Removed`, `Security`, `Docs`), the kind of bullets that would end up in the changelog. Keep each bullet to one line.

### Phase 2 — Ask which new version to publish

Goal: get the new semantic version from the user.

Use `AskUserQuestion` with the **current** version pulled from `plugins/simulator/.claude-plugin/plugin.json` and three suggested bumps:

- Patch (`X.Y.Z+1`) — bug fixes, docs, internal cleanup only.
- Minor (`X.Y+1.0`) — new skills, new MCP tools, additive features.
- Major (`X+1.0.0`) — breaking changes to manifests, MCP tool schemas, or skill contracts.

Recommend a level based on Phase 1 (e.g. if a new skill directory appeared or a new MCP tool landed, suggest a minor bump). Always allow the user to override with a custom version via the "Other" answer.

The version the user picks is the only version that should appear anywhere downstream in this skill run. Store it once and reuse it — do not re-derive it.

### Phase 3 — Run the build gates

Goal: refuse to release a build that doesn't compile or has stale generated artifacts.

Run from the repo root:

```bash
make build       # MCP server compiles
make vet         # go vet clean
make discovery   # regenerates public/* from skill frontmatter
git status --short public/  # must be empty after `make discovery`
```

`make test` is also worth running if there's any meaningful change under `plugins/simulator/mcp-server/`. The graph-sync code (`internal/engines/sync_graph.go`, `push_graph.go`) has partial test coverage — if Phase 1 shows changes in those files, re-read [`CLAUDE.md`](../../../CLAUDE.md) §"House rules" before continuing and warn the user that the orchestration / edge-placement branches are only partly covered.

If any of these fail, stop and surface the failure to the user. **Do not** try to "fix it up" silently as part of the release — the release commit should be a clean version bump, not a bugfix commit in disguise. The user decides whether to abort the release or pause it while the underlying issue is fixed in a separate commit.

If `make discovery` produces a diff under `public/`, that diff belongs in this release commit — stage it alongside the manifest bumps in Phase 7.

Also validate that all manifests still parse cleanly (this is cheap and protects against earlier in-flight edits):

```bash
python3 -m json.tool .claude-plugin/marketplace.json >/dev/null
python3 -m json.tool .agents/plugins/marketplace.json >/dev/null
python3 -m json.tool plugins/simulator/.claude-plugin/plugin.json >/dev/null
python3 -m json.tool plugins/simulator/.codex-plugin/plugin.json >/dev/null
python3 -m json.tool plugins/simulator/.kiro-plugin/plugin.json >/dev/null
python3 -m json.tool plugins/simulator/.mcp.json >/dev/null
```

### Phase 4 — Curate the `## [Unreleased]` section

Goal: `## [Unreleased]` reads as this release's notes, in the existing Keep-a-Changelog format.

PRs append their entries under `## [Unreleased]` as they merge, so the material is usually already there — the job is to read it as a whole, merge duplicates, and fix anything written for a reviewer rather than for a user. **Do not write a dated `## [x.y.z]` header yourself**: `scripts/release.sh` inserts it in Phase 5 and leaves `## [Unreleased]` in place, empty, for the next cycle.

Read `CHANGELOG.md`. The current format is:

```
## [X.Y.Z]

Optional one-line summary of the release.

### Added
- …

### Changed
- …

### Fixed
- …
```

Rules:

- Use Keep-a-Changelog subsection headings (`### Added`, `### Changed`, `### Fixed`, `### Removed`, `### Security`, `### Docs`). Only include subsections that actually have bullets — don't emit empty headings.
- One bullet per logical change, not one bullet per commit. Squash related commits.
- Write in the imperative-ish style matching prior entries ("add MCP tool X", "fix Y", "remove Z" — not "added", "fixes"). The existing `[2.0.0]` entry uses noun phrases for the first-release summary; for normal releases prefer verbs.
- Drop trivia: bumps of internal version numbers, merge commits, formatting-only changes. The changelog is for users of the plugin, not for git archeologists.
- `release.yml` extracts the section between `## [X.Y.Z]` and the next `## ` heading into the GitHub Release body — so anything you put here will be visible on the public release page. Write accordingly.

Show the drafted entry to the user before writing it to disk. They will often want to reword a bullet or merge two of them — that's expected.

### Phase 5 — Mint the version with `make release`

Goal: the new version appears in all five manifests and in a dated CHANGELOG section, all in agreement.

```bash
make release VERSION=X.Y.Z      # wraps scripts/release.sh
```

That script is the mechanism the repo documents (`AGENTS.md` → Versioning & releases). It refuses a
non-semver version and a version equal to the current one, promotes `## [Unreleased]` into
`## [X.Y.Z] - <today>`, warns if that section came out empty, and rewrites the `"version"` field in:

| File | Field |
| --- | --- |
| `plugins/simulator/.claude-plugin/plugin.json` | top-level `"version"` |
| `plugins/simulator/.codex-plugin/plugin.json` | top-level `"version"` |
| `plugins/simulator/.kiro-plugin/plugin.json` | top-level `"version"` |
| `.claude-plugin/marketplace.json` | `plugins[0].version` |
| `.agents/plugins/marketplace.json` | `plugins[0].version` |

**Never bump these by hand.** The Kiro manifest is the one people forget, and a Kiro install then
advertises the previous version — `install-kiro.sh` and the release zip read it. The script does an
exact old→new string replacement, so unrelated version fields (swagger specs, `SKILL.md`) are
untouched. It does **not** commit or tag; that is Phase 7.

Verify all five agree before moving on:

```bash
grep -n '"version"' \
  plugins/simulator/.claude-plugin/plugin.json \
  plugins/simulator/.codex-plugin/plugin.json \
  plugins/simulator/.kiro-plugin/plugin.json \
  .claude-plugin/marketplace.json \
  .agents/plugins/marketplace.json
```

If any one disagrees, stop — never proceed to commit with mismatched manifests. Then re-run the JSON
parse check from Phase 3 to confirm no manifest got corrupted.

### Phase 6 — Confirm with the user

Goal: nothing is committed without explicit go-ahead.

Show the user:

1. The new version number.
2. The CHANGELOG.md entry as it will be written.
3. A `git diff --stat` of all currently staged/unstaged changes (including any pre-existing edits from Phase 1 that they confirmed should ship, plus any `public/` diff from Phase 3).
4. The exact commit message and tag name you intend to create.

Then use `AskUserQuestion` with options like "Proceed with commit and tag" / "Let me edit something first". Do not proceed without an affirmative answer. If the user wants to tweak the changelog or change the version, loop back to the relevant phase rather than improvising in place.

### Phase 7 — Commit and tag

Goal: a single release commit plus a matching annotated git tag on the current branch.

Stage only the files this skill touched plus any files the user confirmed should ship:

```bash
git add CHANGELOG.md \
        plugins/simulator/.claude-plugin/plugin.json \
        plugins/simulator/.codex-plugin/plugin.json \
        plugins/simulator/.kiro-plugin/plugin.json \
        .claude-plugin/marketplace.json \
        .agents/plugins/marketplace.json
# Plus any public/* regenerated by `make discovery` and any other files the user
# explicitly approved in Phase 1.
```

Avoid `git add -A` / `git add .` — there may be unrelated untracked files (samples, local docs, IDE state, `.DS_Store`) that must not enter the release commit. The repo `.gitignore` covers most of these but the safest default is still to stage explicitly.

Commit message format — match the release commits already in the history (`chore(release): 2.7.0`,
`chore(release): 2.6.0`), which is also what `scripts/release.sh` prints as its next step:

```
chore(release): X.Y.Z
```

Use a HEREDOC for the commit so multiline formatting is preserved. Do not add Claude co-author trailers to release commits — these are public and authored by the maintainer.

**The tag goes on `main`, not on the branch you just committed on.** Releases are cut on
`develop`, merged into `main`, and tagged there — that is what the history shows (`Merge develop
into main` immediately followed by `chore(release): x.y.z`) and what `scripts/release.sh` prints as
its next step. So: commit on the current branch, merge into `main`, then tag `main`:

```bash
git tag -a vX.Y.Z -m "Release vX.Y.Z"
```

Use the **annotated** form (`-a`) so `git describe` and GitHub Releases pick it up correctly. The tag **must** start with `v` — `.github/workflows/release.yml` only triggers on `v*` tags and strips the leading `v` to derive the changelog section, so a bare `X.Y.Z` tag will silently fail to publish.

Do **not** push automatically. After the commit and tag are created, tell the user exactly what to run to push:

```bash
git push origin <current-branch>
git push origin main
git push origin vX.Y.Z
```

This deliberate pause is a safety net — pushing the tag triggers `release.yml` and a public GitHub Release with attached `simulator-mcp-*` binaries, which is hard to undo. Let the user do that last step themselves.

## Things to watch for

- **Wrong branch.** The release commit lands on `develop` (that is where PRs merge) and the tag lands on `main` after `develop` is merged into it. If the current branch is neither, surface this in Phase 6 so the user can decide before anything is committed.
- **Tag already exists.** Before Phase 7, run `git tag -l vX.Y.Z`. If it returns the tag, stop — the version is already taken. Ask the user to pick a different one and loop back to Phase 2.
- **`public/` drift.** If `make discovery` in Phase 3 changes files under `public/`, those changes must be part of the release commit. Don't ship a release where manifests advertise a new version but `public/` still describes the old skill frontmatter. Never hand-edit `public/*`.
- **Unrelated untracked files.** The repo regularly has WIP files (`comparison.html`, `plan.html`, sample `.conv.json` files, IDE config, `.DS_Store`). These must not enter the release commit unless the user explicitly says they should.
- **Manifest drift in the diff.** If Phase 1 shows a manifest file already changed by a previous (incomplete) attempt, treat that as suspect — re-read all five manifests before Phase 5 and reconcile from a known state, don't blindly bump on top of a half-finished bump.
- **CHANGELOG.md already has an entry for the chosen version.** That means a previous release attempt was partially completed. Show the existing entry to the user and ask whether to extend it, replace it, or pick a different version.
- **Secrets in `.mcp.json`.** [`RELEASE_CHECKLIST.md`](../../../RELEASE_CHECKLIST.md) explicitly requires this — eyeball `plugins/simulator/.mcp.json` in Phase 3 to confirm it contains no credentials or private URLs before shipping.
- **`License` field.** All five manifests must say `"license": "MIT"` (not `ISC`, not absent). Spot-check this in Phase 5 when you grep for `"version"` — `grep -n '"license"' ...` is a one-liner.

## When the user wants a one-shot, no-questions release

If the user explicitly says something like "just release a patch, no questions" or "автоматический релиз патча", you can fold Phase 2 (pick patch bump) and Phase 6 (confirmation) into a single approval at the end, but never skip:

- The build gates in Phase 3 — a broken Go build or stale `public/` is not negotiable for speed.
- Showing the proposed changelog entry and the proposed version.
- The `make release VERSION=x.y.z` bump in Phase 5 — never a hand-edited manifest.
