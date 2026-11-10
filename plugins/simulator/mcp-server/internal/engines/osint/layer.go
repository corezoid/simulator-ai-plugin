package osint

import (
	"context"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"
)

// Limits of reading one layer.
const (
	maxLayerNodes        = 2000
	maxAccountPages      = 200 // pages of 100 transactions per account side
	transactionsPageSize = 100
)

// FromLayer reads a layer of a workspace as a case: its actors become
// entities, its links relations, the accounts on its actors accounts, and the
// transfers between those accounts transactions. Any layer can be read, not
// only one built from a case; what a layer does not hold — the source of each
// claim — is said in the notes.
func FromLayer(ctx context.Context, layerID string) (*Case, []string, error) {
	b := &builder{ctx: ctx, base: ecore.BuildBaseURLForContext(ctx), res: &BuildResult{LayerID: layerID}}
	c := &Case{Meta: Meta{ID: "layer-" + layerID[:8], Title: "Layer " + layerID}}
	var notes []string

	nodes, err := layerPage[struct {
		ID        string         `json:"id"`
		Title     string         `json:"title"`
		FormTitle string         `json:"formTitle"`
		Data      map[string]any `json:"data"`
	}](b, "nodes")
	if err != nil {
		return nil, nil, err
	}
	if len(nodes) == 0 {
		return nil, nil, fmt.Errorf("layer %s has no actors on it, or is not a layer", layerID)
	}
	if len(nodes) > maxLayerNodes {
		return nil, nil, fmt.Errorf("the layer has %d actors; at most %d are read", len(nodes), maxLayerNodes)
	}

	// A layer does not hold where a claim came from. An actor built from a case
	// keeps its grade, which is carried as a source of that reliability; the
	// rest is attributed to the layer itself, a source that cannot be judged.
	const layerSource = "s-layer"
	c.Sources = []Source{{ID: layerSource, Title: "Simulator layer " + layerID, Kind: "corporate", Reliability: "F",
		Note: "The layer as it stands. It does not record where each claim came from."}}
	gradeSource := map[string]string{}
	sourceFor := func(grade string) ([]string, int) {
		if len(grade) != 2 || grade[0] < 'A' || grade[0] > 'F' || grade[1] < '1' || grade[1] > '6' {
			return []string{layerSource}, 0
		}
		letter := grade[:1]
		if gradeSource[letter] == "" {
			gradeSource[letter] = "s-grade-" + strings.ToLower(letter)
			c.Sources = append(c.Sources, Source{ID: gradeSource[letter], Title: "Sources graded " + letter + " in the case this layer was built from",
				Kind: "other", Reliability: letter})
		}
		return []string{gradeSource[letter]}, int(grade[1] - '0')
	}

	entityOf := map[string]string{} // actor uuid → entity id
	typeOf := map[string]string{}   // entity id → type
	used := map[string]bool{}
	unknownForms := map[string]bool{}
	for i, n := range nodes {
		str := func(key string) string { s, _ := n.Data[fieldPrefix+key].(string); return s }
		id := str("id")
		if !idRe.MatchString(id) || used[id] {
			id = fmt.Sprintf("n%d", i+1)
		}
		used[id] = true
		entityOf[n.ID] = id
		e := Entity{ID: id, Title: strings.TrimSpace(n.Title), Type: layerEntityType(n.FormTitle), Attrs: map[string]string{}}
		if e.Title == "" {
			e.Title = id
		}
		if e.Type == "other" && !strings.HasPrefix(n.FormTitle, "OSINT ") && n.FormTitle != "" {
			e.Attrs["kind"] = n.FormTitle
			unknownForms[n.FormTitle] = true
		}
		for _, f := range typeFields[e.Type] {
			if v := str(f[0]); v != "" {
				e.Attrs[f[0]] = v
			}
		}
		if a := str("aliases"); a != "" {
			e.Aliases = strings.Split(a, "; ")
		}
		e.Sources, e.Credibility = sourceFor(str("grade"))
		if len(e.Attrs) == 0 {
			e.Attrs = nil
		}
		typeOf[id] = e.Type
		c.Entities = append(c.Entities, e)
	}
	if len(unknownForms) > 0 {
		notes = append(notes, fmt.Sprintf("Actors of the forms %s are read as type \"other\": rules that need to know a person from an organization (beneficial owners, directorships, conflicts of interest) do not see them.",
			strings.Join(sortedKeys(unknownForms), ", ")))
	}

	edges, err := layerPage[struct {
		Source string `json:"source"`
		Target string `json:"target"`
		Name   string `json:"name"`
	}](b, "edges")
	if err != nil {
		return nil, nil, err
	}
	for _, e := range edges {
		from, to := entityOf[e.Source], entityOf[e.Target]
		if from == "" || to == "" || from == to {
			continue
		}
		for _, r := range relationsOf(e.Name) {
			r.From, r.To, r.Sources = from, to, []string{layerSource}
			if r.Type == "owns" && typeOf[to] == "person" {
				r.Type, r.Share, r.Note = "other", nil, e.Name
			}
			c.Relations = append(c.Relations, r)
		}
	}

	// accounts: the two sides Simulator keeps for one (name, currency) on an actor
	type side struct{ account, incomeType string }
	sideOf := map[string]side{} // simulator account id → case account
	accountN := 0
	for _, n := range nodes {
		var rows []struct {
			ID           string `json:"id"`
			NameID       string `json:"nameId"`
			AccountName  string `json:"accountName"`
			CurrencyID   int    `json:"currencyId"`
			CurrencyName string `json:"currencyName"`
			IncomeType   string `json:"incomeType"`
			Type         string `json:"type"`
			IsSystem     bool   `json:"isSystem"`
			CounterType  string `json:"counterType"`
		}
		if err := b.get("/accounts/"+ecore.Seg(n.ID)+"?limit=100", &rows); err != nil {
			return nil, nil, fmt.Errorf("read the accounts of %q: %w", n.Title, err)
		}
		byPair := map[string]string{}
		for _, r := range rows {
			// the platform's own tallies (edges, reactions, …) are not the actor's money
			if r.IsSystem || r.CounterType != "amount" || (r.Type != "" && r.Type != "fact") {
				continue
			}
			pair := fmt.Sprintf("%s|%d", r.NameID, r.CurrencyID)
			if byPair[pair] == "" {
				accountN++
				byPair[pair] = fmt.Sprintf("acc%d", accountN)
				kind := strings.TrimPrefix(strings.ToLower(r.AccountName), "osint ")
				if i := strings.Index(kind, " #"); i > 0 {
					kind = kind[:i]
				}
				acc := Account{ID: byPair[pair], Owner: entityOf[n.ID], Title: r.AccountName, Currency: r.CurrencyName, Sources: []string{layerSource}}
				for _, k := range accountKinds {
					if k == kind {
						acc.Kind = k
					}
				}
				c.Accounts = append(c.Accounts, acc)
			}
			sideOf[r.ID] = side{byPair[pair], r.IncomeType}
		}
	}

	// transfers: the debit leg names the payer's account, the credit leg the payee's
	type leg struct {
		from, to string
		amount   Decimal
		when     int64
		comment  string
	}
	legs := map[string]*leg{}
	var order []string
	for simID, s := range sideOf {
		for page := 0; page < maxAccountPages; page++ {
			var rows []struct {
				Amount       jsonDecimal `json:"amount"`
				Comment      string      `json:"comment"`
				TransferID   string      `json:"transferId"`
				Type         string      `json:"type"`
				OriginalDate int64       `json:"originalDate"`
				CreatedAt    int64       `json:"createdAt"`
			}
			path := fmt.Sprintf("/transactions/list/%s?limit=%d&offset=%d", ecore.Seg(simID), transactionsPageSize, page*transactionsPageSize)
			if err := b.get(path, &rows); err != nil {
				return nil, nil, fmt.Errorf("read the transactions of account %s: %w", s.account, err)
			}
			for _, r := range rows {
				if r.TransferID == "" || r.Type != "completed" || !r.Amount.d.IsSet() {
					continue
				}
				l := legs[r.TransferID]
				if l == nil {
					l = &leg{amount: r.Amount.d, comment: r.Comment, when: r.OriginalDate}
					if l.when == 0 {
						l.when = r.CreatedAt
					}
					legs[r.TransferID] = l
					order = append(order, r.TransferID)
				}
				if s.incomeType == "debit" {
					l.from = s.account
				} else {
					l.to = s.account
				}
			}
			if len(rows) < transactionsPageSize {
				break
			}
			if len(c.Transactions)+len(legs) > maxTransactions {
				return nil, nil, fmt.Errorf("the layer's accounts hold more than %d transfers", maxTransactions)
			}
		}
	}
	offLayer := 0
	usedTx := map[string]bool{}
	// map iteration above is unordered: put the transfers in date order
	sort.SliceStable(order, func(i, j int) bool {
		a, b := legs[order[i]], legs[order[j]]
		if a.when != b.when {
			return a.when < b.when
		}
		return order[i] < order[j]
	})
	for _, id := range order {
		l := legs[id]
		if l.from == "" || l.to == "" || l.from == l.to || l.amount.Rat().Sign() <= 0 {
			offLayer++
			continue
		}
		t := Transaction{From: l.from, To: l.to, Amount: l.amount, Sources: []string{layerSource}}
		purpose, caseID, grade, day := splitComment(l.comment)
		t.Purpose = purpose
		t.ID = caseID
		if !idRe.MatchString(t.ID) || used[t.ID] || usedTx[t.ID] {
			t.ID = fmt.Sprintf("tr%d", len(c.Transactions)+1)
		}
		usedTx[t.ID] = true
		t.Sources, t.Credibility = sourceFor(grade)
		if !day.IsZero() {
			t.Date = day
		} else if l.when > 0 {
			t.Date = dayDate(time.UnixMilli(l.when).UTC().Truncate(24 * time.Hour))
		}
		c.Transactions = append(c.Transactions, t)
	}
	if offLayer > 0 {
		notes = append(notes, fmt.Sprintf("%d transfers have one side outside this layer and are left out.", offLayer))
	}
	notes = append(notes, "A layer does not record sources: relations are attributed to the layer itself (reliability F), so findings that rest on them are graded as weak. For graded findings analyse the case file the layer was built from.")
	return c, notes, nil
}

// layerEntityType maps a form to an entity type: the OSINT forms by name, the
// platform's forms for people, anything else to "other".
func layerEntityType(form string) string {
	if t, ok := strings.CutPrefix(form, "OSINT "); ok {
		t = strings.ToLower(t)
		for _, known := range entityTypes {
			if t == known {
				return t
			}
		}
	}
	switch strings.ToLower(form) {
	case "human", "user", "person", "people", "employee", "employees":
		return "person"
	case "company", "companies", "organization", "organisation", "organizations", "legal entity":
		return "organization"
	}
	return "other"
}

// jsonDecimal reads a JSON number as an exact decimal.
type jsonDecimal struct{ d Decimal }

func (j *jsonDecimal) UnmarshalJSON(b []byte) error {
	text := strings.Trim(string(b), `"`)
	if d, ok := newDecimal(text); ok {
		j.d = d
		return nil
	}
	// an exponent form, which the case format does not accept but JSON may use
	if r, ok := new(big.Rat).SetString(text); ok {
		j.d = Decimal{r}
	}
	return nil
}

var ownsRe = regexp.MustCompile(`^owns\s+(\d+(?:\.\d+)?)\s*%$`)

// relationsOf reads the relations out of an edge's name. An edge built from a
// case is named after its relations ("owns 60% · director"); any other name
// is a plain link that keeps the name as its note.
func relationsOf(name string) []Relation {
	name = strings.TrimSpace(name)
	if name == "" {
		return []Relation{{Type: "other"}}
	}
	var out []Relation
	for _, part := range strings.Split(name, " · ") {
		part = strings.TrimSpace(part)
		if m := ownsRe.FindStringSubmatch(part); m != nil {
			if share, ok := newDecimal(m[1]); ok && share.Rat().Sign() > 0 && share.Rat().Cmp(mustDecimal("100").Rat()) <= 0 {
				out = append(out, Relation{Type: "owns", Share: &share})
				continue
			}
		}
		known := false
		for _, t := range relationTypes {
			if part == t {
				known = true
			}
		}
		if known {
			out = append(out, Relation{Type: part})
		} else {
			out = append(out, Relation{Type: "other", Note: part})
		}
	}
	return out
}

var (
	commentTxRe    = regexp.MustCompile(`^case tx ([A-Za-z0-9][A-Za-z0-9_.-]{0,63})$`)
	commentGradeRe = regexp.MustCompile(`^grade ([A-F][1-6]|unsourced)$`)
	commentDateRe  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	commentPairRe  = regexp.MustCompile(` → `)
)

// splitComment reads back what a build put into a transfer's comment: the
// date, the purpose, the id in the case and the grade. A comment written by
// someone else is the purpose as it stands.
func splitComment(comment string) (purpose, caseID, grade string, day Date) {
	parts := strings.Split(comment, " · ")
	var rest []string
	built := false
	for i, p := range parts {
		p = strings.TrimSpace(p)
		switch {
		case commentTxRe.MatchString(p):
			caseID, built = commentTxRe.FindStringSubmatch(p)[1], true
		case commentGradeRe.MatchString(p):
			grade = commentGradeRe.FindStringSubmatch(p)[1]
		case i == 0 && commentDateRe.MatchString(p):
			day, _ = parseDate(p)
		default:
			rest = append(rest, p)
		}
	}
	if !built {
		return strings.TrimSpace(comment), "", "", Date{}
	}
	// "Payer → Payee" was added for the reader; it is not part of the purpose
	var kept []string
	for _, p := range rest {
		if !commentPairRe.MatchString(p) {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, " · "), caseID, grade, day
}
