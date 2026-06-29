# Publishing

This document describes how to publish a new version of the Simulator.Company AI plugin.

## 1. Validate Manifests Locally

Check that all JSON manifests are well-formed:

```bash
python3 -m json.tool .claude-plugin/marketplace.json >/dev/null
python3 -m json.tool .agents/plugins/marketplace.json >/dev/null
python3 -m json.tool plugins/simulator/.claude-plugin/plugin.json >/dev/null
python3 -m json.tool plugins/simulator/.codex-plugin/plugin.json >/dev/null
python3 -m json.tool plugins/simulator/.mcp.json >/dev/null
```

Verify version sync between manifests:

```bash
grep -E '"version"|^version:' \
  plugins/simulator/.claude-plugin/plugin.json \
  plugins/simulator/.codex-plugin/plugin.json \
  plugins/simulator/.kiro-plugin/plugin.json \
  .claude-plugin/marketplace.json \
  .agents/plugins/marketplace.json \
  POWER.md
```

All six should show the same version number.

## 2. Verify Build & Tests

```bash
make build
make vet
make test
make discovery   # regenerate public/ — commit any diff
```

## 3. Test in Claude Code

Install the plugin from the local clone:

```bash
claude plugin marketplace add ./
claude plugin install simulator@simulator
```

Verify that the Simulator skills load and the MCP server starts. Run a quick smoke test:

```
log in to Simulator
```

## 4. Test in Codex

```bash
codex plugin marketplace add ./
codex plugin install simulator@simulator
```

Restart Codex, open Plugin Directory, select **Simulator.Company**, and confirm the plugin installs and the skills are available.

## 4b. Test in AWS Kiro

```bash
plugins/simulator/scripts/install-kiro.sh "$YOUR_KIRO_WORKSPACE"
```

Open the workspace in Kiro. Confirm:

- `.kiro/settings/mcp.json` is loaded and the `simulator` MCP server appears in Kiro's tool panel.
- `.kiro/steering/simulator.md` shows in the steering UI.
- A prompt like "use simulator" routes through `.kiro/skills/simulator/SKILL.md`.
- A live tool call (e.g. `login`) succeeds end-to-end.

The script is idempotent and supports re-runs to refresh the workspace overlay.

## 5. Update Files

1. Bump the version in **six** files (every host manifest plus the Power manifest):
   - `plugins/simulator/.claude-plugin/plugin.json`
   - `plugins/simulator/.codex-plugin/plugin.json`
   - `plugins/simulator/.kiro-plugin/plugin.json`
   - `.claude-plugin/marketplace.json` (the `plugins[0].version` field)
   - `.agents/plugins/marketplace.json` (the `plugins[0].version` field)
   - `POWER.md` (frontmatter `version:` field)
2. Add a section to `CHANGELOG.md` for the new version.
3. Commit the changes.

## 6. Push to GitHub and Tag

```bash
git push origin main
git tag vX.Y.Z
git push origin vX.Y.Z
```

The `release.yml` workflow fires automatically on any `v*` tag. It cross-compiles the MCP server for `darwin/linux × amd64/arm64`, generates SHA-256 checksums, attests build provenance, regenerates `public/` via `make discovery`, builds the Kiro overlay via `make discovery-kiro`, zips it as `simulator-kiro-vX.Y.Z.zip`, and creates a GitHub Release whose body is the matching `CHANGELOG.md` section. Both the Kiro zip and `POWER.md` are attached to every Release so kiro.dev/powers can resolve the Power manifest from the tag.

## 7. Install from GitHub

**Claude Code:**

```bash
claude plugin marketplace add corezoid/simulator-ai-plugin
claude plugin install simulator@simulator
```

**Codex (stable):**

```bash
codex plugin marketplace add corezoid/simulator-ai-plugin --ref vX.Y.Z
codex plugin install simulator@simulator
```

**Codex (development tracking):**

```bash
codex plugin marketplace add corezoid/simulator-ai-plugin --ref main
codex plugin install simulator@simulator
```

**AWS Kiro:**

```bash
# Option A — install from a cloned repo (developer mode):
git clone https://github.com/corezoid/simulator-ai-plugin
plugins/simulator/scripts/install-kiro.sh "$YOUR_KIRO_WORKSPACE"

# Option B — extract the pre-built overlay from the GitHub Release:
curl -L -o simulator-kiro.zip \
  https://github.com/corezoid/simulator-ai-plugin/releases/download/vX.Y.Z/simulator-kiro-vX.Y.Z.zip
unzip -d "$YOUR_KIRO_WORKSPACE" simulator-kiro.zip
```

To list this Power on **kiro.dev/powers**, submit the release tag URL to the
Kiro Power registry. `POWER.md` is attached to every Release so the registry
can resolve metadata from the tag.

## 8. Notify Users

After tagging, ask users to upgrade their local marketplace and plugin:

- **Claude Code:** `claude plugin marketplace update && claude plugin update simulator@simulator`
- **Codex:** `codex plugin update simulator@simulator`
