package osint

import (
	"fmt"
	"math/big"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// Report is the result of checking a case. An error means the case is
// inconsistent and an analysis of it would mislead; a warning marks a weak
// spot the investigator should know about.
type Report struct {
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

// OK reports whether the case has no errors.
func (r *Report) OK() bool { return len(r.Errors) == 0 }

func (r *Report) errf(format string, a ...any) {
	r.Errors = append(r.Errors, fmt.Sprintf(format, a...))
}
func (r *Report) warnf(format string, a ...any) {
	r.Warnings = append(r.Warnings, fmt.Sprintf(format, a...))
}

// index resolves ids. Every id lives in one namespace, so a hypothesis can
// cite an entity, a transaction or a source alike.
type index struct {
	kind     map[string]string
	sources  map[string]*Source
	entities map[string]*Entity
	accounts map[string]*Account
	txs      map[string]*Transaction
}

func buildIndex(c *Case) *index {
	ix := &index{kind: map[string]string{}, sources: map[string]*Source{}, entities: map[string]*Entity{},
		accounts: map[string]*Account{}, txs: map[string]*Transaction{}}
	add := func(id, kind string) bool {
		if id == "" {
			return false
		}
		if _, dup := ix.kind[id]; dup {
			return false
		}
		ix.kind[id] = kind
		return true
	}
	for i := range c.Sources {
		if add(c.Sources[i].ID, "source") {
			ix.sources[c.Sources[i].ID] = &c.Sources[i]
		}
	}
	for i := range c.Entities {
		if add(c.Entities[i].ID, "entity") {
			ix.entities[c.Entities[i].ID] = &c.Entities[i]
		}
	}
	for i := range c.Accounts {
		if add(c.Accounts[i].ID, "account") {
			ix.accounts[c.Accounts[i].ID] = &c.Accounts[i]
		}
	}
	for i := range c.Transactions {
		if add(c.Transactions[i].ID, "transaction") {
			ix.txs[c.Transactions[i].ID] = &c.Transactions[i]
		}
	}
	for i := range c.Relations {
		add(c.Relations[i].ID, "relation")
	}
	for i := range c.Events {
		add(c.Events[i].ID, "event")
	}
	for i := range c.Hypotheses {
		add(c.Hypotheses[i].ID, "hypothesis")
	}
	return ix
}

// currencyOf is the currency a transaction is counted in.
func (ix *index) currencyOf(c *Case, t *Transaction) string {
	if t.Currency != "" {
		return t.Currency
	}
	if a := ix.accounts[t.From]; a != nil && a.Currency != "" {
		return a.Currency
	}
	if a := ix.accounts[t.To]; a != nil && a.Currency != "" {
		return a.Currency
	}
	return c.Meta.Currency
}

func (ix *index) accountCurrency(c *Case, a *Account) string {
	if a.Currency != "" {
		return a.Currency
	}
	return c.Meta.Currency
}

func label(kind string, i int, id string) string {
	if id == "" {
		return fmt.Sprintf("%s[%d]", kind, i)
	}
	return fmt.Sprintf("%s[%d] %s", kind, i, id)
}

// Check validates a case.
func Check(c *Case) *Report {
	r := &Report{Errors: []string{}, Warnings: []string{}}
	ix := buildIndex(c)

	if !idRe.MatchString(c.Meta.ID) {
		r.errf("case.id %q: use letters, digits, '_', '.', '-' (1-64 chars)", c.Meta.ID)
	}
	if strings.TrimSpace(c.Meta.Title) == "" {
		r.errf("case.title is empty")
	}

	seen := map[string]string{}
	checkID := func(where, id string, required bool) {
		switch {
		case id == "" && required:
			r.errf("%s: id is empty", where)
		case id == "":
		case !idRe.MatchString(id):
			r.errf("%s: id %q: use letters, digits, '_', '.', '-' (1-64 chars)", where, id)
		default:
			if prev, dup := seen[id]; dup {
				r.errf("%s: id %q is already used by %s", where, id, prev)
				return
			}
			seen[id] = where
		}
	}
	checkGrade := func(where string, credibility int) {
		if credibility < 0 || credibility > 6 {
			r.errf("%s: credibility %d is not 1-6", where, credibility)
		}
	}
	usedSources := map[string]bool{}
	checkSources := func(where string, ids []string) {
		for _, id := range ids {
			if ix.kind[id] != "source" {
				r.errf("%s: source %q is not in sources", where, id)
				continue
			}
			usedSources[id] = true
		}
	}
	unsourced := map[string][]string{}
	noteUnsourced := func(kind, id string, sources []string) {
		if len(sources) == 0 {
			unsourced[kind] = append(unsourced[kind], id)
		}
	}

	for i, s := range c.Sources {
		w := label("sources", i, s.ID)
		checkID(w, s.ID, true)
		if strings.TrimSpace(s.Title) == "" {
			r.errf("%s: title is empty", w)
		}
		if s.Reliability != "" && !slices.Contains([]string{"A", "B", "C", "D", "E", "F"}, s.Reliability) {
			r.errf("%s: reliability %q is not A-F", w, s.Reliability)
		}
		if s.Reliability == "" {
			r.warnf("%s: no reliability grade (A-F); it counts as F, cannot be judged", w)
		}
		if s.Kind != "" && !slices.Contains(sourceKinds, s.Kind) {
			r.warnf("%s: kind %q is not one of %s", w, s.Kind, strings.Join(sourceKinds, ", "))
		}
	}

	titles := map[string][]string{}
	for i, e := range c.Entities {
		w := label("entities", i, e.ID)
		checkID(w, e.ID, true)
		if !slices.Contains(entityTypes, e.Type) {
			r.errf("%s: type %q is not one of %s", w, e.Type, strings.Join(entityTypes, ", "))
		}
		if strings.TrimSpace(e.Title) == "" {
			r.errf("%s: title is empty", w)
		}
		for _, name := range sortedKeys(e.Metrics) {
			if !metricRe.MatchString(name) {
				r.errf("%s: metric %q: use lower-case letters, digits and '_' (1-40 chars)", w, name)
			}
		}
		if e.Picture != "" && !pictureOK(e.Picture) {
			r.errf("%s: picture %q must be an https:// URL or a relative path to an image file (%s)", w, e.Picture, strings.Join(pictureExts, ", "))
		}
		checkGrade(w, e.Credibility)
		checkSources(w, e.Sources)
		noteUnsourced("entities", e.ID, e.Sources)
		key := e.Type + "|" + normalize(e.Title)
		titles[key] = append(titles[key], e.ID)
	}
	for _, key := range sortedKeys(titles) {
		if ids := titles[key]; len(ids) > 1 {
			r.warnf("entities %s have the same type and title: one entity entered twice? Merge them or link them with same_as", strings.Join(ids, ", "))
		}
	}

	linked := map[string]bool{}
	shares := map[string]*big.Rat{}
	for i, rel := range c.Relations {
		w := label("relations", i, rel.ID)
		checkID(w, rel.ID, false)
		ok := true
		for _, end := range []struct{ name, id string }{{"from", rel.From}, {"to", rel.To}} {
			if ix.kind[end.id] != "entity" {
				r.errf("%s: %s %q is not an entity", w, end.name, end.id)
				ok = false
			}
		}
		if ok && rel.From == rel.To {
			r.errf("%s: from and to are the same entity %q", w, rel.From)
		}
		if ok {
			linked[rel.From], linked[rel.To] = true, true
		}
		switch {
		case rel.Type == "":
			r.errf("%s: type is empty", w)
		case !slices.Contains(relationTypes, rel.Type):
			r.warnf("%s: type %q is not a known relation type (%s); the analysis treats it as a plain link", w, rel.Type, strings.Join(relationTypes, ", "))
		}
		if rel.Share != nil {
			switch {
			case rel.Type != "owns":
				r.warnf("%s: share is only used on owns relations", w)
			case rel.Share.Rat().Sign() <= 0 || rel.Share.Rat().Cmp(big.NewRat(100, 1)) > 0:
				r.errf("%s: share %s is not a percentage in (0, 100]", w, rel.Share)
			case ok:
				if shares[rel.To] == nil {
					shares[rel.To] = new(big.Rat)
				}
				shares[rel.To].Add(shares[rel.To], rel.Share.Rat())
			}
		}
		if rel.Type == "owns" {
			if rel.Share == nil {
				r.warnf("%s: owns without share; the stake is unknown and is left out of beneficial-ownership figures", w)
			}
			if e := ix.entities[rel.To]; e != nil && e.Type == "person" {
				r.errf("%s: %q is a person and cannot be owned", w, rel.To)
			}
		}
		if !rel.Since.IsZero() && !rel.Until.IsZero() && rel.Until.Time().Before(rel.Since.Time()) {
			r.errf("%s: until %s is before since %s", w, rel.Until, rel.Since)
		}
		checkGrade(w, rel.Credibility)
		checkSources(w, rel.Sources)
		noteUnsourced("relations", relationName(i, rel), rel.Sources)
	}
	for _, id := range sortedKeys(shares) {
		if shares[id].Cmp(big.NewRat(100, 1)) > 0 {
			r.warnf("entity %s: recorded ownership stakes add up to %s%%, more than 100%%: stakes from different dates or conflicting sources?", id, ratString(shares[id]))
		}
	}

	for i, a := range c.Accounts {
		w := label("accounts", i, a.ID)
		checkID(w, a.ID, true)
		if ix.kind[a.Owner] != "entity" {
			r.errf("%s: owner %q is not an entity", w, a.Owner)
		} else {
			linked[a.Owner] = true
		}
		if a.Kind != "" && !slices.Contains(accountKinds, a.Kind) {
			r.errf("%s: kind %q is not one of %s", w, a.Kind, strings.Join(accountKinds, ", "))
		}
		if ix.accountCurrency(c, &c.Accounts[i]) == "" {
			r.warnf("%s: no currency (set it on the account or as case.currency)", w)
		}
		if !a.Opened.IsZero() && !a.Closed.IsZero() && a.Closed.Time().Before(a.Opened.Time()) {
			r.errf("%s: closed %s is before opened %s", w, a.Closed, a.Opened)
		}
		checkGrade(w, a.Credibility)
		checkSources(w, a.Sources)
		noteUnsourced("accounts", a.ID, a.Sources)
	}

	dupTx := map[string][]string{}
	for i := range c.Transactions {
		t := &c.Transactions[i]
		w := label("transactions", i, t.ID)
		checkID(w, t.ID, true)
		from, to := ix.accounts[t.From], ix.accounts[t.To]
		if from == nil {
			r.errf("%s: from %q is not an account", w, t.From)
		}
		if to == nil {
			r.errf("%s: to %q is not an account", w, t.To)
		}
		if from != nil && to != nil && t.From == t.To {
			r.errf("%s: from and to are the same account %q", w, t.From)
		}
		switch {
		case !t.Amount.IsSet():
			r.errf("%s: amount is missing", w)
		case t.Amount.Rat().Sign() <= 0:
			r.errf("%s: amount %s must be positive (direction is from → to)", w, t.Amount)
		}
		cur := ix.currencyOf(c, t)
		if cur == "" {
			r.warnf("%s: no currency (set it on the transaction, its accounts or as case.currency)", w)
		}
		for _, a := range []*Account{from, to} {
			if a == nil {
				continue
			}
			if ac := ix.accountCurrency(c, a); ac != "" && cur != "" && ac != cur {
				r.warnf("%s: currency %s differs from account %s (%s); amounts are counted in %s", w, cur, a.ID, ac, cur)
			}
			if !t.Date.IsZero() {
				if !a.Opened.IsZero() && t.Date.Time().Before(a.Opened.Time()) {
					r.warnf("%s: dated %s, before account %s was opened (%s)", w, t.Date, a.ID, a.Opened)
				}
				if !a.Closed.IsZero() && t.Date.Time().After(a.Closed.Time().AddDate(0, 0, 1)) {
					r.warnf("%s: dated %s, after account %s was closed (%s)", w, t.Date, a.ID, a.Closed)
				}
			}
		}
		if t.Date.IsZero() {
			r.warnf("%s: no date; it is left out of time-based patterns (pass-through, structuring, round trips)", w)
		}
		checkGrade(w, t.Credibility)
		checkSources(w, t.Sources)
		noteUnsourced("transactions", t.ID, t.Sources)
		if from != nil && to != nil && t.Amount.IsSet() {
			key := t.From + "|" + t.To + "|" + t.Amount.String() + "|" + t.Date.String()
			dupTx[key] = append(dupTx[key], t.ID)
		}
	}
	for _, key := range sortedKeys(dupTx) {
		if ids := dupTx[key]; len(ids) > 1 {
			r.warnf("transactions %s have the same accounts, amount and date: one payment seen in two sources?", strings.Join(ids, ", "))
		}
	}

	for i, e := range c.Events {
		w := label("events", i, e.ID)
		checkID(w, e.ID, false)
		if strings.TrimSpace(e.Title) == "" {
			r.errf("%s: title is empty", w)
		}
		if e.Date.IsZero() {
			r.errf("%s: date is missing", w)
		}
		for _, id := range e.Entities {
			if ix.kind[id] != "entity" {
				r.errf("%s: %q is not an entity", w, id)
				continue
			}
			linked[id] = true
		}
		checkGrade(w, e.Credibility)
		checkSources(w, e.Sources)
		noteUnsourced("events", eventName(i, e), e.Sources)
	}

	for i, h := range c.Hypotheses {
		w := label("hypotheses", i, h.ID)
		checkID(w, h.ID, true)
		if strings.TrimSpace(h.Statement) == "" {
			r.errf("%s: statement is empty", w)
		}
		if h.Status != "" && !slices.Contains(hypothesisState, h.Status) {
			r.errf("%s: status %q is not one of %s", w, h.Status, strings.Join(hypothesisState, ", "))
		}
		for _, id := range h.Evidence {
			if _, ok := ix.kind[id]; !ok {
				r.errf("%s: evidence %q is not an id in this case", w, id)
			}
		}
		if h.Status != "" && h.Status != "open" && len(h.Evidence) == 0 {
			r.warnf("%s: marked %s with no evidence listed", w, h.Status)
		}
	}

	if t := c.Truth; t != nil {
		if !c.Meta.Synthetic {
			r.warnf("truth is set but case.synthetic is not: a known answer belongs to a synthetic case only")
		}
		for _, id := range t.Entities {
			if ix.kind[id] != "entity" {
				r.errf("truth.entities: %q is not an entity", id)
			}
		}
		for _, id := range t.Transactions {
			if ix.kind[id] != "transaction" {
				r.errf("truth.transactions: %q is not a transaction", id)
			}
		}
	}

	for _, kind := range []string{"entities", "relations", "accounts", "transactions", "events"} {
		if ids := unsourced[kind]; len(ids) > 0 {
			r.warnf("%d %s have no source: %s. An unsourced claim cannot be verified or graded", len(ids), kind, sample(ids, 8))
		}
	}
	var unused []string
	for _, s := range c.Sources {
		if s.ID != "" && !usedSources[s.ID] {
			unused = append(unused, s.ID)
		}
	}
	if len(unused) > 0 {
		r.warnf("%d sources are cited by nothing: %s", len(unused), sample(unused, 8))
	}
	var isolated []string
	for _, e := range c.Entities {
		if e.ID != "" && !linked[e.ID] {
			isolated = append(isolated, e.ID)
		}
	}
	if len(isolated) > 0 {
		r.warnf("%d entities have no relation, account or event: %s", len(isolated), sample(isolated, 8))
	}
	return r
}

var pictureExts = []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg"}

// pictureOK reports whether a picture is an https URL or a relative path to an
// image file that stays inside the working directory.
func pictureOK(p string) bool {
	if strings.HasPrefix(p, "https://") {
		return len(p) > len("https://")
	}
	if strings.Contains(p, "://") || filepath.IsAbs(p) {
		return false
	}
	clean := filepath.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return false
	}
	return slices.Contains(pictureExts, strings.ToLower(filepath.Ext(clean)))
}

func relationName(i int, rel Relation) string {
	if rel.ID != "" {
		return rel.ID
	}
	return fmt.Sprintf("#%d %s-%s->%s", i, rel.From, rel.Type, rel.To)
}

func eventName(i int, e Event) string {
	if e.ID != "" {
		return e.ID
	}
	return fmt.Sprintf("#%d %s", i, e.Date)
}

// sample lists the first n ids and says how many were left out.
func sample(ids []string, n int) string {
	if len(ids) <= n {
		return strings.Join(ids, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(ids[:n], ", "), len(ids)-n)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// normalize folds case and whitespace so that the same value written twice
// compares equal.
func normalize(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}
