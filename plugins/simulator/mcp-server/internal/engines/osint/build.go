package osint

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/graph"
)

// Limits of one build call. A larger case is built in parts, or by the
// curated tools from osintCasePlan.
const (
	maxBuildActors    = 1000
	maxBuildTransfers = 3000
	layerPageSize     = 50
	layerCell         = 50 // the layer grid, in pixels
	linkBatchSize     = 50
)

// BuildResult says what a build did. A build is repeatable: everything is
// found again by its ref, so a second run of the same case creates nothing,
// and a run that stopped half-way is finished by running it again.
type BuildResult struct {
	Case      string         `json:"case"`
	Workspace string         `json:"workspace"`
	GraphID   string         `json:"graphId,omitempty"`
	LayerID   string         `json:"layerId"`
	Forms     map[string]int `json:"forms"`
	Actors    Tally          `json:"actors"`
	Pictures  Tally          `json:"pictures"`
	Nodes     Tally          `json:"nodesOnLayer"`
	Links     Tally          `json:"links"`
	Edges     Tally          `json:"edgesOnLayer"`
	Accounts  int            `json:"accounts"`
	Transfers Tally          `json:"transfers"`
	Notes     []string       `json:"notes,omitempty"`
}

// Tally counts what one step created and what it found already there.
type Tally struct {
	Created  int `json:"created"`
	Existing int `json:"existing"`
	Skipped  int `json:"skipped,omitempty"`
}

type builder struct {
	ctx      context.Context
	base     string
	ws       string
	res      *BuildResult
	pictures string
	uploaded map[string]string // picture key → storage name, within this build
}

func (b *builder) get(path string, out any) error {
	body, err := ecore.PapiGET(b.ctx, b.base+path)
	if err != nil {
		return err
	}
	return decode(body, out)
}

func (b *builder) send(method, path string, in, out any) error {
	payload, err := json.Marshal(in)
	if err != nil {
		return err
	}
	var body []byte
	if method == http.MethodPut {
		body, err = ecore.PapiPUT(b.ctx, b.base+path, payload)
	} else {
		body, err = ecore.PapiPOST(b.ctx, b.base+path, payload)
	}
	if err != nil {
		return err
	}
	return decode(body, out)
}

// decode reads the `data` member of an API reply into out.
func decode(body []byte, out any) error {
	if out == nil {
		return nil
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("parse reply: %w (body: %.200s)", err, body)
	}
	if len(env.Data) == 0 {
		return fmt.Errorf("reply has no data (body: %.200s)", body)
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("parse reply data: %w (body: %.200s)", err, body)
	}
	return nil
}

func notFound(err error) bool { return ecore.HTTPStatusOf(err) == http.StatusNotFound }

// BuildOptions tune a build.
type BuildOptions struct {
	LayerID  string // an existing layer to build on; empty gives the case its own graph and layer
	Pictures string // PicturesAuto (default), PicturesGiven or PicturesNone
}

// Build creates the case in Simulator as its plan says.
func Build(ctx context.Context, c *Case, workspaceID string, opt BuildOptions) (*BuildResult, error) {
	if opt.Pictures == "" {
		opt.Pictures = PicturesAuto
	}
	layerID := opt.LayerID
	p := BuildPlan(c)
	if len(p.Actors) > maxBuildActors || len(p.Transfers) > maxBuildTransfers {
		return nil, fmt.Errorf("the case has %d entities and %d transactions; one build call takes at most %d and %d",
			len(p.Actors), len(p.Transfers), maxBuildActors, maxBuildTransfers)
	}
	b := &builder{ctx: ctx, base: ecore.BuildBaseURLForContext(ctx), ws: ecore.Seg(workspaceID),
		res:      &BuildResult{Case: p.Case, Workspace: workspaceID, LayerID: layerID, Forms: map[string]int{}},
		pictures: opt.Pictures, uploaded: map[string]string{}}

	steps := []struct {
		name string
		run  func(*Plan) error
	}{
		{"forms", b.forms},
		{"layer", b.layer},
		{"actors", b.actors},
		{"placement", b.placeNodes},
		{"links", b.links},
		{"accounts", b.accounts},
		{"transfers", b.transfers},
	}
	for _, s := range steps {
		if err := ctx.Err(); err != nil {
			return b.res, fmt.Errorf("%s: %w", s.name, err)
		}
		if err := s.run(p); err != nil {
			return b.res, fmt.Errorf("%s: %w", s.name, err)
		}
	}
	return b.res, nil
}

// ---- forms --------------------------------------------------------------------------

// formSections renders a form's fields as one section.
func formSections(fields []PlanField) []map[string]any {
	content := make([]map[string]any, len(fields))
	for i, f := range fields {
		content[i] = map[string]any{"id": f.ID, "class": "edit", "title": f.Title, "visibility": "visible"}
	}
	return []map[string]any{{"title": "Case", "content": content}}
}

// withMissingFields returns the sections of an existing form with the fields
// it lacks appended, and whether anything was added. Fields the workspace
// added to the form itself are kept.
func withMissingFields(sections []map[string]any, fields []PlanField) ([]map[string]any, bool) {
	have := map[string]bool{}
	for _, sec := range sections {
		items, _ := sec["content"].([]any)
		for _, it := range items {
			if m, ok := it.(map[string]any); ok {
				if id, _ := m["id"].(string); id != "" {
					have[id] = true
				}
			}
		}
	}
	var missing []PlanField
	for _, f := range fields {
		if !have[f.ID] {
			missing = append(missing, f)
		}
	}
	if len(missing) == 0 {
		return sections, false
	}
	added := formSections(missing)[0]["content"].([]map[string]any)
	for _, sec := range sections {
		if title, _ := sec["title"].(string); title == "Case" {
			items, _ := sec["content"].([]any)
			for _, a := range added {
				items = append(items, a)
			}
			sec["content"] = items
			return sections, true
		}
	}
	return append(sections, map[string]any{"title": "Case", "content": added}), true
}

func (b *builder) forms(p *Plan) error {
	for _, f := range p.Forms {
		var found struct {
			ID    int    `json:"id"`
			Title string `json:"title"`
			Form  struct {
				Sections []map[string]any `json:"sections"`
			} `json:"form"`
		}
		err := b.get("/forms/ref/"+b.ws+"/"+ecore.Seg(f.Ref), &found)
		switch {
		case err == nil && found.ID != 0:
			if sections, changed := withMissingFields(found.Form.Sections, f.Fields); changed {
				body := map[string]any{"title": found.Title, "sections": sections}
				if err := b.send(http.MethodPut, fmt.Sprintf("/forms/%d", found.ID), body, nil); err != nil {
					return fmt.Errorf("add fields to form %q: %w", f.Title, err)
				}
				b.res.Notes = append(b.res.Notes, fmt.Sprintf("form %q got the fields it lacked", f.Title))
			}
		case err == nil || notFound(err):
			body := map[string]any{"title": f.Title, "ref": f.Ref, "color": f.Color, "sections": formSections(f.Fields),
				"description": "An entity of an investigation case (simulator-osint). Attributes, sources and grades are also in the actor description."}
			if err := b.send(http.MethodPost, "/forms/"+b.ws+"/true", body, &found); err != nil {
				return fmt.Errorf("create form %q: %w", f.Title, err)
			}
			if found.ID == 0 {
				return fmt.Errorf("create form %q: the reply has no id", f.Title)
			}
		default:
			return fmt.Errorf("look up form %q: %w", f.Title, err)
		}
		b.res.Forms[f.Title] = found.ID
	}
	return nil
}

// systemForm resolves a system form (Graphs, Layers) by its title.
func (b *builder) systemForm(title string) (int, error) {
	for offset := 0; ; offset += 200 {
		var page []struct {
			ID    int    `json:"id"`
			Title string `json:"title"`
		}
		path := fmt.Sprintf("/forms/templates/system/%s?formTypes=system&limit=200&offset=%d", b.ws, offset)
		if err := b.get(path, &page); err != nil {
			return 0, fmt.Errorf("list system forms: %w", err)
		}
		for _, f := range page {
			if f.Title == title {
				return f.ID, nil
			}
		}
		if len(page) == 0 {
			return 0, fmt.Errorf("system form %q not found in the workspace", title)
		}
	}
}

func (b *builder) hierarchyEdgeType() (int, error) {
	var types []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if err := b.get("/edge_types/"+b.ws, &types); err != nil {
		return 0, fmt.Errorf("list edge types: %w", err)
	}
	for _, t := range types {
		if t.Name == "hierarchy" {
			return t.ID, nil
		}
	}
	return 0, fmt.Errorf("the workspace has no hierarchy edge type")
}

// ---- actors -------------------------------------------------------------------------

type actorReply struct {
	ID string `json:"id"`
}

// ensureActor finds an actor by (form, ref) or creates it; an existing one is
// brought up to date with the case. It reports whether it was created.
func (b *builder) ensureActor(formID int, ref string, body map[string]any) (string, bool, error) {
	var a actorReply
	err := b.get(fmt.Sprintf("/actors/ref/%d/%s?filter=id", formID, ecore.Seg(ref)), &a)
	switch {
	case err == nil && a.ID != "":
		if err := b.send(http.MethodPut, fmt.Sprintf("/actors/actor/%d/%s", formID, ecore.Seg(a.ID)), body, nil); err != nil {
			return "", false, fmt.Errorf("update actor %s: %w", ref, err)
		}
		return a.ID, false, nil
	case err == nil || notFound(err):
		create := map[string]any{"ref": ref}
		for k, v := range body {
			create[k] = v
		}
		if err := b.send(http.MethodPost, fmt.Sprintf("/actors/actor/%d", formID), create, &a); err != nil {
			return "", false, fmt.Errorf("create actor %s: %w", ref, err)
		}
		if a.ID == "" {
			return "", false, fmt.Errorf("create actor %s: the reply has no id", ref)
		}
		return a.ID, true, nil
	}
	return "", false, fmt.Errorf("look up actor %s: %w", ref, err)
}

func (b *builder) layer(p *Plan) error {
	if b.res.LayerID != "" {
		return nil
	}
	graphs, err := b.systemForm("Graphs")
	if err != nil {
		return err
	}
	layers, err := b.systemForm("Layers")
	if err != nil {
		return err
	}
	body := map[string]any{"title": p.Layer, "data": map[string]any{}}
	graphID, _, err := b.ensureActor(graphs, p.RefPrefix+"graph", body)
	if err != nil {
		return err
	}
	layerID, created, err := b.ensureActor(layers, p.RefPrefix+"layer", body)
	if err != nil {
		return err
	}
	b.res.GraphID, b.res.LayerID = graphID, layerID
	if !created {
		return nil
	}
	edgeType, err := b.hierarchyEdgeType()
	if err != nil {
		return err
	}
	link := map[string]any{"source": graphID, "target": layerID, "edgeTypeId": edgeType}
	if err := b.send(http.MethodPost, "/actors/link/"+b.ws, link, nil); err != nil {
		return fmt.Errorf("link the graph to its layer: %w", err)
	}
	return nil
}

func (b *builder) actors(p *Plan) error {
	for i := range p.Actors {
		a := &p.Actors[i]
		formID := b.res.Forms[a.Form]
		data := map[string]any{}
		for k, v := range a.Data {
			data[k] = v
		}
		body := map[string]any{"title": a.Title, "description": a.Description, "color": a.Color, "data": data}
		if a.GeoName != "" {
			body["geoName"] = a.GeoName
		}

		var have struct {
			ID      string         `json:"id"`
			Picture string         `json:"picture"`
			Data    map[string]any `json:"data"`
		}
		err := b.get(fmt.Sprintf("/actors/ref/%d/%s?filter=id,picture,data", formID, ecore.Seg(a.Ref)), &have)
		if err != nil && !notFound(err) {
			return fmt.Errorf("look up actor %s: %w", a.Ref, err)
		}
		exists := err == nil && have.ID != ""

		// the picture is uploaded once: the actor remembers which one it has
		key := b.pictureKey(a)
		current, _ := have.Data[fieldPrefix+"picture"].(string)
		switch {
		case key == "":
		case exists && have.Picture != "" && current == key:
			data[fieldPrefix+"picture"] = key
			b.res.Pictures.Existing++
		default:
			if name, err := b.picture(a, key); err != nil {
				b.res.Pictures.Skipped++
				b.res.Notes = append(b.res.Notes, fmt.Sprintf("picture of %s was not set: %v", a.CaseID, err))
			} else {
				body["picture"] = name
				data[fieldPrefix+"picture"] = key
				b.res.Pictures.Created++
			}
		}

		if exists {
			if err := b.send(http.MethodPut, fmt.Sprintf("/actors/actor/%d/%s", formID, ecore.Seg(have.ID)), body, nil); err != nil {
				return fmt.Errorf("update actor %s: %w", a.Ref, err)
			}
			a.actorID = have.ID
			b.res.Actors.Existing++
			continue
		}
		body["ref"] = a.Ref
		var made actorReply
		if err := b.send(http.MethodPost, fmt.Sprintf("/actors/actor/%d", formID), body, &made); err != nil {
			return fmt.Errorf("create actor %s: %w", a.Ref, err)
		}
		if made.ID == "" {
			return fmt.Errorf("create actor %s: the reply has no id", a.Ref)
		}
		a.actorID = made.ID
		b.res.Actors.Created++
	}
	return nil
}

// Picture modes of a build.
const (
	PicturesAuto  = "auto"  // the case's pictures, a generated placeholder for the rest
	PicturesGiven = "given" // only the pictures the case gives
	PicturesNone  = "none"  // leave pictures alone
)

// pictureKey names the picture an actor should have, or "" for none.
func (b *builder) pictureKey(a *PlanActor) string {
	switch {
	case b.pictures == PicturesNone:
		return ""
	case a.Picture != "":
		return "src:" + a.Picture
	case b.pictures == PicturesGiven:
		return ""
	}
	return avatarKey(a.EntityType, a.CaseID)
}

// picture uploads the picture named by key and returns its storage name.
func (b *builder) picture(a *PlanActor, key string) (string, error) {
	if name, ok := b.uploaded[key]; ok {
		return name, nil
	}
	var data []byte
	filename := a.CaseID + ".svg"
	switch {
	case a.Picture == "":
		data = avatarSVG(a.EntityType, a.CaseID)
	case strings.HasPrefix(a.Picture, "https://"):
		var contentType string
		var err error
		if data, contentType, err = graph.FetchImage(b.ctx, a.Picture); err != nil {
			return "", err
		}
		filename = a.CaseID + pictureExt(a.Picture, contentType)
	default:
		path, err := localPath(a.Picture)
		if err != nil {
			return "", err
		}
		if data, err = graph.ReadLocalImage(path); err != nil {
			return "", err
		}
		filename = a.CaseID + strings.ToLower(filepath.Ext(a.Picture))
	}
	name, err := graph.UploadImage(b.ctx, b.ws, filename, data, 256)
	if err != nil {
		return "", err
	}
	b.uploaded[key] = name
	return name, nil
}

// pictureExt picks a file extension for a downloaded picture.
func pictureExt(rawURL, contentType string) string {
	if u, err := url.Parse(rawURL); err == nil {
		if ext := strings.ToLower(filepath.Ext(u.Path)); slices.Contains(pictureExts, ext) {
			return ext
		}
	}
	switch {
	case strings.Contains(contentType, "svg"):
		return ".svg"
	case strings.Contains(contentType, "jpeg"):
		return ".jpg"
	case strings.Contains(contentType, "gif"):
		return ".gif"
	case strings.Contains(contentType, "webp"):
		return ".webp"
	}
	return ".png"
}

// ---- layer --------------------------------------------------------------------------

// layerPage reads one kind of layer element (nodes or edges) page by page.
func layerPage[T any](b *builder, kind string) ([]T, error) {
	var all []T
	for offset := 0; ; offset += layerPageSize {
		var page []T
		path := fmt.Sprintf("/graph_layers/paginated/%s?type=%s&limit=%d&offset=%d", ecore.Seg(b.res.LayerID), kind, layerPageSize, offset)
		if err := b.get(path, &page); err != nil {
			return nil, fmt.Errorf("read layer %s: %w", kind, err)
		}
		// a short page is not the last: the server drops deleted elements after LIMIT
		if len(page) == 0 {
			return all, nil
		}
		all = append(all, page...)
	}
}

type layerItem struct {
	Action string         `json:"action"`
	Data   map[string]any `json:"data"`
}

func (b *builder) manageLayer(items []layerItem, out any) error {
	return b.send(http.MethodPost, "/graph_layers/actors/"+ecore.Seg(b.res.LayerID), items, out)
}

func (b *builder) placeNodes(p *Plan) error {
	placed, err := layerPage[struct {
		ID       string `json:"id"`
		LaID     int    `json:"laId"`
		Position struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
		} `json:"position"`
	}](b, "nodes")
	if err != nil {
		return err
	}
	// The layer is a grid: two nodes cannot share a cell. A node already on the
	// layer may sit anywhere (the user moves them), so a new node takes its
	// planned cell only when it is free, and the next free one below otherwise.
	type cell struct{ x, y int }
	at := func(x, y float64) cell { return cell{int(math.Round(x / layerCell)), int(math.Round(y / layerCell))} }
	taken := map[cell]bool{}
	laID := map[string]int{}
	for _, n := range placed {
		laID[n.ID] = n.LaID
		taken[at(n.Position.X, n.Position.Y)] = true
	}
	var items []layerItem
	for i := range p.Actors {
		a := &p.Actors[i]
		if id, ok := laID[a.actorID]; ok {
			a.laID = id
			b.res.Nodes.Existing++
			continue
		}
		x, y := a.X, a.Y
		for taken[at(float64(x), float64(y))] {
			y += planRowStep
		}
		taken[at(float64(x), float64(y))] = true
		items = append(items, layerItem{"create", map[string]any{"id": a.actorID, "type": "node",
			"position": map[string]int{"x": x, "y": y}}})
	}
	for start := 0; start < len(items); start += layerPageSize {
		batch := items[start:min(start+layerPageSize, len(items))]
		var reply struct {
			NodesMap []struct {
				LaID    int    `json:"laId"`
				ActorID string `json:"actorId"`
			} `json:"nodesMap"`
		}
		if err := b.manageLayer(batch, &reply); err != nil {
			return fmt.Errorf("place actors on the layer: %w", err)
		}
		for _, n := range reply.NodesMap {
			laID[n.ActorID] = n.LaID
		}
		b.res.Nodes.Created += len(batch)
	}
	for i := range p.Actors {
		p.Actors[i].laID = laID[p.Actors[i].actorID]
	}
	return nil
}

// links creates one edge per ordered pair of entities. Simulator keeps one
// edge of a type between two actors, so several relations between the same
// two (owner and director) become one edge named after all of them.
func (b *builder) links(p *Plan) error {
	if len(p.Links) == 0 {
		return nil
	}
	actor := map[string]*PlanActor{}
	for i := range p.Actors {
		actor[p.Actors[i].Ref] = &p.Actors[i]
	}
	type pair struct{ source, target string }
	names := map[pair][]string{}
	var order []pair
	for _, l := range p.Links {
		src, dst := actor[l.SourceRef], actor[l.TargetRef]
		if src == nil || dst == nil {
			continue
		}
		k := pair{src.actorID, dst.actorID}
		if names[k] == nil {
			order = append(order, k)
		}
		names[k] = append(names[k], l.Name)
	}
	edgeType, err := b.hierarchyEdgeType()
	if err != nil {
		return err
	}
	onLayer, err := layerPage[struct {
		Source string `json:"source"`
		Target string `json:"target"`
	}](b, "edges")
	if err != nil {
		return err
	}
	drawn := map[pair]bool{}
	for _, e := range onLayer {
		drawn[pair{e.Source, e.Target}] = true
	}
	laID := map[string]int{}
	for _, a := range p.Actors {
		laID[a.actorID] = a.laID
	}

	for start := 0; start < len(order); start += linkBatchSize {
		batch := order[start:min(start+linkBatchSize, len(order))]
		body := make([]map[string]any, len(batch))
		for i, k := range batch {
			body[i] = map[string]any{"source": k.source, "target": k.target, "edgeTypeId": edgeType,
				"name": strings.Join(uniq(names[k]), " · ")}
		}
		var reply []struct {
			Error bool `json:"error"`
			Data  struct {
				ID     string `json:"id"`
				Source string `json:"source"`
				Target string `json:"target"`
			} `json:"data"`
		}
		if err := b.send(http.MethodPost, "/actors/mass_links/"+b.ws+"?forceDirection=true", body, &reply); err != nil {
			return fmt.Errorf("create links: %w", err)
		}
		var items []layerItem
		for i, k := range batch {
			if i >= len(reply) || reply[i].Error || reply[i].Data.ID == "" {
				b.res.Links.Skipped++
				b.res.Notes = append(b.res.Notes, fmt.Sprintf("link %q was refused by Simulator and is not on the graph", strings.Join(names[k], " · ")))
				continue
			}
			// the edge may come back in the other direction when Simulator already
			// holds one between the two actors
			got := pair{reply[i].Data.Source, reply[i].Data.Target}
			if got.source == "" {
				got = k
			}
			if drawn[got] {
				b.res.Links.Existing++
				b.res.Edges.Existing++
				continue
			}
			b.res.Links.Created++
			if laID[got.source] == 0 || laID[got.target] == 0 {
				continue
			}
			drawn[got] = true
			items = append(items, layerItem{"create", map[string]any{"id": reply[i].Data.ID, "type": "edge",
				"laIdSource": laID[got.source], "laIdTarget": laID[got.target]}})
		}
		if len(items) > 0 {
			if err := b.manageLayer(items, nil); err != nil {
				return fmt.Errorf("place links on the layer: %w", err)
			}
			b.res.Edges.Created += len(items)
		}
	}
	return nil
}

// ---- accounts and transfers ------------------------------------------------------------

// sides are the two Simulator accounts behind one case account: money leaving
// is recorded on the debit side, money arriving on the credit side, and the
// balance is credit minus debit.
type sides struct{ debit, credit string }

func (b *builder) accounts(p *Plan) error {
	if len(p.Accounts) == 0 {
		return nil
	}
	type pairIDs struct {
		name     string
		currency int
	}
	pairs := map[PlanPair]pairIDs{}
	for _, pr := range p.AccountPairs {
		var reply struct {
			AccountName struct {
				ID string `json:"id"`
			} `json:"accountName"`
			Currency struct {
				ID int `json:"id"`
			} `json:"currency"`
		}
		body := map[string]any{"accountName": pr.AccountName, "currencyName": pr.CurrencyName}
		if err := b.send(http.MethodPost, "/accounts/pair/"+b.ws, body, &reply); err != nil {
			return fmt.Errorf("account pair %s / %s: %w", pr.AccountName, pr.CurrencyName, err)
		}
		if reply.AccountName.ID == "" || reply.Currency.ID == 0 {
			return fmt.Errorf("account pair %s / %s: the reply has no ids", pr.AccountName, pr.CurrencyName)
		}
		pairs[pr] = pairIDs{reply.AccountName.ID, reply.Currency.ID}
	}
	actor := map[string]string{}
	for _, a := range p.Actors {
		actor[a.Ref] = a.actorID
	}
	for i := range p.Accounts {
		a := &p.Accounts[i]
		ids := pairs[PlanPair{a.AccountName, a.CurrencyName}]
		actorID := actor[a.ActorRef]
		body := map[string]any{"nameId": ids.name, "currencyId": ids.currency}
		if err := b.send(http.MethodPost, "/accounts/"+ecore.Seg(actorID)+"?ignoreIfExist=true", body, nil); err != nil {
			return fmt.Errorf("account %s: %w", a.CaseID, err)
		}
		var rows []struct {
			ID         string `json:"id"`
			NameID     string `json:"nameId"`
			CurrencyID int    `json:"currencyId"`
			IncomeType string `json:"incomeType"`
			Type       string `json:"type"`
		}
		q := url.Values{"limit": {"100"}, "query": {a.AccountName}}
		if err := b.get("/accounts/"+ecore.Seg(actorID)+"?"+q.Encode(), &rows); err != nil {
			return fmt.Errorf("read account %s: %w", a.CaseID, err)
		}
		for _, r := range rows {
			if r.NameID != ids.name || r.CurrencyID != ids.currency || (r.Type != "" && r.Type != "fact") {
				continue
			}
			switch r.IncomeType {
			case "debit":
				a.sides.debit = r.ID
			case "credit":
				a.sides.credit = r.ID
			}
		}
		if a.sides.debit == "" || a.sides.credit == "" {
			return fmt.Errorf("account %s: Simulator did not return its debit and credit sides", a.CaseID)
		}
		b.res.Accounts++
	}
	return nil
}

func (b *builder) transfers(p *Plan) error {
	account := map[string]*PlanAccount{}
	for i := range p.Accounts {
		account[p.Accounts[i].CaseID] = &p.Accounts[i]
	}
	for _, t := range p.Transfers {
		from, to := account[t.From], account[t.To]
		if from == nil || to == nil {
			continue
		}
		if from.CurrencyName != to.CurrencyName {
			b.res.Transfers.Skipped++
			b.res.Notes = append(b.res.Notes, fmt.Sprintf("transfer %s is between %s and %s accounts and was not recorded", t.Ref, from.CurrencyName, to.CurrencyName))
			continue
		}
		err := b.get("/transfers/"+b.ws+"/"+ecore.Seg(t.Ref), nil)
		if err == nil {
			b.res.Transfers.Existing++
			continue
		}
		if !notFound(err) {
			return fmt.Errorf("look up transfer %s: %w", t.Ref, err)
		}
		amount := json.Number(t.Amount)
		body := map[string]any{
			"from":    []map[string]any{{"accountId": from.sides.debit, "amount": amount}},
			"to":      []map[string]any{{"accountId": to.sides.credit, "amount": amount}},
			"ref":     t.Ref,
			"comment": t.Comment,
		}
		if err := b.send(http.MethodPost, "/transfers/"+b.ws, body, nil); err != nil {
			return fmt.Errorf("transfer %s: %w", t.Ref, err)
		}
		b.res.Transfers.Created++
	}
	return nil
}
