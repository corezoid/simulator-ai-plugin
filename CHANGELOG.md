# Changelog

## [1.3.2]

- Fix `pushGraphFile` not propagating actor positions to the canvas. The internal `updatePositions` helper was sending a bare JSON array to `PUT /graph_layers/actors/{layerId}` and passing `laId` as an integer; the endpoint expects `{"items": [...]}` with `id` as a string and silently no-ops otherwise. Positions in YAML now reach the server on every push.

## [1.3.1]

- Rename edge title fields to camelCase.

## [1.3.0]

- Rebrand to simulator-ai-plugin and ship MCP server.
