package sim

import (
	"fmt"
	"sort"
	"strings"
)

// Static model check before any run: finds what would fail in the middle of a run or
// silently do nothing (same checks as sim-morrow's `check`).

type CheckReport struct {
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

func (r *CheckReport) errf(where, format string, a ...any) {
	r.Errors = append(r.Errors, where+": "+fmt.Sprintf(format, a...))
}

func (r *CheckReport) warnf(where, format string, a ...any) {
	r.Warnings = append(r.Warnings, where+": "+fmt.Sprintf(format, a...))
}

func (r *CheckReport) OK() bool { return len(r.Errors) == 0 }

var topKeys = map[string]bool{"value_types": true, "refs": true, "params": true, "horizon": true, "initial_events": true,
	"behaviors": true, "metrics": true, "type_field": true, "actors": true, "goals": true}

var required = map[string][]string{
	"set": {}, "set_on": {"actor", "fields"}, "create": {"type"}, "link": {"from", "to"},
	"transfer": {"from", "to", "amount"}, "add": {"account", "amount"}, "set_account": {"account", "value"},
	"schedule": {"event"}, "enqueue": {"resource", "event"}, "release": {}, "dequeue": {"resource"},
	"decide": {"options"}, "log": {},
}

var exprFields = map[string]bool{"if": true, "rule": true, "amount": true, "after": true, "at": true, "target": true,
	"actor": true, "value": true, "resource": true}

var baseNames = map[string]bool{"self": true, "event": true, "params": true, "refs": true, "now": true}

var knownFuncs = map[string]bool{"actor": true, "actors": true, "count": true, "total": true, "dur": true, "dec": true,
	"min": true, "max": true, "abs": true, "len": true, "round": true, "str": true, "rand": true, "pick": true,
	"where": true, "chance": true}

type checker struct {
	raw          *OMap
	graph        *Graph
	scenarios    []*OMap
	r            *CheckReport
	params       map[string]bool
	optional     map[string]bool
	refs         map[string]bool
	vtypes       *OMap
	handled      map[string][]string
	emitted      map[string][]string
	createdTypes map[string]bool
}

// walk collects names, calls, params.X, params.get('X'), refs.X from an expression tree.
type exprUse struct {
	names, calls, params, optional, refs map[string]bool
}

func collect(n node, u *exprUse) {
	switch x := n.(type) {
	case nName:
		u.names[x.id] = true
	case nAttr:
		if nm, ok := x.obj.(nName); ok {
			switch nm.id {
			case "params":
				u.params[x.attr] = true
			case "refs":
				u.refs[x.attr] = true
			}
		}
		collect(x.obj, u)
	case nIndex:
		collect(x.obj, u)
		collect(x.key, u)
	case nCall:
		switch f := x.fn.(type) {
		case nName:
			u.calls[f.id] = true
		case nAttr:
			if nm, ok := f.obj.(nName); ok && nm.id == "params" && f.attr == "get" {
				if len(x.args) > 0 {
					if c, ok := x.args[0].(nConst); ok {
						u.optional[show(c.v)] = true
					}
				}
				for _, a := range x.args[1:] {
					collect(a, u)
				}
				return
			}
			collect(f.obj, u)
		}
		for _, a := range x.args {
			collect(a, u)
		}
		for _, v := range x.kwVals {
			collect(v, u)
		}
	case nBin:
		collect(x.l, u)
		collect(x.r, u)
	case nUnary:
		collect(x.x, u)
	case nBool:
		for _, it := range x.items {
			collect(it, u)
		}
	case nCmp:
		collect(x.first, u)
		for _, it := range x.rest {
			collect(it, u)
		}
	case nIf:
		collect(x.test, u)
		collect(x.body, u)
		collect(x.orelse, u)
	case nList:
		for _, it := range x.items {
			collect(it, u)
		}
	case nDict:
		for i := range x.keys {
			collect(x.keys[i], u)
			collect(x.vals[i], u)
		}
	}
}

func newUse() *exprUse {
	return &exprUse{map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (c *checker) expr(where string, src any, local map[string]bool) {
	s, ok := src.(string)
	if !ok {
		return
	}
	n, err := compileExpr(s)
	if err != nil {
		c.r.errf(where, "%v", err)
		return
	}
	u := newUse()
	collect(n, u)
	for _, name := range sortedKeys(u.names) {
		if !baseNames[name] && !local[name] && !knownFuncs[name] {
			c.r.errf(where, "unknown name %q in %q", name, s)
		}
	}
	for _, f := range sortedKeys(u.calls) {
		if !knownFuncs[f] {
			c.r.errf(where, "unknown function %q in %q", f, s)
		}
	}
	for _, p := range sortedKeys(u.params) {
		if p != "get" && !c.params[p] {
			c.r.errf(where, "params.%s is not defined in params or any scenario", p)
		}
	}
	for _, rf := range sortedKeys(u.refs) {
		if !c.refs[rf] {
			c.r.errf(where, "refs.%s is not defined in refs", rf)
		}
	}
	for k := range u.optional {
		c.optional[k] = true
	}
}

func (c *checker) value(where string, spec any, local map[string]bool) {
	switch x := spec.(type) {
	case string:
		if strings.HasPrefix(x, "=") {
			c.expr(where, x[1:], local)
		}
	case *OMap:
		for _, k := range x.Keys() {
			c.value(where+"."+k, x.m[k], local)
		}
	case []any:
		for i, v := range x {
			c.value(fmt.Sprintf("%s[%d]", where, i), v, local)
		}
	}
}

func (c *checker) durationOrExpr(where string, raw any, local map[string]bool) {
	if s, ok := raw.(string); ok {
		if _, err := parseDuration(s); err != nil {
			c.expr(where, s, local)
		}
	}
}

func (c *checker) actions(where string, actions any, local map[string]bool, actorType string) {
	list, ok := actions.([]any)
	if !ok {
		if actions != nil {
			c.r.errf(where, "must be a list of actions")
		}
		return
	}
	for i, it := range list {
		w := fmt.Sprintf("%s[%d]", where, i)
		a, ok := it.(*OMap)
		if !ok || a.Len() == 0 {
			c.r.errf(w, "bad action %s", show(it))
			continue
		}
		if a.Has("if") {
			c.expr(w+".if", a.m["if"], local)
			for _, k := range a.Keys() {
				if k != "if" && k != "then" && k != "else" {
					c.r.errf(w, "unexpected key %q next to if", k)
				}
			}
			then, _ := a.Get("then")
			c.actions(w+".then", then, local, actorType)
			els, _ := a.Get("else")
			c.actions(w+".else", els, local, actorType)
			continue
		}
		if a.Len() > 1 {
			c.r.errf(w, "one action per list item, got %v", a.Keys())
		}
		name := a.keys[0]
		if _, ok := required[name]; !ok {
			known := make([]string, 0, len(required))
			for k := range required {
				known = append(known, k)
			}
			sort.Strings(known)
			c.r.errf(w, "unknown action %q; known: %s", name, strings.Join(known, ", "))
			continue
		}
		c.action(w+"."+name, name, a.m[name], local)
	}
}

func (c *checker) action(w, name string, raw any, local map[string]bool) {
	if name == "log" {
		c.value(w, raw, local)
		return
	}
	spec, ok := raw.(*OMap)
	if !ok {
		c.r.errf(w, "expects a mapping")
		return
	}
	for _, req := range required[name] {
		if !spec.Has(req) {
			c.r.errf(w, "missing field %q", req)
		}
	}
	if name == "set" {
		for _, k := range spec.Keys() {
			c.value(w+"."+k, spec.m[k], local)
		}
		return
	}
	for _, k := range spec.Keys() {
		v := spec.m[k]
		switch {
		case (k == "from" || k == "to") && name == "transfer":
			m, ok := v.(*OMap)
			if !ok || !m.Has("account") {
				c.r.errf(w+"."+k, "needs {actor, account}")
			} else {
				actor, has := m.Get("actor")
				if !has {
					actor = "self"
				}
				c.expr(w+"."+k+".actor", actor, local)
			}
		case k == "after" || k == "at":
			c.durationOrExpr(w+"."+k, v, local)
		case exprFields[k] || (name == "link" && (k == "from" || k == "to")) || k == "link_from":
			c.expr(w+"."+k, v, local)
		case k == "data" || k == "payload" || k == "fields" || k == "title":
			c.value(w+"."+k, v, local)
		}
	}
	switch name {
	case "create":
		c.createdTypes[getString(spec, "type")] = true
		if as := getString(spec, "as"); as != "" {
			local[as] = true
		}
	case "decide":
		v := getString(spec, "var")
		if v == "" {
			v = "choice"
		}
		local[v] = true
		if !spec.Has("rule") && !spec.Has("jev") {
			c.r.warnf(w, "no rule and no jev block: the choice will be uniformly random")
		}
		if spec.Has("jev") && !spec.Has("rule") {
			c.r.warnf(w, "jev without a rule: if Jev is unavailable the run stops (MODEL_UNAVAILABLE)")
		}
	case "schedule", "enqueue":
		ev := getString(spec, "event")
		c.emitted[ev] = append(c.emitted[ev], w)
	case "add":
		vt := getString(spec, "value_type")
		if vt != "" && !c.vtypes.Has(vt) {
			c.r.warnf(w, "value_type %q is not declared in value_types (decimal, 2 places is used)", vt)
		}
		if vt != "" && c.conserved(vt) {
			c.r.errf(w, "%q is conserved: use transfer, not add", vt)
		}
		if c.graph != nil {
			acc := getString(spec, "account")
			types := map[string]bool{}
			for _, a := range c.graph.accounts {
				if a.Name == acc {
					types[a.ValueType] = true
				}
			}
			all := len(types) > 0
			for t := range types {
				if !c.conserved(t) {
					all = false
				}
			}
			if all {
				c.r.errf(w, "account %q holds a conserved type (%s): use transfer, not add", acc, strings.Join(sortedKeys(types), ", "))
			}
		}
	}
}

func (c *checker) conserved(vt string) bool {
	spec := getMap(c.vtypes, vt)
	v, _ := spec.Get("conserved")
	return truthy(v)
}

func (c *checker) actorType(a *Actor) string {
	tf := getString(c.raw, "type_field")
	if tf != "" {
		if v, ok := a.Data.Get(tf); ok {
			return show(v)
		}
	}
	return a.Type
}

func (c *checker) resolve(where string, spec any, model *Model) {
	if c.graph == nil {
		return
	}
	g := c.graph
	if model != nil && len(model.Actors) > 0 {
		g = g.clone()
		params := &Namespace{model.Params, "params"}
		resolveFn := func(v any) (any, error) {
			if s, ok := v.(string); ok && strings.HasPrefix(s, "=") {
				return evaluate(s[1:], map[string]any{"params": params}, map[string]Func{})
			}
			return v, nil
		}
		for name, vt := range model.ValueTypes {
			g.valueTypes[name] = vt
		}
		if err := addModelActors(g, model, resolveFn); err != nil {
			c.r.errf(where, "%v", err)
			return
		}
	}
	if _, err := resolveActor(g, spec); err != nil {
		c.r.errf(where, "%v", err)
	}
}

// stepLimit is the engine's default MaxSteps.
const stepLimit = 1_000_000

// staticDuration is a duration known before the run (not an =expression over params).
func staticDuration(v any) (int64, bool) {
	if s, ok := v.(string); ok && strings.HasPrefix(s, "=") {
		return 0, false
	}
	d, err := parseDuration(v)
	return d, err == nil
}

// occurrences warns when a recurring event over the horizon, times its targets, exceeds
// the step limit.
func (c *checker) occurrences(w string, e *OMap, model *Model) {
	ev, _ := e.Get("every")
	every, ok := staticDuration(ev)
	if ev == nil || !ok || every <= 0 {
		return
	}
	atRaw, has := e.Get("at")
	if !has {
		atRaw = ratZero
	}
	at, ok := staticDuration(atRaw)
	if !ok {
		return
	}
	horizon := model.Horizon
	for _, sc := range c.scenarios {
		if h, ok := sc.Get("horizon"); ok && h != nil {
			if d, ok := staticDuration(h); ok && d > horizon {
				horizon = d
			}
		}
	}
	targets := int64(1)
	if ft, ok := e.Get("for_type"); ok && c.graph != nil {
		targets = 0
		for _, a := range c.graph.actors {
			if c.actorType(a) == show(ft) {
				targets++
			}
		}
	}
	if horizon < at {
		return
	}
	if n := ((horizon-at)/every + 1) * targets; n > stepLimit {
		c.r.warnf(w, "%s occurrences (horizon / every x %d target(s)) exceed the step limit %s: the run will stop "+
			"early; use a longer `every` or a shorter horizon", thousands(n), targets, thousands(stepLimit))
	}
}

func thousands(n int64) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// Check validates a model, optionally against a graph and scenarios.
func Check(modelSrc []byte, graph *Graph, scenarios []*OMap) *CheckReport {
	r := &CheckReport{Errors: []string{}, Warnings: []string{}}
	raw, err := parseYAML(modelSrc)
	if err != nil {
		r.errf("model", "cannot parse: %v", err)
		return r
	}
	d, _ := raw.(*OMap)
	if d == nil {
		d = NewOMap()
	}
	c := &checker{raw: d, graph: graph, scenarios: scenarios, r: r, params: map[string]bool{}, optional: map[string]bool{},
		refs: map[string]bool{}, handled: map[string][]string{}, emitted: map[string][]string{}, createdTypes: map[string]bool{}}
	for _, k := range getMap(d, "params").Keys() {
		c.params[k] = true
	}
	for _, sc := range scenarios {
		for _, k := range getMap(sc, "params").Keys() {
			c.params[k] = true
		}
	}
	for _, k := range getMap(d, "refs").Keys() {
		c.refs[k] = true
	}
	c.vtypes = getMap(d, "value_types")
	if c.vtypes == nil {
		c.vtypes = NewOMap()
	}
	for _, k := range d.Keys() {
		if !topKeys[k] {
			r.warnf("model", "unknown top-level key %q (ignored)", k)
		}
	}
	model, err := LoadModel(modelSrc)
	if err != nil {
		r.errf("model", "cannot load: %v", err)
		return r
	}
	if model.Horizon == 0 {
		r.warnf("horizon", "horizon is 0: only events at time 0 run")
	}
	if textNumberSnapshot(graph) {
		r.warnf("graph", "this snapshot was written by v2.9.0, which saved the numbers in actor data as text: "+
			"sums concatenate and numeric filters match nothing. Re-take it with simulationSnapshot (overwrite: true)")
	}
	behaviors := getMap(d, "behaviors")
	for _, t := range behaviors.Keys() {
		events, ok := behaviors.m[t].(*OMap)
		if !ok {
			r.errf("behaviors."+t, "must map event names to action lists")
			continue
		}
		for _, ev := range events.Keys() {
			c.handled[ev] = append(c.handled[ev], t)
			c.actions("behaviors."+t+"."+ev, events.m[ev], map[string]bool{}, t)
		}
	}
	for i, e := range model.InitialEvents {
		w := fmt.Sprintf("initial_events[%d]", i)
		if !e.Has("event") {
			r.errf(w, "missing field 'event'")
			continue
		}
		ev := getString(e, "event")
		c.emitted[ev] = append(c.emitted[ev], w)
		if !e.Has("for_type") && !e.Has("target") {
			r.errf(w, "needs for_type or target")
		}
		for _, k := range []string{"at", "every"} {
			if s, ok := e.m[k].(string); ok && strings.HasPrefix(s, "=") {
				c.expr(w+"."+k, s[1:], map[string]bool{})
			}
		}
		if t, ok := e.Get("target"); ok && !c.refs[show(t)] {
			c.resolve(w, t, model)
		}
		if ft, ok := e.Get("for_type"); ok && c.graph != nil {
			found := false
			for _, a := range c.graph.actors {
				if c.actorType(a) == show(ft) {
					found = true
					break
				}
			}
			if !found {
				r.warnf(w, "no actor of type %q in the graph: this event never fires", show(ft))
			}
		}
		c.occurrences(w, e, model)
	}
	for _, ev := range sortedKeys(keysOf(c.emitted)) {
		if len(c.handled[ev]) == 0 {
			r.warnf(c.emitted[ev][0], "event %q is emitted but no behavior handles it", ev)
		}
	}
	for _, ev := range sortedKeys(keysOf(c.handled)) {
		if len(c.emitted[ev]) == 0 {
			types := append([]string(nil), c.handled[ev]...)
			sort.Strings(types)
			r.warnf("behaviors.*."+ev, "handler for %q (%s) is never triggered", ev, strings.Join(types, ", "))
		}
	}
	if c.graph != nil {
		present := map[string]bool{}
		for _, a := range c.graph.actors {
			present[c.actorType(a)] = true
		}
		for t := range c.createdTypes {
			present[t] = true
		}
		for _, a := range model.Actors {
			t := getString(a, "type")
			if t == "" {
				t = "helper"
			}
			present[t] = true
		}
		for _, t := range behaviors.Keys() {
			if !present[t] {
				r.warnf("behaviors."+t, "no actor of type %q in the graph and none is created", t)
			}
		}
		refs := getMap(d, "refs")
		for _, name := range refs.Keys() {
			c.resolve("refs."+name, refs.m[name], model)
		}
	}
	metrics := getMap(d, "metrics")
	for _, name := range metrics.Keys() {
		c.expr("metrics."+name, metrics.m[name], map[string]bool{})
	}
	goals := getMap(d, "goals")
	for _, name := range goals.Keys() {
		src, _ := goals.m[name].(string)
		n, err := compileExpr(src)
		if err != nil {
			r.errf("goals."+name, "%v", err)
			continue
		}
		u := newUse()
		collect(n, u)
		for _, nm := range sortedKeys(u.names) {
			if !metrics.Has(nm) && !baseNames[nm] {
				r.errf("goals."+name, "%q is not a metric", nm)
			}
		}
	}
	declared := getMap(d, "params")
	for i, sc := range scenarios {
		w := strings.TrimSpace(fmt.Sprintf("scenarios[%d] %s", i, getString(sc, "name")))
		for _, p := range getMap(sc, "params").Keys() {
			if !declared.Has(p) && !c.optional[p] {
				r.warnf(w, "param %q is not in the model's params (typo?)", p)
			}
		}
		for _, it := range getList(sc, "set") {
			item, _ := it.(*OMap)
			ref, ok := item.Get("actor")
			if !ok {
				r.errf(w, "set item needs actor")
				continue
			}
			if !c.refs[show(ref)] {
				c.resolve(w, ref, model)
			}
		}
	}
	return r
}

func keysOf(m map[string][]string) map[string]bool {
	out := map[string]bool{}
	for k := range m {
		out[k] = true
	}
	return out
}

// textNumberSnapshot reports a simulationSnapshot file written by v2.9.0. That version wrote
// every number as a YAML string, sim.source.taken_at included; later ones write a number.
func textNumberSnapshot(g *Graph) bool {
	if g == nil || getString(g.Source, "kind") != "simulator" {
		return false
	}
	t, _ := g.Source.Get("taken_at")
	_, isText := t.(string)
	return isText
}
