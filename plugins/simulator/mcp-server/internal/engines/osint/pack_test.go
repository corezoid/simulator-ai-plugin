package osint

import (
	"slices"
	"strings"
	"testing"
)

func titles(a *Analysis, kind string) []string {
	var out []string
	for _, f := range a.Findings {
		if f.Kind == kind {
			out = append(out, f.Title)
		}
	}
	return out
}

func TestShippedPacksLoadAndFindWhatTheySay(t *testing.T) {
	if got := ShippedPacks(); !slices.Equal(got, []string{"due-diligence", "financial-investigation", "market-map"}) {
		t.Fatalf("shipped packs: %v", got)
	}
	fin, err := ShippedPack("financial-investigation")
	if err != nil {
		t.Fatal(err)
	}
	dd, err := ShippedPack("due-diligence")
	if err != nil {
		t.Fatal(err)
	}

	// the kickback: the consultant was incorporated three months before it was paid
	kick := loadExample(t, "procurement-kickback")
	an := Analyze(kick, Options{Pack: fin})
	recent := titles(an, "recently-incorporated-recipient")
	if len(recent) != 4 || !strings.Contains(strings.Join(recent, "\n"), "Greywater Consulting LP was paid 360000 EUR") ||
		strings.Contains(strings.Join(recent, "\n"), "Kessel Bay Construct") {
		t.Errorf("recently incorporated: %v", recent)
	}
	if got := titles(an, "money-to-unowned-organization"); len(got) != 2 {
		t.Errorf("unowned recipients: %v", got)
	}
	// every built-in finding is still there, and each rule's finding names what it rests on
	for _, k := range []string{"conflict_of_interest", "layering_chain", "indirect_beneficial_owner"} {
		finding(t, an, k)
	}
	f := finding(t, an, "recently-incorporated-recipient")
	if len(f.Evidence) != 1 || len(f.Subjects) != 2 || f.WeakestEvidence != "A1" || f.Severity != "medium" {
		t.Errorf("rule finding: %+v", f)
	}

	// due diligence on the same case: the ownership that rests on a leak, and no money-movement findings
	an = Analyze(kick, Options{Pack: dd})
	weak := titles(an, "owner-not-evidenced")
	if len(weak) != 2 || !strings.Contains(weak[0]+weak[1], "Halden Family Holdings Ltd in Greywater Consulting LP rests on weak evidence (D3)") {
		t.Errorf("owner not evidenced: %v", weak)
	}
	if count(an, "pass_through")+count(an, "structuring")+count(an, "funnel") != 0 || count(an, "layering_chain") == 0 {
		t.Errorf("built-in choice: %v", kinds(an))
	}
	if f := finding(t, an, "unknown_owner"); f.Severity != "medium" {
		t.Errorf("unknown_owner severity %s, want the pack's medium", f.Severity)
	}
	if got := titles(an, "director-of-many"); len(got) != 1 || !strings.Contains(got[0], "Ilsa Vorren has a controlling role in 2") {
		t.Errorf("director of many: %v", got)
	}
	// the trade case: the offshore owner
	an = Analyze(loadExample(t, "trade-overinvoicing"), Options{Pack: dd})
	if got := titles(an, "offshore-owner"); len(got) != 2 || !strings.Contains(strings.Join(got, "\n"), "Calder Reef Holdings Inc, registered in Kessel Isles, holds 70% of Quillon Trade Finance LP") {
		t.Errorf("offshore owner: %v", got)
	}
	// the pack's 10 % bar reports the director's 18 % of the lender, which the default 25 % does not
	if count(an, "indirect_beneficial_owner") <= count(Analyze(loadExample(t, "trade-overinvoicing"), Options{}), "indirect_beneficial_owner") {
		t.Errorf("min_share of the pack was not applied: %v", titles(an, "indirect_beneficial_owner"))
	}
	// a threshold given to the analysis wins over the pack's
	fraud := loadExample(t, "investment-fraud-cashout")
	if count(Analyze(fraud, Options{Pack: fin}), "structuring") != 3 || count(Analyze(fraud, Options{Pack: fin, Threshold: mustDecimal("3000").Rat()}), "structuring") != 0 {
		t.Error("threshold: the pack's 10000 or the call's 3000 was not applied")
	}
}

func TestOwnRulesOnEveryKindOfItem(t *testing.T) {
	pack, err := LoadPack([]byte(`
pack: {id: mine}
builtin: {unknown_owner: "off"}
rules:
  - {id: big-org, for: entity, when: "type == 'organization' and received > 1000000 and payers == 1", severity: high,
     title: "{title} got {received} {currency} from one payer"}
  - {id: family, for: relation, when: "type == 'family'", severity: low, title: "{source.title} and {target.title} are family"}
  - {id: thin-account, for: account, when: "received > 0 and sent / received > 0.95", title: "{id} of {owner.title} keeps under 5%"}
  - {id: salary, for: transaction, when: "purpose == 'salary'", severity: low, title: "{payer.title} pays {payee.title} {amount}"}
  - {id: broken, for: entity, when: "received / sent > 2", title: "x"}
`))
	if err != nil {
		t.Fatal(err)
	}
	an := Analyze(loadExample(t, "procurement-kickback"), Options{Pack: pack})
	if got := titles(an, "big-org"); len(got) != 1 || got[0] != "Kessel Bay Construct Ltd got 2000000 EUR from one payer" {
		t.Errorf("entity rule: %v", got)
	}
	if got := titles(an, "family"); len(got) != 1 || got[0] != "Tobren Halvik and Marenna Halvik are family" {
		t.Errorf("relation rule: %v", got)
	}
	if got := titles(an, "thin-account"); len(got) != 2 {
		t.Errorf("account rule: %v", got)
	}
	if got := titles(an, "salary"); len(got) != 3 || got[0] != "Port Avel Roads Authority pays Tobren Halvik 4200" {
		t.Errorf("transaction rule: %v", got)
	}
	// a rule that fails on some items (a division by zero) is reported, and the others still run
	if count(an, "unknown_owner") != 0 || !strings.Contains(strings.Join(an.Notes, "\n"), "rule broken could not be evaluated") {
		t.Errorf("notes: %v", an.Notes)
	}
	if !strings.Contains(strings.Join(an.Assumptions, "\n"), `pack "mine"`) {
		t.Errorf("assumptions: %v", an.Assumptions)
	}
}

func TestPackIsCheckedBeforeItRuns(t *testing.T) {
	for name, src := range map[string]string{
		"a typo in a key":                      "pack: {id: p}\nrulez: []\n",
		"no id":                                "pack: {title: P}\n",
		"an unknown built-in":                  "pack: {id: p}\nbuiltin: {round_tripp: off}\n",
		"a bad built-in setting":               "pack: {id: p}\nbuiltin: {round_trip: critical}\n",
		"a rule named as a built-in":           "pack: {id: p}\nrules:\n  - {id: round_trip, for: entity, when: \"true\", title: T}\n",
		"an unknown kind of item":              "pack: {id: p}\nrules:\n  - {id: r, for: wallet, when: \"true\", title: T}\n",
		"a condition that does not parse":      "pack: {id: p}\nrules:\n  - {id: r, for: entity, when: \"received >\", title: T}\n",
		"a field an entity does not have":      "pack: {id: p}\nrules:\n  - {id: r, for: entity, when: \"amount > 5\", title: T}\n",
		"a field of the wrong kind in a title": "pack: {id: p}\nrules:\n  - {id: r, for: entity, when: \"received > 5\", title: \"{payer.title}\"}\n",
		"an unknown function":                  "pack: {id: p}\nrules:\n  - {id: r, for: entity, when: \"exec('rm') > 5\", title: T}\n",
		"a bad severity":                       "pack: {id: p}\nrules:\n  - {id: r, for: entity, when: \"received > 5\", severity: urgent, title: T}\n",
		"a negative threshold":                 "pack: {id: p}\nsettings: {threshold: -1}\n",
	} {
		if _, err := LoadPack([]byte(src)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := ShippedPack("../../etc/passwd"); err == nil {
		t.Error("a path was taken as a pack id")
	}
}

func TestAnalyzeToolTakesAPack(t *testing.T) {
	src := string(must(MarshalCase(loadExample(t, "procurement-kickback"))))
	out, isErr := call(t, handleAnalyze, map[string]any{"case": src, "pack": "due-diligence"})
	if isErr || out["pack"] != "due-diligence" {
		t.Fatalf("shipped pack: %v", out)
	}
	out, isErr = call(t, handleAnalyze, map[string]any{"case": src, "pack": "pack: {id: mine}\nrules:\n  - {id: any, for: entity, when: \"type == 'asset'\", title: \"{title}\"}\n"})
	found := false
	for _, f := range out["analysis"].(map[string]any)["findings"].([]any) {
		if f.(map[string]any)["kind"] == "any" {
			found = true
		}
	}
	if isErr || !found {
		t.Errorf("own pack as text: %v", out["pack"])
	}
	for _, bad := range []string{"no-such-pack", "pack: {id: x}\nrules:\n  - {id: r, for: entity, when: \"nope > 1\", title: T}\n"} {
		if _, isErr := call(t, handleAnalyze, map[string]any{"case": src, "pack": bad}); !isErr {
			t.Errorf("pack %q accepted", bad)
		}
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// A market map has no payments: the pack reads relations, sources and metrics.
func TestMarketMapPackOnTheExample(t *testing.T) {
	pack, err := ShippedPack("market-map")
	if err != nil {
		t.Fatal(err)
	}
	c := loadExample(t, "market-map")
	an := Analyze(c, Options{Pack: pack, From: "bank", To: "rival-open"})
	if got := titles(an, "larger-competitor"); len(got) != 1 || !strings.Contains(got[0], "Northgate Process Systems Ltd is a competitor at least five times the size of Lumen Flow Inc") {
		t.Errorf("larger competitor: %v", got)
	}
	big := finding(t, an, "larger-competitor")
	if !strings.Contains(big.Detail, "2400 employees") || !strings.Contains(big.Detail, "Lumen Flow Inc: 120") {
		t.Errorf("metrics in the detail: %q", big.Detail)
	}
	if got := titles(an, "more-visible-competitor"); len(got) != 1 || !strings.Contains(got[0], "Tessel Works") {
		t.Errorf("more visible competitor: %v", got)
	}
	// the two customers named only on the vendor's site, and the supplier named by one article
	if got := titles(an, "claim-from-one-side-only"); len(got) != 3 {
		t.Errorf("one-sided claims: %v", got)
	}
	if got := titles(an, "relationship-not-evidenced"); len(got) != 1 || !strings.Contains(got[0], "Calder Automation") || !strings.Contains(got[0], "E4") {
		t.Errorf("not evidenced: %v", got)
	}
	// no money-movement findings on a map with no payments; ownership still read
	for _, k := range []string{"structuring", "pass_through", "layering_chain", "round_trip", "unknown_owner"} {
		if count(an, k) != 0 {
			t.Errorf("%s reported on a market map", k)
		}
	}
	if !an.Connection.Found || an.Connection.Hops != 2 {
		t.Errorf("bank → open-source rival: %+v", an.Connection)
	}
	// the plan carries each metric as an account of its actor, dated as of the case
	p := BuildPlan(c)
	var stars *PlanMetric
	for i, m := range p.Metrics {
		if m.Entity == "rival-open" && m.Name == "github_stars" {
			stars = &p.Metrics[i]
		}
	}
	if len(p.Metrics) != 9 || stars == nil || stars.Value != "9200" || stars.AccountName != "Metric · github_stars" || stars.Date != "2026-06-30" {
		t.Errorf("planned metrics: %d, %+v", len(p.Metrics), stars)
	}
	for _, a := range p.Actors {
		if a.CaseID == "lumen" && (!strings.Contains(a.Description, "| employees | 120 |") || !strings.Contains(a.Description, "Metrics as of 2026-06-30")) {
			t.Errorf("description:\n%s", a.Description)
		}
	}
	if rep := Check(mustLoad(t, "case: {id: m, title: M}\nentities:\n  - {id: a, type: other, title: A, metrics: {Bad-Name: 1}}\n")); rep.OK() {
		t.Error("a malformed metric name was accepted")
	}
}
