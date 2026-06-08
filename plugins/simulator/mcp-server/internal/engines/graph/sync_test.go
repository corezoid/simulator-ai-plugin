package graph

import (
	"context"
	"encoding/json"
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

// TestFetchHierarchyEdgeTypeID drives fetchHierarchyEdgeTypeID against a mock
// backend: it parses the edge-types list, returns the id of the "hierarchy"
// entry, caches it, and errors when no such type exists.
func TestFetchHierarchyEdgeTypeID(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"data":[{"id":3,"name":"link"},{"id":42,"name":"hierarchy"}]}`))
	}))
	t.Cleanup(srv.Close)

	s := newGraphSyncer(srv.URL, "t", "ws-edgetype-test")
	id, err := s.fetchHierarchyEdgeTypeID(context.Background())
	if err != nil {
		t.Fatalf("fetchHierarchyEdgeTypeID: %v", err)
	}
	if id != 42 {
		t.Errorf("edge type id = %d, want 42", id)
	}
	// Second call must be served from cache (no extra HTTP request).
	if _, err := s.fetchHierarchyEdgeTypeID(context.Background()); err != nil {
		t.Fatalf("fetchHierarchyEdgeTypeID (cached): %v", err)
	}
	if calls != 1 {
		t.Errorf("HTTP calls = %d, want 1 (second call should hit cache)", calls)
	}

	// A workspace whose backend has no "hierarchy" type yields an error.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":1,"name":"link"}]}`))
	}))
	t.Cleanup(srv2.Close)
	s2 := newGraphSyncer(srv2.URL, "t", "ws-edgetype-missing")
	_, err = s2.fetchHierarchyEdgeTypeID(context.Background())
	if err == nil {
		t.Fatal("fetchHierarchyEdgeTypeID with no hierarchy type = nil error, want error")
	}
	if !strings.Contains(err.Error(), "hierarchy") {
		t.Errorf("error = %q, want it to mention the missing 'hierarchy' edge type", err)
	}
}

// TestInjectMassLinkData checks that injectMassLinkData stamps the resolved
// hierarchy edgeTypeId onto every link item that lacks one, while leaving an
// explicit edgeTypeId untouched.
func TestInjectMassLinkData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":42,"name":"hierarchy"}]}`))
	}))
	t.Cleanup(srv.Close)

	s := newGraphSyncer(srv.URL, "t", "ws-masslink-test")
	args := map[string]interface{}{
		"body": `[{"source":"s1","target":"t1"},{"source":"s2","target":"t2","edgeTypeId":99}]`,
	}
	if err := s.injectMassLinkData(context.Background(), args); err != nil {
		t.Fatalf("injectMassLinkData: %v", err)
	}

	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(args["body"].(string)), &items); err != nil {
		t.Fatalf("result body not a JSON array: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if got := toInt(items[0]["edgeTypeId"]); got != 42 {
		t.Errorf("item[0].edgeTypeId = %d, want 42 (injected)", got)
	}
	if got := toInt(items[1]["edgeTypeId"]); got != 99 {
		t.Errorf("item[1].edgeTypeId = %d, want 99 (explicit, untouched)", got)
	}
}

// TestInjectManageLayerData checks the manage-layer enrichment: when the form
// referenced by an item carries a pictureBase64, the item gets an areaPicture
// (img/type) and layerSettings (height/width/blockId/shape) derived from the
// form's view; items whose form has no picture are left unmodified.
func TestInjectManageLayerData(t *testing.T) {
	const formWithPic = 555
	const formNoPic = 556
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/forms/555"):
			_, _ = w.Write([]byte(`{"data":{"form":{"pictureBase64":"PIC","sections":[{"content":[` +
				`{"id":"blockId","value":"blk-1"},` +
				`{"id":"view","value":"{\"size\":{\"h\":120,\"w\":80},\"shape\":\"diamond\"}"}` +
				`]}]}}}`))
		case strings.Contains(r.URL.Path, "/forms/556"):
			_, _ = w.Write([]byte(`{"data":{"form":{"pictureBase64":"","sections":[]}}}`))
		default:
			_, _ = w.Write([]byte(`{"data":{}}`))
		}
	}))
	t.Cleanup(srv.Close)

	s := newGraphSyncer(srv.URL, "t", "ws-managelayer-test")
	args := map[string]interface{}{
		"body": `[{"action":"create","data":{"id":555,"type":"node"}},` +
			`{"action":"create","data":{"id":556,"type":"node"}}]`,
	}
	if err := s.injectManageLayerData(context.Background(), args); err != nil {
		t.Fatalf("injectManageLayerData: %v", err)
	}

	var body []map[string]interface{}
	if err := json.Unmarshal([]byte(args["body"].(string)), &body); err != nil {
		t.Fatalf("result body not a JSON array: %v", err)
	}

	withPic := body[0]["data"].(map[string]interface{})
	area, ok := withPic["areaPicture"].(map[string]interface{})
	if !ok {
		t.Fatalf("form-with-picture item missing areaPicture: %v", withPic)
	}
	if area["img"] != "PIC" || area["type"] != "flowchart" {
		t.Errorf("areaPicture = %v, want img=PIC type=flowchart", area)
	}
	if toInt(area["height"]) != 120 || toInt(area["width"]) != 80 {
		t.Errorf("areaPicture size = h%v w%v, want h120 w80", area["height"], area["width"])
	}
	ls, ok := withPic["layerSettings"].(map[string]interface{})
	if !ok {
		t.Fatalf("form-with-picture item missing layerSettings: %v", withPic)
	}
	if ls["blockId"] != "blk-1" || ls["shape"] != "diamond" {
		t.Errorf("layerSettings = %v, want blockId=blk-1 shape=diamond", ls)
	}

	noPic := body[1]["data"].(map[string]interface{})
	if _, has := noPic["areaPicture"]; has {
		t.Errorf("form-without-picture item should be left unmodified, got %v", noPic)
	}
}

// TestUpdateGraphActor exercises the update-decision logic: when title /
// description / color / picture all match the server actor it issues no PUT and
// reports unchanged; when any differs it PUTs and reports changed.
func TestUpdateGraphActor(t *testing.T) {
	var puts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			puts++
			_, _ = w.Write([]byte(`{"data":{"id":"a1"}}`))
			return
		}
		// loadSysForms / any GET → empty so apiFormID stays the child id.
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(srv.Close)

	s := newGraphSyncer(srv.URL, "t", "ws-updateactor-test")

	sa := layerActor{ID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", Title: "Same", Description: "d", Color: "#fff", Picture: "p", FormID: 10}

	// Identical → no update.
	same := GraphActor{Title: "Same", Description: "d", Color: "#fff", Picture: "p"}
	changed, err := s.updateGraphActor(context.Background(), sa, same)
	if err != nil {
		t.Fatalf("updateGraphActor (same): %v", err)
	}
	if changed {
		t.Error("updateGraphActor (identical) = changed, want unchanged")
	}
	if puts != 0 {
		t.Errorf("PUT calls after identical update = %d, want 0", puts)
	}

	// Different title → PUT.
	diff := GraphActor{Title: "Renamed", Description: "d", Color: "#fff", Picture: "p"}
	changed, err = s.updateGraphActor(context.Background(), sa, diff)
	if err != nil {
		t.Fatalf("updateGraphActor (diff): %v", err)
	}
	if !changed {
		t.Error("updateGraphActor (changed title) = unchanged, want changed")
	}
	if puts != 1 {
		t.Errorf("PUT calls after changed update = %d, want 1", puts)
	}
}

// TestFetchLayerActorsPagination checks fetchLayerActors parses the paginated
// nodes response and follows pages until a short page ends iteration.
func TestFetchLayerActorsPagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offset := r.URL.Query().Get("offset")
		if offset == "0" {
			// A full page (50) forces a second fetch.
			var sb strings.Builder
			sb.WriteString(`{"data":[`)
			for i := 0; i < 50; i++ {
				if i > 0 {
					sb.WriteString(",")
				}
				sb.WriteString(`{"id":"a","laId":1,"title":"T"}`)
			}
			sb.WriteString(`]}`)
			_, _ = w.Write([]byte(sb.String()))
			return
		}
		// Short page (1) ends pagination.
		_, _ = w.Write([]byte(`{"data":[{"id":"last","laId":2,"title":"Last"}]}`))
	}))
	t.Cleanup(srv.Close)

	s := newGraphSyncer(srv.URL, "t", "ws-paginate-test")
	actors, err := s.fetchLayerActors(context.Background(), "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatalf("fetchLayerActors: %v", err)
	}
	if len(actors) != 51 {
		t.Errorf("actors = %d, want 51 (50 + 1 across two pages)", len(actors))
	}
	if actors[len(actors)-1].Title != "Last" {
		t.Errorf("last actor title = %q, want Last", actors[len(actors)-1].Title)
	}
}
