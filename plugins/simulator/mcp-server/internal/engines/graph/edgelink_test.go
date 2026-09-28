package graph

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// massLinkStub serves the two endpoints createEdgeLink hits: the edge-type lookup
// (so injectMassLinkData can resolve the "hierarchy" type) and mass_links itself,
// whose response body is the caller-supplied massLinkResp.
func massLinkStub(t *testing.T, massLinkResp string) *GraphSyncer {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "mass_links") {
			_, _ = w.Write([]byte(massLinkResp))
			return
		}
		// edge_types lookup
		_, _ = w.Write([]byte(`{"data":[{"id":1,"name":"hierarchy"}]}`))
	}))
	t.Cleanup(srv.Close)
	return newGraphSyncer(srv.URL, "t", t.Name())
}

// TestCreateEdgeLinkRejectsErrorItem guards issue #88: a per-item error must not be
// read as a created edge even when the backend echoes back an id, or the graph
// records a ghost link that silently disappears.
func TestCreateEdgeLinkRejectsErrorItem(t *testing.T) {
	s := massLinkStub(t, `{"data":[{"error":true,"data":{"id":"ghost-id"}}]}`)
	id, err := s.createEdgeLink(context.Background(), "src-uuid", "tgt-uuid")
	if err == nil {
		t.Fatalf("expected an error for an error:true item, got id=%q", id)
	}
	if id != "" {
		t.Errorf("expected empty id on error, got %q", id)
	}
}

// TestCreateEdgeLinkAcceptsSuccessItem is the positive counterpart: a clean item
// still returns its id.
func TestCreateEdgeLinkAcceptsSuccessItem(t *testing.T) {
	s := massLinkStub(t, `{"data":[{"error":false,"data":{"id":"real-id"}}]}`)
	id, err := s.createEdgeLink(context.Background(), "src-uuid", "tgt-uuid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "real-id" {
		t.Errorf("id = %q, want %q", id, "real-id")
	}
}
