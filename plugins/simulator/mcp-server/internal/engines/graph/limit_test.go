package graph

import "testing"

// TestMaxLayerPageLimit guards the page size shared by every paginated layer
// reader (getAllLayerPlacements, compactGraphLayout, pruneLongEdges). The
// backend rejects /graph_layers/paginated requests with ?limit > 50
// ("querystring/limit must be <= 50"), so the cap must stay within that bound —
// regressing it to 100 (the original bug) makes those tools fail on any layer
// with more than 50 nodes.
func TestMaxLayerPageLimit(t *testing.T) {
	if maxLayerPageLimit < 1 || maxLayerPageLimit > 50 {
		t.Fatalf("maxLayerPageLimit = %d, must be in [1,50] (backend caps ?limit at 50)", maxLayerPageLimit)
	}
}
