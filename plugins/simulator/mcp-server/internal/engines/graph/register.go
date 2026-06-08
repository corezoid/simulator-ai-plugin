package graph

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Register adds all graph engine tools to the MCP server.
func Register(s *server.MCPServer) {
	s.AddTool(
		mcp.NewTool("pullGraphFile",
			mcp.WithDescription("Fetch all actors and edges from a layer and write them to <layerId>.yaml in the current working directory."),
			mcp.WithString("layerId", mcp.Description("Layer actor UUID to pull."), mcp.Required()),
		),
		handlePullGraphFile,
	)

	s.AddTool(
		mcp.NewTool("pushGraphFile",
			mcp.WithDescription("Read <layerId>.yaml from the current working directory and sync it with the server layer: creates missing actors/edges, updates changed ones, removes extras. Updates the file in place with server-assigned UUIDs."),
			mcp.WithString("layerId", mcp.Description("Layer actor UUID — file <layerId>.yaml must exist in the current working directory."), mcp.Required()),
		),
		handlePushGraphFile,
	)

	s.AddTool(
		mcp.NewTool("getAllLayerPlacements",
			mcp.WithDescription("Return every placement (actorId, laId, formId, title, position) on a layer in one call. Walks the paginated /graph_layers/paginated/{layerId}?type=nodes endpoint internally, so the caller does not need to enumerate formIds."),
			mcp.WithString("layerId", mcp.Description("UUID of the layer to enumerate."), mcp.Required()),
		),
		handleGetAllLayerPlacements,
	)

	s.AddTool(
		mcp.NewTool("compactGraphLayout",
			mcp.WithDescription("Auto-layout: repositions every placement on a layer into a tight domain-clustered grid. Buckets (actors with more incoming edges than bucketThreshold, default 3) become cluster headers; their children are arranged in a grid below; orphan actors stack into a Misc zone. Tunable via clustersPerRow / nodesPerRow / nodeDX / nodeDY."),
			mcp.WithString("layerId", mcp.Description("UUID of the layer to lay out."), mcp.Required()),
			mcp.WithString("strategy", mcp.Description("Layout strategy. Today only `domain-clusters` is implemented.")),
			mcp.WithNumber("bucketThreshold", mcp.Description("Minimum incoming-edge count for an actor to be treated as a bucket. Default 3.")),
			mcp.WithNumber("clustersPerRow", mcp.Description("Number of clusters per super-grid row. Default 4.")),
			mcp.WithNumber("nodesPerRow", mcp.Description("Number of children per row inside a cluster. Default 4.")),
			mcp.WithNumber("nodeDX", mcp.Description("Horizontal spacing between nodes, px. Default 130.")),
			mcp.WithNumber("nodeDY", mcp.Description("Vertical spacing between nodes, px. Default 95.")),
		),
		handleCompactGraphLayout,
	)

	s.AddTool(
		mcp.NewTool("pruneLongEdges",
			mcp.WithDescription("Delete edges whose Manhattan distance between endpoints exceeds maxDistancePx (default 600). By default preserveParentEdges:true keeps edges where either endpoint is a hierarchy bucket. Use dryRun:true to preview. Returns scanned/deleted/kept_short/kept_parent counts plus up to 10 example deletions."),
			mcp.WithString("layerId", mcp.Description("UUID of the layer."), mcp.Required()),
			mcp.WithNumber("maxDistancePx", mcp.Description("Distance threshold in pixels (Manhattan). Default 600.")),
			mcp.WithNumber("bucketThreshold", mcp.Description("Min incoming-edge count for an actor to count as a hierarchy bucket. Default 3.")),
			mcp.WithBoolean("preserveParentEdges", mcp.Description("Keep long edges where either endpoint is a bucket. Default true.")),
			mcp.WithBoolean("dryRun", mcp.Description("Don't delete; just count what would be deleted. Default false.")),
		),
		handlePruneLongEdges,
	)

	s.AddTool(
		mcp.NewTool("uploadActorPicture",
			mcp.WithDescription("Upload an image and set it as the actor's picture (graph node avatar). Source can be an HTTP URL, a local path, or a raw base64 string. SVG sources are auto-rasterised to PNG."),
			mcp.WithString("actorId", mcp.Description("Actor UUID whose picture should be set."), mcp.Required()),
			mcp.WithNumber("formId", mcp.Description("Form ID the actor belongs to (needed for the updateActor endpoint)."), mcp.Required()),
			mcp.WithString("imageUrl", mcp.Description("Public HTTP(S) URL of the image. One of imageUrl / localPath / base64 is required.")),
			mcp.WithString("localPath", mcp.Description("Absolute path to an image file on the MCP server host. One of imageUrl / localPath / base64 is required.")),
			mcp.WithString("base64", mcp.Description("Raw base64-encoded image bytes (optionally with a data: prefix). One of imageUrl / localPath / base64 is required.")),
			mcp.WithString("filename", mcp.Description("Override the upload filename (extension drives Content-Type).")),
			mcp.WithNumber("pngWidth", mcp.Description("PNG width when the source is SVG and gets auto-rasterised. Default 256.")),
			mcp.WithNumber("pngHeight", mcp.Description("PNG height when the source is SVG and gets auto-rasterised. Default 256.")),
			mcp.WithString("svgFillColor", mcp.Description("Optional brand colour injected on the <svg> root before rasterising.")),
		),
		handleUploadActorPicture,
	)

	s.AddTool(
		mcp.NewTool("uploadActorPictureBulk",
			mcp.WithDescription("Set pictures on many actors in one call. Identical source images are uploaded once and reused (SHA-256 dedup). Each item supports imageUrl / localPath / base64 / picture plus optional filename, pngWidth, pngHeight, svgFillColor."),
			mcp.WithArray("items", mcp.Description("Array of {actorId, formId, [imageUrl|localPath|base64|picture], filename?, pngWidth?, pngHeight?, svgFillColor?}. Max 500 per call."), mcp.Required()),
			mcp.WithNumber("pngWidth", mcp.Description("Default PNG width for SVG auto-rasterisation. Per-item value wins. Default 256.")),
			mcp.WithNumber("pngHeight", mcp.Description("Default PNG height for SVG auto-rasterisation. Per-item value wins. Default 256.")),
			mcp.WithString("svgFillColor", mcp.Description("Default brand colour to inject on the <svg> root. Per-item value wins.")),
		),
		handleUploadActorPictureBulk,
	)

	s.AddTool(
		mcp.NewTool("createChart",
			mcp.WithDescription("Create a Dashboard (chart) actor on a graph layer. Two source modes: actorFilter (default — charts top-N actors from a form filtered by accountName+currency) or direct accounts (an explicit list). Returns {dashboardActorId, filterActorId, laId}."),
			mcp.WithString("layerId", mcp.Description("Layer actor UUID where the chart will be placed."), mcp.Required()),
			mcp.WithString("title", mcp.Description("Chart title shown on the dashboard actor."), mcp.Required()),
			mcp.WithString("description", mcp.Description("Optional description for the chart.")),
			mcp.WithString("chartType", mcp.Description("Chart visual type: line (default), bar, or area.")),
			mcp.WithString("counterType", mcp.Description("Metric type: amount (default) or turnover.")),
			mcp.WithString("range", mcp.Description("Time range: lastHour (default), lastDay, lastWeek, lastMonth.")),
			mcp.WithNumber("positionX", mcp.Description("X position on the layer canvas. Default -100.")),
			mcp.WithNumber("positionY", mcp.Description("Y position on the layer canvas. Default 0.")),
			mcp.WithString("filterActorId", mcp.Description("(actorFilter mode) UUID of an existing ActorFilters actor to reuse.")),
			mcp.WithString("filterTitle", mcp.Description("(actorFilter mode) Title for a newly created ActorFilters actor. Defaults to the chart title.")),
			mcp.WithNumber("sourceFormId", mcp.Description("(actorFilter mode) Numeric form ID whose actors will be charted. Required when filterActorId is not provided.")),
			mcp.WithString("accountNameId", mcp.Description("(actorFilter mode) UUID of the account name to chart. Required when filterActorId is not provided.")),
			mcp.WithNumber("currencyId", mcp.Description("(actorFilter mode) Numeric currency ID. Required when filterActorId is not provided.")),
			mcp.WithNumber("top", mcp.Description("(actorFilter mode) Number of top actors to show. Default 20.")),
			mcp.WithArray("accounts", mcp.Description("(direct accounts mode) Explicit series: each {actorId, currencyId, nameId, color?, incomeType?}. When provided, actorFilter params are ignored.")),
		),
		handleCreateChart,
	)
}
