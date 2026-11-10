package osint

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
)

// fakeSimulator is the part of the Simulator API a build uses, held in memory.
type fakeSimulator struct {
	mu         sync.Mutex
	next       int
	forms      map[string]int               // ref → id
	actors     map[string]string            // form/ref → id
	updates    int                          // PUTs to existing actors
	nodes      []map[string]any             // layer nodes
	edges      []map[string]any             // layer edges
	links      map[string]map[string]string // source|target → link
	accounts   map[string][]map[string]any  // actor → rows
	transfers  map[string]string            // ref → raw body
	posts      map[string]int               // creating requests, by kind
	bodies     map[string]map[string]any    // actor id → what it holds
	sections   map[string]any               // form ref → sections
	formPuts   int
	formTitle  map[string]string           // form id → title
	names      map[string]string           // account name id → name
	rows       map[string][]map[string]any // account id → its transactions
	refs       map[string]bool             // account id + ref of a plain transaction
	currencies map[string]int              // currency name → id
}

func (f *fakeSimulator) currencyName(id float64) string {
	for name, n := range f.currencies {
		if float64(n) == id {
			return name
		}
	}
	return ""
}

func newFakeSimulator() *fakeSimulator {
	return &fakeSimulator{forms: map[string]int{}, actors: map[string]string{}, links: map[string]map[string]string{},
		accounts: map[string][]map[string]any{}, transfers: map[string]string{}, posts: map[string]int{},
		bodies: map[string]map[string]any{}, sections: map[string]any{}, formTitle: map[string]string{"1": "Graphs", "2": "Layers"},
		names: map[string]string{}, rows: map[string][]map[string]any{}, refs: map[string]bool{}, currencies: map[string]int{}}
}

func (f *fakeSimulator) id() int { f.next++; return 1000 + f.next }

func (f *fakeSimulator) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, _ := io.ReadAll(r.Body)
	seg := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	reply := func(data any) { _ = json.NewEncoder(w).Encode(map[string]any{"data": data}) }
	missing := func() { http.Error(w, `{"statusCode":404,"message":"not found"}`, http.StatusNotFound) }
	var obj map[string]any
	var list []map[string]any
	_ = json.Unmarshal(body, &obj)
	_ = json.Unmarshal(body, &list)
	path := r.Method + " " + strings.Join(seg[:min(2, len(seg))], "/")
	switch {
	case path == "GET forms/ref":
		if id, ok := f.forms[seg[3]]; ok {
			reply(map[string]any{"id": id, "title": "form " + seg[3], "form": map[string]any{"sections": f.sections[seg[3]]}})
			return
		}
		missing()
	case r.Method == "PUT" && seg[0] == "forms":
		f.formPuts++
		for ref, id := range f.forms {
			if strconv.Itoa(id) == seg[1] {
				f.sections[ref] = obj["sections"]
			}
		}
		reply(map[string]any{"id": seg[1]})
	case r.Method == "POST" && seg[0] == "upload":
		f.posts["upload"]++
		reply(map[string]any{"fileName": fmt.Sprintf("stored-%d.png", f.id())})
	case path == "GET forms/templates":
		if r.URL.Query().Get("offset") == "0" {
			reply([]map[string]any{{"id": 1, "title": "Graphs"}, {"id": 2, "title": "Layers"}})
			return
		}
		reply([]any{})
	case r.Method == "POST" && seg[0] == "forms":
		f.posts["form"]++
		id := f.id()
		f.forms[obj["ref"].(string)] = id
		f.formTitle[strconv.Itoa(id)] = obj["title"].(string)
		f.sections[obj["ref"].(string)] = obj["sections"]
		reply(map[string]any{"id": id})
	case path == "GET edge_types/ws":
		reply([]map[string]any{{"id": 7, "name": "reference"}, {"id": 13, "name": "hierarchy"}})
	case path == "GET actors/ref":
		if id, ok := f.actors[seg[2]+"/"+seg[3]]; ok {
			reply(map[string]any{"id": id, "picture": f.bodies[id]["picture"], "data": f.bodies[id]["data"]})
			return
		}
		missing()
	case path == "POST actors/actor":
		f.posts["actor"]++
		id := fmt.Sprintf("actor-%d", f.id())
		f.actors[seg[2]+"/"+obj["ref"].(string)] = id
		obj["formTitle"] = f.formTitle[seg[2]]
		f.bodies[id] = obj
		reply(map[string]any{"id": id})
	case path == "PUT actors/actor":
		f.updates++
		for k, v := range obj {
			f.bodies[seg[3]][k] = v
		}
		reply(map[string]any{"id": seg[3]})
	case path == "POST actors/link":
		f.posts["graphLink"]++
		reply(map[string]any{"id": "graph-link"})
	case path == "POST actors/mass_links":
		var out []map[string]any
		for _, l := range list {
			key := l["source"].(string) + "|" + l["target"].(string)
			if f.links[key] == nil {
				f.posts["link"]++
				f.links[key] = map[string]string{"id": fmt.Sprintf("link-%d", f.id()), "name": l["name"].(string)}
			}
			out = append(out, map[string]any{"error": false, "data": map[string]any{"id": f.links[key]["id"],
				"source": l["source"], "target": l["target"]}})
		}
		reply(out)
	case path == "GET graph_layers/paginated":
		var items []map[string]any
		if r.URL.Query().Get("type") == "edges" {
			items = f.edges
		} else {
			// a node carries what the actor holds, as the real API returns it
			for _, n := range f.nodes {
				b := f.bodies[n["id"].(string)]
				items = append(items, map[string]any{"id": n["id"], "laId": n["laId"], "title": b["title"], "formTitle": b["formTitle"], "data": b["data"]})
			}
		}
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if offset >= len(items) {
			reply([]any{})
			return
		}
		reply(items[offset:min(offset+50, len(items))])
	case path == "POST graph_layers/actors":
		var nodesMap []map[string]any
		for _, it := range list {
			data := it["data"].(map[string]any)
			if data["type"] == "node" {
				f.posts["node"]++
				la := f.id()
				f.nodes = append(f.nodes, map[string]any{"id": data["id"], "laId": la})
				nodesMap = append(nodesMap, map[string]any{"laId": la, "actorId": data["id"]})
				continue
			}
			f.posts["edge"]++
			var src, dst any
			for _, n := range f.nodes {
				if float64(n["laId"].(int)) == data["laIdSource"] {
					src = n["id"]
				}
				if float64(n["laId"].(int)) == data["laIdTarget"] {
					dst = n["id"]
				}
			}
			name := ""
			for _, l := range f.links {
				if l["id"] == data["id"] {
					name = l["name"]
				}
			}
			f.edges = append(f.edges, map[string]any{"id": data["id"], "source": src, "target": dst, "name": name})
		}
		reply(map[string]any{"nodesMap": nodesMap})
	case path == "POST accounts/pair":
		f.names["name-"+obj["accountName"].(string)] = obj["accountName"].(string)
		cur := obj["currencyName"].(string)
		if f.currencies[cur] == 0 {
			f.currencies[cur] = 100 * (len(f.currencies) + 1)
		}
		reply(map[string]any{"accountName": map[string]any{"id": "name-" + obj["accountName"].(string)},
			"currency": map[string]any{"id": f.currencies[cur]}})
	case r.Method == "POST" && seg[0] == "accounts":
		for _, row := range f.accounts[seg[1]] {
			if row["nameId"] == obj["nameId"] && row["currencyId"] == obj["currencyId"] {
				reply([]any{})
				return
			}
		}
		f.posts["account"]++
		for _, side := range []string{"debit", "credit"} {
			f.accounts[seg[1]] = append(f.accounts[seg[1]], map[string]any{"id": fmt.Sprintf("acc-%d-%s", f.id(), side),
				"nameId": obj["nameId"], "currencyId": obj["currencyId"], "incomeType": side, "type": "fact",
				"accountName": f.names[obj["nameId"].(string)], "currencyName": f.currencyName(obj["currencyId"].(float64)),
				"isSystem": false, "counterType": "amount"})
		}
		reply(f.accounts[seg[1]])
	case r.Method == "GET" && seg[0] == "accounts":
		// the platform's own tallies sit beside the actor's accounts
		rows := []map[string]any{{"id": "sys-" + seg[1], "nameId": "sys", "currencyId": 1, "incomeType": "credit", "type": "fact",
			"accountName": "Total edges", "currencyName": "count", "isSystem": true, "counterType": "systemCounter"}}
		reply(append(rows, f.accounts[seg[1]]...))
	case path == "GET transactions/list":
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		rows := f.rows[seg[2]]
		if offset >= len(rows) {
			reply([]any{})
			return
		}
		reply(rows[offset:min(offset+100, len(rows))])
	case path == "GET transactions/ref":
		if f.refs[seg[2]+"|"+seg[3]] {
			reply(map[string]any{"ref": seg[3]})
			return
		}
		missing()
	case r.Method == "POST" && seg[0] == "transactions":
		f.posts["entry"]++
		f.refs[seg[1]+"|"+obj["ref"].(string)] = true
		f.rows[seg[1]] = append(f.rows[seg[1]], map[string]any{"amount": obj["amount"], "comment": obj["comment"], "type": "completed",
			"originalDate": obj["originalDate"], "createdAt": 1})
		reply(map[string]any{"id": 1})
	case r.Method == "GET" && seg[0] == "transfers":
		if _, ok := f.transfers[seg[2]]; ok {
			reply(map[string]any{"ref": seg[2]})
			return
		}
		missing()
	case r.Method == "POST" && seg[0] == "transfers":
		f.posts["transfer"]++
		f.transfers[obj["ref"].(string)] = string(body)
		for _, side := range []string{"from", "to"} {
			for _, l := range obj[side].([]any) {
				leg := l.(map[string]any)
				when := leg["originalDate"]
				if when == nil {
					when = float64(1794320000000)
				}
				acc := leg["accountId"].(string)
				f.rows[acc] = append(f.rows[acc], map[string]any{"amount": leg["amount"], "comment": obj["comment"],
					"transferId": "tr-" + obj["ref"].(string), "type": "completed", "originalDate": when, "createdAt": 1794320000000})
			}
		}
		reply(map[string]any{"id": "tr"})
	default:
		http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusInternalServerError)
	}
}

const buildCase = `
case: {id: demo, title: Demo, currency: USD, synthetic: true}
sources: [{id: s, title: Registry, reliability: B}]
entities:
  - {id: p, type: person, title: P, sources: [s], credibility: 2, aliases: [Pp], attrs: {dob: 1980-02-03, phone: "+000 555 0101", hobby: chess}}
  - {id: o, type: organization, title: O, sources: [s]}
  - {id: x, type: organization, title: X}
relations:
  - {from: p, to: o, type: owns, share: 60, sources: [s]}
  - {from: p, to: o, type: director, sources: [s]}
  - {from: o, to: x, type: associate}
accounts:
  - {id: ap, owner: p, kind: bank}
  - {id: ao, owner: o, kind: bank}
  - {id: ax, owner: x, kind: bank, currency: EUR}
transactions:
  - {id: t1, from: ao, to: ap, amount: 1234.56, date: 2025-01-02, purpose: dividends}
  - {id: t2, from: ap, to: ao, amount: 0.1, date: 2025-01-03}
  - {id: t3, from: ao, to: ax, amount: 5}
`

func TestBuildCreatesTheTwinOnceAndIsRepeatable(t *testing.T) {
	fake := newFakeSimulator()
	srv := httptest.NewServer(fake)
	defer srv.Close()
	ctx := apiclient.WithBaseURL(context.Background(), srv.URL)
	c := mustLoad(t, buildCase)

	res, err := Build(ctx, c, "ws", BuildOptions{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if res.GraphID == "" || res.LayerID == "" || len(res.Forms) != 2 {
		t.Fatalf("result: %+v", res)
	}
	want := map[string]int{"form": 2, "actor": 5, "graphLink": 1, "node": 3, "link": 2, "edge": 2, "account": 3, "transfer": 2}
	if n := fake.posts["upload"]; n < 2 || n > 3 || res.Pictures.Created != 3 {
		t.Errorf("uploaded %d pictures for 3 entities of 2 types: %+v", n, res.Pictures)
	}
	uploads := fake.posts["upload"]
	// the person's fields are filled from the case; an attribute with no field stays in the description
	person := fake.bodies[fake.actors[fmt.Sprint(res.Forms["OSINT Person"])+"/osint:demo:p"]]
	data, _ := person["data"].(map[string]any)
	for k, v := range map[string]string{"item_osint_case": "demo", "item_osint_id": "p", "item_osint_grade": "B2", "item_osint_aliases": "Pp",
		"item_osint_sources": "Registry", "item_osint_dob": "1980-02-03", "item_osint_phone": "+000 555 0101"} {
		if data[k] != v {
			t.Errorf("person field %s = %v, want %q", k, data[k], v)
		}
	}
	if _, has := data["item_osint_hobby"]; has || !strings.Contains(person["description"].(string), "chess") {
		t.Errorf("an attribute without a field: data %v", data)
	}
	if pic, _ := person["picture"].(string); !strings.HasPrefix(pic, "stored-") || !strings.HasPrefix(data["item_osint_picture"].(string), "avatar:") {
		t.Errorf("person picture %q, marker %v", pic, data["item_osint_picture"])
	}
	for kind, n := range want {
		if fake.posts[kind] != n {
			t.Errorf("created %d %s, want %d (all: %v)", fake.posts[kind], kind, n, fake.posts)
		}
	}
	if res.Actors.Created != 3 || res.Links.Created != 2 || res.Transfers.Created != 2 || res.Transfers.Skipped != 1 || res.Accounts != 3 {
		t.Errorf("tallies: %+v", res)
	}
	// two relations between the same two entities are one edge named after both
	for _, l := range fake.links {
		if strings.Contains(l["name"], "owns") && l["name"] != "owns 60% · director" {
			t.Errorf("merged link name = %q", l["name"])
		}
	}
	// the amount goes out as the exact decimal, from the payer's debit side to the payee's credit side
	t1 := fake.transfers["osint:demo:t1"]
	if !strings.Contains(t1, `"amount":1234.56`) || !strings.Contains(t1, "-debit") || !strings.Contains(t1, "-credit") ||
		strings.Index(t1, "-debit") > strings.Index(t1, "-credit") && strings.Index(t1, `"from"`) < strings.Index(t1, `"to"`) {
		t.Errorf("transfer body: %s", t1)
	}
	// the payment's date travels as the transfer's original date: 2025-01-02 UTC in milliseconds
	if strings.Count(t1, `"originalDate":1735776000000`) != 2 {
		t.Errorf("original date: %s", t1)
	}
	if !strings.Contains(fake.transfers["osint:demo:t2"], `"amount":0.1`) {
		t.Errorf("transfer body: %s", fake.transfers["osint:demo:t2"])
	}
	if _, recorded := fake.transfers["osint:demo:t3"]; recorded || len(res.Notes) != 1 {
		t.Errorf("a cross-currency transfer was recorded, or not noted: %v", res.Notes)
	}

	// a second run finds everything by ref and creates nothing
	before := map[string]int{}
	for k, v := range fake.posts {
		before[k] = v
	}
	res, err = Build(ctx, c, "ws", BuildOptions{})
	if err != nil {
		t.Fatalf("second build: %v", err)
	}
	for kind, n := range before {
		if fake.posts[kind] != n {
			t.Errorf("the second run created %d more %s", fake.posts[kind]-n, kind)
		}
	}
	if res.Actors.Existing != 3 || res.Actors.Created != 0 || res.Nodes.Existing != 3 || res.Edges.Existing != 2 ||
		res.Transfers.Existing != 2 || res.Transfers.Created != 0 {
		t.Errorf("second run tallies: %+v", res)
	}
	if fake.posts["upload"] != uploads || res.Pictures.Existing != 3 || res.Pictures.Created != 0 {
		t.Errorf("the second run uploaded pictures again: %d → %d, %+v", uploads, fake.posts["upload"], res.Pictures)
	}
	if fake.formPuts != 0 {
		t.Errorf("complete forms were rewritten %d times", fake.formPuts)
	}

	// a form that lacks fields gets them, and keeps the ones it has
	fake.sections["osint-person"] = []any{map[string]any{"title": "Mine", "content": []any{map[string]any{"id": "item_own", "class": "edit"}}}}
	if _, err = Build(ctx, c, "ws", BuildOptions{Pictures: PicturesNone}); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(fake.sections["osint-person"])
	if fake.formPuts != 1 || !strings.Contains(string(raw), "item_own") || !strings.Contains(string(raw), "item_osint_dob") {
		t.Errorf("form update: puts %d, sections %s", fake.formPuts, raw)
	}
	if fake.posts["upload"] != uploads {
		t.Error("pictures: none still uploaded")
	}
	fake.updates -= 5
	if fake.updates != 5 {
		t.Errorf("existing actors were updated %d times, want 5 (graph, layer, 3 entities)", fake.updates)
	}
}

func TestBuildStopsWithWhatItDidAndRefusesLargeCases(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
	}))
	defer srv.Close()
	ctx := apiclient.WithBaseURL(context.Background(), srv.URL)
	res, err := Build(ctx, mustLoad(t, buildCase), "ws", BuildOptions{})
	if err == nil || !strings.HasPrefix(err.Error(), "forms:") || res == nil {
		t.Errorf("err = %v, res = %+v", err, res)
	}

	big, _ := Generate(GenOptions{Seed: 1, Noise: 300})
	for len(big.Entities) <= maxBuildActors {
		big.Entities = append(big.Entities, Entity{ID: fmt.Sprintf("extra%d", len(big.Entities)), Type: "other", Title: "x"})
	}
	if _, err := Build(ctx, big, "ws", BuildOptions{}); err == nil || !strings.Contains(err.Error(), "at most") {
		t.Errorf("a case over the limit: %v", err)
	}
}

// A case built into Simulator and read back from its layer gives the same
// analysis: the same findings on the same subjects, the same sums and dates.
func TestLayerReadBackGivesTheSameAnalysis(t *testing.T) {
	for _, name := range []string{"procurement-kickback", "trade-overinvoicing", "minimal"} {
		fake := newFakeSimulator()
		srv := httptest.NewServer(fake)
		ctx := apiclient.WithBaseURL(context.Background(), srv.URL)
		c := loadExample(t, name)
		res, err := Build(ctx, c, "ws", BuildOptions{Pictures: PicturesNone})
		if err != nil {
			t.Fatalf("%s: build: %v", name, err)
		}
		back, notes, err := FromLayer(ctx, res.LayerID)
		srv.Close()
		if err != nil {
			t.Fatalf("%s: read back: %v", name, err)
		}
		if rep := Check(back); !rep.OK() {
			t.Fatalf("%s: the layer reads as an inconsistent case: %v", name, rep.Errors)
		}
		if len(back.Entities) != len(c.Entities) || len(back.Relations) != len(c.Relations) ||
			len(back.Accounts) != len(c.Accounts) || len(back.Transactions) != len(c.Transactions) {
			t.Errorf("%s: read back %d entities, %d relations, %d accounts, %d transactions; the case has %d, %d, %d, %d", name,
				len(back.Entities), len(back.Relations), len(back.Accounts), len(back.Transactions),
				len(c.Entities), len(c.Relations), len(c.Accounts), len(c.Transactions))
		}
		want, got := Analyze(c, Options{}), Analyze(back, Options{})
		signature := func(a *Analysis) []string {
			var out []string
			for _, f := range a.Findings {
				out = append(out, f.Severity+" "+f.Kind+" "+strings.Join(sortedCopy(f.Subjects), ","))
			}
			sort.Strings(out)
			return out
		}
		if w, g := signature(want), signature(got); strings.Join(w, "\n") != strings.Join(g, "\n") {
			t.Errorf("%s: findings differ.\ncase:\n%s\nlayer:\n%s", name, strings.Join(w, "\n"), strings.Join(g, "\n"))
		}
		if fmt.Sprint(want.Summary.Volume) != fmt.Sprint(got.Summary.Volume) || want.Summary.FirstDate != got.Summary.FirstDate ||
			want.Summary.LastDate != got.Summary.LastDate {
			t.Errorf("%s: sums or dates differ: %v %s–%s, layer %v %s–%s", name, want.Summary.Volume, want.Summary.FirstDate, want.Summary.LastDate,
				got.Summary.Volume, got.Summary.FirstDate, got.Summary.LastDate)
		}
		// ids, dates and grades of what the build wrote come back; a relation's source does not
		ix := buildIndex(back)
		for _, tr := range c.Transactions {
			b := ix.txs[tr.ID]
			if b == nil || b.Date.String() != tr.Date.String() || b.Purpose != tr.Purpose || b.Amount.String() != tr.Amount.String() {
				t.Errorf("%s: transaction %s came back as %+v", name, tr.ID, b)
			}
		}
		if len(notes) == 0 || !strings.Contains(strings.Join(notes, " "), "does not record sources") {
			t.Errorf("%s: notes: %v", name, notes)
		}
	}
}

// Any layer is read, not only one a case was built into.
func TestLayerOfForeignActorsIsRead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case strings.Contains(r.URL.Path, "/graph_layers/paginated/") && q.Get("offset") != "0":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case q.Get("type") == "nodes":
			_, _ = w.Write([]byte(`{"data":[{"id":"u1","title":"Ann","formTitle":"Human"},{"id":"u2","title":"Shop","formTitle":"Store"},{"id":"u3","title":"","formTitle":"Store"}]}`))
		case q.Get("type") == "edges":
			_, _ = w.Write([]byte(`{"data":[{"source":"u1","target":"u2","name":"runs"},{"source":"u2","target":"u1","name":"owns 40%"},{"source":"u1","target":"gone","name":"x"}]}`))
		case strings.HasPrefix(r.URL.Path, "/accounts/"):
			_, _ = w.Write([]byte(`{"data":[{"id":"x","isSystem":true,"counterType":"systemCounter","incomeType":"credit"}]}`))
		default:
			http.Error(w, "unexpected "+r.URL.String(), http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	c, notes, err := FromLayer(apiclient.WithBaseURL(context.Background(), srv.URL), "0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if rep := Check(c); !rep.OK() {
		t.Fatalf("errors: %v", rep.Errors)
	}
	if len(c.Entities) != 3 || c.Entities[0].Type != "person" || c.Entities[1].Type != "other" || c.Entities[1].Attrs["kind"] != "Store" || c.Entities[2].Title != "n3" {
		t.Errorf("entities: %+v", c.Entities)
	}
	// a name that is not a relation type is a plain link that keeps it; a person is not owned
	if len(c.Relations) != 2 || c.Relations[0].Type != "other" || c.Relations[0].Note != "runs" || c.Relations[1].Type != "other" || c.Relations[1].Share != nil {
		t.Errorf("relations: %+v", c.Relations)
	}
	if len(c.Accounts) != 0 || !strings.Contains(strings.Join(notes, " "), "Store") {
		t.Errorf("accounts %d, notes %v", len(c.Accounts), notes)
	}
}

func TestRecordActivityWritesDatedEntriesOnce(t *testing.T) {
	fake := newFakeSimulator()
	srv := httptest.NewServer(fake)
	defer srv.Close()
	ctx := apiclient.WithBaseURL(context.Background(), srv.URL)
	c := &Collection{Source: KindTelegram, Target: "demo_channel", Retrieved: "2026-03-10T12:00:00Z"}
	for _, it := range []struct {
		date  string
		views float64
	}{{"2026-01-05T10:00:00Z", 100}, {"2026-01-20T10:00:00Z", 50}, {"2026-02-03T10:00:00Z", 70}, {"2026-03-09T10:00:00Z", 999}, {"", 5}} {
		c.Items = append(c.Items, Item{ID: it.date, Date: it.date, Metrics: map[string]float64{"views": it.views}})
	}
	res, err := RecordActivity(ctx, "ws", "actor-1", c, BucketMonth)
	if err != nil {
		t.Fatal(err)
	}
	// March had not ended when the collection was made, and an undated item has no bucket
	if len(res.Accounts) != 2 || res.From != "2026-01-01" || res.To != "2026-02-01" || fake.posts["entry"] != 4 {
		t.Fatalf("recorded: %+v, entries %d", res, fake.posts["entry"])
	}
	items, views := res.Accounts[0], res.Accounts[1]
	if items.Name != "Activity · items" || items.Total != 3 || views.Name != "Activity · views" || views.Total != 220 || views.Entries.Created != 2 {
		t.Errorf("series: %+v %+v", items, views)
	}
	rows := fake.rows[views.Account]
	if len(rows) != 2 || rows[0]["amount"] != float64(150) || rows[0]["originalDate"] != float64(1767225600000) {
		t.Errorf("entries of views: %v", rows)
	}
	// again: nothing new
	res, err = RecordActivity(ctx, "ws", "actor-1", c, BucketMonth)
	if err != nil || fake.posts["entry"] != 4 || res.Accounts[1].Entries.Existing != 2 || res.Accounts[1].Entries.Created != 0 {
		t.Errorf("second recording: %v, entries %d, %+v", err, fake.posts["entry"], res.Accounts)
	}
	// a collection cut at its limit starts mid-bucket: its first bucket is left out
	c.Truncated = true
	res, _ = RecordActivity(ctx, "ws", "actor-2", c, BucketMonth)
	if res.From != "2026-02-01" || res.Accounts[0].Total != 1 {
		t.Errorf("truncated collection: %+v", res)
	}
	if _, err = RecordActivity(ctx, "ws", "actor-1", c, "fortnight"); err == nil {
		t.Error("an unknown bucket was accepted")
	}
	if res, _ = RecordActivity(ctx, "ws", "actor-1", &Collection{Source: "x", Target: "y"}, ""); res.Skipped == "" {
		t.Error("an empty collection was not reported as skipped")
	}
}
