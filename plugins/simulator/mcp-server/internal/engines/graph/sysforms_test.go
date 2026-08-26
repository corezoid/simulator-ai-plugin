package graph

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// TestLoadSysFormsRetriesAfterTransientFailure guards issue #87: a transient
// failure must not be cached (it would otherwise poison the workspace until the
// process restarts). The first call fails; the second must retry and succeed,
// returning valid forms with no lingering error.
func TestLoadSysFormsRetriesAfterTransientFailure(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":10,"title":"Graphs","parentId":null}]}`))
	}))
	t.Cleanup(srv.Close)

	s := newGraphSyncer(srv.URL, "t", t.Name())

	if _, err := s.loadSysForms(context.Background()); err == nil {
		t.Fatal("expected an error on the first (failing) load")
	}
	// The failure must NOT have been cached — the retry hits the server again.
	forms, err := s.loadSysForms(context.Background())
	if err != nil {
		t.Fatalf("retry after transient failure should succeed, got: %v", err)
	}
	if len(forms) == 0 {
		t.Fatal("retry returned no forms — the failure poisoned the cache")
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("server calls = %d, want 2 (failure not cached → retried)", got)
	}
}
