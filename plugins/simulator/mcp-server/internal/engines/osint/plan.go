package osint

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Plan says how to build a case as a digital twin in Simulator: which forms,
// actors, links, accounts and transfers to create, each with a stable ref so
// that building it twice changes nothing. The plan writes nothing itself; the
// curated tools named in Steps carry it out.
type Plan struct {
	Case         string         `json:"case"`
	Synthetic    bool           `json:"synthetic"`
	RefPrefix    string         `json:"refPrefix"`
	Layer        string         `json:"layerTitle"`
	Forms        []PlanForm     `json:"forms"`
	Actors       []PlanActor    `json:"actors"`
	Links        []PlanLink     `json:"links"`
	AccountPairs []PlanPair     `json:"accountPairs"`
	Accounts     []PlanAccount  `json:"accounts"`
	Transfers    []PlanTransfer `json:"transfers"`
	Chronology   []string       `json:"chronology,omitempty"`
	Steps        []string       `json:"steps"`
	Notes        []string       `json:"notes"`
}

// PlanForm is one actor template, one per entity type in the case.
type PlanForm struct {
	Title      string      `json:"title"`
	Ref        string      `json:"ref"`
	EntityType string      `json:"entityType"`
	Color      string      `json:"color"`
	Fields     []PlanField `json:"fields"`
}

// PlanField is one field of a form; an actor's data is keyed by ID.
type PlanField struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// PlanActor is one entity as a graph node.
type PlanActor struct {
	Ref         string `json:"ref"`
	CaseID      string `json:"caseId"`
	Form        string `json:"form"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Color       string `json:"color"`
	Grade       string `json:"evidenceGrade"`
	// Data are the actor's field values, keyed by form field id.
	Data map[string]string `json:"data"`
	// Picture is the picture given in the case; empty means a generated one.
	Picture    string `json:"picture,omitempty"`
	EntityType string `json:"entityType"`
	GeoName    string `json:"geoName,omitempty"`
	X          int    `json:"x"`
	Y          int    `json:"y"`

	// set while the plan is being built in Simulator
	actorID string
	laID    int
}

// PlanLink is one relation as a named edge.
type PlanLink struct {
	SourceRef string `json:"sourceRef"`
	TargetRef string `json:"targetRef"`
	Name      string `json:"name"`
	CaseID    string `json:"caseId"`
}

// PlanPair is one (account name, currency) pair to bootstrap.
type PlanPair struct {
	AccountName  string `json:"accountName"`
	CurrencyName string `json:"currencyName"`
}

// PlanAccount is one account on its owner's actor.
type PlanAccount struct {
	CaseID       string `json:"caseId"`
	ActorRef     string `json:"actorRef"`
	AccountName  string `json:"accountName"`
	CurrencyName string `json:"currencyName"`
	Label        string `json:"label,omitempty"`

	sides sides // the Simulator accounts, set while the plan is being built
}

// PlanTransfer is one transaction as a transfer between two planned accounts.
type PlanTransfer struct {
	Ref     string `json:"ref"`
	From    string `json:"fromAccount"`
	To      string `json:"toAccount"`
	Amount  string `json:"amount"`
	Comment string `json:"comment"`
}

// Planned positions: one column per entity type, one row per entity. Both
// steps are multiples of the layer's grid cell.
const (
	planColStep = 400
	planRowStep = 150
)

// fieldPrefix starts the id of every field the OSINT forms have.
const fieldPrefix = "item_osint_"

// commonFields are on every OSINT form.
var commonFields = [][2]string{{"case", "Case id"}, {"id", "Id in the case"}, {"grade", "Evidence grade"},
	{"aliases", "Also known as"}, {"sources", "Sources"}, {"picture", "Picture source"}}

// typeFields are the attributes that get a field of their own, per entity
// type: an entity's attrs with these keys fill them. Any other attribute
// stays in the actor's description only.
var typeFields = map[string][][2]string{
	"person": {{"dob", "Date of birth"}, {"nationality", "Nationality"}, {"country", "Country"}, {"occupation", "Occupation"},
		{"phone", "Phone"}, {"email", "Email"}, {"address", "Address"}, {"passport", "Passport"}, {"tax_id", "Tax id"}},
	"organization": {{"legal_form", "Legal form"}, {"reg_number", "Registration number"}, {"tax_id", "Tax id"}, {"country", "Country"},
		{"incorporated", "Incorporated"}, {"status", "Status"}, {"industry", "Industry"}, {"address", "Address"},
		{"registered_address", "Registered address"}, {"registered_agent", "Registered agent"}, {"website", "Website"},
		{"phone", "Phone"}, {"email", "Email"}},
	"asset":    {{"kind", "Kind"}, {"identifier", "Identifier"}, {"value", "Value"}, {"acquired", "Acquired"}, {"country", "Country"}, {"address", "Address"}},
	"location": {{"address", "Address"}, {"city", "City"}, {"country", "Country"}},
	"document": {{"kind", "Kind"}, {"number", "Number"}, {"date", "Date"}, {"issuer", "Issuer"}},
	"digital":  {{"kind", "Kind"}, {"value", "Value"}, {"registered", "Registered"}, {"provider", "Provider"}},
	"other":    {{"kind", "Kind"}},
}

// formFieldsOf lists the fields of the form for one entity type.
func formFieldsOf(entityType string) []PlanField {
	var out []PlanField
	for _, f := range append(append([][2]string{}, commonFields...), typeFields[entityType]...) {
		out = append(out, PlanField{ID: fieldPrefix + f[0], Title: f[1]})
	}
	return out
}

// actorData fills an entity's form fields from the case.
func actorData(c *Case, ix *index, e *Entity) map[string]string {
	data := map[string]string{fieldPrefix + "case": c.Meta.ID, fieldPrefix + "id": e.ID,
		fieldPrefix + "grade": claimGrade(ix, e.Sources, e.Credibility)}
	if len(e.Aliases) > 0 {
		data[fieldPrefix+"aliases"] = strings.Join(e.Aliases, "; ")
	}
	var titles []string
	for _, id := range e.Sources {
		if s := ix.sources[id]; s != nil {
			titles = append(titles, s.Title)
		}
	}
	if len(titles) > 0 {
		data[fieldPrefix+"sources"] = strings.Join(titles, "; ")
	}
	for _, f := range typeFields[e.Type] {
		if v := strings.TrimSpace(e.Attrs[f[0]]); v != "" {
			data[fieldPrefix+f[0]] = v
		}
	}
	return data
}

var formColors = map[string]string{"person": "#3b82f6", "organization": "#f59e0b", "asset": "#10b981",
	"location": "#8b5cf6", "document": "#64748b", "digital": "#06b6d4", "other": "#94a3b8"}

// BuildPlan turns a checked case into a plan.
func BuildPlan(c *Case) *Plan {
	ix := buildIndex(c)
	prefix := "osint:" + c.Meta.ID + ":"
	p := &Plan{Case: c.Meta.ID, Synthetic: c.Meta.Synthetic, RefPrefix: prefix, Layer: "OSINT · " + c.Meta.Title,
		Forms: []PlanForm{}, Actors: []PlanActor{}, Links: []PlanLink{}, AccountPairs: []PlanPair{}, Accounts: []PlanAccount{},
		Transfers: []PlanTransfer{}}

	formTitle := func(t string) string { return "OSINT " + strings.ToUpper(t[:1]) + t[1:] }
	rows := map[string]int{}
	for col, t := range entityTypes {
		used := false
		for _, e := range c.Entities {
			if e.Type != t {
				continue
			}
			used = true
			a := PlanActor{Ref: prefix + e.ID, CaseID: e.ID, Form: formTitle(t), Title: e.Title,
				Description: describeEntity(c, ix, &e), Color: formColors[t], Grade: claimGrade(ix, e.Sources, e.Credibility),
				Data: actorData(c, ix, &e), Picture: e.Picture, EntityType: t,
				X: col * planColStep, Y: rows[t] * planRowStep}
			if t == "location" {
				a.GeoName = e.Title
			}
			p.Actors = append(p.Actors, a)
			rows[t]++
		}
		if used {
			p.Forms = append(p.Forms, PlanForm{Title: formTitle(t), Ref: "osint-" + t, EntityType: t, Color: formColors[t], Fields: formFieldsOf(t)})
		}
	}

	for i, r := range c.Relations {
		if ix.entities[r.From] == nil || ix.entities[r.To] == nil {
			continue
		}
		name := r.Type
		if r.Type == "owns" && r.Share != nil {
			name = fmt.Sprintf("owns %s%%", r.Share)
		}
		id := r.ID
		if id == "" {
			id = fmt.Sprintf("relations[%d]", i)
		}
		p.Links = append(p.Links, PlanLink{SourceRef: prefix + r.From, TargetRef: prefix + r.To, Name: name, CaseID: id})
	}

	// One Simulator account is (actor, account name, currency): a second
	// account of the same kind and currency on one owner needs its own name.
	pairs := map[PlanPair]bool{}
	taken := map[string]int{}
	for _, a := range c.Accounts {
		if ix.entities[a.Owner] == nil {
			continue
		}
		kind := a.Kind
		if kind == "" {
			kind = "other"
		}
		cur := ix.accountCurrency(c, &a)
		slot := a.Owner + "|" + kind + "|" + cur
		taken[slot]++
		name := "OSINT " + kind
		if taken[slot] > 1 {
			name = fmt.Sprintf("OSINT %s #%d", kind, taken[slot])
		}
		pair := PlanPair{AccountName: name, CurrencyName: cur}
		if !pairs[pair] {
			pairs[pair] = true
			p.AccountPairs = append(p.AccountPairs, pair)
		}
		label := strings.TrimSpace(a.Title + " " + a.Number)
		p.Accounts = append(p.Accounts, PlanAccount{CaseID: a.ID, ActorRef: prefix + a.Owner, AccountName: name, CurrencyName: cur, Label: label})
	}

	// transfers in date order; undated ones last, in file order
	type row struct {
		t   *Transaction
		cur string
	}
	var txs []row
	for i := range c.Transactions {
		t := &c.Transactions[i]
		if ix.accounts[t.From] == nil || ix.accounts[t.To] == nil || !t.Amount.IsSet() || t.Amount.Rat().Sign() <= 0 {
			continue
		}
		txs = append(txs, row{t, ix.currencyOf(c, t)})
	}
	sort.SliceStable(txs, func(i, j int) bool {
		x, y := txs[i].t.Date, txs[j].t.Date
		if x.IsZero() != y.IsZero() {
			return !x.IsZero()
		}
		return x.Time().Before(y.Time())
	})
	crossCurrency := 0
	for _, r := range txs {
		t := r.t
		parts := []string{}
		if !t.Date.IsZero() {
			parts = append(parts, t.Date.String())
		}
		if t.Purpose != "" {
			parts = append(parts, t.Purpose)
		}
		if from, to := ix.entities[ix.accounts[t.From].Owner], ix.entities[ix.accounts[t.To].Owner]; from != nil && to != nil {
			parts = append(parts, from.Title+" → "+to.Title)
		}
		parts = append(parts, "case tx "+t.ID)
		p.Transfers = append(p.Transfers, PlanTransfer{Ref: prefix + t.ID, From: t.From, To: t.To, Amount: t.Amount.String(),
			Comment: strings.Join(parts, " · ")})
		if ix.accountCurrency(c, ix.accounts[t.From]) != ix.accountCurrency(c, ix.accounts[t.To]) {
			crossCurrency++
		}
	}
	for _, e := range c.Events {
		p.Chronology = append(p.Chronology, strings.TrimSpace(e.Date.String()+" — "+e.Title))
	}

	p.Steps = []string{
		"osintCaseBuild carries out this whole plan in one call and can be repeated safely. The steps below are what it does, for doing them by hand with the curated tools.",
		"Confirm the target workspace and this plan's counts with the user before any write.",
		"Forms: find each form by its ref; createForm the missing ones with the fields listed in forms[].fields, and add any of those fields an existing form lacks.",
		"Layer: find the graph and the layer by the refs <refPrefix>graph and <refPrefix>layer, or create them (forms Graphs and Layers) and link the graph to the layer.",
		"Actors: for each entry of actors, getActorByRef(formId, ref); update it when found, otherwise createActor(formId, ref, title, description, color, data). Set its picture with uploadActorPicture (the case's picture, or any placeholder). Keep a map ref → actor id.",
		"Placement: manageLayerActors on the layer with one create item per actor that is not on it yet, at its x/y.",
		"Links: massLink in batches of up to 50, each {source, target, name}; one edge per pair of actors, named after all the relations between them. Then place the edges on the layer.",
		"Accounts: createAccountPair for each entry of accountPairs, then createAccount(actorId, nameId, currencyId, ignoreIfExist=true) for each entry of accounts. It gives each account a debit side and a credit side.",
		"Transfers: for each entry of transfers, in order, skip it when getTransferByRef finds it; otherwise createTransfer(from=[{accountId: the payer's debit side, amount}], to=[{accountId: the payee's credit side, amount}], ref, comment).",
		"Chronology and findings: post them as comment reactions on the layer or on the case's event actor (see the simulator-reactions skill).",
	}
	p.Notes = []string{
		"An account's balance in Simulator is its credit side minus its debit side: what it received minus what it paid.",
		"Simulator stamps a transfer with the time it is made. The date of the original payment is kept in the transfer comment, so balances are right but turnover-by-period in Simulator reflects the day of the build.",
		"Refs start with " + prefix + " — search by that prefix to find everything this case created.",
	}
	if crossCurrency > 0 {
		p.Notes = append(p.Notes, fmt.Sprintf("%d transfers are between accounts in different currencies; record each leg in its own account's currency or skip them.", crossCurrency))
	}
	if !c.Meta.Synthetic {
		p.Notes = append(p.Notes, "This case is about real people or organizations: build it only in a workspace whose members may see it, and keep unverified claims marked as such in the descriptions.")
	}
	return p
}

// claimGrade grades one claim: the best reliability among its sources plus
// its credibility, or "unsourced".
func claimGrade(ix *index, sources []string, credibility int) string {
	if len(sources) == 0 {
		return "unsourced"
	}
	best := "F"
	for _, id := range sources {
		if s := ix.sources[id]; s != nil && s.Reliability != "" && s.Reliability < best {
			best = s.Reliability
		}
	}
	if credibility < 1 || credibility > 6 {
		credibility = 6
	}
	return fmt.Sprintf("%s%d", best, credibility)
}

// describeEntity renders what the case knows about an entity as a markdown
// actor description.
func describeEntity(c *Case, ix *index, e *Entity) string {
	var b strings.Builder
	b.WriteString("[md]\n")
	if c.Meta.Synthetic {
		b.WriteString("**Synthetic** — invented by the case generator.\n\n")
	}
	fmt.Fprintf(&b, "**Case:** %s · **id:** %s · **type:** %s\n", c.Meta.ID, e.ID, e.Type)
	if len(e.Aliases) > 0 {
		fmt.Fprintf(&b, "\n**Also known as:** %s\n", plainText(strings.Join(e.Aliases, "; ")))
	}
	if len(e.Attrs) > 0 {
		b.WriteString("\n| attribute | value |\n|---|---|\n")
		for _, k := range sortedKeys(e.Attrs) {
			fmt.Fprintf(&b, "| %s | %s |\n", cell(k), cell(e.Attrs[k]))
		}
	}
	if e.Note != "" {
		fmt.Fprintf(&b, "\n%s\n", plainText(e.Note))
	}
	if len(e.Sources) == 0 {
		b.WriteString("\n**Sources:** none recorded — unverified.\n")
	} else {
		b.WriteString("\n**Sources:**\n")
		for _, id := range e.Sources {
			s := ix.sources[id]
			if s == nil {
				continue
			}
			grade := s.Reliability
			if grade == "" {
				grade = "F"
			}
			line := fmt.Sprintf("- %s (reliability %s)", plainText(s.Title), grade)
			if s.URL != "" {
				line += " — " + plainText(s.URL)
			}
			b.WriteString(line + "\n")
		}
	}
	if e.Credibility >= 1 && e.Credibility <= 6 {
		fmt.Fprintf(&b, "\n**Information credibility:** %d of 6 (1 = confirmed)\n", e.Credibility)
	}
	b.WriteString("[/md]")
	return b.String()
}

// cell keeps a value inside one markdown table cell.
func cell(s string) string {
	s = strings.ReplaceAll(plainText(s), "|", "\\|")
	return strings.Join(strings.Fields(s), " ")
}

var mdTagRe = regexp.MustCompile(`(?i)\[/?md\]`)

// plainText removes the markers that open or close the markdown block of a
// description. Case text comes from outside sources; without this a value
// could end the block and be rendered as BBCode after it.
func plainText(s string) string { return mdTagRe.ReplaceAllString(s, "") }
