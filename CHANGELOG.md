# Changelog

## [1.5.0]

- Add `moveActorToForm` MCP tool: recreate an actor under a different formId while preserving title, picture, color, description, data, ref, all incoming/outgoing links, and every layer placement (laId, position). The Simulator Public API doesn't expose a "change actor type" mutation; this tool orchestrates `getActor` → `createActor` in the target form → `deleteLink`/`createLink` replay for every edge → `manageLayer` delete-then-create for every placement → optional `deleteActor` on the original. Returns the old/new actor IDs and per-step counters; partial failures don't abort the run.

## [1.4.0]

- Add `uploadActorPictureBulk` MCP tool: set pictures on up to 500 actors per call, dedupes identical source images by SHA-256 so the same icon is uploaded once and reused, supports `picture` shortcut to bind an already-uploaded storage path without re-uploading bytes.
- Auto-rasterise SVG sources to PNG inside `uploadActorPicture` (and bulk variant) via pure-Go `oksvg`+`rasterx`: defaults to 256×256, optional `pngWidth`/`pngHeight` overrides, and `svgFillColor` injects a brand colour on the `<svg>` root for monochrome simpleicons. The graph UI doesn't render SVG storage paths, so callers no longer need a local `rsvg-convert` install.

## [1.3.1]

- Rename edge title fields to camelCase.

## [1.3.0]

- Rebrand to simulator-ai-plugin and ship MCP server.
