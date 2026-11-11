package osint

import (
	"bytes"
	"embed"
	"fmt"
	"math/big"
	"regexp"
	"slices"
	"strings"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/sim"
	"gopkg.in/yaml.v3"
)

// A pack says what an analysis looks for: the thresholds of the built-in
// patterns, which of them to report and how severe each is, and the user's
// own rules — conditions written as data, in the plugin's expression
// language. One engine then serves any field: a pack for investigations,
// another for checking a counterparty, another for a market map.

// Pack is a parsed pack file.
type Pack struct {
	Meta     PackMeta          `yaml:"pack"`
	Settings PackSettings      `yaml:"settings,omitempty"`
	Builtin  map[string]string `yaml:"builtin,omitempty"`
	Rules    []Rule            `yaml:"rules,omitempty"`
}

// PackMeta names a pack.
type PackMeta struct {
	ID    string `yaml:"id"`
	Title string `yaml:"title,omitempty"`
	Note  string `yaml:"note,omitempty"`
}

// PackSettings are the thresholds of the built-in patterns. A value given to
// the analysis directly (threshold, windowDays, minShare) wins over the pack.
type PackSettings struct {
	Threshold  *Decimal `yaml:"threshold,omitempty"`
	WindowDays int      `yaml:"window_days,omitempty"`
	MinShare   *Decimal `yaml:"min_share,omitempty"`
}

// Rule is one condition of the user's. It is tried on every item of the kind
// named by For, and each item it holds for becomes a finding.
type Rule struct {
	ID       string `yaml:"id"`
	For      string `yaml:"for"`
	When     string `yaml:"when"`
	Severity string `yaml:"severity,omitempty"`
	Title    string `yaml:"title"`
	Detail   string `yaml:"detail,omitempty"`
}

// What a rule can be tried on.
const (
	ForEntity      = "entity"
	ForRelation    = "relation"
	ForAccount     = "account"
	ForTransaction = "transaction"
)

const (
	maxPackBytes    = 1 << 20
	maxPackRules    = 100
	maxRuleFindings = 50        // findings one rule may raise; the rest are counted
	maxRuleEvals    = 2_000_000 // conditions evaluated in one analysis
)

var (
	ruleFor        = []string{ForEntity, ForRelation, ForAccount, ForTransaction}
	severities     = []string{"high", "medium", "low"}
	builtinKinds   = []string{"circular_ownership", "conflict_of_interest", "fan_out", "funnel", "indirect_beneficial_owner", "layering_chain", "multiple_directorships", "pass_through", "related_party_payments", "round_trip", "shared_anchor", "shared_attribute", "structuring", "unknown_owner"}
	builtinSetting = append([]string{"off"}, severities...)
	fieldRe        = regexp.MustCompile(`\{([a-z_][a-z0-9_.]*)\}`)
)

//go:embed packs/*.yaml
var shippedPacks embed.FS

// ShippedPacks lists the packs that come with the plugin.
func ShippedPacks() []string {
	entries, _ := shippedPacks.ReadDir("packs")
	var out []string
	for _, e := range entries {
		out = append(out, strings.TrimSuffix(e.Name(), ".yaml"))
	}
	return out
}

// ShippedPack loads a pack that comes with the plugin, by its id.
func ShippedPack(id string) (*Pack, error) {
	if !idRe.MatchString(id) {
		return nil, fmt.Errorf("no pack %q", id)
	}
	src, err := shippedPacks.ReadFile("packs/" + id + ".yaml")
	if err != nil {
		return nil, fmt.Errorf("no pack %q: the plugin ships %s", id, strings.Join(ShippedPacks(), ", "))
	}
	return LoadPack(src)
}

var unknownNameRe = regexp.MustCompile(`^(unknown name|unknown function|no field|no attribute|no method)`)

// ruleFuncs are the functions a rule may call beside the language's own.
var ruleFuncs = map[string]sim.Func{
	// str(x): a number or a text as text
	"str": func(args []any, _ *sim.OMap) (any, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("str takes one value")
		}
		switch x := args[0].(type) {
		case *big.Rat:
			return ratString(x), nil
		case string:
			return x, nil
		}
		return fmt.Sprint(args[0]), nil
	},
	// weak(grade): the evidence is unsourced or its best source is graded D, E or F
	"weak": func(args []any, _ *sim.OMap) (any, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("weak takes a grade")
		}
		g, _ := args[0].(string)
		return g == "unsourced" || (len(g) > 0 && g[0] >= 'D' && g[0] <= 'F'), nil
	},
	// year(date): the year of a date written YYYY-MM-DD, 0 when there is none
	"year": func(args []any, _ *sim.OMap) (any, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("year takes a date")
		}
		s, _ := args[0].(string)
		if len(s) >= 4 {
			if r, ok := new(big.Rat).SetString(s[:4]); ok && r.IsInt() {
				return r, nil
			}
		}
		return new(big.Rat), nil
	},
	// int(x): a text or a number as a whole number; 0 when it is not one
	"int": func(args []any, _ *sim.OMap) (any, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("int takes one value")
		}
		switch x := args[0].(type) {
		case *big.Rat:
			return new(big.Rat).SetInt(new(big.Int).Quo(x.Num(), x.Denom())), nil
		case string:
			if r, ok := new(big.Rat).SetString(strings.TrimSpace(x)); ok {
				return new(big.Rat).SetInt(new(big.Int).Quo(r.Num(), r.Denom())), nil
			}
		}
		return new(big.Rat), nil
	},
}

// LoadPack parses a pack and checks it: every key known, every condition
// well formed.
func LoadPack(src []byte) (*Pack, error) {
	if len(src) > maxPackBytes {
		return nil, fmt.Errorf("pack is %d bytes; the limit is %d", len(src), maxPackBytes)
	}
	dec := yaml.NewDecoder(bytes.NewReader(src))
	dec.KnownFields(true)
	p := &Pack{}
	if err := dec.Decode(p); err != nil {
		return nil, fmt.Errorf("pack YAML: %w", err)
	}
	if !idRe.MatchString(p.Meta.ID) {
		return nil, fmt.Errorf("pack.id %q: use letters, digits, '_', '.', '-' (1-64 chars)", p.Meta.ID)
	}
	if len(p.Rules) > maxPackRules {
		return nil, fmt.Errorf("pack has %d rules; the limit is %d", len(p.Rules), maxPackRules)
	}
	for _, d := range []*Decimal{p.Settings.Threshold, p.Settings.MinShare} {
		if d != nil && d.Rat().Sign() <= 0 {
			return nil, fmt.Errorf("settings: thresholds must be positive")
		}
	}
	if p.Settings.WindowDays < 0 {
		return nil, fmt.Errorf("settings.window_days must be positive")
	}
	for kind, v := range p.Builtin {
		if !slices.Contains(builtinKinds, kind) {
			return nil, fmt.Errorf("builtin.%s is not a built-in finding (%s)", kind, strings.Join(builtinKinds, ", "))
		}
		if !slices.Contains(builtinSetting, v) {
			return nil, fmt.Errorf("builtin.%s: %q is not one of %s", kind, v, strings.Join(builtinSetting, ", "))
		}
	}
	seen := map[string]bool{}
	for i := range p.Rules {
		r := &p.Rules[i]
		where := fmt.Sprintf("rules[%d] %s", i, r.ID)
		switch {
		case !idRe.MatchString(r.ID):
			return nil, fmt.Errorf("%s: id is missing or malformed", where)
		case seen[r.ID] || slices.Contains(builtinKinds, r.ID):
			return nil, fmt.Errorf("%s: id is already taken", where)
		case !slices.Contains(ruleFor, r.For):
			return nil, fmt.Errorf("%s: for %q is not one of %s", where, r.For, strings.Join(ruleFor, ", "))
		case strings.TrimSpace(r.When) == "":
			return nil, fmt.Errorf("%s: when is empty", where)
		case strings.TrimSpace(r.Title) == "":
			return nil, fmt.Errorf("%s: title is empty", where)
		}
		seen[r.ID] = true
		if r.Severity == "" {
			r.Severity = "medium"
		}
		if !slices.Contains(severities, r.Severity) {
			return nil, fmt.Errorf("%s: severity %q is not one of %s", where, r.Severity, strings.Join(severities, ", "))
		}
		if err := sim.CheckExpr(r.When); err != nil {
			return nil, fmt.Errorf("%s: when: %v", where, err)
		}
		// a condition that names something an item of this kind does not have
		// would fail on every item: try it once on an empty one. Other errors
		// (a division by zero, an empty text indexed) depend on the item.
		if _, err := sim.EvalExpr(r.When, blankFields(r.For), ruleFuncs); err != nil && unknownNameRe.MatchString(err.Error()) {
			return nil, fmt.Errorf("%s: when: %v. A %s has: %s", where, err, r.For, strings.Join(sortedKeys(blankFields(r.For)), ", "))
		}
		for _, m := range fieldRe.FindAllStringSubmatch(r.Title+" "+r.Detail, -1) {
			// an attribute is whatever the case gave the entity: any name may follow attrs
			path := m[1]
			for _, open := range []string{".attrs.", ".metrics."} {
				if i := strings.Index("."+path, open); i >= 0 {
					path = ("." + path)[1 : i+len(open)-1]
				}
			}
			if _, ok := lookup(blankFields(r.For), path); !ok {
				return nil, fmt.Errorf("%s: {%s} is not a field of a %s", where, m[1], r.For)
			}
		}
	}
	return p, nil
}

// apply puts a pack's settings under the options given to the analysis.
func (p *Pack) apply(opt Options) Options {
	if p == nil {
		return opt
	}
	if opt.Threshold == nil && p.Settings.Threshold != nil {
		opt.Threshold = p.Settings.Threshold.Rat()
	}
	if opt.MinShare == nil && p.Settings.MinShare != nil {
		opt.MinShare = p.Settings.MinShare.Rat()
	}
	if opt.WindowDays == 0 {
		opt.WindowDays = p.Settings.WindowDays
	}
	return opt
}

// ---- what a rule sees ---------------------------------------------------------------

func num(n int) *big.Rat { return big.NewRat(int64(n), 1) }

func record(fields map[string]any) *sim.OMap {
	o := sim.NewOMap()
	for _, k := range sortedKeys(fields) {
		o.Set(k, fields[k])
	}
	return o
}

func metricMap(m map[string]Decimal) *sim.OMap {
	o := sim.NewOMap()
	for _, k := range sortedKeys(m) {
		o.Set(k, m[k].Rat())
	}
	return o
}

func textMap(m map[string]string) *sim.OMap {
	o := sim.NewOMap()
	for _, k := range sortedKeys(m) {
		o.Set(k, m[k])
	}
	return o
}

// entityFields are what a rule knows about an entity. The same record is what
// a relation, an account or a transaction carries for the entities at its ends.
func (a *analyzer) entityFields(id string, st *entityStats) map[string]any {
	e := a.ix.entities[id]
	if e == nil {
		e = &Entity{}
	}
	if st == nil {
		st = &entityStats{}
	}
	grade, _ := a.gradeOf(id)
	zero := new(big.Rat)
	orZero := func(r *big.Rat) *big.Rat {
		if r == nil {
			return zero
		}
		return r
	}
	return map[string]any{
		"id": id, "type": e.Type, "title": e.Title, "attrs": textMap(e.Attrs), "metrics": metricMap(e.Metrics), "note": e.Note,
		"sources": num(len(e.Sources)), "grade": grade,
		"owners": num(st.owners), "owned": num(st.owned), "controls": num(st.controls), "controllers": num(st.controllers),
		"relations": num(st.relations), "neighbours": num(st.neighbours), "accounts": num(st.accounts),
		"received": orZero(st.received), "sent": orZero(st.sent), "payments_in": num(st.paymentsIn), "payments_out": num(st.paymentsOut),
		"payers": num(len(st.payers)), "payees": num(len(st.payees)), "currency": st.currency,
	}
}

// entityStats are the counts a rule can use about an entity.
type entityStats struct {
	owners, owned, controls, controllers, relations, neighbours, accounts int
	received, sent                                                        *big.Rat
	paymentsIn, paymentsOut                                               int
	payers, payees                                                        map[string]bool
	currency                                                              string
}

// blankFields is an item of a kind with nothing in it: the names a rule of
// that kind may use, for checking a pack before there is a case.
func blankFields(kind string) map[string]any {
	a := &analyzer{ix: &index{entities: map[string]*Entity{}, sources: map[string]*Source{}}, claims: map[string]claim{}}
	ent := func() *sim.OMap { return record(a.entityFields("", nil)) }
	zero := new(big.Rat)
	switch kind {
	case ForEntity:
		return a.entityFields("", nil)
	case ForRelation:
		return map[string]any{"id": "", "type": "", "share": zero, "since": "", "until": "", "note": "", "sources": zero, "grade": "",
			"source": ent(), "target": ent()}
	case ForAccount:
		return map[string]any{"id": "", "kind": "", "title": "", "currency": "", "sources": zero, "grade": "", "owner": ent(),
			"received": zero, "sent": zero, "payments_in": zero, "payments_out": zero, "payers": zero, "payees": zero}
	case ForTransaction:
		return map[string]any{"id": "", "amount": zero, "currency": "", "date": "", "purpose": "", "sources": zero, "grade": "",
			"payer": ent(), "payee": ent(), "from_account": "", "to_account": ""}
	}
	return nil
}

// lookup reads a field, or a field of a record in it ("payer.title").
func lookup(fields map[string]any, path string) (any, bool) {
	head, rest, nested := strings.Cut(path, ".")
	v, ok := fields[head]
	if !ok || !nested {
		return v, ok
	}
	for {
		o, isRecord := v.(*sim.OMap)
		if !isRecord {
			return nil, false
		}
		head, rest, nested = strings.Cut(rest, ".")
		if v, ok = o.Get(head); !ok || !nested {
			return v, ok
		}
	}
}

// fill writes a finding's text from a template: {field} is the item's field.
func fill(template string, fields map[string]any) string {
	return fieldRe.ReplaceAllStringFunc(template, func(ref string) string {
		v, ok := lookup(fields, ref[1:len(ref)-1])
		if !ok {
			return "" // an attribute this item does not have
		}
		switch x := v.(type) {
		case *big.Rat:
			return ratString(x)
		case string:
			return x
		case bool:
			return fmt.Sprint(x)
		}
		return ""
	})
}

// ---- running the rules ----------------------------------------------------------------

// rules tries every rule of the pack on every item of its kind.
func (a *analyzer) rules(p *Pack) {
	if p == nil || len(p.Rules) == 0 {
		return
	}
	stats := a.stats()
	evals := 0
	for _, r := range p.Rules {
		raised, more, failed := 0, 0, ""
		try := func(fields map[string]any, subjects, evidence []string) bool {
			evals++
			if evals > maxRuleEvals {
				return false
			}
			v, err := sim.EvalExpr(r.When, fields, ruleFuncs)
			if err != nil {
				if failed == "" {
					failed = err.Error()
				}
				return true
			}
			if !sim.Truthy(v) {
				return true
			}
			if raised >= maxRuleFindings {
				more++
				return true
			}
			raised++
			a.add(r.ID, r.Severity, fill(r.Title, fields), fill(r.Detail, fields), subjects, evidence)
			return true
		}
		a.each(r.For, stats, try)
		if failed != "" {
			a.out.Notes = append(a.out.Notes, fmt.Sprintf("rule %s could not be evaluated on some items: %s", r.ID, failed))
		}
		if more > 0 {
			a.out.Notes = append(a.out.Notes, fmt.Sprintf("rule %s holds for %d more items than the %d reported.", r.ID, more, maxRuleFindings))
		}
		if evals > maxRuleEvals {
			a.out.Notes = append(a.out.Notes, "The rules stopped at their limit; some items were not tried.")
			break
		}
	}
	a.out.Assumptions = append(a.out.Assumptions, fmt.Sprintf(
		"Rules of the pack %q are conditions written by its author; a finding from one says the condition holds, nothing more.", p.Meta.ID))
}

// each calls try on every item of a kind, until try returns false.
func (a *analyzer) each(kind string, stats map[string]*entityStats, try func(map[string]any, []string, []string) bool) {
	ent := func(id string) *sim.OMap { return record(a.entityFields(id, stats[id])) }
	switch kind {
	case ForEntity:
		for _, e := range a.c.Entities {
			if !try(a.entityFields(e.ID, stats[e.ID]), []string{e.ID}, []string{e.ID}) {
				return
			}
		}
	case ForRelation:
		for i, r := range a.c.Relations {
			if a.ix.entities[r.From] == nil || a.ix.entities[r.To] == nil {
				continue
			}
			share := new(big.Rat)
			if r.Share != nil {
				share = r.Share.Rat()
			}
			grade, _ := a.gradeOf(a.relKeys[i])
			fields := map[string]any{"id": a.relKeys[i], "type": r.Type, "share": share, "since": r.Since.String(), "until": r.Until.String(),
				"note": r.Note, "sources": num(len(r.Sources)), "grade": grade, "source": ent(r.From), "target": ent(r.To)}
			if !try(fields, []string{r.From, r.To}, []string{a.relKeys[i]}) {
				return
			}
		}
	case ForAccount:
		type flow struct {
			in, out        *big.Rat
			nIn, nOut      int
			payers, payees map[string]bool
		}
		flows := map[string]*flow{}
		get := func(id string) *flow {
			if flows[id] == nil {
				flows[id] = &flow{in: new(big.Rat), out: new(big.Rat), payers: map[string]bool{}, payees: map[string]bool{}}
			}
			return flows[id]
		}
		for _, t := range a.txs {
			o, i := get(t.fromA), get(t.toA)
			o.out.Add(o.out, t.amount)
			o.nOut++
			o.payees[t.toA] = true
			i.in.Add(i.in, t.amount)
			i.nIn++
			i.payers[t.fromA] = true
		}
		for i := range a.c.Accounts {
			acc := &a.c.Accounts[i]
			if a.ix.entities[acc.Owner] == nil {
				continue
			}
			f := get(acc.ID)
			grade, _ := a.gradeOf(acc.ID)
			fields := map[string]any{"id": acc.ID, "kind": acc.Kind, "title": acc.Title, "currency": a.ix.accountCurrency(a.c, acc),
				"sources": num(len(acc.Sources)), "grade": grade, "owner": ent(acc.Owner), "received": f.in, "sent": f.out,
				"payments_in": num(f.nIn), "payments_out": num(f.nOut), "payers": num(len(f.payers)), "payees": num(len(f.payees))}
			if !try(fields, []string{acc.Owner}, []string{acc.ID}) {
				return
			}
		}
	case ForTransaction:
		for _, t := range a.txs {
			grade, _ := a.gradeOf(t.t.ID)
			fields := map[string]any{"id": t.t.ID, "amount": t.amount, "currency": t.cur, "date": t.t.Date.String(), "purpose": t.t.Purpose,
				"sources": num(len(t.t.Sources)), "grade": grade, "payer": ent(t.from), "payee": ent(t.to),
				"from_account": t.fromA, "to_account": t.toA}
			if !try(fields, []string{t.from, t.to}, []string{t.t.ID}) {
				return
			}
		}
	}
}

// stats counts, for every entity, what a rule may ask about it. Money is in
// the entity's main currency: the one it moved most payments in.
func (a *analyzer) stats() map[string]*entityStats {
	out := map[string]*entityStats{}
	get := func(id string) *entityStats {
		if out[id] == nil {
			out[id] = &entityStats{payers: map[string]bool{}, payees: map[string]bool{}}
		}
		return out[id]
	}
	for _, r := range a.c.Relations {
		if a.ix.entities[r.From] == nil || a.ix.entities[r.To] == nil || r.From == r.To {
			continue
		}
		from, to := get(r.From), get(r.To)
		from.relations++
		to.relations++
		switch {
		case r.Type == "owns":
			from.owned++
			to.owners++
		case controlTypes[r.Type]:
			from.controls++
			to.controllers++
		}
	}
	for id, edges := range a.adj {
		seen := map[string]bool{}
		for _, e := range edges {
			seen[e.to] = true
		}
		get(id).neighbours = len(seen)
	}
	for _, acc := range a.c.Accounts {
		if a.ix.entities[acc.Owner] != nil {
			get(acc.Owner).accounts++
		}
	}
	// the main currency of each entity, then its money in that currency
	count := map[string]map[string]int{}
	for _, t := range a.txs {
		for _, id := range []string{t.from, t.to} {
			if count[id] == nil {
				count[id] = map[string]int{}
			}
			count[id][t.cur]++
		}
	}
	for id, by := range count {
		best := ""
		for _, cur := range sortedKeys(by) {
			if best == "" || by[cur] > by[best] {
				best = cur
			}
		}
		st := get(id)
		st.currency, st.received, st.sent = best, new(big.Rat), new(big.Rat)
	}
	for _, t := range a.txs {
		if t.from == t.to {
			continue
		}
		if st := out[t.from]; st != nil && st.currency == t.cur {
			st.sent.Add(st.sent, t.amount)
			st.paymentsOut++
			st.payees[t.to] = true
		}
		if st := out[t.to]; st != nil && st.currency == t.cur {
			st.received.Add(st.received, t.amount)
			st.paymentsIn++
			st.payers[t.from] = true
		}
	}
	return out
}

// builtin applies the pack's choice of built-in findings: which to leave out,
// and the severity of the rest.
func (a *analyzer) builtin(p *Pack) {
	if p == nil || len(p.Builtin) == 0 {
		return
	}
	kept := a.findings[:0]
	for _, f := range a.findings {
		switch setting, chosen := p.Builtin[f.Kind]; {
		case !chosen:
		case setting == "off":
			continue
		default:
			f.Severity = setting
		}
		kept = append(kept, f)
	}
	a.findings = kept
}
