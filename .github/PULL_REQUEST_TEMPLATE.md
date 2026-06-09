<!-- See CONTRIBUTING.md before opening a PR. -->

## What & why

<!-- What does this change do, and what problem does it solve? -->

## Type of change

- [ ] Bug fix
- [ ] New MCP tool / capability
- [ ] Skill behaviour
- [ ] Docs
- [ ] Chore / refactor

## Checklist

- [ ] `make build && make vet && make test` pass locally
- [ ] If I added/renamed a tool: updated the MCP-tools table in `README.md` and §4 of `docs/ARCHITECTURE.md`, and it validates against the drift gate
- [ ] If I changed skill frontmatter: regenerated `public/` with `make discovery` (no hand-edits)
- [ ] Reference docs that skills load at runtime stay under `plugins/simulator/docs/`
- [ ] No tokens or `.env` committed; TLS verification left on by default
- [ ] Version bumped in all manifests + `CHANGELOG.md` entry added (if this is a release-worthy change)

## Notes for reviewers

<!-- Anything that needs extra attention — e.g. graph-sync logic, which has no unit tests yet. -->
