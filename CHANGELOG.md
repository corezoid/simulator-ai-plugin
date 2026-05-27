# Changelog

## [1.3.5]

- Add `pruneLongEdges(layerId, maxDistancePx?, bucketThreshold?, preserveParentEdges?, dryRun?)` MCP tool. Walks every edge on a layer, deletes those whose Manhattan distance between endpoints exceeds `maxDistancePx` (default 600 px). By default keeps edges where either endpoint is a hierarchy bucket (≥ `bucketThreshold` incoming edges, default 3). `dryRun:true` previews without deleting. Returns scanned/deleted/kept_short/kept_parent counts plus up to 10 example deletions.

## [1.3.1]

- Rename edge title fields to camelCase.

## [1.3.0]

- Rebrand to simulator-ai-plugin and ship MCP server.
