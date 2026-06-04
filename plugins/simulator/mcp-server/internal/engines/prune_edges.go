package engines

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"
)

// pruneLongEdges removes edges on a layer whose Manhattan distance between
// endpoints exceeds `maxDistancePx`. Optionally, `preserveParentEdges:true`
// (default) keeps any edge where either endpoint is a "bucket" (an actor with
// `>= bucketThreshold` incoming edges) — those are usually hierarchical and
// rightly span the canvas.
//
// Output: {scanned, deleted, kept_short, kept_parent, errors[]}.
//
// Use case: after a layout pass moves clusters apart, leftover cross-cluster
// edges turn into long diagonals that visually obscure the graph. This tool
// trims those diagonals while preserving meaningful parent → child wiring.

type pruneStats struct {
	Scanned    int      `json:"scanned"`
	Deleted    int      `json:"deleted"`
	KeptShort  int      `json:"kept_short"`
	KeptParent int      `json:"kept_parent"`
	Errors     []string `json:"errors,omitempty"`
	DryRun     bool     `json:"dryRun"`
	Examples   []string `json:"deletedExamples,omitempty"`
}

func handlePruneLongEdges(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if authResult := ensureAuth(ctx); authResult != nil {
		return authResult, nil
	}
	args := req.GetArguments()
	layerID, _ := args["layerId"].(string)
	if layerID == "" {
		return mcp.NewToolResultError("[Error] layerId is required"), nil
	}
	maxDist := toInt(args["maxDistancePx"])
	if maxDist <= 0 {
		maxDist = 600
	}
	bucketThreshold := toInt(args["bucketThreshold"])
	if bucketThreshold == 0 {
		bucketThreshold = 3
	}
	dryRun, _ := args["dryRun"].(bool)
	preserveParents := true
	if v, ok := args["preserveParentEdges"].(bool); ok {
		preserveParents = v
	}

	client := apiHTTPClient()
	apiGet := func(url string) ([]byte, error) {
		hr, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		hr.Header.Set("Authorization", Cfg.Authorization)
		resp, err := client.Do(hr)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 300 {
			return nil, fmt.Errorf("HTTP %d: %.200s", resp.StatusCode, b)
		}
		return b, nil
	}
	apiDelete := func(url string) error {
		hr, _ := http.NewRequestWithContext(ctx, "DELETE", url, nil)
		hr.Header.Set("Authorization", Cfg.Authorization)
		resp, err := client.Do(hr)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 300 {
			return fmt.Errorf("HTTP %d: %.200s", resp.StatusCode, b)
		}
		return nil
	}

	// 1) Pull all placements (for positions).
	type plac struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Position struct {
			X int `json:"x"`
			Y int `json:"y"`
		} `json:"position"`
	}
	positionOf := map[string]struct{ X, Y int }{}
	titleOf := map[string]string{}
	const limit = 100
	offset := 0
	for {
		u := fmt.Sprintf("%s/graph_layers/paginated/%s?type=nodes&limit=%d&offset=%d",
			buildBaseURL(), layerID, limit, offset)
		body, err := apiGet(u)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] fetch placements: %v", err)), nil
		}
		var page struct {
			Data []plac `json:"data"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] parse placements: %v", err)), nil
		}
		for _, p := range page.Data {
			positionOf[p.ID] = struct{ X, Y int }{X: p.Position.X, Y: p.Position.Y}
			titleOf[p.ID] = p.Title
		}
		if len(page.Data) < limit {
			break
		}
		offset += limit
	}

	// 2) Pull all edges.
	type edgeRow struct {
		ID     string `json:"id"`
		Source string `json:"source"`
		Target string `json:"target"`
	}
	var edges []edgeRow
	offset = 0
	for {
		u := fmt.Sprintf("%s/graph_layers/paginated/%s?type=edges&limit=%d&offset=%d",
			buildBaseURL(), layerID, limit, offset)
		body, err := apiGet(u)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] fetch edges: %v", err)), nil
		}
		var page struct {
			Data []edgeRow `json:"data"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] parse edges: %v", err)), nil
		}
		edges = append(edges, page.Data...)
		if len(page.Data) < limit {
			break
		}
		offset += limit
	}

	// 3) Compute incoming-edge counts to decide buckets.
	incoming := map[string]int{}
	for _, e := range edges {
		incoming[e.Target]++
	}
	isBucket := func(id string) bool {
		return incoming[id] >= bucketThreshold
	}

	stats := pruneStats{DryRun: dryRun}

	// 4) For each edge: compute Manhattan distance, decide.
	abs := func(a int) int {
		if a < 0 {
			return -a
		}
		return a
	}
	for _, e := range edges {
		stats.Scanned++
		ps, sok := positionOf[e.Source]
		pt, tok := positionOf[e.Target]
		if !sok || !tok {
			// Edge endpoints missing from layer — leave it alone.
			stats.KeptShort++
			continue
		}
		dist := abs(ps.X-pt.X) + abs(ps.Y-pt.Y)
		if dist <= maxDist {
			stats.KeptShort++
			continue
		}
		if preserveParents && (isBucket(e.Source) || isBucket(e.Target)) {
			stats.KeptParent++
			continue
		}
		// Delete it.
		if !dryRun {
			u := fmt.Sprintf("%s/actors/link/%s", buildBaseURL(), e.ID)
			if err := apiDelete(u); err != nil {
				stats.Errors = append(stats.Errors,
					fmt.Sprintf("%s→%s: %v", titleOf[e.Source], titleOf[e.Target], err))
				continue
			}
		}
		stats.Deleted++
		if len(stats.Examples) < 10 {
			stats.Examples = append(stats.Examples,
				fmt.Sprintf("%s → %s (d=%d)",
					titleOf[e.Source], titleOf[e.Target], dist))
		}
	}

	out, _ := json.Marshal(stats)
	return mcp.NewToolResultText(string(out)), nil
}
