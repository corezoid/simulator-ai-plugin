package engines

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// compactGraphLayout repositions every placement on a layer into a tight
// domain-clustered grid. For each "bucket" actor (one that has more than
// `bucketThreshold` incoming edges, default 3), the tool collects its
// children, lays them out in a fixed-size grid under the bucket, and the
// buckets themselves into a super-grid (4 columns by default). Actors that
// don't fit any cluster are stacked into a "Misc" zone below the main grid.
//
// The tool reads the existing graph state via /graph_layers/paginated and
// /actors/link, applies the new positions via PUT /graph_layers/actors/{layerId}
// (the same path used by `layerActorsPosition`), and returns counters.
//
// Strategy `domain-clusters` is the only one implemented today; the
// `strategy` arg is reserved for future force-directed / hierarchical layouts.

type compactStats struct {
	Clusters           int `json:"clusters"`
	BucketActors       int `json:"bucketActors"`
	ChildrenPositioned int `json:"childrenPositioned"`
	LeftoverPositioned int `json:"leftoverPositioned"`
	PlacementsMoved    int `json:"placementsMoved"`
}

type compactPlacement struct {
	ActorID string `json:"actorId"`
	LaID    int    `json:"laId"`
	Title   string `json:"title"`
}

type compactEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

func handleCompactGraphLayout(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if authResult := ensureAuth(ctx); authResult != nil {
		return authResult, nil
	}
	args := req.GetArguments()
	layerID, _ := args["layerId"].(string)
	if layerID == "" {
		return mcp.NewToolResultError("[Error] layerId is required"), nil
	}
	strategy, _ := args["strategy"].(string)
	if strategy == "" {
		strategy = "domain-clusters"
	}
	if strategy != "domain-clusters" {
		return mcp.NewToolResultError(
			"[Error] only strategy=domain-clusters is supported today"), nil
	}
	bucketThreshold := toInt(args["bucketThreshold"])
	if bucketThreshold == 0 {
		bucketThreshold = 3
	}
	clustersPerRow := toInt(args["clustersPerRow"])
	if clustersPerRow == 0 {
		clustersPerRow = 4
	}
	nodesPerRow := toInt(args["nodesPerRow"])
	if nodesPerRow == 0 {
		nodesPerRow = 4
	}
	nodeDX := toInt(args["nodeDX"])
	if nodeDX == 0 {
		nodeDX = 130
	}
	nodeDY := toInt(args["nodeDY"])
	if nodeDY == 0 {
		nodeDY = 95
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

	// 1) Harvest all placements on the layer.
	placementsByActor := map[string][]compactPlacement{}
	titleByActor := map[string]string{}
	allPlacements := []compactPlacement{}
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
			Data []struct {
				ID    string `json:"id"`
				LaID  int    `json:"laId"`
				Title string `json:"title"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] parse placements: %v", err)), nil
		}
		for _, p := range page.Data {
			pl := compactPlacement{ActorID: p.ID, LaID: p.LaID, Title: p.Title}
			placementsByActor[p.ID] = append(placementsByActor[p.ID], pl)
			titleByActor[p.ID] = p.Title
			allPlacements = append(allPlacements, pl)
		}
		if len(page.Data) < limit {
			break
		}
		offset += limit
	}

	// 2) Harvest edges on the layer.
	var edges []compactEdge
	offset = 0
	for {
		u := fmt.Sprintf("%s/graph_layers/paginated/%s?type=edges&limit=%d&offset=%d",
			buildBaseURL(), layerID, limit, offset)
		body, err := apiGet(u)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] fetch edges: %v", err)), nil
		}
		var page struct {
			Data []struct {
				Source string `json:"source"`
				Target string `json:"target"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] parse edges: %v", err)), nil
		}
		for _, e := range page.Data {
			edges = append(edges, compactEdge{Source: e.Source, Target: e.Target})
		}
		if len(page.Data) < limit {
			break
		}
		offset += limit
	}

	// 3) Count incoming edges per actor — actors that exceed `bucketThreshold`
	//    are treated as buckets.
	incoming := map[string]int{}
	parentOf := map[string][]string{} // child → list of parents (any incident edge counts)
	for _, e := range edges {
		incoming[e.Target]++
		// Both directions are considered "parent candidates" since the graph
		// uses mixed-direction hierarchies in practice.
		parentOf[e.Source] = append(parentOf[e.Source], e.Target)
		parentOf[e.Target] = append(parentOf[e.Target], e.Source)
	}
	buckets := []string{}
	for actorID := range placementsByActor {
		if incoming[actorID] >= bucketThreshold {
			buckets = append(buckets, actorID)
		}
	}
	// Sort buckets by descending in-degree, then alphabetically for stable layout.
	sort.Slice(buckets, func(i, j int) bool {
		if incoming[buckets[i]] != incoming[buckets[j]] {
			return incoming[buckets[i]] > incoming[buckets[j]]
		}
		return strings.ToLower(titleByActor[buckets[i]]) <
			strings.ToLower(titleByActor[buckets[j]])
	})

	// 4) Assign actors to clusters — greedy: each actor goes to the first
	//    bucket it has an edge to (in bucket-priority order). Buckets themselves
	//    aren't members of other clusters.
	bucketSet := map[string]bool{}
	for _, b := range buckets {
		bucketSet[b] = true
	}
	clusterChildren := map[string][]string{}
	used := map[string]bool{}
	for _, b := range buckets {
		used[b] = true
	}
	for actorID := range placementsByActor {
		if used[actorID] {
			continue
		}
		parents := parentOf[actorID]
		bestBucket := ""
		bestIdx := len(buckets) + 1
		for _, p := range parents {
			if !bucketSet[p] {
				continue
			}
			// Take the bucket that appears earliest in the sorted list.
			for i, b := range buckets {
				if b == p && i < bestIdx {
					bestIdx = i
					bestBucket = b
					break
				}
			}
		}
		if bestBucket != "" {
			clusterChildren[bestBucket] = append(clusterChildren[bestBucket], actorID)
			used[actorID] = true
		}
	}
	for _, kids := range clusterChildren {
		sort.Slice(kids, func(i, j int) bool {
			return strings.ToLower(titleByActor[kids[i]]) <
				strings.ToLower(titleByActor[kids[j]])
		})
	}

	// 5) Layout: clusters in a super-grid, children within each cluster.
	// Reserve vertical space for the tallest cluster so rows of clusters don't
	// overlap. A previous fixed rowsPerCluster=5 truncated the reserved height
	// for clusters with more than 5*nodesPerRow children, overlapping the row
	// of clusters below them.
	maxChildren := 0
	for _, kids := range clusterChildren {
		if len(kids) > maxChildren {
			maxChildren = len(kids)
		}
	}
	rowsPerCluster := (maxChildren + nodesPerRow - 1) / nodesPerRow
	if rowsPerCluster < 1 {
		rowsPerCluster = 1
	}
	clusterW := nodesPerRow*nodeDX + 40
	clusterH := (rowsPerCluster+1)*nodeDY + 40
	gapX := 60
	gapY := 50
	padX := 200
	padY := 250

	positions := map[string]struct{ X, Y int }{}
	for idx, b := range buckets {
		col := idx % clustersPerRow
		row := idx / clustersPerRow
		cx := padX + col*(clusterW+gapX)
		cy := padY + row*(clusterH+gapY)
		positions[b] = struct{ X, Y int }{X: cx + (nodesPerRow-1)*nodeDX/2, Y: cy}
		for k, child := range clusterChildren[b] {
			mr := k / nodesPerRow
			mc := k % nodesPerRow
			positions[child] = struct{ X, Y int }{
				X: cx + mc*nodeDX,
				Y: cy + nodeDY + mr*nodeDY,
			}
		}
	}
	stats := compactStats{
		Clusters:           len(buckets),
		BucketActors:       len(buckets),
		ChildrenPositioned: 0,
	}
	for _, kids := range clusterChildren {
		stats.ChildrenPositioned += len(kids)
	}

	// 6) Leftovers — actors not in any cluster.
	leftoverRow := (len(buckets) + clustersPerRow - 1) / clustersPerRow
	leftoverY := padY + leftoverRow*(clusterH+gapY) + 100
	leftover := []string{}
	for actorID := range placementsByActor {
		if _, placed := positions[actorID]; !placed {
			leftover = append(leftover, actorID)
		}
	}
	sort.Slice(leftover, func(i, j int) bool {
		return strings.ToLower(titleByActor[leftover[i]]) <
			strings.ToLower(titleByActor[leftover[j]])
	})
	leftoverPerRow := clustersPerRow * nodesPerRow
	for k, actorID := range leftover {
		col := k % leftoverPerRow
		row := k / leftoverPerRow
		positions[actorID] = struct{ X, Y int }{
			X: padX + col*nodeDX,
			Y: leftoverY + row*nodeDY,
		}
	}
	stats.LeftoverPositioned = len(leftover)

	// 7) Build the position update batch — one entry per placement (not per actor).
	type updateItem struct {
		ID       string         `json:"id"`
		Position map[string]int `json:"position"`
	}
	items := []updateItem{}
	for actorID, pls := range placementsByActor {
		pos, ok := positions[actorID]
		if !ok {
			continue
		}
		for _, pl := range pls {
			items = append(items, updateItem{
				ID:       fmt.Sprintf("%d", pl.LaID),
				Position: map[string]int{"x": pos.X, "y": pos.Y},
			})
		}
	}
	// 8) Push in chunks of 100.
	apiPut := func(url string, body interface{}) error {
		bodyBytes, _ := json.Marshal(body)
		hr, _ := http.NewRequestWithContext(ctx, "PUT", url, strings.NewReader(string(bodyBytes)))
		hr.Header.Set("Authorization", Cfg.Authorization)
		hr.Header.Set("Content-Type", "application/json")
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
	const batchSize = 100
	for i := 0; i < len(items); i += batchSize {
		end := i + batchSize
		if end > len(items) {
			end = len(items)
		}
		batch := items[i:end]
		u := fmt.Sprintf("%s/graph_layers/actors/%s", buildBaseURL(), layerID)
		if err := apiPut(u, map[string]interface{}{"items": batch}); err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("[Error] applyPositions batch %d: %v", i/batchSize, err)), nil
		}
		stats.PlacementsMoved += len(batch)
	}

	out, _ := json.Marshal(stats)
	return mcp.NewToolResultText(string(out)), nil
}
