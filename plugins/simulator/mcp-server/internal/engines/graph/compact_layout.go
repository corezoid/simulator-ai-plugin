package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"
	"github.com/mark3labs/mcp-go/mcp"
)

// actorGridSize mirrors pong-server's ACTOR_SIZE: every node position is snapped
// to this pixel grid before it is stored and before the occupied-cell check runs.
const actorGridSize = 50

// snapCoord replicates pong-server's snapGeomCoord (round to the nearest
// ACTOR_SIZE cell). pong uses JS Math.round, which rounds halves toward +∞, so
// we use floor(v/size + 0.5) rather than Go's math.Round (which rounds halves
// away from zero) to agree with the server on negative coordinates.
func snapCoord(v int) int {
	return int(math.Floor(float64(v)/float64(actorGridSize)+0.5)) * actorGridSize
}

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
	// X, Y are the placement's current position on the layer (raw, pre-snap),
	// used to skip placements that are already on their computed cell.
	X int `json:"-"`
	Y int `json:"-"`
}

type compactEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

func handleCompactGraphLayout(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if authResult := ecore.EnsureAuth(ctx); authResult != nil {
		return authResult, nil
	}
	args := req.GetArguments()
	layerID, _ := args["layerId"].(string)
	if layerID == "" {
		return mcp.NewToolResultError("[Error] layerId is required"), nil
	}
	if r := ecore.RequireUUID("layerId", layerID); r != nil {
		return r, nil
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

	client := ecore.APIHTTPClient()
	apiGet := func(url string) ([]byte, error) {
		hr, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		hr.Header.Set("Authorization", ecore.AuthHeaderForContext(ctx))
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
	const limit = maxLayerPageLimit
	offset := 0
	for {
		u := fmt.Sprintf("%s/graph_layers/paginated/%s?type=nodes&limit=%d&offset=%d",
			ecore.BuildBaseURLForContext(ctx), layerID, limit, offset)
		body, err := apiGet(u)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] fetch placements: %v", err)), nil
		}
		var page struct {
			Data []struct {
				ID       string `json:"id"`
				LaID     int    `json:"laId"`
				Title    string `json:"title"`
				Position struct {
					X jsonInt `json:"x"`
					Y jsonInt `json:"y"`
				} `json:"position"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] parse placements: %v", err)), nil
		}
		for _, p := range page.Data {
			pl := compactPlacement{
				ActorID: p.ID, LaID: p.LaID, Title: p.Title,
				X: int(p.Position.X), Y: int(p.Position.Y),
			}
			placementsByActor[p.ID] = append(placementsByActor[p.ID], pl)
			titleByActor[p.ID] = p.Title
			allPlacements = append(allPlacements, pl)
		}
		// a short page is not the last: the server drops deleted elements after LIMIT
		if len(page.Data) == 0 {
			break
		}
		offset += limit
		if offset >= limit*maxLayerPages {
			return mcp.NewToolResultError("[Error] " + errTooManyPages().Error()), nil
		}
	}

	// 2) Harvest edges on the layer.
	var edges []compactEdge
	offset = 0
	for {
		u := fmt.Sprintf("%s/graph_layers/paginated/%s?type=edges&limit=%d&offset=%d",
			ecore.BuildBaseURLForContext(ctx), layerID, limit, offset)
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
		// a short page is not the last: the server drops deleted elements after LIMIT
		if len(page.Data) == 0 {
			break
		}
		offset += limit
		if offset >= limit*maxLayerPages {
			return mcp.NewToolResultError("[Error] " + errTooManyPages().Error()), nil
		}
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

	// 7) Build the position updates — one entry per placement, snapped to pong's
	// grid. Two correctness rules the server's occupied-cell check forces on us:
	//
	//   - Skip placements already on their computed cell. pong snaps every
	//     incoming position and then rejects the whole request if a target cell
	//     is occupied by a node that is NOT also moving in the same request. A
	//     node sent to the cell it already sits on counts as occupying itself, so
	//     including it fails the request — and makes a second identical run fail.
	//   - Keep two placements out of the same cell within one request. pong only
	//     checks occupancy against rows already in the DB, not duplicates inside
	//     the request, so two placements aimed at one cell would stack silently.
	//     Every placement of a given actor otherwise gets that actor's single
	//     computed position; nudge the extras onto neighbouring free cells.
	type updateItem struct {
		ID       string         `json:"id"`
		Position map[string]int `json:"position"`
	}
	// occupied holds every cell already claimed. Seed it with the snapped current
	// cells of the placements we are NOT moving, so a mover is never nudged onto a
	// stationary node (which pong would then reject as occupied).
	occupied := map[[2]int]bool{}
	type pendingMove struct {
		laID   int
		tx, ty int // snapped target cell
	}
	var movers []pendingMove
	// Deterministic order so the layout (and the tests) are stable.
	sort.Slice(allPlacements, func(i, j int) bool { return allPlacements[i].LaID < allPlacements[j].LaID })
	for _, pl := range allPlacements {
		pos, ok := positions[pl.ActorID]
		if !ok {
			continue
		}
		tx, ty := snapCoord(pos.X), snapCoord(pos.Y)
		cx, cy := snapCoord(pl.X), snapCoord(pl.Y)
		if tx == cx && ty == cy {
			occupied[[2]int{cx, cy}] = true // already in place — reserve its cell
			continue
		}
		movers = append(movers, pendingMove{laID: pl.LaID, tx: tx, ty: ty})
	}
	// Assign each mover a free cell, nudging off any collision (duplicate
	// placements of one actor, or two actors that snapped to the same cell).
	rowWidth := nodesPerRow * clustersPerRow
	if rowWidth < 1 {
		rowWidth = 1
	}
	items := make([]updateItem, 0, len(movers))
	for _, m := range movers {
		x, y := m.tx, m.ty
		for step := 1; occupied[[2]int{x, y}]; step++ {
			x = snapCoord(m.tx + (step%rowWidth)*nodeDX)
			y = snapCoord(m.ty + (step/rowWidth)*nodeDY)
		}
		occupied[[2]int{x, y}] = true
		items = append(items, updateItem{
			ID:       fmt.Sprintf("%d", m.laID),
			Position: map[string]int{"x": x, "y": y},
		})
	}

	// 8) Apply all placements in a single request. The schema has no maxItems,
	// and one PUT also avoids running pong's per-request side effects (realtime
	// publish, sendLayerChangesProcess, queueLayerToGit, coordinate transactions)
	// once per batch. Splitting into batches is what broke it: a node moving into
	// a cell another node vacates is fine only when both are in the same request —
	// pong exempts an occupant that is itself moving — so a move split across
	// batches hit "Occupied cells" and left the layer half-compacted.
	//
	// The body is a bare JSON array (Fastify body schema `type: array`, items
	// validated against actorPosition) — the same contract the declarative
	// `updateLayerPositions` tool uses via InBodyRoot. Wrapping it as
	// {"items": [...]} is rejected with 400 "body must be array".
	stats.PlacementsMoved = len(items)
	if len(items) > 0 {
		apiPut := func(url string, body interface{}) error {
			bodyBytes, _ := json.Marshal(body)
			hr, _ := http.NewRequestWithContext(ctx, "PUT", url, strings.NewReader(string(bodyBytes)))
			hr.Header.Set("Authorization", ecore.AuthHeaderForContext(ctx))
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
		u := fmt.Sprintf("%s/graph_layers/actors/%s", ecore.BuildBaseURLForContext(ctx), layerID)
		if err := apiPut(u, items); err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("[Error] applyPositions: %v", err)), nil
		}
	}

	out, _ := json.Marshal(stats)
	return mcp.NewToolResultText(string(out)), nil
}
