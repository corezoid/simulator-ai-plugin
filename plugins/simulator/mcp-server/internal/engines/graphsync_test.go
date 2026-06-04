package engines

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestFormIDFromLayerActor covers the rule that a `__form__<id>:...` data key
// (the child form id) wins over the top-level formId, with a fallback.
func TestFormIDFromLayerActor(t *testing.T) {
	cases := []struct {
		name string
		sa   layerActor
		want int
	}{
		{"top-level only", layerActor{FormID: 100}, 100},
		{
			"form key overrides",
			layerActor{FormID: 100, Data: map[string]interface{}{"__form__408962:view": 1}},
			408962,
		},
		{
			"malformed key falls back",
			layerActor{FormID: 100, Data: map[string]interface{}{"__form__bad": 1, "other": 2}},
			100,
		},
		{"no form id at all", layerActor{}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formIDFromLayerActor(tc.sa); got != tc.want {
				t.Errorf("formIDFromLayerActor = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestIsUUID(t *testing.T) {
	valid := []string{
		"ab636ecd-b68b-4a33-97cb-309521b330e6",
		"AB636ECD-B68B-4A33-97CB-309521B330E6", // case-insensitive
	}
	invalid := []string{"", "not-a-uuid", "12345", "ab636ecd-b68b-4a33-97cb"}
	for _, s := range valid {
		if !isUUID(s) {
			t.Errorf("isUUID(%q) = false, want true", s)
		}
	}
	for _, s := range invalid {
		if isUUID(s) {
			t.Errorf("isUUID(%q) = true, want false", s)
		}
	}
}

// TestBuildAndResolveFormNameID checks the form-name cache is built from the
// full SysFormItem tree (including nested childs) and that a rebuild clears it.
func TestBuildAndResolveFormNameID(t *testing.T) {
	s := newGraphSyncer("http://x", "t", "ws-formname-test")

	forms := []SysFormItem{
		{ID: 1, Title: "Graph", Childs: []SysFormItem{
			{ID: 2, Title: "Layer"},
			{ID: 3, Title: "Car", Childs: []SysFormItem{{ID: 4, Title: "Wheel"}}},
		}},
	}
	s.cache.mu.Lock()
	s.buildFormNameIDCache(forms)
	s.cache.mu.Unlock()

	for name, want := range map[string]int{"Graph": 1, "Layer": 2, "Car": 3, "Wheel": 4} {
		if got := s.resolveFormNameToID(name); got != want {
			t.Errorf("resolveFormNameToID(%q) = %d, want %d", name, got, want)
		}
	}
	if got := s.resolveFormNameToID("Missing"); got != 0 {
		t.Errorf("resolveFormNameToID(Missing) = %d, want 0", got)
	}

	// Rebuild with a different set clears the old names.
	s.cache.mu.Lock()
	s.buildFormNameIDCache([]SysFormItem{{ID: 9, Title: "Only"}})
	s.cache.mu.Unlock()
	if got := s.resolveFormNameToID("Car"); got != 0 {
		t.Errorf("after rebuild resolveFormNameToID(Car) = %d, want 0", got)
	}
	if got := s.resolveFormNameToID("Only"); got != 9 {
		t.Errorf("resolveFormNameToID(Only) = %d, want 9", got)
	}
}

// TestCacheActorFormIDFromResult checks createActor/getActor responses are parsed
// into the UUID→formId cache, preferring the __form__ data key over top-level formId.
func TestCacheActorFormIDFromResult(t *testing.T) {
	s := newGraphSyncer("http://x", "t", "ws-actorform-test")

	s.cacheActorFormIDFromResult(`{"data":{"id":"a1","formId":5,"data":{"__form__408962:view":1}}}`)
	s.cacheActorFormIDFromResult(`{"data":{"id":"a2","formId":7}}`)
	s.cacheActorFormIDFromResult(`{"data":{"id":"","formId":9}}`) // no id → ignored
	s.cacheActorFormIDFromResult(`not json`)                      // ignored

	s.cache.mu.RLock()
	defer s.cache.mu.RUnlock()
	if got := s.cache.actorFormIDs["a1"]; got != 408962 {
		t.Errorf("a1 formId = %d, want 408962 (__form__ key wins)", got)
	}
	if got := s.cache.actorFormIDs["a2"]; got != 7 {
		t.Errorf("a2 formId = %d, want 7 (top-level)", got)
	}
	if _, ok := s.cache.actorFormIDs[""]; ok {
		t.Error("empty id should not be cached")
	}
}

// TestPushGraphDeletesServerExtras drives the full diff orchestration against a
// mock backend: in full-sync mode, a server actor absent from the file is
// deleted via a manage-layer "delete" item. This covers the create/update/delete
// decision path of pushGraph end-to-end, not just its helpers.
func TestPushGraphDeletesServerExtras(t *testing.T) {
	const layer = "11111111-1111-1111-1111-111111111111"
	const serverActor = "22222222-2222-2222-2222-222222222222"

	var manageBodies []string
	nodeFetches := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/graph_layers/paginated/"):
			if r.URL.Query().Get("type") == "edges" {
				_, _ = w.Write([]byte(`{"data":[]}`))
				return
			}
			nodeFetches++
			if nodeFetches == 1 { // initial fetch: one actor on the layer
				_, _ = w.Write([]byte(`{"data":[{"id":"` + serverActor + `","laId":7}]}`))
			} else { // re-fetch after the delete
				_, _ = w.Write([]byte(`{"data":[]}`))
			}
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/graph_layers/actors/"):
			b, _ := io.ReadAll(r.Body)
			manageBodies = append(manageBodies, string(b))
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			_, _ = w.Write([]byte(`{"data":[]}`))
		}
	}))
	t.Cleanup(srv.Close)

	s := newGraphSyncer(srv.URL, "t", "ws-pushdiff-test")
	res, err := s.pushGraph(context.Background(), GraphFile{LayerID: layer}, layer)
	if err != nil {
		t.Fatalf("pushGraph: %v", err)
	}
	if res.ActorsDeleted != 1 {
		t.Errorf("ActorsDeleted = %d, want 1", res.ActorsDeleted)
	}
	found := false
	for _, b := range manageBodies {
		if strings.Contains(b, "delete") && strings.Contains(b, serverActor) {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a manage-layer delete for %s; got bodies: %v", serverActor, manageBodies)
	}
}

// TestOverrideActorFormID checks the cached formId is corrected to the child id.
func TestOverrideActorFormID(t *testing.T) {
	s := newGraphSyncer("http://x", "t", "ws-override-test")
	s.cacheActorFormIDFromResult(`{"data":{"id":"a1","formId":100}}`)
	s.overrideActorFormID(`{"data":{"id":"a1"}}`, 333)

	s.cache.mu.RLock()
	defer s.cache.mu.RUnlock()
	if got := s.cache.actorFormIDs["a1"]; got != 333 {
		t.Errorf("a1 formId = %d, want 333 (overridden)", got)
	}
}
