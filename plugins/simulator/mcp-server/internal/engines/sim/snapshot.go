package sim

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"regexp"
	"sort"
	"time"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"
	"gopkg.in/yaml.v3"
)

// systemForms are graph-of-graph placements, not domain actors.
var systemForms = map[string]bool{"Layers": true, "Graphs": true}

// noAccess matches the API errors for an actor whose accounts the caller cannot read.
var noAccess = regexp.MustCompile(`HTTP (403|404)\b`)

const pageLimit = 50 // /graph_layers/paginated rejects limit > 50

type layerNode struct {
	ID        string         `json:"id"`
	Title     string         `json:"title"`
	FormID    json.Number    `json:"formId"`
	FormTitle string         `json:"formTitle"`
	AccID     string         `json:"accId"`
	Data      map[string]any `json:"data"`
	Position  *struct {
		X json.Number `json:"x"`
		Y json.Number `json:"y"`
	} `json:"position"`
}

type layerEdge struct {
	ID            string `json:"id"`
	Source        string `json:"source"`
	Target        string `json:"target"`
	EdgeType      string `json:"edgeType"`
	LinkedActorID string `json:"linkedActorId"`
}

type accountRow struct {
	ID           string      `json:"id"`
	AccountName  string      `json:"accountName"`
	CurrencyName string      `json:"currencyName"`
	CurrencyID   json.Number `json:"currencyId"`
	NameID       string      `json:"nameId"`
	Amount       json.Number `json:"amount"`
	IncomeType   string      `json:"incomeType"`
	Type         string      `json:"type"`
	IsSystem     bool        `json:"isSystem"`
}

func papiGet(ctx context.Context, path string, q url.Values, out any) error {
	u := ecore.BuildBaseURLForContext(ctx) + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	body, err := ecore.PapiGET(ctx, u)
	if err != nil {
		return err
	}
	var wrap struct {
		Data json.RawMessage `json:"data"`
	}
	dec := json.NewDecoder(bytesReader(body))
	dec.UseNumber()
	if err := dec.Decode(&wrap); err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	d := json.NewDecoder(bytesReader(wrap.Data))
	d.UseNumber()
	return d.Decode(out)
}

// readPaged reads every page. With untilEmpty it stops only on an empty page: the layer
// endpoints apply LIMIT before dropping deleted elements, so a short page is not the last.
func readPaged[T any](ctx context.Context, path string, q url.Values, limit int, untilEmpty bool) ([]T, error) {
	var all []T
	for offset := 0; ; offset += limit {
		qq := url.Values{}
		for k, v := range q {
			qq[k] = v
		}
		qq.Set("limit", fmt.Sprint(limit))
		qq.Set("offset", fmt.Sprint(offset))
		var page []T
		if err := papiGet(ctx, path, qq, &page); err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) == 0 || (!untilEmpty && len(page) < limit) {
			return all, nil
		}
	}
}

func jsonNum(n json.Number) any {
	if n == "" {
		return nil
	}
	if r, ok := parseNum(n.String()); ok {
		return r
	}
	return n.String()
}

// Snapshot reads a layer with its actors, links and account values. With period > 0 the
// values are the turnover of the last period (getAccounts from/to) instead of the balance.
func Snapshot(ctx context.Context, layerID string, period time.Duration) (*Graph, error) {
	nodes, err := readPaged[layerNode](ctx, "/graph_layers/paginated/"+ecore.Seg(layerID), url.Values{"type": {"nodes"}}, pageLimit, true)
	if err != nil {
		return nil, fmt.Errorf("read layer nodes: %w", err)
	}
	edges, err := readPaged[layerEdge](ctx, "/graph_layers/paginated/"+ecore.Seg(layerID), url.Values{"type": {"edges"}}, pageLimit, true)
	if err != nil {
		return nil, fmt.Errorf("read layer edges: %w", err)
	}
	g := layerGraph(layerID, nodes, edges)
	if period > 0 {
		g.Source.Set("period_seconds", ratInt(int64(period.Seconds())))
	}
	window := url.Values{"highPrecision": {"true"}} // exact sums, not JS numbers
	if period > 0 {
		now := time.Now()
		window.Set("from", fmt.Sprint(now.Add(-period).UnixMilli()))
		window.Set("to", fmt.Sprint(now.UnixMilli()))
	}
	var skipped []string
	for _, a := range append([]*Actor(nil), g.actors...) {
		rows, err := readPaged[accountRow](ctx, "/accounts/"+ecore.Seg(a.ID), window, 100, false)
		if err != nil {
			// the layer lists actors the caller cannot view, but their accounts need view access
			if noAccess.MatchString(err.Error()) {
				skipped = append(skipped, a.Title)
				continue
			}
			return nil, fmt.Errorf("read accounts of %s: %w", a.Title, err)
		}
		readAccounts(g, a, rows)
	}
	if len(skipped) > 0 {
		list := make([]any, len(skipped))
		for i, t := range skipped {
			list[i] = t
		}
		g.Source.Set("accounts_not_readable", list)
	}
	return g, nil
}

// layerGraph builds the snapshot graph from the layer's nodes and edges.
func layerGraph(layerID string, nodes []layerNode, edges []layerEdge) *Graph {
	// Graph order drives the random draws, and the layer endpoints return elements in
	// different orders, so a snapshot lists actors and links sorted by id (model format §Graph).
	sort.SliceStable(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	sort.SliceStable(edges, func(i, j int) bool { return edges[i].ID < edges[j].ID })
	g := newGraph()
	g.Source.Set("kind", "simulator")
	g.Source.Set("layer", layerID)
	g.Source.Set("taken_at", ratInt(time.Now().Unix()))
	for _, n := range nodes {
		if g.actorIdx[n.ID] != nil || systemForms[n.FormTitle] {
			continue // one actor placed twice is one actor
		}
		if n.AccID != "" && !g.Source.Has("workspace") {
			g.Source.Set("workspace", n.AccID)
		}
		data, _ := fromJSON(anyMap(n.Data)).(*OMap)
		if data == nil {
			data = NewOMap()
		}
		data.Set("_form_id", jsonNum(n.FormID))
		if n.Position != nil {
			pos := NewOMap()
			pos.Set("x", jsonNum(n.Position.X))
			pos.Set("y", jsonNum(n.Position.Y))
			data.Set("_position", pos)
		}
		typ := n.FormTitle
		if typ == "" {
			typ = n.FormID.String()
		}
		g.addActor(&Actor{ID: n.ID, Type: typ, Title: n.Title, Data: data, OriginID: n.ID})
	}
	for _, e := range edges {
		// an edge placed twice on the layer is one link; edges to system actors are dropped
		if g.linkIdx[e.ID] != nil || g.actorIdx[e.Source] == nil || g.actorIdx[e.Target] == nil {
			continue
		}
		et := e.EdgeType
		if et == "" {
			et = hierarchy
		}
		g.addLink(&Link{ID: e.ID, Source: e.Source, Target: e.Target, EdgeType: et, Mediator: e.LinkedActorID})
	}
	return g
}

func anyMap(m map[string]any) any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

// readAccounts folds Simulator's debit/credit pair per (account name, currency) into one value.
func readAccounts(g *Graph, a *Actor, rows []accountRow) {
	type pair struct {
		name, currency, nameID, currencyID string
		value                              *big.Rat
	}
	var order []string
	pairs := map[string]*pair{}
	names := map[string]int{}
	for _, r := range rows {
		if r.IsSystem || (r.Type != "" && r.Type != "fact") {
			continue
		}
		key := r.NameID + "|" + r.CurrencyID.String()
		p := pairs[key]
		if p == nil {
			name := r.AccountName
			if name == "" {
				name = r.NameID
			}
			p = &pair{name: name, currency: r.CurrencyName, nameID: r.NameID, currencyID: r.CurrencyID.String(), value: new(big.Rat)}
			pairs[key] = p
			order = append(order, key)
			names[name]++
		}
		amt, _ := parseNum(r.Amount.String())
		if amt == nil {
			amt = new(big.Rat)
		}
		if r.IncomeType == "debit" {
			p.value.Sub(p.value, amt)
		} else {
			p.value.Add(p.value, amt)
		}
	}
	meta := NewOMap()
	for _, key := range order {
		p := pairs[key]
		vt := p.currency
		if vt == "" {
			vt = "value"
		}
		if _, ok := g.valueTypes[vt]; !ok {
			g.valueTypes[vt] = ValueType{Name: vt, Scale: 8} // Simulator stores Numeric(100,8)
		}
		name := p.name
		if names[p.name] > 1 {
			name = fmt.Sprintf("%s [%s]", p.name, p.currency)
		}
		q, _ := g.valueTypes[vt].quantize(p.value)
		g.addAccount(&Account{ActorID: a.ID, Name: name, ValueType: vt, Value: q})
		m := NewOMap()
		m.Set("nameId", p.nameID)
		m.Set("currencyId", p.currencyID)
		m.Set("currency", p.currency)
		meta.Set(name, m)
	}
	if meta.Len() > 0 {
		a.Data.Set("_accounts", meta)
	}
}

// GraphFile renders g in the plugin's layer YAML with sim: sections (spec §1).
func GraphFile(g *Graph, layerID string) ([]byte, error) {
	type pos struct {
		X any `yaml:"x"`
		Y any `yaml:"y"`
	}
	var actors []map[string]any
	for _, a := range g.actors {
		public, internal := map[string]any{}, map[string]any{}
		for _, k := range a.Data.Keys() {
			v := toYAML(a.Data.m[k])
			if len(k) > 0 && k[0] == '_' {
				internal[k] = v
			} else {
				public[k] = v
			}
		}
		entry := map[string]any{"id": a.ID, "title": a.Title, "formName": a.Type,
			"sim": map[string]any{"type": a.Type, "origin_id": nilIfEmpty(a.OriginID), "created_by": nilIfEmpty(a.CreatedBy), "state": internal}}
		if f, ok := a.Data.Get("_form_id"); ok && f != nil {
			entry["formId"] = toYAML(f)
		}
		if len(public) > 0 {
			entry["data"] = public
		}
		p := pos{0, 0}
		if pm, ok := a.Data.m["_position"].(*OMap); ok {
			x, _ := pm.Get("x")
			y, _ := pm.Get("y")
			p = pos{toYAML(x), toYAML(y)}
		}
		entry["position"] = p
		actors = append(actors, entry)
	}
	var edges []map[string]any
	for _, l := range g.links {
		edges = append(edges, map[string]any{"source": l.Source, "target": l.Target,
			"sim": map[string]any{"id": l.ID, "edgeType": l.EdgeType, "mediator": nilIfEmpty(l.Mediator)}})
	}
	vts := map[string]any{}
	for name, vt := range g.valueTypes {
		kind := "decimal"
		if vt.Integer {
			kind = "integer"
		}
		spec := map[string]any{"kind": kind, "scale": vt.Scale, "conserved": vt.Conserved, "min": nil, "max": nil}
		if vt.Min != nil {
			spec["min"] = numString(vt.Min)
		}
		if vt.Max != nil {
			spec["max"] = numString(vt.Max)
		}
		vts[name] = spec
	}
	var accounts []map[string]any
	for _, a := range g.accounts {
		accounts = append(accounts, map[string]any{"actor_id": a.ActorID, "name": a.Name, "value_type": a.ValueType, "value": numString(a.Value)})
	}
	if layerID == "" {
		layerID = getString(g.Source, "layer")
	}
	return yaml.Marshal(map[string]any{"layerId": layerID, "actors": actors, "edges": edges,
		"sim": map[string]any{"format": "sim-morrow/1", "source": toYAML(g.Source), "valueTypes": vts, "accounts": accounts}})
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// unnamedFormTypes returns, per form id, the actors whose type is a bare form id: a
// pullGraphFile output has formId but no formName, so its types are numbers.
func unnamedFormTypes(g *Graph) map[string][]*Actor {
	out := map[string][]*Actor{}
	for _, a := range g.actors {
		if f, ok := a.Data.Get("_form_id"); ok && f != nil && a.Type != "" && a.Type == show(f) {
			out[a.Type] = append(out[a.Type], a)
		}
	}
	return out
}

// nameFormTypes replaces bare form-id types with form titles read from Simulator
// (GET /forms/{id}). Forms that cannot be read keep their id. Returns how many actors
// got a title.
func nameFormTypes(ctx context.Context, g *Graph) int {
	n := 0
	for id, actors := range unnamedFormTypes(g) {
		var form struct {
			Title string `json:"title"`
		}
		if err := papiGet(ctx, "/forms/"+ecore.Seg(id), nil, &form); err != nil || form.Title == "" {
			continue
		}
		for _, a := range actors {
			a.Type = form.Title
			n++
		}
	}
	return n
}
