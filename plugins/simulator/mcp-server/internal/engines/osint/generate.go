package osint

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"time"
)

// Typologies the generator can plant in a synthetic case.
var Typologies = []string{"layering", "round_trip", "smurfing", "nominee"}

// GenOptions configure a synthetic case. The same options always give the
// same case.
type GenOptions struct {
	Typologies []string // which schemes to plant (default: all)
	Seed       uint64
	Noise      int    // ordinary people and companies around the schemes (0: default 12; negative: none; max 300)
	Currency   string // default USD
	Start      Date   // first day of the case (default 2025-01-06)
}

const (
	defaultNoise = 12
	maxNoise     = 300
)

// Generate builds a synthetic investigation: ordinary background activity
// with the chosen schemes hidden in it, and the known answer in Truth. Every
// name in it is invented.
func Generate(opt GenOptions) (*Case, error) {
	if len(opt.Typologies) == 0 {
		opt.Typologies = Typologies
	}
	for _, t := range opt.Typologies {
		if !slices.Contains(Typologies, t) {
			return nil, fmt.Errorf("unknown typology %q: use %s", t, strings.Join(Typologies, ", "))
		}
	}
	if opt.Noise == 0 {
		opt.Noise = defaultNoise
	}
	if opt.Noise < 0 {
		opt.Noise = 0 // a negative value asks for no background at all
	}
	if opt.Noise > maxNoise {
		return nil, fmt.Errorf("noise must be at most %d", maxNoise)
	}
	if opt.Currency == "" {
		opt.Currency = "USD"
	}
	start := opt.Start.Time()
	if opt.Start.IsZero() {
		start = time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
	}
	g := &gen{rng: rand.New(rand.NewPCG(opt.Seed, opt.Seed^0x9e3779b97f4a7c15)),
		attrRng: rand.New(rand.NewPCG(opt.Seed, opt.Seed^0x51ed270b0b2f1f3d)), start: start, used: map[string]bool{},
		c: &Case{Meta: Meta{
			ID:        fmt.Sprintf("synthetic-%d", opt.Seed),
			Title:     fmt.Sprintf("Synthetic investigation (seed %d)", opt.Seed),
			Question:  "Which entities and payments in this data form a scheme, what kind of scheme is it, and who benefits?",
			Synthetic: true,
			Currency:  opt.Currency,
			Note:      "Every person, company, address, account and payment here is invented by the generator. Any resemblance to real ones is accidental.",
		}, Truth: &Truth{Note: "The planted schemes. An analysis is scored against this list; it never reads it to find anything."}}}
	g.c.Sources = []Source{
		{ID: "s-registry", Title: "Company registry extract (synthetic)", Kind: "registry", Reliability: "B"},
		{ID: "s-bank", Title: "Bank statements (synthetic)", Kind: "bank", Reliability: "A"},
		{ID: "s-media", Title: "Press report (synthetic)", Kind: "media", Reliability: "C"},
		{ID: "s-tip", Title: "Anonymous tip (synthetic)", Kind: "human", Reliability: "E"},
	}
	g.background(opt.Noise)
	for _, t := range opt.Typologies {
		if slices.Contains(g.c.Truth.Typologies, t) {
			continue
		}
		g.c.Truth.Typologies = append(g.c.Truth.Typologies, t)
		switch t {
		case "layering":
			g.layering()
		case "round_trip":
			g.roundTrip()
		case "smurfing":
			g.smurfing()
		case "nominee":
			g.nominee()
		}
	}
	return g.c, nil
}

type gen struct {
	rng *rand.Rand
	// attrRng fills descriptive attributes. It is a stream of its own, so that
	// adding an attribute never changes which entities and payments a seed gives.
	attrRng            *rand.Rand
	c                  *Case
	start              time.Time
	used               map[string]bool
	nP, nO, nL, nA, nT int
	nR, nE, nH         int
}

func (g *gen) between(lo, hi int) int { return lo + g.rng.IntN(hi-lo+1) }

func (g *gen) pick(list []string) string { return list[g.rng.IntN(len(list))] }

func (g *gen) day(offset int) Date { return dayDate(g.start.AddDate(0, 0, offset)) }

var (
	nameStarts   = []string{"Al", "Bren", "Cor", "Dav", "El", "Fen", "Gar", "Hul", "Iv", "Jor", "Kel", "Lum", "Mar", "Nev", "Or", "Pel", "Quin", "Ros", "Sil", "Tor", "Ul", "Ver", "Wen", "Yar", "Zel"}
	nameEnds     = []string{"an", "en", "is", "or", "a", "ia", "ek", "us", "on", "in"}
	surnameEnds  = []string{"vek", "dane", "moor", "sten", "lund", "rak", "wick", "mere", "holt", "garde"}
	companyKinds = []string{"Trading", "Holdings", "Logistics", "Capital", "Consult", "Invest", "Supply", "Partners", "Systems", "Resources"}
	companyForms = []string{"Ltd", "LLC", "LP", "Inc"}
	streets      = []string{"Harbour Row", "Mill Lane", "Quay Street", "Foundry Road", "Orchard Way", "Station Square", "Bridge End", "Market Hill"}
	cities       = []string{"Port Avel", "Nordhaven", "Sant Erro", "Kessel Bay", "Thornwick"}
	countries    = []string{"Avelan Republic", "Nordhaven Union", "Erro Federation", "Kessel Isles", "Thornwick Commonwealth"}
	cityCountry  = map[string]string{"Port Avel": "Avelan Republic", "Nordhaven": "Nordhaven Union", "Sant Erro": "Erro Federation",
		"Kessel Bay": "Kessel Isles", "Thornwick": "Thornwick Commonwealth"}
	occupations = []string{"accountant", "driver", "sales manager", "engineer", "lawyer", "shop owner", "consultant", "logistics clerk", "student", "unemployed"}
	industries  = []string{"wholesale trade", "construction", "transport", "consulting", "agriculture", "real estate", "IT services", "metals"}
	purposes    = []string{"consulting services", "loan agreement", "equipment supply", "marketing services", "advance payment", "agency fee"}
)

func (g *gen) unique(make func() string) string {
	for {
		s := make()
		if !g.used[s] {
			g.used[s] = true
			return s
		}
	}
}

func (g *gen) personName() string {
	return g.unique(func() string {
		return g.pick(nameStarts) + g.pick(nameEnds) + " " + g.pick(nameStarts) + g.pick(surnameEnds)
	})
}

func (g *gen) companyName() string {
	return g.unique(func() string {
		return g.pick(nameStarts) + g.pick(nameEnds) + g.pick(surnameEnds) + " " + g.pick(companyKinds) + " " + g.pick(companyForms)
	})
}

func (g *gen) address() string {
	return g.unique(func() string {
		return fmt.Sprintf("%d %s, %s", g.between(1, 240), g.pick(streets), g.pick(cities))
	})
}

func (g *gen) attrPick(list []string) string { return list[g.attrRng.IntN(len(list))] }

func (g *gen) person(src string) string {
	g.nP++
	id := fmt.Sprintf("p%d", g.nP)
	name := g.personName()
	born := time.Date(1955+g.attrRng.IntN(45), time.Month(1+g.attrRng.IntN(12)), 1+g.attrRng.IntN(28), 0, 0, 0, 0, time.UTC)
	attrs := map[string]string{
		"dob":         born.Format("2006-01-02"),
		"nationality": g.attrPick(countries),
		"occupation":  g.attrPick(occupations),
		"phone":       fmt.Sprintf("+000 555 %04d", g.nP),
		"email":       strings.ToLower(strings.ReplaceAll(name, " ", ".")) + "@mail.example",
	}
	g.c.Entities = append(g.c.Entities, Entity{ID: id, Type: "person", Title: name, Attrs: attrs, Sources: []string{src}, Credibility: 2})
	return id
}

func (g *gen) org(attrs map[string]string) string {
	g.nO++
	id := fmt.Sprintf("o%d", g.nO)
	name := g.companyName()
	if attrs == nil {
		attrs = map[string]string{}
	}
	founded := g.start.AddDate(-g.attrRng.IntN(12), -g.attrRng.IntN(12), -g.attrRng.IntN(28))
	for k, v := range map[string]string{
		"legal_form":   name[strings.LastIndex(name, " ")+1:],
		"reg_number":   fmt.Sprintf("SYN-%06d", 100000+g.nO*37+g.attrRng.IntN(30)),
		"incorporated": founded.Format("2006-01-02"),
		"status":       "active",
		"industry":     g.attrPick(industries),
	} {
		attrs[k] = v
	}
	if attrs["country"] == "" {
		attrs["country"] = g.attrPick(countries)
	}
	g.c.Entities = append(g.c.Entities, Entity{ID: id, Type: "organization", Title: name, Attrs: attrs,
		Sources: []string{"s-registry"}, Credibility: 1})
	return id
}

func (g *gen) location() string {
	g.nL++
	id := fmt.Sprintf("l%d", g.nL)
	addr := g.address()
	city := addr[strings.LastIndex(addr, ", ")+2:]
	g.c.Entities = append(g.c.Entities, Entity{ID: id, Type: "location", Title: addr,
		Attrs: map[string]string{"address": addr, "city": city, "country": cityCountry[city]}, Sources: []string{"s-registry"}, Credibility: 1})
	return id
}

func (g *gen) account(owner string) string {
	g.nA++
	id := fmt.Sprintf("a%d", g.nA)
	g.c.Accounts = append(g.c.Accounts, Account{ID: id, Owner: owner, Kind: "bank",
		Number: fmt.Sprintf("SYN%010d", g.rng.Int64N(10_000_000_000)), Sources: []string{"s-bank"}, Credibility: 1})
	return id
}

// rel adds a relation; share is a percentage, 0 for none.
func (g *gen) rel(from, to, typ string, share int, src string, cred int) {
	g.nR++
	r := Relation{ID: fmt.Sprintf("r%d", g.nR), From: from, To: to, Type: typ, Sources: []string{src}, Credibility: cred}
	if share > 0 {
		d := mustDecimal(fmt.Sprint(share))
		r.Share = &d
	}
	g.c.Relations = append(g.c.Relations, r)
}

// pay adds a payment of cents/100 on the given day and returns its id.
func (g *gen) pay(from, to string, cents int64, dayOffset int, purpose string) string {
	g.nT++
	id := fmt.Sprintf("t%d", g.nT)
	g.c.Transactions = append(g.c.Transactions, Transaction{ID: id, From: from, To: to,
		Amount: mustDecimal(fmt.Sprintf("%d.%02d", cents/100, cents%100)), Date: g.day(dayOffset), Purpose: purpose,
		Sources: []string{"s-bank"}, Credibility: 1})
	return id
}

func (g *gen) event(dayOffset int, title, src string, cred int, entities ...string) {
	g.nE++
	g.c.Events = append(g.c.Events, Event{ID: fmt.Sprintf("e%d", g.nE), Date: g.day(dayOffset), Title: title,
		Entities: entities, Sources: []string{src}, Credibility: cred})
}

func (g *gen) hypothesis(statement string) {
	g.nH++
	g.c.Hypotheses = append(g.c.Hypotheses, Hypothesis{ID: fmt.Sprintf("h%d", g.nH), Statement: statement, Status: "open"})
}

func (g *gen) scheme(entities []string, txs []string) {
	g.c.Truth.Entities = append(g.c.Truth.Entities, entities...)
	g.c.Truth.Transactions = append(g.c.Truth.Transactions, txs...)
}

// background adds ordinary activity: companies owned and run by one person
// each, staff on a payroll, invoices between companies and rent. It is built
// so that it does not form the patterns the schemes do: invoices only go one
// way down the list of companies, nobody is paid from the account they pay
// into, and no amount sits just under the threshold.
func (g *gen) background(n int) {
	if n == 0 {
		return
	}
	nCompanies := max(2, n/3)
	if nCompanies > n {
		nCompanies = n
	}
	nPeople := n - nCompanies
	var companies, companyAcc, people, peopleAcc []string
	for range nCompanies {
		addr := g.address()
		o := g.org(map[string]string{"address": addr})
		owner := g.person("s-registry")
		g.account(owner)
		g.rel(owner, o, "owns", 100, "s-registry", 1)
		g.rel(owner, o, "director", 0, "s-registry", 1)
		companies = append(companies, o)
		companyAcc = append(companyAcc, g.account(o))
	}
	for range nPeople {
		p := g.person("s-media")
		people = append(people, p)
		peopleAcc = append(peopleAcc, g.account(p))
	}
	landlord := len(companies) - 1
	// staff: up to 4 per company, none at the landlord
	for i, p := range people {
		employer := i % max(landlord, 1)
		if landlord == 0 || i/max(landlord, 1) >= 4 {
			continue
		}
		g.rel(p, companies[employer], "employee", 0, "s-media", 3)
		salary := int64(g.between(2500, 6000)) * 100
		for month := range 3 {
			g.pay(companyAcc[employer], peopleAcc[i], salary, 25+30*month, "salary")
		}
	}
	for i := range people {
		rent := int64(g.between(800, 1500)) * 100
		for month := range 3 {
			g.pay(peopleAcc[i], companyAcc[landlord], rent, 3+30*month, "rent")
		}
	}
	for i := 0; i < landlord; i++ {
		for j := i + 1; j < landlord; j++ {
			if g.rng.IntN(2) == 0 {
				continue
			}
			for range g.between(1, 3) {
				g.pay(companyAcc[i], companyAcc[j], int64(g.between(12_000, 60_000))*100, g.between(1, 85), "invoice")
			}
		}
	}
}

// fee takes a cut of 1-2 %, as each hop of a scheme does.
func (g *gen) fee(cents int64) int64 { return cents * int64(100-g.between(1, 2)) / 100 }

// layering: money from one company moves through three shells in a few days
// and reaches a person who owns the last shell through a holding.
func (g *gen) layering() {
	d := g.between(10, 30)
	origin := g.org(map[string]string{"address": g.address()})
	beneficiary, nominee := g.person("s-registry"), g.person("s-registry")
	holding := g.org(map[string]string{"address": g.address()})
	office := g.location()
	var shells, accs []string
	for range 3 {
		s := g.org(nil)
		shells = append(shells, s)
		accs = append(accs, g.account(s))
		g.rel(s, office, "registered_at", 0, "s-registry", 1)
		g.rel(nominee, s, "director", 0, "s-registry", 1)
	}
	g.rel(nominee, shells[0], "owns", 100, "s-registry", 2)
	g.rel(nominee, shells[1], "owns", 100, "s-tip", 4)
	g.rel(holding, shells[2], "owns", 100, "s-registry", 1)
	g.rel(beneficiary, holding, "owns", 100, "s-registry", 2)
	originAcc, beneficiaryAcc := g.account(origin), g.account(beneficiary)

	amount := int64(g.between(400_000, 900_000)) * 100
	var txs []string
	chain := append(append([]string{originAcc}, accs...), beneficiaryAcc)
	for i := 0; i+1 < len(chain); i++ {
		txs = append(txs, g.pay(chain[i], chain[i+1], amount, d, g.pick(purposes)))
		amount = g.fee(amount)
		d += g.between(1, 2)
	}
	g.event(d-20, "Three companies incorporated at one address on the same day", "s-registry", 1, shells...)
	g.event(d+5, "Press reports a large unexplained outflow from the company", "s-media", 3, origin)
	g.hypothesis("Funds of one company were moved through shell companies to a hidden beneficiary.")
	g.scheme(append([]string{origin, beneficiary, nominee, holding}, shells...), txs)
}

// roundTrip: a company lends money abroad and receives it back as an
// investment from a second company its owner secretly holds.
func (g *gen) roundTrip() {
	d := g.between(15, 40)
	company := g.org(map[string]string{"address": g.address()})
	owner := g.person("s-registry")
	x, y := g.org(map[string]string{"country": "offshore"}), g.org(map[string]string{"country": "offshore"})
	g.rel(owner, company, "owns", 100, "s-registry", 1)
	g.rel(owner, company, "director", 0, "s-registry", 1)
	g.rel(owner, y, "owns", 100, "s-tip", 4)
	a, ax, ay := g.account(company), g.account(x), g.account(y)
	amount := int64(g.between(250_000, 600_000)) * 100
	t1 := g.pay(a, ax, amount, d, "loan agreement")
	amount = g.fee(amount)
	d += g.between(3, 10)
	t2 := g.pay(ax, ay, amount, d, "assignment of claim")
	amount = g.fee(amount)
	d += g.between(5, 15)
	t3 := g.pay(ay, a, amount, d, "investment in share capital")
	g.event(d+2, "Company announces a foreign investor", "s-media", 3, company, y)
	g.hypothesis("The foreign investment in the company is the company's own money returned to it.")
	g.scheme([]string{company, owner, x, y}, []string{t1, t2, t3})
}

// smurfing: several people each make deposits just under the threshold into
// one collector's account, who wires the total on.
func (g *gen) smurfing() {
	d := g.between(20, 50)
	collector := g.person("s-media")
	organizer := g.org(map[string]string{"address": g.address()})
	collectorAcc, organizerAcc := g.account(collector), g.account(organizer)
	scheme := []string{collector, organizer}
	var txs []string
	var total int64
	for range g.between(6, 8) {
		mule := g.person("s-tip")
		acc := g.account(mule)
		scheme = append(scheme, mule)
		for range 2 {
			cents := int64(g.between(8_200, 9_900)) * 100
			total += cents
			txs = append(txs, g.pay(acc, collectorAcc, cents, d+g.between(0, 5), "private transfer"))
		}
	}
	txs = append(txs, g.pay(collectorAcc, organizerAcc, g.fee(total), d+6, "goods"))
	g.event(d+8, "Bank files a report on unusual deposits", "s-tip", 4, collector)
	g.hypothesis("Cash was split into deposits under the reporting threshold and consolidated through one account.")
	g.scheme(scheme, txs)
}

// nominee: one person is the director of five companies with one address and
// one registration agent, and no recorded owner.
func (g *gen) nominee() {
	d := g.between(5, 60)
	front := g.person("s-registry")
	office := g.location()
	agent := g.companyName()
	scheme := []string{front}
	for range 5 {
		s := g.org(map[string]string{"registered_agent": agent})
		scheme = append(scheme, s)
		g.rel(front, s, "director", 0, "s-registry", 1)
		g.rel(s, office, "registered_at", 0, "s-registry", 1)
	}
	g.event(d, "Five companies change director on the same day", "s-registry", 1, scheme[1:]...)
	g.hypothesis("One person fronts a group of companies whose real owners are not recorded.")
	g.scheme(scheme, nil)
}
