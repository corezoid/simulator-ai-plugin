# Release Checklist

Use this before tagging a public release.

## Manifests

- [ ] `plugins/simulator/.claude-plugin/plugin.json` version is updated.
- [ ] `plugins/simulator/.codex-plugin/plugin.json` version matches Claude manifest.
- [ ] `.claude-plugin/marketplace.json` `plugins[0].version` matches both manifests.
- [ ] `.agents/plugins/marketplace.json` `plugins[0].version` matches all manifests.
- [ ] `.agents/plugins/marketplace.json` `plugins[0].license` is `"MIT"`.
- [ ] No TODO or placeholder values remain in any manifest.
- [ ] Manifest asset and skill paths resolve under `plugins/simulator/`.
- [ ] All four manifests have `"license": "MIT"` (not ISC).
- [ ] All plugin `source` paths listed in marketplace manifests exist on disk.

## MCP Server

- [ ] `plugins/simulator/.mcp.json` contains no credentials or private URLs.
- [ ] Go source in `plugins/simulator/mcp-server/` compiles without errors (`make build`).
- [ ] `make vet` and `make test` pass.
- [ ] `make discovery` produces no `public/` diff (or the diff is committed).

## Content

- [ ] `CHANGELOG.md` has an entry for the new version.
- [ ] `README.md` install commands reference `corezoid/simulator-ai-plugin`.
- [ ] MCP-tools table in `README.md` and §4 of `docs/ARCHITECTURE.md` are up to date for any added/renamed tools.
- [ ] No `.env` files or credentials are tracked in git.

## JSON Validation

All manifests parse cleanly:

```bash
python3 -m json.tool .claude-plugin/marketplace.json >/dev/null
python3 -m json.tool .agents/plugins/marketplace.json >/dev/null
python3 -m json.tool plugins/simulator/.claude-plugin/plugin.json >/dev/null
python3 -m json.tool plugins/simulator/.codex-plugin/plugin.json >/dev/null
python3 -m json.tool plugins/simulator/.mcp.json >/dev/null
```

## Testing

- [ ] Claude Code can install the plugin from the local clone.
- [ ] Codex can install the plugin from the local clone.
- [ ] MCP server starts and the `login` flow responds.

## Git

- [ ] All changes are committed on `main` (or merged from a feature branch).
- [ ] Release tag matches the manifest version, e.g. `vX.Y.Z`.
- [ ] Tag is pushed to `origin`.
