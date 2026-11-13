package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
)

// The paginated layer endpoint applies LIMIT and then drops deleted elements, so a page
// can be short in the middle of a layer. Reading must go on until an empty page.
func TestFetchLayerActorsReadsPastShortPage(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		off := r.URL.Query().Get("offset")
		calls = append(calls, off)
		var data []map[string]any
		switch off {
		case "0": // 50 asked, one deleted actor dropped
			for i := 0; i < 49; i++ {
				data = append(data, map[string]any{"id": fmt.Sprint("a", i)})
			}
		case "50":
			for i := 50; i < 60; i++ {
				data = append(data, map[string]any{"id": fmt.Sprint("a", i)})
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer srv.Close()
	ctx := apiclient.WithAuthorization(apiclient.WithBaseURL(context.Background(), srv.URL), "Simulator test")
	actors, err := fetchLayerActors(ctx, "layer")
	if err != nil {
		t.Fatal(err)
	}
	if len(actors) != 59 {
		t.Errorf("actors = %d, want 59 (pages at offsets %v)", len(actors), calls)
	}
}

// Review of #115: with "read until an empty page", a server that ignores offset must not loop.
func TestFetchLayerActorsStopsAtThePageCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": "same"}}})
	}))
	defer srv.Close()
	prev := maxLayerPages
	maxLayerPages = 3
	defer func() { maxLayerPages = prev }()
	ctx := apiclient.WithAuthorization(apiclient.WithBaseURL(context.Background(), srv.URL), "Simulator test")
	if _, err := fetchLayerActors(ctx, "layer"); err == nil {
		t.Fatal("want an error after the page cap")
	}
}
