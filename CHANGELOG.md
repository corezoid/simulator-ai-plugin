# Changelog

## [1.3.3]

- Add `getAllLayerPlacements(layerId)` MCP tool: returns every `(actorId, laId, formId, title, position)` row on a layer in one call. The existing `getLayerActorsByFormId` requires the caller to enumerate every formId in use on the layer (often 15+); this tool walks `/graph_layers/paginated/{layerId}?type=nodes` internally instead, paginating to completion.

## [1.3.1]

- Rename edge title fields to camelCase.

## [1.3.0]

- Rebrand to simulator-ai-plugin and ship MCP server.
