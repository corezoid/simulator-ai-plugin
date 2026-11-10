package osint

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

func mustLoad(t *testing.T, src string) *Case {
	t.Helper()
	c, err := LoadCase([]byte(src))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return c
}

func kinds(a *Analysis) []string {
	var out []string
	for _, f := range a.Findings {
		out = append(out, f.Kind)
	}
	return out
}

func finding(t *testing.T, a *Analysis, kind string) Finding {
	t.Helper()
	for _, f := range a.Findings {
		if f.Kind == kind {
			return f
		}
	}
	t.Fatalf("no %s finding; got %v", kind, kinds(a))
	return Finding{}
}

func TestGenerateIsDeterministicAndClean(t *testing.T) {
	a, err := Generate(GenOptions{Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Generate(GenOptions{Seed: 7})
	other, _ := Generate(GenOptions{Seed: 8})
	ya, _ := MarshalCase(a)
	yb, _ := MarshalCase(b)
	yo, _ := MarshalCase(other)
	if string(ya) != string(yb) {
		t.Error("the same seed gave two different cases")
	}
	if string(ya) == string(yo) {
		t.Error("two seeds gave the same case")
	}
	c, err := LoadCase(ya)
	if err != nil {
		t.Fatalf("generated case does not load: %v", err)
	}
	if rep := Check(c); len(rep.Errors) > 0 || len(rep.Warnings) > 0 {
		t.Errorf("generated case is not clean: %+v", rep)
	}
	yc, _ := MarshalCase(c)
	if string(yc) != string(ya) {
		t.Error("case changed in a load/marshal round trip")
	}
	if _, err := Generate(GenOptions{Typologies: []string{"bribery"}}); err == nil {
		t.Error("unknown typology accepted")
	}
}

// Every planted scheme is found, on many seeds, and the background activity
// raises no medium or high finding of its own.
func TestAnalyzeFindsPlantedSchemes(t *testing.T) {
	want := []string{"layering_chain", "round_trip", "structuring", "funnel", "multiple_directorships",
		"indirect_beneficial_owner", "related_party_payments", "shared_anchor", "shared_attribute"}
	for seed := uint64(1); seed <= 60; seed++ {
		c, err := Generate(GenOptions{Seed: seed, Noise: 30})
		if err != nil {
			t.Fatal(err)
		}
		an := Analyze(c, Options{})
		got := kinds(an)
		for _, k := range want {
			if !slices.Contains(got, k) {
				t.Errorf("seed %d: no %s finding; got %v", seed, k, got)
			}
		}
		if an.Truth == nil {
			t.Fatalf("seed %d: no truth score", seed)
		}
		if len(an.Truth.MissedTx) > 0 || len(an.Truth.MissedEntities) > 0 {
			t.Errorf("seed %d: missed %v %v", seed, an.Truth.MissedEntities, an.Truth.MissedTx)
		}
		ix := buildIndex(c)
		for _, id := range an.Truth.OutsideScheme {
			// the shared addresses are flagged with the shells registered at them
			if ix.entities[id].Type != "location" {
				t.Errorf("seed %d: %s (%s) flagged outside the planted schemes", seed, id, ix.entities[id].Title)
			}
		}
	}
}

func TestGenerateWithoutBackgroundOrWithOneTypology(t *testing.T) {
	c, err := Generate(GenOptions{Seed: 3, Noise: -1, Typologies: []string{"smurfing"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Entities) != len(c.Truth.Entities) {
		t.Errorf("noise -1 left %d entities outside the scheme", len(c.Entities)-len(c.Truth.Entities))
	}
	got := kinds(Analyze(c, Options{}))
	if !slices.Contains(got, "structuring") || slices.Contains(got, "layering_chain") {
		t.Errorf("findings %v", got)
	}
}

const ownershipCase = `
case: {id: own, title: Ownership, currency: EUR}
sources:
  - {id: s1, title: Registry, kind: registry, reliability: B}
  - {id: s2, title: Tip, kind: human, reliability: E}
entities:
  - {id: p, type: person, title: P, sources: [s1]}
  - {id: q, type: person, title: Q, sources: [s1]}
  - {id: h, type: organization, title: H, sources: [s1]}
  - {id: t, type: organization, title: T, sources: [s1]}
  - {id: x, type: organization, title: X, sources: [s1]}
  - {id: y, type: organization, title: Y, sources: [s1]}
  - {id: lone, type: organization, title: Lone, sources: [s1]}
relations:
  - {id: r1, from: p, to: h, type: owns, share: 60, sources: [s1], credibility: 1}
  - {id: r2, from: h, to: t, type: owns, share: 50, sources: [s2], credibility: 4}
  - {id: r3, from: q, to: t, type: owns, share: 50, sources: [s1], credibility: 1}
  - {id: r4, from: x, to: y, type: owns, share: 100, sources: [s1]}
  - {id: r5, from: y, to: x, type: owns, share: 100, sources: [s1]}
`

func TestOwnership(t *testing.T) {
	c := mustLoad(t, ownershipCase)
	if rep := Check(c); !rep.OK() {
		t.Fatalf("check: %v", rep.Errors)
	}
	an := Analyze(c, Options{})

	var pOfT *Ownership
	for i, o := range an.Ownership {
		if o.Entity == "t" && o.Owner == "p" {
			pOfT = &an.Ownership[i]
		}
	}
	if pOfT == nil || pOfT.Share != "30" || pOfT.Hops != 2 {
		t.Fatalf("P's stake in T: %+v (all: %+v)", pOfT, an.Ownership)
	}
	f := finding(t, an, "indirect_beneficial_owner")
	if !strings.Contains(f.Title, "30%") || !slices.Contains(f.Evidence, "r2") {
		t.Errorf("indirect owner finding: %+v", f)
	}
	// the chain rests on a tip: reliability E, credibility 4
	if f.WeakestEvidence != "E4" {
		t.Errorf("weakest evidence = %s, want E4", f.WeakestEvidence)
	}
	if an.Evidence.WeakFindings == 0 {
		t.Error("a finding on an E-grade source is not counted as weak")
	}
	loop := finding(t, an, "circular_ownership")
	if len(loop.Subjects) != 2 || !slices.Contains(loop.Subjects, "x") || !slices.Contains(loop.Subjects, "y") {
		t.Errorf("circular ownership subjects: %v", loop.Subjects)
	}
	n := 0
	for _, f := range an.Findings {
		if f.Kind == "circular_ownership" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("the one loop was reported %d times", n)
	}
	if u := finding(t, an, "unknown_owner"); !slices.Contains(u.Subjects, "lone") || slices.Contains(u.Subjects, "t") {
		t.Errorf("unknown owner subjects: %v", u.Subjects)
	}

	// a higher bar removes the 30 % owner
	an = Analyze(c, Options{MinShare: mustDecimal("40").Rat()})
	if slices.Contains(kinds(an), "indirect_beneficial_owner") {
		t.Error("a 30 % stake reported with minShare 40")
	}
}

func TestConnection(t *testing.T) {
	c := mustLoad(t, ownershipCase)
	an := Analyze(c, Options{From: "p", To: "q"})
	conn := an.Connection
	if conn == nil || !conn.Found || conn.Hops != 3 || len(conn.Paths) != 1 {
		t.Fatalf("connection p-q: %+v", conn)
	}
	route := strings.Join(conn.Paths[0], " | ")
	for _, want := range []string{"P (p)", "[r1]", "[r2]", "[r3]", "Q (q)"} {
		if !strings.Contains(route, want) {
			t.Errorf("route %q lacks %q", route, want)
		}
	}
	if conn := Analyze(c, Options{From: "p", To: "lone"}).Connection; conn.Found {
		t.Errorf("found a route to an isolated entity: %+v", conn)
	}
	if conn := Analyze(c, Options{From: "p", To: "nobody"}).Connection; conn.Found || conn.Note == "" {
		t.Errorf("unknown id: %+v", conn)
	}
}

func TestCheckReportsInconsistencies(t *testing.T) {
	c := mustLoad(t, `
case: {id: "bad id", title: ""}
sources:
  - {id: s1, title: Registry, reliability: Z}
entities:
  - {id: a, type: person, title: A}
  - {id: a, type: company, title: B}
  - {id: o, type: organization, title: O, sources: [nope]}
relations:
  - {from: a, to: a, type: owns, share: 150}
  - {from: o, to: a, type: owns, share: 10}
  - {from: a, to: ghost, type: friend}
accounts:
  - {id: acc1, owner: ghost}
  - {id: acc2, owner: o, kind: piggybank, currency: USD}
transactions:
  - {id: t1, from: acc2, to: acc2, amount: 5}
  - {id: t2, from: acc2, to: nowhere, amount: -1}
events:
  - {title: Something, date: 2025-01-01, entities: [ghost]}
hypotheses:
  - {id: h1, statement: S, status: maybe, evidence: [t9]}
`)
	rep := Check(c)
	all := strings.Join(rep.Errors, "\n")
	for _, want := range []string{
		`case.id "bad id"`, "case.title is empty", "reliability \"Z\"", `id "a" is already used`, `type "company"`,
		`source "nope" is not in sources`, "from and to are the same entity", "share 150", "is a person and cannot be owned",
		`to "ghost" is not an entity`, `owner "ghost" is not an entity`, `kind "piggybank"`, "from and to are the same account",
		`to "nowhere" is not an account`, "must be positive", `"ghost" is not an entity`, `status "maybe"`, `evidence "t9"`,
	} {
		if !strings.Contains(all, want) {
			t.Errorf("no error containing %q in:\n%s", want, all)
		}
	}
	if !strings.Contains(strings.Join(rep.Warnings, "\n"), "have no source") {
		t.Errorf("no warning about unsourced claims: %v", rep.Warnings)
	}
}

func TestLoadRejectsUnknownKeysAndBadValues(t *testing.T) {
	for name, src := range map[string]string{
		"typo in a key":     "case: {id: c, title: C}\nentites: []\n",
		"amount with words": "case: {id: c, title: C}\ntransactions:\n  - {id: t, from: a, to: b, amount: about 5}\n",
		"exponent amount":   "case: {id: c, title: C}\ntransactions:\n  - {id: t, from: a, to: b, amount: 1e3}\n",
		"bad date":          "case: {id: c, title: C}\nevents:\n  - {title: E, date: yesterday}\n",
	} {
		if _, err := LoadCase([]byte(src)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// Amounts are exact: 0.1 + 0.2 is 0.3, not 0.30000000000000004.
func TestAmountsAreExact(t *testing.T) {
	c := mustLoad(t, `
case: {id: exact, title: Exact, currency: USD}
sources: [{id: s, title: S, reliability: A}]
entities:
  - {id: a, type: person, title: A, sources: [s]}
  - {id: b, type: person, title: B, sources: [s]}
accounts:
  - {id: aa, owner: a, sources: [s]}
  - {id: bb, owner: b, sources: [s]}
transactions:
  - {id: t1, from: aa, to: bb, amount: 0.1, date: 2025-01-01, sources: [s]}
  - {id: t2, from: aa, to: bb, amount: 0.2, date: 2025-01-02, sources: [s]}
`)
	an := Analyze(c, Options{})
	if got := an.Summary.Volume["USD"]; got != "0.3" {
		t.Errorf("volume = %s, want 0.3", got)
	}
}

func TestPlan(t *testing.T) {
	c := mustLoad(t, `
case: {id: plan, title: Plan, currency: USD}
sources: [{id: s, title: "Registry [/md][b]x[/b]", reliability: B, url: "https://example.org/r"}]
entities:
  - {id: a, type: person, title: A, sources: [s], attrs: {address: "1 | Main [/MD] St"}, note: "see [md] notes"}
  - {id: o, type: organization, title: O}
relations:
  - {from: a, to: o, type: owns, share: 75, sources: [s]}
accounts:
  - {id: a1, owner: a, kind: bank}
  - {id: a2, owner: a, kind: bank}
  - {id: a3, owner: a, kind: bank, currency: EUR}
  - {id: o1, owner: o, kind: crypto}
transactions:
  - {id: t2, from: a1, to: o1, amount: 40, date: 2025-02-01}
  - {id: t1, from: o1, to: a1, amount: 100, date: 2025-01-01, purpose: refund}
  - {id: t3, from: a3, to: o1, amount: 5}
`)
	p := BuildPlan(c)
	if p.RefPrefix != "osint:plan:" || len(p.Forms) != 2 || p.Forms[0].Title != "OSINT Person" || p.Forms[0].Ref != "osint-person" {
		t.Fatalf("plan head: %+v %+v", p.RefPrefix, p.Forms)
	}
	desc := p.Actors[0].Description
	if strings.Count(strings.ToLower(desc), "[md]") != 1 || strings.Count(strings.ToLower(desc), "[/md]") != 1 {
		t.Errorf("description carries block markers from the case text:\n%s", desc)
	}
	if !strings.Contains(desc, `1 \| Main St`) || !strings.Contains(desc, "reliability B") {
		t.Errorf("description:\n%s", desc)
	}
	if p.Actors[0].Grade != "B6" || p.Actors[1].Grade != "unsourced" {
		t.Errorf("grades: %q %q", p.Actors[0].Grade, p.Actors[1].Grade)
	}
	if !strings.Contains(p.Actors[1].Description, "none recorded") {
		t.Errorf("an unsourced entity is not marked unverified:\n%s", p.Actors[1].Description)
	}
	if len(p.Links) != 1 || p.Links[0].Name != "owns 75%" || p.Links[0].SourceRef != "osint:plan:a" {
		t.Errorf("links: %+v", p.Links)
	}
	names := map[string]string{}
	for _, a := range p.Accounts {
		names[a.CaseID] = a.AccountName + "/" + a.CurrencyName
	}
	if names["a1"] != "OSINT bank/USD" || names["a2"] != "OSINT bank #2/USD" || names["a3"] != "OSINT bank/EUR" || names["o1"] != "OSINT crypto/USD" {
		t.Errorf("account names: %v", names)
	}
	// dated transfers in date order, the undated one last
	var order []string
	for _, tr := range p.Transfers {
		order = append(order, strings.TrimPrefix(tr.Ref, p.RefPrefix))
	}
	if strings.Join(order, ",") != "t1,t2,t3" {
		t.Errorf("transfer order: %v", order)
	}
	if !strings.Contains(p.Transfers[0].Comment, "2025-01-01 · refund · O → A · case tx t1") {
		t.Errorf("comment: %q", p.Transfers[0].Comment)
	}
	if !strings.Contains(strings.Join(p.Notes, "\n"), "real people or organizations") ||
		!strings.Contains(strings.Join(p.Notes, "\n"), "different currencies") {
		t.Errorf("notes: %v", p.Notes)
	}
}

// ---- tools ---------------------------------------------------------------------

func call(t *testing.T, h func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), args map[string]any) (map[string]any, bool) {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	res, err := h(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	var out map[string]any
	if json.Unmarshal([]byte(text), &out) != nil {
		return map[string]any{"text": text}, res.IsError
	}
	return out, res.IsError
}

func TestToolsWorkFromFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIMULATOR_WORK_DIR", dir)

	out, isErr := call(t, handleGenerate, map[string]any{"seed": float64(5), "outPath": "cases/demo.yaml"})
	if isErr || out["file"] != filepath.Join(dir, "cases", "demo.yaml") || out["yaml"] != nil {
		t.Fatalf("generate: %v", out)
	}
	if _, isErr = call(t, handleGenerate, map[string]any{"seed": float64(6), "outPath": "cases/demo.yaml"}); !isErr {
		t.Error("an existing file was overwritten without overwrite: true")
	}
	if _, isErr = call(t, handleGenerate, map[string]any{"seed": float64(6), "outPath": "cases/demo.yaml", "overwrite": true}); isErr {
		t.Error("overwrite: true was refused")
	}
	for _, p := range []string{"../demo.yaml", "/tmp/demo.yaml", "demo.sh"} {
		if _, isErr := call(t, handleGenerate, map[string]any{"outPath": p}); !isErr {
			t.Errorf("outPath %q was accepted", p)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "demo.yaml")); err == nil {
		t.Error("a file was written outside the working directory")
	}

	out, isErr = call(t, handleCheck, map[string]any{"casePath": "cases/demo.yaml"})
	if isErr || out["ok"] != true {
		t.Fatalf("check: %v", out)
	}
	out, isErr = call(t, handleAnalyze, map[string]any{"casePath": "cases/demo.yaml", "threshold": "10000", "top": float64(3)})
	an, _ := out["analysis"].(map[string]any)
	if isErr || out["ok"] != true || len(an["findings"].([]any)) == 0 || len(an["central"].([]any)) != 3 {
		t.Fatalf("analyze: %v", out)
	}
	// with the threshold far above every payment, nothing sits just under it
	out, _ = call(t, handleAnalyze, map[string]any{"casePath": "cases/demo.yaml", "threshold": float64(5_000_000)})
	for _, f := range out["analysis"].(map[string]any)["findings"].([]any) {
		if f.(map[string]any)["kind"] == "structuring" {
			t.Error("structuring reported under a threshold no payment is near")
		}
	}
	if _, isErr = call(t, handleAnalyze, map[string]any{"casePath": "cases/demo.yaml", "threshold": "lots"}); !isErr {
		t.Error("a non-numeric threshold was accepted")
	}
	for _, p := range []string{"../../etc/passwd", "/etc/passwd"} {
		if _, isErr := call(t, handleCheck, map[string]any{"casePath": p}); !isErr {
			t.Errorf("casePath %q was accepted", p)
		}
	}
	if _, isErr = call(t, handleCheck, map[string]any{}); !isErr {
		t.Error("a call with no case was accepted")
	}
}

func TestPlanToolPages(t *testing.T) {
	gen, _ := call(t, handleGenerate, map[string]any{"seed": float64(2)})
	src := gen["yaml"].(string)

	out, isErr := call(t, handlePlan, map[string]any{"case": src})
	if isErr || out["ok"] != true || out["actors"] != nil {
		t.Fatalf("summary: %v", out)
	}
	total := int(out["counts"].(map[string]any)["actors"].(float64))
	seen := 0
	for offset := 0; ; offset += 20 {
		out, _ = call(t, handlePlan, map[string]any{"case": src, "part": "actors", "offset": float64(offset), "limit": float64(20)})
		items := out["actors"].([]any)
		seen += len(items)
		if out["more"] != true {
			break
		}
		if len(items) != 20 {
			t.Fatalf("a page before the last has %d items", len(items))
		}
	}
	if seen != total {
		t.Errorf("paged through %d actors of %d", seen, total)
	}
	if _, isErr = call(t, handlePlan, map[string]any{"case": src, "part": "everything"}); !isErr {
		t.Error("an unknown part was accepted")
	}
}

func TestBrokenCaseIsNotAnalysedOrPlanned(t *testing.T) {
	src := "case: {id: c, title: C}\nentities:\n  - {id: a, type: robot, title: A}\n"
	for name, h := range map[string]func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error){"analyze": handleAnalyze, "plan": handlePlan} {
		out, _ := call(t, h, map[string]any{"case": src})
		if out["ok"] != false || out["analysis"] != nil || out["plan"] != nil {
			t.Errorf("%s ran on a broken case: %v", name, out)
		}
	}
}

// The example shipped with the format documentation loads, passes the check
// and shows the patterns the documentation says it does.
func TestDocumentedExample(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "osint", "examples", "minimal.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := LoadCase(src)
	if err != nil {
		t.Fatal(err)
	}
	if rep := Check(c); len(rep.Errors) > 0 || len(rep.Warnings) > 0 {
		t.Fatalf("example is not clean: %+v", rep)
	}
	an := Analyze(c, Options{From: "director", To: "supplier"})
	owner := finding(t, an, "indirect_beneficial_owner")
	if !strings.Contains(owner.Title, "80%") || owner.WeakestEvidence != "D3" {
		t.Errorf("owner finding: %+v", owner)
	}
	finding(t, an, "pass_through")
	finding(t, an, "shared_attribute")
	if !an.Connection.Found {
		t.Errorf("connection: %+v", an.Connection)
	}
}

func loadExample(t *testing.T, name string) *Case {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "osint", "examples", name+".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := LoadCase(src)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if rep := Check(c); len(rep.Errors) > 0 || len(rep.Warnings) > 0 {
		t.Fatalf("%s is not clean: %+v", name, rep)
	}
	return c
}

func count(a *Analysis, kind string) int {
	n := 0
	for _, f := range a.Findings {
		if f.Kind == kind {
			n++
		}
	}
	return n
}

// A kickback: the official who ran the tender is married to the owner of the
// company a third of the contract money ends up in.
func TestExampleProcurementKickback(t *testing.T) {
	c := loadExample(t, "procurement-kickback")
	an := Analyze(c, Options{From: "official", To: "consultant"})

	conflict := finding(t, an, "conflict_of_interest")
	for _, id := range []string{"authority", "official", "spouse", "consultant", "holding"} {
		if !slices.Contains(conflict.Subjects, id) {
			t.Errorf("conflict of interest does not name %s: %v", id, conflict.Subjects)
		}
	}
	for _, id := range []string{"r-official", "r-family", "r-spouse", "r-holding", "t01", "t06"} {
		if !slices.Contains(conflict.Evidence, id) {
			t.Errorf("conflict of interest does not rest on %s: %v", id, conflict.Evidence)
		}
	}
	// it is only as strong as the leaked ownership records
	if conflict.WeakestEvidence != "D3" || conflict.Severity != "high" {
		t.Errorf("conflict of interest: %s %s", conflict.Severity, conflict.WeakestEvidence)
	}
	// the official's salary is not a conflict; the contractor's owner-director is not one either
	if count(an, "conflict_of_interest") != 1 {
		t.Errorf("%d conflict findings, want 1", count(an, "conflict_of_interest"))
	}
	owner := finding(t, an, "indirect_beneficial_owner")
	if !slices.Contains(owner.Subjects, "spouse") || !slices.Contains(owner.Subjects, "consultant") {
		t.Errorf("beneficial owner: %+v", owner)
	}
	chain := finding(t, an, "layering_chain")
	if !slices.Contains(chain.Subjects, "contractor") || !slices.Contains(chain.Subjects, "spouse") {
		t.Errorf("the chain's ends are not named: %v", chain.Subjects)
	}
	finding(t, an, "related_party_payments")
	if !an.Connection.Found || an.Connection.Hops != 3 {
		t.Errorf("official → consultant: %+v", an.Connection)
	}
	// 15 payments, 5 324 600 EUR in all
	if an.Summary.Volume["EUR"] != "5324600" {
		t.Errorf("volume: %v", an.Summary.Volume)
	}
}

// Deposits under the threshold through three mules, into a stablecoin,
// through three wallets, out through an OTC desk.
func TestExampleInvestmentFraudCashout(t *testing.T) {
	c := loadExample(t, "investment-fraud-cashout")
	an := Analyze(c, Options{From: "v1", To: "car"})
	if count(an, "structuring") != 3 || count(an, "pass_through") != 3 {
		t.Errorf("structuring %d, pass-through %d; want 3 and 3: %v", count(an, "structuring"), count(an, "pass_through"), kinds(an))
	}
	chain := finding(t, an, "layering_chain")
	for _, id := range []string{"w-org", "w1", "w2", "c1", "c2", "c3"} {
		if !slices.Contains(chain.Evidence, id) {
			t.Errorf("the on-chain hops lack %s: %v", id, chain.Evidence)
		}
	}
	if !slices.Contains(chain.Subjects, "organizer") || !slices.Contains(chain.Subjects, "otc") {
		t.Errorf("chain subjects: %v", chain.Subjects)
	}
	if an.Summary.Volume["USD"] != "328275" || an.Summary.Volume["USDT"] != "431900" {
		t.Errorf("volumes: %v", an.Summary.Volume)
	}
	if !an.Connection.Found {
		t.Errorf("a victim is not connected to the car: %+v", an.Connection)
	}
	// with the threshold of another jurisdiction nothing sits under it
	if n := count(Analyze(c, Options{Threshold: mustDecimal("3000").Rat()}), "structuring"); n != 0 {
		t.Errorf("%d structuring findings under a 3000 threshold", n)
	}
	// two currencies, three account names
	p := BuildPlan(c)
	if len(p.AccountPairs) != 3 || len(p.Transfers) != 22 {
		t.Errorf("plan: %v, %d transfers", p.AccountPairs, len(p.Transfers))
	}
}

// Inflated invoices, the excess returned as a loan, and a trader and a lender
// that own each other.
func TestExampleTradeOverinvoicing(t *testing.T) {
	c := loadExample(t, "trade-overinvoicing")
	an := Analyze(c, Options{})
	// the two shipments make several trips round the same four companies: one finding
	if count(an, "round_trip") != 1 {
		t.Fatalf("%d round-trip findings, want 1: %v", count(an, "round_trip"), kinds(an))
	}
	trip := finding(t, an, "round_trip")
	if !strings.Contains(trip.Title, "12 day(s)") || !strings.Contains(trip.Detail, "such trips") {
		t.Errorf("round trip: %s — %s", trip.Title, trip.Detail)
	}
	for _, id := range []string{"p1", "k1", "q1", "l1", "p2", "l2"} {
		if !slices.Contains(trip.Evidence, id) {
			t.Errorf("round trip does not rest on %s", id)
		}
	}
	loop := finding(t, an, "circular_ownership")
	if !slices.Contains(loop.Subjects, "trader") || !slices.Contains(loop.Subjects, "quillon") {
		t.Errorf("loop: %v", loop.Subjects)
	}
	finding(t, an, "layering_chain")
	if count(an, "related_party_payments") < 2 || count(an, "conflict_of_interest") != 0 {
		t.Errorf("related parties %d, conflicts %d", count(an, "related_party_payments"), count(an, "conflict_of_interest"))
	}
	// the owner's stake in the trader exists only through the loop: 100 % × 70 % × 40 %
	var stake string
	for _, o := range an.Ownership {
		if o.Entity == "trader" && o.Owner == "owner" {
			stake = o.Share
		}
	}
	if stake != "28" {
		t.Errorf("owner's effective stake in the trader = %q, want 28", stake)
	}
}

// The largest case the loader accepts is analysed in seconds: no pattern
// search grows with the square of the number of payments.
func TestAnalyzeScalesToTheLargestCase(t *testing.T) {
	c := &Case{Meta: Meta{ID: "big", Title: "Big", Currency: "USD"}}
	const people = 2000
	for i := range people {
		id := fmt.Sprintf("p%d", i)
		c.Entities = append(c.Entities, Entity{ID: id, Type: "person", Title: id})
		c.Accounts = append(c.Accounts, Account{ID: "a" + id, Owner: id})
		if i > 0 {
			c.Relations = append(c.Relations, Relation{From: id, To: "p0", Type: "family"})
		}
	}
	c.Entities = append(c.Entities, Entity{ID: "org", Type: "organization", Title: "Org"})
	c.Accounts = append(c.Accounts, Account{ID: "aorg", Owner: "org"})
	c.Relations = append(c.Relations, Relation{From: "p0", To: "org", Type: "director"})
	day := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	// everything through two accounts, all in one week, all just under the threshold
	for i := range maxTransactions {
		from, to := "aorg", fmt.Sprintf("ap%d", i%people)
		if i%2 == 1 {
			from, to = to, "ap0"
			if from == to {
				to = "aorg"
			}
		}
		c.Transactions = append(c.Transactions, Transaction{ID: fmt.Sprintf("t%d", i), From: from, To: to,
			Amount: mustDecimal("9500"), Date: dayDate(day.AddDate(0, 0, i%7))})
	}
	if rep := Check(c); !rep.OK() {
		t.Fatal(rep.Errors)
	}
	start := time.Now()
	an := Analyze(c, Options{})
	if d := time.Since(start); d > 20*time.Second {
		t.Errorf("analysis of %d payments took %s", maxTransactions, d)
	}
	if count(an, "structuring") == 0 {
		t.Errorf("no structuring found in %d payments under the threshold: %v", maxTransactions, kinds(an))
	}
}
