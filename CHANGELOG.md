# Changelog


## [1.4.0]

- Add `uploadActorPictureBulk` MCP tool: set pictures on up to 500 actors per call, dedupes identical source images by SHA-256 so the same icon is uploaded once and reused, supports `picture` shortcut to bind an already-uploaded storage path without re-uploading bytes.
- Auto-rasterise SVG sources to PNG inside `uploadActorPicture` (and bulk variant) via pure-Go `oksvg`+`rasterx`: defaults to 256×256, optional `pngWidth`/`pngHeight` overrides, and `svgFillColor` injects a brand colour on the `<svg>` root for monochrome simpleicons. The graph UI doesn't render SVG storage paths, so callers no longer need a local `rsvg-convert` install.

## [1.3.5]

- Add `pruneLongEdges(layerId, maxDistancePx?, bucketThreshold?, preserveParentEdges?, dryRun?)` MCP tool. Walks every edge on a layer, deletes those whose Manhattan distance between endpoints exceeds `maxDistancePx` (default 600 px). By default keeps edges where either endpoint is a hierarchy bucket (≥ `bucketThreshold` incoming edges, default 3). `dryRun:true` previews without deleting. Returns scanned/deleted/kept_short/kept_parent counts plus up to 10 example deletions.

## [1.3.3]

- Add `getAllLayerPlacements(layerId)` MCP tool: returns every `(actorId, laId, formId, title, position)` row on a layer in one call. The existing `getLayerActorsByFormId` requires the caller to enumerate every formId in use on the layer (often 15+); this tool walks `/graph_layers/paginated/{layerId}?type=nodes` internally instead, paginating to completion.

## [1.3.2]

- Fix `pushGraphFile` not propagating actor positions to the canvas. The internal `updatePositions` helper was sending a bare JSON array to `PUT /graph_layers/actors/{layerId}` and passing `laId` as an integer; the endpoint expects `{"items": [...]}` with `id` as a string and silently no-ops otherwise. Positions in YAML now reach the server on every push.


## [1.3.1]

- Rename edge title fields to camelCase.

## [1.3.0]

- Rebrand to simulator-ai-plugin and ship MCP server.
