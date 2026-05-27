# Changelog

## [1.3.4]

- Add `compactGraphLayout(layerId, strategy)` MCP tool. Implements the `domain-clusters` strategy: actors with `>= bucketThreshold` incoming edges become cluster headers, their children are arranged in a grid under them, and the clusters themselves are laid out in a super-grid (default 4 columns). Orphans stack in a Misc zone. Tunable via `clustersPerRow` / `nodesPerRow` / `nodeDX` / `nodeDY`. One MCP call replaces the full pull → YAML → reposition → push loop. Strategy arg is reserved for future `hierarchical` / `force-directed` layouts.

## [1.3.1]

- Rename edge title fields to camelCase.

## [1.3.0]

- Rebrand to simulator-ai-plugin and ship MCP server.
