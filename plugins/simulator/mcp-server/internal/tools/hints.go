package tools

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Safety hints (MCP tool annotations) for every registered tool.
//
// mcp.NewTool defaults every tool to readOnlyHint:false, destructiveHint:true,
// so without this pass getActor and deleteActor look the same to a client,
// and connector directories reject tools whose hints contradict what they do.
// Curated operations are classified by HTTP method, with the exceptions
// below; engine and helper tools (no Operation) are listed by name.
//
// openWorldHint is false by default: the workspace is a bounded private
// account, and a hosted API alone does not make a tool open-world. Only the
// tools in openWorldTools reach arbitrary destinations.

type toolHints struct{ readOnly, destructive, idempotent bool }

var (
	hintRead        = toolHints{readOnly: true, idempotent: true}
	hintAdditive    = toolHints{}                                    // creates or appends; nothing lost
	hintDestructive = toolHints{destructive: true}                   // deletes, overwrites or moves value
	hintOverwrite   = toolHints{destructive: true, idempotent: true} // replaces state with the given value
)

// hintsByName overrides the method-derived class: POST endpoints that only
// read, POST endpoints that only add, and tools that are not Operations.
var hintsByName = map[string]toolHints{
	// POST, read-only
	"getCounters":       hintRead,
	"existLink":         hintRead,
	"existLayerElement": hintRead,
	"filterTransfers":   hintRead,

	// POST, additive
	"createAccount":      hintAdditive,
	"createAccountName":  hintAdditive,
	"createAccountPair":  hintAdditive,
	"createCurrency":     hintAdditive,
	"createForm":         hintAdditive,
	"createFormAccount":  hintAdditive,
	"createActor":        hintAdditive,
	"createReaction":     hintAdditive,
	"markReactionsRead":  {idempotent: true},
	"setCounters":        hintOverwrite, // sets fixed values, not increments
	"requestAccess":      hintAdditive,
	"addAttachments":     hintAdditive,
	"uploadBase64":       hintAdditive,
	"generatePublicLink": hintAdditive,

	// createLink and massLink (POST, so destructive) replace a placeholder
	// "hole" link with a real one, dropping the hole's history.

	// PUT that sets the pinned flag to the given value
	"togglePinnedReaction": {idempotent: true},

	// not Operations: read-only
	"buildLink":             hintRead,
	"diffReleases":          hintRead,
	"findAgent":             hintRead,
	"findSkill":             hintRead,
	"getSkill":              hintRead,
	"getAllLayerPlacements": hintRead,
	"getBbcodeTags":         hintRead,
	"getFileHistory":        hintRead,
	"getFileVersion":        hintRead,
	"getTaskStatus":         hintRead,
	"listReleases":          hintRead,
	"listTrash":             hintRead,
	"readAttachment":        hintRead,
	"simulationCheck":       hintRead,
	"simulationRun":         hintRead,

	// not Operations: additive
	"getAgent":           hintAdditive, // get-or-creates a user twin
	"getSystemActor":     hintAdditive, // get-or-creates a user twin
	"appGetPage":         hintAdditive, // rendering runs the app's Corezoid process
	"exportGraph":        hintAdditive,
	"uploadGraphFile":    hintAdditive,
	"createChart":        hintAdditive,
	"createSmartForm":    hintAdditive,
	"restoreFromTrash":   hintAdditive,
	"simulationSnapshot": hintAdditive,
	"pullGraphFile":      hintAdditive, // writes a local file only
	"pullSmartForm":      hintAdditive,

	// not Operations: destructive
	"importGraph":        hintDestructive, // "reuse" strategy merges into existing objects
	"deploySmartForm":    hintDestructive,
	"rollbackFile":       hintDestructive,
	"rollbackRelease":    hintDestructive,
	"compactGraphLayout": hintDestructive, // rewrites node positions
	"pruneLongEdges":     hintDestructive, // deletes edges
	"updateChart":        hintDestructive, // overwrites the chart config; re-placing gives a new laId
}

// openWorldTools fetch an arbitrary URL the caller names, or publish
// workspace data to anyone holding the link.
var openWorldTools = map[string]bool{
	"uploadActorPicture":     true, // imageUrl
	"uploadActorPictureBulk": true, // imageUrl per item
	"uploadGraphFile":        true, // fileUrl
	"generatePublicLink":     true, // public, unauthenticated link
}

// hintsFor returns the hints of a tool, or false when it is not classified
// (it then keeps whatever its registration declared).
func hintsFor(name, method string) (toolHints, bool) {
	if h, ok := hintsByName[name]; ok {
		return h, true
	}
	switch method {
	case "GET":
		return hintRead, true
	case "PUT", "DELETE":
		return hintOverwrite, true
	case "POST":
		return hintDestructive, true
	}
	return toolHints{}, false
}

// ApplyHints sets the safety hints on every tool registered on s. Call it
// after all tools are registered.
func ApplyHints(s *server.MCPServer) {
	methods := make(map[string]string)
	for _, op := range allOps() {
		methods[op.Name] = op.Method
	}
	for name, st := range s.ListTools() {
		h, ok := hintsFor(name, methods[name])
		if !ok {
			continue
		}
		t := st.Tool
		t.Annotations.ReadOnlyHint = mcp.ToBoolPtr(h.readOnly)
		t.Annotations.DestructiveHint = mcp.ToBoolPtr(h.destructive)
		t.Annotations.IdempotentHint = mcp.ToBoolPtr(h.idempotent)
		t.Annotations.OpenWorldHint = mcp.ToBoolPtr(openWorldTools[name])
		s.AddTool(t, st.Handler)
	}
}
