# Changelog

## [1.4.0]

- Add `uploadActorPictureBulk` MCP tool: set pictures on up to 500 actors per call, dedupes identical source images by SHA-256 so the same icon is uploaded once and reused, supports `picture` shortcut to bind an already-uploaded storage path without re-uploading bytes.
- Auto-rasterise SVG sources to PNG inside `uploadActorPicture` (and bulk variant) via pure-Go `oksvg`+`rasterx`: defaults to 256×256, optional `pngWidth`/`pngHeight` overrides, and `svgFillColor` injects a brand colour on the `<svg>` root for monochrome simpleicons. The graph UI doesn't render SVG storage paths, so callers no longer need a local `rsvg-convert` install.

## [1.3.1]

- Rename edge title fields to camelCase.

## [1.3.0]

- Rebrand to simulator-ai-plugin and ship MCP server.
