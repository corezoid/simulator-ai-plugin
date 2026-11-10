package osint

import (
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"
)

// Options tune the analysis. Zero values take the defaults.
type Options struct {
	From, To   string   // find the connection between these two ids
	Threshold  *big.Rat // reporting threshold that structuring stays under (default 10000)
	WindowDays int      // days within which money moving on counts as one move (default 7)
	MinShare   *big.Rat // effective stake, in percent, that makes a beneficial owner (default 25)
	Top        int      // rows in the ranked tables (default 10)
	Timeline   int      // chronology entries (default 40)
}

const (
	defaultThreshold  = 10000
	defaultWindowDays = 7
	defaultMinShare   = 25
	defaultTop        = 10
	defaultTimeline   = 40

	cycleDays        = 120 // a round trip closes within this many days
	maxCycleLen      = 6   // payments in one round trip
	maxCycleSteps    = 300_000
	maxCycleFindings = 25
	maxOwnershipWalk = 200_000
	maxOwnershipRows = 200
	maxPaths         = 5
)

func (o Options) withDefaults() Options {
	if o.Threshold == nil || o.Threshold.Sign() <= 0 {
		o.Threshold = big.NewRat(defaultThreshold, 1)
	}
	if o.WindowDays <= 0 {
		o.WindowDays = defaultWindowDays
	}
	if o.MinShare == nil || o.MinShare.Sign() <= 0 {
		o.MinShare = big.NewRat(defaultMinShare, 1)
	}
	if o.Top <= 0 {
		o.Top = defaultTop
	}
	if o.Timeline <= 0 {
		o.Timeline = defaultTimeline
	}
	return o
}

// Analysis is what the engine reads out of a case.
type Analysis struct {
	Summary     Summary        `json:"summary"`
	Findings    []Finding      `json:"findings"`
	Ownership   []Ownership    `json:"beneficialOwnership,omitempty"`
	Central     []Central      `json:"central,omitempty"`
	Flows       []Flow         `json:"flows,omitempty"`
	Connection  *Connection    `json:"connection,omitempty"`
	Timeline    []TimelineItem `json:"timeline,omitempty"`
	Evidence    EvidenceStats  `json:"evidence"`
	Truth       *TruthScore    `json:"truth,omitempty"`
	Notes       []string       `json:"notes,omitempty"`
	Assumptions []string       `json:"assumptions"`
}

// Summary sizes the case.
type Summary struct {
	Case         string            `json:"case"`
	Synthetic    bool              `json:"synthetic"`
	Entities     int               `json:"entities"`
	ByType       map[string]int    `json:"entitiesByType"`
	Relations    int               `json:"relations"`
	Accounts     int               `json:"accounts"`
	Transactions int               `json:"transactions"`
	Volume       map[string]string `json:"volumeByCurrency,omitempty"`
	FirstDate    string            `json:"firstTransaction,omitempty"`
	LastDate     string            `json:"lastTransaction,omitempty"`
	Components   int               `json:"connectedComponents"`
}

// Finding is one pattern the analysis found. It is a lead, not a conclusion:
// Evidence lists the case items it rests on and WeakestEvidence grades the
// weakest of them (source reliability A-F + information credibility 1-6).
type Finding struct {
	ID              string   `json:"id"`
	Kind            string   `json:"kind"`
	Severity        string   `json:"severity"`
	Title           string   `json:"title"`
	Detail          string   `json:"detail"`
	Subjects        []string `json:"subjects"`
	Evidence        []string `json:"evidence"`
	WeakestEvidence string   `json:"weakestEvidence"`
}

// Ownership is one ultimate owner of one entity.
type Ownership struct {
	Entity   string `json:"entity"`
	Owner    string `json:"owner"`
	Share    string `json:"effectiveSharePercent"`
	Hops     int    `json:"hops"`
	Via      string `json:"via"`
	Complete bool   `json:"allStakesKnown"`
}

// Central ranks an entity by how many others it touches.
type Central struct {
	Entity         string `json:"entity"`
	Title          string `json:"title"`
	Type           string `json:"type"`
	Neighbours     int    `json:"neighbours"`
	Relations      int    `json:"relations"`
	Counterparties int    `json:"paymentCounterparties"`
}

// Flow is the money one entity moved in one currency.
type Flow struct {
	Entity   string `json:"entity"`
	Title    string `json:"title"`
	Currency string `json:"currency"`
	In       string `json:"in"`
	Out      string `json:"out"`
	Internal string `json:"betweenOwnAccounts,omitempty"`
	InCount  int    `json:"inPayments"`
	OutCount int    `json:"outPayments"`
	InFrom   int    `json:"payers"`
	OutTo    int    `json:"payees"`
}

// Connection is how two entities are linked.
type Connection struct {
	From  string     `json:"from"`
	To    string     `json:"to"`
	Found bool       `json:"found"`
	Hops  int        `json:"hops,omitempty"`
	Paths [][]string `json:"paths,omitempty"`
	Note  string     `json:"note,omitempty"`
}

// TimelineItem is one dated entry of the chronology.
type TimelineItem struct {
	Date string   `json:"date"`
	Kind string   `json:"kind"`
	Text string   `json:"text"`
	IDs  []string `json:"ids,omitempty"`
}

// EvidenceStats says how well the case is sourced.
type EvidenceStats struct {
	Claims       int            `json:"claims"`
	Unsourced    int            `json:"unsourced"`
	SingleSource int            `json:"singleSource"`
	ByBestSource map[string]int `json:"byBestSourceReliability"`
	WeakFindings int            `json:"findingsOnWeakEvidence"`
}

// TruthScore compares the findings of a synthetic case with its known answer.
type TruthScore struct {
	Typologies        []string `json:"typologies,omitempty"`
	EntitiesFound     string   `json:"entitiesFound"`
	TransactionsFound string   `json:"transactionsFound"`
	MissedEntities    []string `json:"missedEntities,omitempty"`
	MissedTx          []string `json:"missedTransactions,omitempty"`
	OutsideScheme     []string `json:"flaggedOutsideScheme,omitempty"`
}

// ---- working model -------------------------------------------------------------

type claim struct {
	sources     []string
	credibility int
}

type edge struct {
	to    string
	label string
	ref   string // id of the relation or a transaction behind the edge
	pay   bool   // the edge is payments, not a relation
}

type tx struct {
	t          *Transaction
	from, to   string // owner entities
	cur        string
	amount     *big.Rat
	dated      bool
	when       time.Time
	fromA, toA string // accounts
}

type analyzer struct {
	c        *Case
	ix       *index
	opt      Options
	out      *Analysis
	claims   map[string]claim
	relKeys  []string
	adj      map[string][]edge
	txs      []tx
	findings []Finding
	// ultimate[entity][owner] is the chain of ownership relations from the
	// entity up to one of its ultimate owners
	ultimate map[string]map[string][]string
}

func (a *analyzer) name(id string) string {
	if e := a.ix.entities[id]; e != nil && e.Title != "" {
		return fmt.Sprintf("%s (%s)", e.Title, id)
	}
	return id
}

func (a *analyzer) names(ids []string) string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = a.name(id)
	}
	return strings.Join(out, ", ")
}

func money(r *big.Rat, cur string) string {
	if cur == "" {
		return ratString(r)
	}
	return ratString(r) + " " + cur
}

// Analyze reads a case that passed Check.
func Analyze(c *Case, opt Options) *Analysis {
	a := &analyzer{c: c, ix: buildIndex(c), opt: opt.withDefaults(), claims: map[string]claim{}, adj: map[string][]edge{},
		ultimate: map[string]map[string][]string{}}
	a.out = &Analysis{Findings: []Finding{}, Assumptions: []string{}}
	a.prepare()
	a.summary()
	a.ownership()
	a.control()
	a.sharedAttributes()
	a.flows()
	a.relatedParties()
	a.conflicts()
	a.passThrough()
	a.roundTrips()
	a.structuring()
	a.funnels()
	a.central()
	a.connection()
	a.timeline()
	a.finish()
	return a.out
}

func (a *analyzer) prepare() {
	c := a.c
	for _, e := range c.Entities {
		a.claims[e.ID] = claim{e.Sources, e.Credibility}
	}
	a.relKeys = make([]string, len(c.Relations))
	for i, r := range c.Relations {
		key := r.ID
		if key == "" {
			key = fmt.Sprintf("relations[%d]", i)
		}
		a.relKeys[i] = key
		a.claims[key] = claim{r.Sources, r.Credibility}
		if a.ix.entities[r.From] == nil || a.ix.entities[r.To] == nil || r.From == r.To {
			continue
		}
		lbl := r.Type
		if r.Type == "owns" && r.Share != nil {
			lbl = fmt.Sprintf("owns %s%%", r.Share)
		}
		a.adj[r.From] = append(a.adj[r.From], edge{to: r.To, label: lbl + " →", ref: key})
		a.adj[r.To] = append(a.adj[r.To], edge{to: r.From, label: "← " + lbl, ref: key})
	}
	for _, acc := range c.Accounts {
		a.claims[acc.ID] = claim{acc.Sources, acc.Credibility}
	}
	for i := range c.Transactions {
		t := &c.Transactions[i]
		a.claims[t.ID] = claim{t.Sources, t.Credibility}
		from, to := a.ix.accounts[t.From], a.ix.accounts[t.To]
		if from == nil || to == nil || !t.Amount.IsSet() || t.Amount.Rat().Sign() <= 0 {
			continue
		}
		a.txs = append(a.txs, tx{t: t, from: from.Owner, to: to.Owner, cur: a.ix.currencyOf(c, t), amount: t.Amount.Rat(),
			dated: !t.Date.IsZero(), when: t.Date.Time(), fromA: t.From, toA: t.To})
	}
	sort.SliceStable(a.txs, func(i, j int) bool {
		x, y := a.txs[i], a.txs[j]
		if x.dated != y.dated {
			return x.dated
		}
		if !x.when.Equal(y.when) {
			return x.when.Before(y.when)
		}
		return x.t.ID < y.t.ID
	})
	for i, e := range c.Events {
		key := e.ID
		if key == "" {
			key = fmt.Sprintf("events[%d]", i)
		}
		a.claims[key] = claim{e.Sources, e.Credibility}
	}
	// payments between two entities are one edge, labelled with their total
	type pair struct{ from, to, cur string }
	sums, counts, first := map[pair]*big.Rat{}, map[pair]int{}, map[pair]string{}
	var order []pair
	for _, t := range a.txs {
		if t.from == t.to {
			continue
		}
		p := pair{t.from, t.to, t.cur}
		if sums[p] == nil {
			sums[p] = new(big.Rat)
			first[p] = t.t.ID
			order = append(order, p)
		}
		sums[p].Add(sums[p], t.amount)
		counts[p]++
	}
	for _, p := range order {
		lbl := fmt.Sprintf("paid %s in %d payment(s)", money(sums[p], p.cur), counts[p])
		a.adj[p.from] = append(a.adj[p.from], edge{to: p.to, label: lbl + " →", ref: first[p], pay: true})
		a.adj[p.to] = append(a.adj[p.to], edge{to: p.from, label: "← " + lbl, ref: first[p], pay: true})
	}
}

func (a *analyzer) summary() {
	c := a.c
	s := Summary{Case: c.Meta.ID, Synthetic: c.Meta.Synthetic, Entities: len(c.Entities), ByType: map[string]int{},
		Relations: len(c.Relations), Accounts: len(c.Accounts), Transactions: len(c.Transactions)}
	for _, e := range c.Entities {
		s.ByType[e.Type]++
	}
	vol := map[string]*big.Rat{}
	for _, t := range a.txs {
		if vol[t.cur] == nil {
			vol[t.cur] = new(big.Rat)
		}
		vol[t.cur].Add(vol[t.cur], t.amount)
		if t.dated {
			d := t.t.Date.String()
			if s.FirstDate == "" {
				s.FirstDate = d
			}
			s.LastDate = d
		}
	}
	if len(vol) > 0 {
		s.Volume = map[string]string{}
		for cur, v := range vol {
			s.Volume[cur] = ratString(v)
		}
	}
	// connected components over relations and payments
	seen := map[string]bool{}
	for _, e := range c.Entities {
		if seen[e.ID] {
			continue
		}
		s.Components++
		stack := []string{e.ID}
		seen[e.ID] = true
		for len(stack) > 0 {
			id := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, ed := range a.adj[id] {
				if !seen[ed.to] {
					seen[ed.to] = true
					stack = append(stack, ed.to)
				}
			}
		}
	}
	a.out.Summary = s
}

func (a *analyzer) add(kind, severity, title, detail string, subjects, evidence []string) {
	a.findings = append(a.findings, Finding{Kind: kind, Severity: severity, Title: title, Detail: detail,
		Subjects: uniq(subjects), Evidence: uniq(evidence)})
}

// ---- ownership -------------------------------------------------------------------

type stake struct {
	owner string
	share *big.Rat // nil: the stake is not known
	ref   string
}

func (a *analyzer) ownership() {
	owners := map[string][]stake{}
	hasOwns := false
	for i, r := range a.c.Relations {
		if r.Type != "owns" || a.ix.entities[r.From] == nil || a.ix.entities[r.To] == nil || r.From == r.To {
			continue
		}
		hasOwns = true
		st := stake{owner: r.From, ref: a.relKeys[i]}
		if r.Share != nil && r.Share.Rat().Sign() > 0 {
			st.share = r.Share.Rat()
		}
		owners[r.To] = append(owners[r.To], st)
	}
	if !hasOwns {
		return
	}
	a.out.Assumptions = append(a.out.Assumptions,
		"Effective ownership multiplies the recorded stakes along each chain and adds the chains up; stakes are taken as current, whatever their dates.")

	type hold struct {
		share *big.Rat
		hops  int
		path  []string
		refs  []string
		best  *big.Rat
	}
	hundred := big.NewRat(100, 1)
	steps := 0
	cycles := map[string]bool{}
	truncated := false

	targets := sortedKeys(owners)
	for _, target := range targets {
		acc := map[string]*hold{}
		complete := true
		onPath := map[string]bool{target: true}
		var walk func(node string, frac *big.Rat, path, refs []string)
		walk = func(node string, frac *big.Rat, path, refs []string) {
			for _, st := range owners[node] {
				steps++
				if steps > maxOwnershipWalk {
					truncated = true
					return
				}
				if onPath[st.owner] {
					// report each loop once, whichever entity the walk entered it from
					loop := append(append([]string{}, path...), st.owner)
					start := 0
					for i, id := range loop {
						if id == st.owner {
							start = i
							break
						}
					}
					members := append([]string{}, loop[start:len(loop)-1]...)
					key := strings.Join(sortedCopy(members), "|")
					if !cycles[key] {
						cycles[key] = true
						a.add("circular_ownership", "high",
							"Circular ownership: "+a.names(members),
							"These entities own each other in a loop, so no outside owner can be read off the recorded stakes: "+strings.Join(loop[start:], " ← ")+".",
							members, append(append([]string{}, refs[min(start, len(refs)):]...), st.ref))
					}
					continue
				}
				if st.share == nil {
					complete = false
					continue
				}
				f := new(big.Rat).Mul(frac, st.share)
				f.Quo(f, hundred)
				p := append(append([]string{}, path...), st.owner)
				rs := append(append([]string{}, refs...), st.ref)
				if len(owners[st.owner]) == 0 {
					h := acc[st.owner]
					if h == nil {
						h = &hold{share: new(big.Rat), best: new(big.Rat)}
						acc[st.owner] = h
					}
					h.share.Add(h.share, f)
					if f.Cmp(h.best) > 0 {
						h.best, h.path, h.refs = f, p, rs
					}
					h.hops = max(h.hops, len(p)-1)
					continue
				}
				onPath[st.owner] = true
				walk(st.owner, f, p, rs)
				delete(onPath, st.owner)
			}
		}
		walk(target, big.NewRat(100, 1), []string{target}, nil)

		for _, owner := range sortedKeys(acc) {
			h := acc[owner]
			if a.ultimate[target] == nil {
				a.ultimate[target] = map[string][]string{}
			}
			a.ultimate[target][owner] = h.refs
			if len(a.out.Ownership) < maxOwnershipRows {
				a.out.Ownership = append(a.out.Ownership, Ownership{Entity: target, Owner: owner, Share: ratString(roundRat(h.share, 4)),
					Hops: h.hops, Via: strings.Join(h.path, " ← "), Complete: complete})
			}
			if h.hops >= 2 && h.share.Cmp(a.opt.MinShare) >= 0 {
				sev := "medium"
				if h.hops >= 3 {
					sev = "high"
				}
				a.add("indirect_beneficial_owner", sev,
					fmt.Sprintf("%s ultimately holds %s%% of %s through %d layer(s)", a.name(owner), ratString(roundRat(h.share, 2)), a.name(target), h.hops-1),
					fmt.Sprintf("Largest chain: %s. The stake is not visible in the direct owners of %s; it comes from multiplying the stakes along the chain.",
						strings.Join(h.path, " ← "), target),
					h.path, h.refs)
			}
		}
	}
	if truncated {
		a.out.Notes = append(a.out.Notes, "The ownership graph is too branched to walk in full; beneficial-ownership figures may be incomplete.")
	}
	sort.SliceStable(a.out.Ownership, func(i, j int) bool {
		x, y := a.out.Ownership[i], a.out.Ownership[j]
		if x.Entity != y.Entity {
			return x.Entity < y.Entity
		}
		return x.Owner < y.Owner
	})

	var unknown []string
	for _, e := range a.c.Entities {
		if e.Type == "organization" && len(owners[e.ID]) == 0 {
			unknown = append(unknown, e.ID)
		}
	}
	if len(unknown) > 0 {
		a.add("unknown_owner", "low",
			fmt.Sprintf("%d organization(s) have no recorded owner", len(unknown)),
			"No owns relation points at: "+a.names(unknown)+". Whoever stands behind them is not in the case yet.",
			unknown, nil)
	}
}

// roundRat rounds to the given number of decimal places.
func roundRat(r *big.Rat, places int) *big.Rat {
	out, _ := new(big.Rat).SetString(r.FloatString(places))
	return out
}

// ---- control and shared anchors ------------------------------------------------------

func (a *analyzer) control() {
	type seat struct{ orgs, refs []string }
	seats := map[string]*seat{}
	anchored := map[string]*seat{}
	for i, r := range a.c.Relations {
		from, to := a.ix.entities[r.From], a.ix.entities[r.To]
		if from == nil || to == nil {
			continue
		}
		if controlTypes[r.Type] && from.Type == "person" && to.Type == "organization" {
			if seats[r.From] == nil {
				seats[r.From] = &seat{}
			}
			seats[r.From].orgs = append(seats[r.From].orgs, r.To)
			seats[r.From].refs = append(seats[r.From].refs, a.relKeys[i])
		}
		if anchorTypes[r.Type] {
			if anchored[r.To] == nil {
				anchored[r.To] = &seat{}
			}
			anchored[r.To].orgs = append(anchored[r.To].orgs, r.From)
			anchored[r.To].refs = append(anchored[r.To].refs, a.relKeys[i])
		}
	}
	for _, person := range sortedKeys(seats) {
		orgs := uniq(seats[person].orgs)
		if len(orgs) < 3 {
			continue
		}
		a.add("multiple_directorships", "medium",
			fmt.Sprintf("%s holds a controlling role in %d organizations", a.name(person), len(orgs)),
			"Director, officer, agent or controller of: "+a.names(orgs)+". One person fronting many companies is a common sign of a nominee; it is also what a corporate-services provider looks like.",
			append([]string{person}, orgs...), seats[person].refs)
	}
	for _, anchor := range sortedKeys(anchored) {
		users := uniq(anchored[anchor].orgs)
		if len(users) < 2 {
			continue
		}
		sev := "low"
		if len(users) >= 3 {
			sev = "medium"
		}
		a.add("shared_anchor", sev,
			fmt.Sprintf("%d entities share %s", len(users), a.name(anchor)),
			"Registered at, located at or using the same address or identifier: "+a.names(users)+".",
			append([]string{anchor}, users...), anchored[anchor].refs)
	}
}

// sharedAttrKeys are attributes that identify something: two entities with
// the same value are probably connected. Descriptive attributes (country,
// industry, status) are left out, since sharing them means nothing.
var sharedAttrKeys = map[string]bool{"address": true, "registered_address": true, "phone": true, "email": true,
	"website": true, "domain": true, "ip": true, "registered_agent": true, "tax_id": true, "passport": true, "wallet": true}

func (a *analyzer) sharedAttributes() {
	groups := map[string][]string{}
	for _, e := range a.c.Entities {
		for k, v := range e.Attrs {
			if sharedAttrKeys[k] && normalize(v) != "" {
				key := k + "\x00" + normalize(v)
				groups[key] = append(groups[key], e.ID)
			}
		}
	}
	for _, key := range sortedKeys(groups) {
		ids := uniq(groups[key])
		if len(ids) < 2 {
			continue
		}
		attr, value, _ := strings.Cut(key, "\x00")
		sev := "low"
		if len(ids) >= 3 {
			sev = "medium"
		}
		a.add("shared_attribute", sev,
			fmt.Sprintf("%d entities have the same %s", len(ids), attr),
			fmt.Sprintf("%s = %q on: %s.", attr, value, a.names(ids)),
			ids, ids)
	}
}

// ---- money flows -----------------------------------------------------------------------

type flowAcc struct {
	in, out, internal *big.Rat
	nIn, nOut         int
	payers, payees    map[string]bool
}

func newFlowAcc() *flowAcc {
	return &flowAcc{in: new(big.Rat), out: new(big.Rat), internal: new(big.Rat), payers: map[string]bool{}, payees: map[string]bool{}}
}

type flowKey struct{ entity, cur string }

func (a *analyzer) entityFlows() map[flowKey]*flowAcc {
	flows := map[flowKey]*flowAcc{}
	get := func(entity, cur string) *flowAcc {
		k := flowKey{entity, cur}
		if flows[k] == nil {
			flows[k] = newFlowAcc()
		}
		return flows[k]
	}
	for _, t := range a.txs {
		if t.from == t.to {
			f := get(t.from, t.cur)
			f.internal.Add(f.internal, t.amount)
			continue
		}
		f := get(t.from, t.cur)
		f.out.Add(f.out, t.amount)
		f.nOut++
		f.payees[t.to] = true
		g := get(t.to, t.cur)
		g.in.Add(g.in, t.amount)
		g.nIn++
		g.payers[t.from] = true
	}
	return flows
}

func (a *analyzer) flows() {
	flows := a.entityFlows()
	keys := make([]flowKey, 0, len(flows))
	for k := range flows {
		keys = append(keys, k)
	}
	turnover := func(k flowKey) *big.Rat { return new(big.Rat).Add(flows[k].in, flows[k].out) }
	sort.Slice(keys, func(i, j int) bool {
		if c := turnover(keys[i]).Cmp(turnover(keys[j])); c != 0 {
			return c > 0
		}
		if keys[i].entity != keys[j].entity {
			return keys[i].entity < keys[j].entity
		}
		return keys[i].cur < keys[j].cur
	})
	for i, k := range keys {
		if i >= 2*a.opt.Top {
			a.out.Notes = append(a.out.Notes, fmt.Sprintf("flows lists the %d largest of %d entity/currency rows.", 2*a.opt.Top, len(keys)))
			break
		}
		f := flows[k]
		row := Flow{Entity: k.entity, Currency: k.cur, In: ratString(f.in), Out: ratString(f.out),
			InCount: f.nIn, OutCount: f.nOut, InFrom: len(f.payers), OutTo: len(f.payees)}
		if e := a.ix.entities[k.entity]; e != nil {
			row.Title = e.Title
		}
		if f.internal.Sign() > 0 {
			row.Internal = ratString(f.internal)
		}
		a.out.Flows = append(a.out.Flows, row)
	}
}

// relatedParties finds payments between two entities that have the same
// ultimate owner: the two sides of the deal are one interest.
func (a *analyzer) relatedParties() {
	type pair struct{ x, y, cur string }
	sums, ids := map[pair]*big.Rat{}, map[pair][]string{}
	var order []pair
	for _, t := range a.txs {
		if t.from == t.to || a.ultimate[t.from] == nil || a.ultimate[t.to] == nil {
			continue
		}
		p := pair{min(t.from, t.to), max(t.from, t.to), t.cur}
		if sums[p] == nil {
			sums[p] = new(big.Rat)
			order = append(order, p)
		}
		sums[p].Add(sums[p], t.amount)
		ids[p] = append(ids[p], t.t.ID)
	}
	for _, p := range order {
		for _, owner := range sortedKeys(a.ultimate[p.x]) {
			refsY, shared := a.ultimate[p.y][owner]
			if !shared {
				continue
			}
			evidence := append(append(append([]string{}, ids[p]...), a.ultimate[p.x][owner]...), refsY...)
			a.add("related_party_payments", "medium",
				fmt.Sprintf("%s and %s pay each other and have the same ultimate owner, %s", a.name(p.x), a.name(p.y), a.name(owner)),
				fmt.Sprintf("%s in %d payment(s) between them. A deal between two companies of one owner is not at arm's length: its price and purpose are whatever the owner wants them to be.",
					money(sums[p], p.cur), len(ids[p])),
				[]string{p.x, p.y, owner}, evidence)
		}
	}
}

// circleTypes link a person to the people close to them.
var circleTypes = map[string]bool{"family": true, "associate": true, "same_as": true}

const (
	conflictDepth    = 4         // payments between the organization and the beneficiary
	maxConflictSteps = 2_000_000 // work the search may do over the whole case
	maxCircle        = 50        // people counted as close to one person
)

// conflicts finds money that leaves an organization and reaches someone close
// to a person who has a say in that organization without owning it: an
// official whose relative owns a subcontractor, a director paying a friend's
// company. An owner dealing with their own companies is a related-party
// payment, reported separately.
func (a *analyzer) conflicts() {
	insiders := map[string][]stake{} // organization → people with a controlling role
	circle := map[string][]stake{}   // person → people close to them
	owned := map[string]map[string]bool{}
	for i, r := range a.c.Relations {
		from, to := a.ix.entities[r.From], a.ix.entities[r.To]
		if from == nil || to == nil || r.From == r.To {
			continue
		}
		key := a.relKeys[i]
		switch {
		case controlTypes[r.Type] && from.Type == "person" && to.Type == "organization":
			insiders[r.To] = append(insiders[r.To], stake{owner: r.From, ref: key})
		case circleTypes[r.Type] && from.Type == "person" && to.Type == "person":
			circle[r.From] = append(circle[r.From], stake{owner: r.To, ref: key})
			circle[r.To] = append(circle[r.To], stake{owner: r.From, ref: key})
		case r.Type == "owns":
			if owned[r.To] == nil {
				owned[r.To] = map[string]bool{}
			}
			owned[r.To][r.From] = true
		}
	}
	if len(insiders) == 0 {
		return
	}
	// payments, as one edge per pair of entities
	type hop struct{ to, tx string }
	out := map[string][]hop{}
	seenPair := map[string]bool{}
	for _, t := range a.txs {
		if k := t.from + "|" + t.to; t.from != t.to && !seenPair[k] {
			seenPair[k] = true
			out[t.from] = append(out[t.from], hop{t.to, t.t.ID})
		}
	}
	flagged := false
	steps, truncated := 0, false
	for _, org := range sortedKeys(insiders) {
		if truncated {
			break
		}
		// everyone the organization's money reaches within conflictDepth payments
		type reach struct {
			depth int
			via   []string
		}
		reached := map[string]reach{org: {}}
		queue := []string{org}
		for len(queue) > 0 {
			at := queue[0]
			queue = queue[1:]
			if reached[at].depth >= conflictDepth {
				continue
			}
			for _, h := range out[at] {
				steps++
				if steps > maxConflictSteps {
					truncated = true
					break
				}
				if _, ok := reached[h.to]; !ok {
					reached[h.to] = reach{reached[at].depth + 1, append(append([]string{}, reached[at].via...), h.tx)}
					queue = append(queue, h.to)
				}
			}
		}
		if len(reached) == 1 {
			continue
		}
		done := map[string]bool{}
		for _, in := range insiders[org] {
			if done[in.owner] || owned[org][in.owner] || a.ultimate[org][in.owner] != nil {
				continue
			}
			done[in.owner] = true
			near := map[string]string{in.owner: ""} // person → the relation that makes them close
			for _, c := range circle[in.owner] {
				if len(near) > maxCircle {
					break
				}
				near[c.owner] = c.ref
			}
			steps += len(reached) * len(near)
			if steps > maxConflictSteps {
				truncated = true
				break
			}
			var subjects, evidence, lines []string
			for _, got := range sortedKeys(reached) {
				r := reached[got]
				if got == org || r.depth == 0 {
					continue
				}
				for _, person := range sortedKeys(near) {
					var how string
					var refs []string
					switch {
					case got == person && person == in.owner && r.depth == 1:
						continue // the organization paying its own officer: a salary
					case got == person:
						how = fmt.Sprintf("%s receives it", a.name(person))
					case a.ultimate[got][person] != nil:
						how = fmt.Sprintf("%s, ultimately owned by %s, receives it", a.name(got), a.name(person))
						refs = a.ultimate[got][person]
					default:
						continue
					}
					if person != in.owner {
						how += fmt.Sprintf(" (close to %s)", a.name(in.owner))
						evidence = append(evidence, near[person])
					}
					lines = append(lines, fmt.Sprintf("%s, %d payment(s) away", how, r.depth))
					subjects = append(subjects, got, person)
					evidence = append(append(evidence, refs...), r.via...)
				}
			}
			if len(lines) == 0 {
				continue
			}
			flagged = true
			a.add("conflict_of_interest", "high",
				fmt.Sprintf("Money paid by %s reaches people close to %s, who has a controlling role in it", a.name(org), a.name(in.owner)),
				strings.Join(lines, "; ")+". Whoever decides for an organization should not be where its money ends up; check whether the role had a say in these payments and whether the interest was declared.",
				append([]string{org, in.owner}, subjects...), append([]string{in.ref}, evidence...))
		}
	}
	if truncated {
		a.out.Notes = append(a.out.Notes, "The search for conflicts of interest stopped at its limit; more may exist.")
	}
	if flagged {
		a.out.Assumptions = append(a.out.Assumptions, fmt.Sprintf(
			"A conflict of interest is money that reaches, within %d payments, a person with a controlling role in the paying organization (other than its owners), someone linked to them as family, associate or the same person, or a company such a person ultimately owns. A payment straight to the officer is taken as pay.", conflictDepth))
	}
}

// passThrough finds accounts that forward what they receive within the
// window, and joins accounts that forward to each other into one chain.
func (a *analyzer) passThrough() {
	window := time.Duration(a.opt.WindowDays) * 24 * time.Hour
	type accKey struct{ acc, cur string }
	type accFlow struct {
		in, out, matched *big.Rat
		ins, outs        []tx
		payers, payees   map[string]bool
	}
	byAcc := map[accKey]*accFlow{}
	get := func(acc, cur string) *accFlow {
		k := accKey{acc, cur}
		if byAcc[k] == nil {
			byAcc[k] = &accFlow{in: new(big.Rat), out: new(big.Rat), matched: new(big.Rat), payers: map[string]bool{}, payees: map[string]bool{}}
		}
		return byAcc[k]
	}
	for _, t := range a.txs {
		o := get(t.fromA, t.cur)
		o.out.Add(o.out, t.amount)
		o.outs = append(o.outs, t)
		o.payees[t.toA] = true
		i := get(t.toA, t.cur)
		i.in.Add(i.in, t.amount)
		i.ins = append(i.ins, t)
		i.payers[t.fromA] = true
	}
	passing := map[accKey]bool{}
	for k, f := range byAcc {
		if f.in.Sign() == 0 || f.out.Sign() == 0 || len(f.payers) > 3 || len(f.payees) > 3 {
			continue
		}
		// an account that pays out much more than it received is spending its
		// own money, not forwarding someone else's
		if new(big.Rat).Mul(f.out, big.NewRat(10, 1)).Cmp(new(big.Rat).Mul(f.in, big.NewRat(11, 1))) > 0 {
			continue
		}
		// incoming payments are in date order, dated ones first (a.txs is), so
		// the latest one not after an outgoing payment is found by bisection
		dated := sort.Search(len(f.ins), func(i int) bool { return !f.ins[i].dated })
		for _, o := range f.outs {
			if !o.dated {
				continue
			}
			i := sort.Search(dated, func(i int) bool { return f.ins[i].when.After(o.when) })
			if i > 0 && o.when.Sub(f.ins[i-1].when) <= window {
				f.matched.Add(f.matched, o.amount)
			}
		}
		// at least 90 % of what came in left again within the window
		if new(big.Rat).Mul(f.matched, big.NewRat(10, 1)).Cmp(new(big.Rat).Mul(f.in, big.NewRat(9, 1))) >= 0 {
			passing[k] = true
		}
	}
	if len(passing) == 0 {
		return
	}
	a.out.Assumptions = append(a.out.Assumptions, fmt.Sprintf(
		"A pass-through account sent on at least 90%% of what it received within %d day(s) of receiving it, paid out no more than 110%% of it in all, and deals with at most 3 payers and 3 payees.", a.opt.WindowDays))

	// chains: pass-through accounts that pay each other
	parent := map[accKey]accKey{}
	var find func(k accKey) accKey
	find = func(k accKey) accKey {
		if p, ok := parent[k]; ok && p != k {
			root := find(p)
			parent[k] = root
			return root
		}
		return k
	}
	for k := range passing {
		parent[k] = k
	}
	for _, t := range a.txs {
		x, y := accKey{t.fromA, t.cur}, accKey{t.toA, t.cur}
		if passing[x] && passing[y] {
			parent[find(x)] = find(y)
		}
	}
	groups := map[accKey][]accKey{}
	for k := range passing {
		root := find(k)
		groups[root] = append(groups[root], k)
	}
	roots := make([]accKey, 0, len(groups))
	for r := range groups {
		sort.Slice(groups[r], func(i, j int) bool { return groups[r][i].acc < groups[r][j].acc })
		roots = append(roots, r)
	}
	sort.Slice(roots, func(i, j int) bool { return groups[roots[i]][0].acc < groups[roots[j]][0].acc })

	for _, root := range roots {
		members := groups[root]
		var subjects, evidence, lines []string
		inChain := map[string]bool{}
		for _, k := range members {
			inChain[k.acc] = true
		}
		// who feeds the chain and who it pays out to
		entry, exit := map[string]bool{}, map[string]bool{}
		for _, k := range members {
			for _, t := range byAcc[k].ins {
				if !inChain[t.fromA] {
					entry[t.from] = true
				}
			}
			for _, t := range byAcc[k].outs {
				if !inChain[t.toA] {
					exit[t.to] = true
				}
			}
		}
		ends := fmt.Sprintf(" The money comes from %s and ends with %s.", a.names(sortedKeys(entry)), a.names(sortedKeys(exit)))
		for _, k := range members {
			f := byAcc[k]
			acc := a.ix.accounts[k.acc]
			subjects = append(subjects, acc.Owner)
			evidence = append(evidence, k.acc)
			for _, t := range append(append([]tx{}, f.ins...), f.outs...) {
				evidence = append(evidence, t.t.ID)
			}
			lines = append(lines, fmt.Sprintf("%s of %s: received %s, sent on %s", k.acc, a.name(acc.Owner), money(f.in, k.cur), money(f.out, k.cur)))
		}
		if len(members) >= 2 {
			a.add("layering_chain", "high",
				fmt.Sprintf("Money passes through a chain of %d accounts", len(members)),
				"Each account forwards what it receives almost at once, to the next: "+strings.Join(lines, "; ")+"."+ends+" Moving funds through several hands in days is how their origin is obscured; a legitimate chain of resale or agency looks the same, so check what each payment was for.",
				append(append(subjects, sortedKeys(entry)...), sortedKeys(exit)...), evidence)
			continue
		}
		a.add("pass_through", "medium",
			fmt.Sprintf("Account %s of %s forwards what it receives", members[0].acc, a.name(subjects[0])),
			lines[0]+"."+ends+" The account holds nothing: it looks like a conduit rather than a place where money is used.",
			subjects, evidence)
	}
}

// roundTrips finds money that returns to the entity it left: a chain of
// payments, each no earlier than the one before, of similar size.
func (a *analyzer) roundTrips() {
	outBy := map[string][]int{}
	for i, t := range a.txs {
		if t.dated && t.from != t.to {
			outBy[t.from] = append(outBy[t.from], i)
		}
	}
	limit := time.Duration(cycleDays) * 24 * time.Hour
	seen := map[string]bool{}
	steps, found := 0, 0
	type trip struct {
		origin string
		path   []int
	}
	// every way round the same set of entities is one loop; it is reported once
	loops := map[string][]trip{}
	var loopOrder []string
	truncated := false
	two := big.NewRat(2, 1)

	var path []int
	var walk func(origin string, at string, carried *big.Rat, visited map[string]bool)
	walk = func(origin string, at string, carried *big.Rat, visited map[string]bool) {
		first, last := a.txs[path[0]], a.txs[path[len(path)-1]]
		for _, i := range outBy[at] {
			if truncated {
				return
			}
			n := a.txs[i]
			if n.when.Before(last.when) || n.cur != first.cur {
				continue
			}
			if n.when.Sub(first.when) > limit {
				break // outBy is in date order
			}
			steps++
			if steps > maxCycleSteps {
				truncated = true
				return
			}
			// a similar amount: between half and double what is being carried
			if new(big.Rat).Mul(n.amount, two).Cmp(carried) < 0 || n.amount.Cmp(new(big.Rat).Mul(carried, two)) > 0 {
				continue
			}
			if inPath(path, i) {
				continue
			}
			if n.to == origin {
				ids := make([]string, 0, len(path)+1)
				for _, p := range append(append([]int{}, path...), i) {
					ids = append(ids, a.txs[p].t.ID)
				}
				key := strings.Join(sortedCopy(ids), "|")
				if seen[key] {
					continue
				}
				seen[key] = true
				found++
				if found > maxCycleFindings {
					truncated = true
					return
				}
				full := append(append([]int{}, path...), i)
				var members []string
				for _, p := range full {
					members = append(members, a.txs[p].from)
				}
				loop := strings.Join(sortedCopy(uniq(members)), "|")
				if loops[loop] == nil {
					loopOrder = append(loopOrder, loop)
				}
				loops[loop] = append(loops[loop], trip{origin, full})
				continue
			}
			if len(path) >= maxCycleLen-1 || visited[n.to] {
				continue
			}
			visited[n.to] = true
			path = append(path, i)
			walk(origin, n.to, minRat(carried, n.amount), visited)
			path = path[:len(path)-1]
			delete(visited, n.to)
		}
	}
	for i, t := range a.txs {
		if !t.dated || t.from == t.to {
			continue
		}
		path = []int{i}
		walk(t.from, t.to, t.amount, map[string]bool{t.from: true, t.to: true})
		if truncated {
			break
		}
	}
	duration := func(t trip) time.Duration {
		return a.txs[t.path[len(t.path)-1]].when.Sub(a.txs[t.path[0]].when)
	}
	for _, loop := range loopOrder {
		trips := loops[loop]
		// the quickest trip stands for the loop: it is the one hardest to explain away
		best := trips[0]
		for _, t := range trips[1:] {
			if duration(t) < duration(best) {
				best = t
			}
		}
		var others []string
		for _, t := range trips {
			for _, p := range t.path {
				others = append(others, a.txs[p].t.ID)
			}
		}
		a.reportRoundTrip(best.origin, best.path, len(trips), others)
	}
	if found > 0 {
		a.out.Assumptions = append(a.out.Assumptions, fmt.Sprintf(
			"A round trip is a chain of up to %d payments that returns to the entity it started from within %d days, each payment no earlier than the previous one and between half and double its size.", maxCycleLen, cycleDays))
	}
	if truncated {
		a.out.Notes = append(a.out.Notes, "The search for round trips stopped at its limit; more may exist.")
	}
}

func (a *analyzer) reportRoundTrip(origin string, path []int, trips int, allTx []string) {
	var subjects, evidence, hops []string
	carried := a.txs[path[0]].amount
	for _, p := range path {
		t := a.txs[p]
		subjects = append(subjects, t.from, t.to)
		evidence = append(evidence, t.t.ID)
		hops = append(hops, fmt.Sprintf("%s %s → %s %s (%s)", t.t.Date, t.from, t.to, money(t.amount, t.cur), t.t.ID))
		carried = minRat(carried, t.amount)
	}
	first, last := a.txs[path[0]], a.txs[path[len(path)-1]]
	days := int(last.when.Sub(first.when).Hours() / 24)
	sev := "high"
	if len(path) == 2 {
		sev = "medium"
	}
	repeats := ""
	if trips > 1 {
		repeats = fmt.Sprintf(" The same entities form %d such trips in the data; this is the quickest.", trips)
	}
	a.add("round_trip", sev,
		fmt.Sprintf("%s returns to %s after %d payment(s) in %d day(s)", money(carried, first.cur), a.name(origin), len(path), days),
		"Money leaves and comes back: "+strings.Join(hops, "; ")+"."+repeats+" A round trip can dress one's own money up as a loan, an investment or revenue; a refund or a repaid loan looks the same, so check the stated purposes.",
		subjects, append(evidence, allTx...))
}

func inPath(path []int, i int) bool {
	for _, p := range path {
		if p == i {
			return true
		}
	}
	return false
}

func minRat(x, y *big.Rat) *big.Rat {
	if x.Cmp(y) <= 0 {
		return x
	}
	return y
}

// structuring finds runs of payments kept just under the threshold.
func (a *analyzer) structuring() {
	lo := new(big.Rat).Mul(a.opt.Threshold, big.NewRat(4, 5))
	window := time.Duration(a.opt.WindowDays) * 24 * time.Hour
	type group struct{ role, entity, cur string }
	groups := map[group][]tx{}
	for _, t := range a.txs {
		if !t.dated || t.from == t.to || t.amount.Cmp(lo) < 0 || t.amount.Cmp(a.opt.Threshold) >= 0 {
			continue
		}
		groups[group{"received by", t.to, t.cur}] = append(groups[group{"received by", t.to, t.cur}], t)
		groups[group{"sent by", t.from, t.cur}] = append(groups[group{"sent by", t.from, t.cur}], t)
	}
	keys := make([]group, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	// receivers first: many payers into one account is the stronger pattern,
	// and the payers' own runs are then usually part of it
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].role != keys[j].role {
			return keys[i].role < keys[j].role
		}
		if keys[i].entity != keys[j].entity {
			return keys[i].entity < keys[j].entity
		}
		return keys[i].cur < keys[j].cur
	})
	reported := map[string]bool{}
	flagged := false
	for _, k := range keys {
		list := groups[k]
		// the longest run that fits the window: both ends only move forward
		best, bestFrom := 0, 0
		for i, j := 0, 0; i < len(list); i++ {
			j = max(j, i)
			for j+1 < len(list) && list[j+1].when.Sub(list[i].when) <= window {
				j++
			}
			if n := j - i + 1; n > best {
				best, bestFrom = n, i
			}
		}
		if best < 3 {
			continue
		}
		run := list[bestFrom : bestFrom+best]
		fresh := false
		for _, t := range run {
			if !reported[t.t.ID] {
				fresh = true
			}
		}
		if !fresh {
			continue
		}
		total := new(big.Rat)
		var subjects, evidence []string
		subjects = append(subjects, k.entity)
		for _, t := range run {
			reported[t.t.ID] = true
			total.Add(total, t.amount)
			evidence = append(evidence, t.t.ID)
			subjects = append(subjects, t.from, t.to)
		}
		sev := "medium"
		if best >= 6 {
			sev = "high"
		}
		flagged = true
		a.add("structuring", sev,
			fmt.Sprintf("%d payments just under %s %s %s in %d day(s)", best, money(a.opt.Threshold, k.cur), k.role, a.name(k.entity),
				int(run[len(run)-1].when.Sub(run[0].when).Hours()/24)+1),
			fmt.Sprintf("From %s to %s, each between %s and %s, %s in total. Splitting a sum into amounts below a reporting threshold avoids the report the whole sum would trigger.",
				run[0].t.Date, run[len(run)-1].t.Date, ratString(lo), ratString(a.opt.Threshold), money(total, k.cur)),
			subjects, evidence)
	}
	if flagged {
		a.out.Assumptions = append(a.out.Assumptions, fmt.Sprintf(
			"Structuring is 3 or more payments of 80-100%% of the threshold (%s, the same number in every currency) within %d day(s). Set the threshold of the jurisdiction in question.",
			ratString(a.opt.Threshold), a.opt.WindowDays))
	}
}

// funnels finds entities that gather money from many payers and pass it to a
// few payees, or the reverse.
func (a *analyzer) funnels() {
	flows := a.entityFlows()
	keys := make([]flowKey, 0, len(flows))
	for k := range flows {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].entity != keys[j].entity {
			return keys[i].entity < keys[j].entity
		}
		return keys[i].cur < keys[j].cur
	})
	// the payments of each entity, gathered in one pass
	paid := map[flowKey][]string{}
	for _, t := range a.txs {
		if t.from != t.to {
			paid[flowKey{t.from, t.cur}] = append(paid[flowKey{t.from, t.cur}], t.t.ID)
			paid[flowKey{t.to, t.cur}] = append(paid[flowKey{t.to, t.cur}], t.t.ID)
		}
	}
	evidence := func(entity, cur string) []string { return paid[flowKey{entity, cur}] }
	for _, k := range keys {
		f := flows[k]
		if f.in.Sign() == 0 || f.out.Sign() == 0 {
			continue
		}
		// what went out is what came in: between 80 % and 120 % of it
		if new(big.Rat).Mul(f.out, big.NewRat(5, 1)).Cmp(new(big.Rat).Mul(f.in, big.NewRat(4, 1))) < 0 ||
			new(big.Rat).Mul(f.out, big.NewRat(5, 1)).Cmp(new(big.Rat).Mul(f.in, big.NewRat(6, 1))) > 0 {
			continue
		}
		switch {
		case len(f.payers) >= 5 && len(f.payees) <= 2:
			a.add("funnel", "medium",
				fmt.Sprintf("%s collects from %d payers and passes it to %d payee(s)", a.name(k.entity), len(f.payers), len(f.payees)),
				fmt.Sprintf("Received %s in %d payments, sent %s on. Many small sources feeding one outlet is how collected funds are consolidated.",
					money(f.in, k.cur), f.nIn, money(f.out, k.cur)),
				append([]string{k.entity}, sortedKeys(f.payees)...), evidence(k.entity, k.cur))
		case len(f.payees) >= 5 && len(f.payers) <= 2:
			// a weak lead on its own: every employer with a payroll looks like this
			a.add("fan_out", "low",
				fmt.Sprintf("%s receives from %d payer(s) and spreads it over %d payees", a.name(k.entity), len(f.payers), len(f.payees)),
				fmt.Sprintf("Received %s, sent %s on in %d payments. One source split across many recipients can be a payout network; a payroll looks the same.",
					money(f.in, k.cur), money(f.out, k.cur), f.nOut),
				append([]string{k.entity}, sortedKeys(f.payers)...), evidence(k.entity, k.cur))
		}
	}
}

// ---- ranking, connection, chronology ---------------------------------------------------

func (a *analyzer) central() {
	rows := make([]Central, 0, len(a.c.Entities))
	for _, e := range a.c.Entities {
		neighbours, rel, pay := map[string]bool{}, map[string]bool{}, map[string]bool{}
		for _, ed := range a.adj[e.ID] {
			neighbours[ed.to] = true
			if ed.pay {
				pay[ed.to] = true
			} else {
				rel[ed.to] = true
			}
		}
		if len(neighbours) == 0 {
			continue
		}
		rows = append(rows, Central{Entity: e.ID, Title: e.Title, Type: e.Type, Neighbours: len(neighbours), Relations: len(rel), Counterparties: len(pay)})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Neighbours != rows[j].Neighbours {
			return rows[i].Neighbours > rows[j].Neighbours
		}
		return rows[i].Entity < rows[j].Entity
	})
	if len(rows) > a.opt.Top {
		rows = rows[:a.opt.Top]
	}
	a.out.Central = rows
}

// entityOf maps an entity or account id to the entity.
func (a *analyzer) entityOf(id string) string {
	if a.ix.entities[id] != nil {
		return id
	}
	if acc := a.ix.accounts[id]; acc != nil {
		return acc.Owner
	}
	return ""
}

func (a *analyzer) connection() {
	if a.opt.From == "" && a.opt.To == "" {
		return
	}
	conn := &Connection{From: a.opt.From, To: a.opt.To}
	a.out.Connection = conn
	from, to := a.entityOf(a.opt.From), a.entityOf(a.opt.To)
	switch {
	case from == "" || to == "":
		conn.Note = "from and to must both be ids of entities or accounts in the case"
		return
	case from == to:
		conn.Found = true
		conn.Note = "both ids are the same entity"
		return
	}
	// breadth-first, keeping every parent on a shortest route
	dist := map[string]int{from: 0}
	// parents[node][previous node] holds every edge between the two, so that a
	// pair linked by both a relation and payments is one step, not two routes
	parents := map[string]map[string][]edge{}
	queue := []string{from}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if d, ok := dist[to]; ok && dist[id] >= d {
			break
		}
		for _, ed := range a.adj[id] {
			d, ok := dist[ed.to]
			if !ok {
				dist[ed.to] = dist[id] + 1
				queue = append(queue, ed.to)
				d = dist[id] + 1
			}
			if d == dist[id]+1 {
				if parents[ed.to] == nil {
					parents[ed.to] = map[string][]edge{}
				}
				parents[ed.to][id] = append(parents[ed.to][id], ed)
			}
		}
	}
	if _, ok := dist[to]; !ok {
		conn.Note = "no chain of relations or payments connects them in this case"
		return
	}
	conn.Found = true
	conn.Hops = dist[to]
	var build func(id string, tail []string)
	build = func(id string, tail []string) {
		if len(conn.Paths) >= maxPaths {
			return
		}
		if id == from {
			conn.Paths = append(conn.Paths, append([]string{a.name(from)}, tail...))
			return
		}
		for _, prev := range sortedKeys(parents[id]) {
			var links []string
			for _, ed := range parents[id][prev] {
				links = append(links, fmt.Sprintf("%s [%s]", ed.label, ed.ref))
			}
			step := strings.Join(links, " + ") + " " + a.name(id)
			build(prev, append([]string{step}, tail...))
		}
	}
	build(to, nil)
	if len(conn.Paths) >= maxPaths {
		conn.Note = fmt.Sprintf("the first %d shortest routes are listed", maxPaths)
	}
}

func (a *analyzer) timeline() {
	type item struct {
		when   time.Time
		amount *big.Rat
		TimelineItem
	}
	var fixed, payments []item
	for i, e := range a.c.Events {
		if e.Date.IsZero() {
			continue
		}
		key := e.ID
		if key == "" {
			key = fmt.Sprintf("events[%d]", i)
		}
		fixed = append(fixed, item{e.Date.Time(), nil, TimelineItem{e.Date.String(), "event", e.Title, append([]string{key}, e.Entities...)}})
	}
	for i, r := range a.c.Relations {
		if a.ix.entities[r.From] == nil || a.ix.entities[r.To] == nil {
			continue
		}
		what := fmt.Sprintf("%s — %s — %s", a.name(r.From), r.Type, a.name(r.To))
		if !r.Since.IsZero() {
			fixed = append(fixed, item{r.Since.Time(), nil, TimelineItem{r.Since.String(), "relation starts", what, []string{a.relKeys[i]}}})
		}
		if !r.Until.IsZero() {
			fixed = append(fixed, item{r.Until.Time(), nil, TimelineItem{r.Until.String(), "relation ends", what, []string{a.relKeys[i]}}})
		}
	}
	for _, acc := range a.c.Accounts {
		if !acc.Opened.IsZero() {
			fixed = append(fixed, item{acc.Opened.Time(), nil, TimelineItem{acc.Opened.String(), "account opened", acc.ID + " of " + a.name(acc.Owner), []string{acc.ID}}})
		}
		if !acc.Closed.IsZero() {
			fixed = append(fixed, item{acc.Closed.Time(), nil, TimelineItem{acc.Closed.String(), "account closed", acc.ID + " of " + a.name(acc.Owner), []string{acc.ID}}})
		}
	}
	for _, t := range a.txs {
		if !t.dated {
			continue
		}
		text := fmt.Sprintf("%s pays %s %s", a.name(t.from), a.name(t.to), money(t.amount, t.cur))
		if t.t.Purpose != "" {
			text += ": " + t.t.Purpose
		}
		payments = append(payments, item{t.when, t.amount, TimelineItem{t.t.Date.String(), "payment", text, []string{t.t.ID}}})
	}
	items := fixed
	if room := a.opt.Timeline - len(fixed); room < len(payments) {
		// keep the largest payments
		sort.SliceStable(payments, func(i, j int) bool { return payments[i].amount.Cmp(payments[j].amount) > 0 })
		kept := max(room, 0)
		a.out.Notes = append(a.out.Notes, fmt.Sprintf("timeline keeps the %d largest of %d payments.", kept, len(payments)))
		payments = payments[:kept]
	}
	items = append(items, payments...)
	sort.SliceStable(items, func(i, j int) bool { return items[i].when.Before(items[j].when) })
	if len(items) > a.opt.Timeline {
		a.out.Notes = append(a.out.Notes, fmt.Sprintf("timeline shows the first %d of %d entries.", a.opt.Timeline, len(items)))
		items = items[:a.opt.Timeline]
	}
	for _, it := range items {
		a.out.Timeline = append(a.out.Timeline, it.TimelineItem)
	}
}

// ---- grading and scoring -----------------------------------------------------------------

// gradeOf grades one claim: the best reliability among its sources plus its
// credibility. rank orders grades from strongest (low) to weakest.
func (a *analyzer) gradeOf(id string) (grade string, rank int) {
	cl, ok := a.claims[id]
	if !ok {
		return "", -1
	}
	if len(cl.sources) == 0 {
		return "unsourced", 100
	}
	best := "F"
	for _, sid := range cl.sources {
		if s := a.ix.sources[sid]; s != nil && s.Reliability != "" && s.Reliability < best {
			best = s.Reliability
		}
	}
	cred := cl.credibility
	if cred < 1 || cred > 6 {
		cred = 6
	}
	return fmt.Sprintf("%s%d", best, cred), int(best[0]-'A')*10 + cred
}

var severityRank = map[string]int{"high": 0, "medium": 1, "low": 2, "info": 3}

func (a *analyzer) finish() {
	sort.SliceStable(a.findings, func(i, j int) bool {
		x, y := a.findings[i], a.findings[j]
		if severityRank[x.Severity] != severityRank[y.Severity] {
			return severityRank[x.Severity] < severityRank[y.Severity]
		}
		if x.Kind != y.Kind {
			return x.Kind < y.Kind
		}
		return x.Title < y.Title
	})
	ev := EvidenceStats{ByBestSource: map[string]int{}}
	for id := range a.claims {
		ev.Claims++
		cl := a.claims[id]
		switch len(cl.sources) {
		case 0:
			ev.Unsourced++
		case 1:
			ev.SingleSource++
		}
		if grade, _ := a.gradeOf(id); grade != "unsourced" {
			ev.ByBestSource[grade[:1]]++
		}
	}
	for i := range a.findings {
		f := &a.findings[i]
		f.ID = fmt.Sprintf("f%d", i+1)
		worst, worstRank := "", -1
		for _, id := range f.Evidence {
			if grade, rank := a.gradeOf(id); rank > worstRank {
				worst, worstRank = grade, rank
			}
		}
		if worst == "" {
			worst = "n/a"
		}
		f.WeakestEvidence = worst
		// D or worse, or no source at all
		if worstRank >= 30 {
			ev.WeakFindings++
		}
	}
	a.out.Findings = a.findings
	a.out.Evidence = ev
	a.out.Assumptions = append(a.out.Assumptions,
		"Findings are leads to check, not conclusions: each pattern also has innocent explanations, and the analysis sees only what is in the case.")
	a.scoreTruth()
}

func (a *analyzer) scoreTruth() {
	t := a.c.Truth
	if t == nil {
		return
	}
	flaggedE, flaggedT := map[string]bool{}, map[string]bool{}
	for _, f := range a.findings {
		if f.Severity != "high" && f.Severity != "medium" {
			continue
		}
		for _, id := range f.Subjects {
			flaggedE[id] = true
		}
		for _, id := range f.Evidence {
			if a.ix.kind[id] == "transaction" {
				flaggedT[id] = true
			}
		}
	}
	score := &TruthScore{Typologies: t.Typologies}
	hitE := 0
	inTruth := map[string]bool{}
	for _, id := range t.Entities {
		inTruth[id] = true
		if flaggedE[id] {
			hitE++
		} else {
			score.MissedEntities = append(score.MissedEntities, id)
		}
	}
	hitT := 0
	for _, id := range t.Transactions {
		if flaggedT[id] {
			hitT++
		} else {
			score.MissedTx = append(score.MissedTx, id)
		}
	}
	for _, id := range sortedKeys(flaggedE) {
		if !inTruth[id] {
			score.OutsideScheme = append(score.OutsideScheme, id)
		}
	}
	score.EntitiesFound = fmt.Sprintf("%d/%d", hitE, len(t.Entities))
	score.TransactionsFound = fmt.Sprintf("%d/%d", hitT, len(t.Transactions))
	a.out.Truth = score
}

func uniq(ids []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func sortedCopy(ids []string) []string {
	out := append([]string{}, ids...)
	sort.Strings(out)
	return out
}
