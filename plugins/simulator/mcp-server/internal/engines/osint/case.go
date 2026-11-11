// Package osint models an investigation as a case file: sources, entities,
// relations, accounts, transactions, events and hypotheses, each claim carrying
// its provenance. The engine checks a case, analyses its graph and money flows,
// generates synthetic cases with a known answer, and builds the case as a
// digital twin in Simulator. It collects nothing itself: a case is filled by
// the investigator (or the host's own search tools). Only Build calls the
// Simulator API; everything else is local.
package osint

import (
	"bytes"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Limits keep one case within what a single tool call can analyse.
const (
	maxCaseBytes    = 4 << 20
	maxEntities     = 5000
	maxRelations    = 20000
	maxAccounts     = 10000
	maxTransactions = 50000
)

// Case is one investigation.
type Case struct {
	Meta         Meta          `yaml:"case"`
	Sources      []Source      `yaml:"sources,omitempty"`
	Entities     []Entity      `yaml:"entities,omitempty"`
	Relations    []Relation    `yaml:"relations,omitempty"`
	Accounts     []Account     `yaml:"accounts,omitempty"`
	Transactions []Transaction `yaml:"transactions,omitempty"`
	Events       []Event       `yaml:"events,omitempty"`
	Hypotheses   []Hypothesis  `yaml:"hypotheses,omitempty"`
	Truth        *Truth        `yaml:"truth,omitempty"`
}

// Meta describes the case as a whole.
type Meta struct {
	ID        string `yaml:"id"`
	Title     string `yaml:"title"`
	Question  string `yaml:"question,omitempty"`
	Synthetic bool   `yaml:"synthetic,omitempty"`
	Currency  string `yaml:"currency,omitempty"`
	// AsOf is the day the case describes: the date its metrics were read.
	AsOf Date   `yaml:"as_of,omitempty"`
	Note string `yaml:"note,omitempty"`
}

// Source is where a claim comes from. Reliability is the Admiralty grade of
// the source itself: A (completely reliable) … E (unreliable), F (cannot be
// judged).
type Source struct {
	ID          string `yaml:"id"`
	Title       string `yaml:"title"`
	Kind        string `yaml:"kind,omitempty"`
	URL         string `yaml:"url,omitempty"`
	Retrieved   Date   `yaml:"retrieved,omitempty"`
	Reliability string `yaml:"reliability,omitempty"`
	Note        string `yaml:"note,omitempty"`
}

// Entity is a node of the investigation graph. Credibility is the Admiralty
// grade of the information: 1 (confirmed) … 5 (improbable), 6 (cannot be
// judged). Picture is an https URL or an image file in the working directory;
// without one the twin gets a generated placeholder.
type Entity struct {
	ID      string            `yaml:"id"`
	Type    string            `yaml:"type"`
	Title   string            `yaml:"title"`
	Aliases []string          `yaml:"aliases,omitempty"`
	Attrs   map[string]string `yaml:"attrs,omitempty"`
	// Metrics are quantities of the entity as of case.as_of: followers,
	// employees, stars, revenue. In Simulator each becomes an account.
	Metrics     map[string]Decimal `yaml:"metrics,omitempty"`
	Picture     string             `yaml:"picture,omitempty"`
	Sources     []string           `yaml:"sources,omitempty"`
	Credibility int                `yaml:"credibility,omitempty"`
	Note        string             `yaml:"note,omitempty"`
}

// Relation is a directed, typed link between two entities. Share is the
// ownership percentage of an `owns` relation.
type Relation struct {
	ID          string   `yaml:"id,omitempty"`
	From        string   `yaml:"from"`
	To          string   `yaml:"to"`
	Type        string   `yaml:"type"`
	Share       *Decimal `yaml:"share,omitempty"`
	Since       Date     `yaml:"since,omitempty"`
	Until       Date     `yaml:"until,omitempty"`
	Sources     []string `yaml:"sources,omitempty"`
	Credibility int      `yaml:"credibility,omitempty"`
	Note        string   `yaml:"note,omitempty"`
}

// Account holds value for its owner entity: a bank account, a wallet, cash.
type Account struct {
	ID          string   `yaml:"id"`
	Owner       string   `yaml:"owner"`
	Kind        string   `yaml:"kind,omitempty"`
	Title       string   `yaml:"title,omitempty"`
	Number      string   `yaml:"number,omitempty"`
	Institution string   `yaml:"institution,omitempty"`
	Currency    string   `yaml:"currency,omitempty"`
	Opened      Date     `yaml:"opened,omitempty"`
	Closed      Date     `yaml:"closed,omitempty"`
	Sources     []string `yaml:"sources,omitempty"`
	Credibility int      `yaml:"credibility,omitempty"`
	Note        string   `yaml:"note,omitempty"`
}

// Transaction moves an amount from one account to another.
type Transaction struct {
	ID          string   `yaml:"id"`
	From        string   `yaml:"from"`
	To          string   `yaml:"to"`
	Amount      Decimal  `yaml:"amount"`
	Currency    string   `yaml:"currency,omitempty"`
	Date        Date     `yaml:"date,omitempty"`
	Purpose     string   `yaml:"purpose,omitempty"`
	Sources     []string `yaml:"sources,omitempty"`
	Credibility int      `yaml:"credibility,omitempty"`
	Note        string   `yaml:"note,omitempty"`
}

// Event is a dated fact of the chronology.
type Event struct {
	ID          string   `yaml:"id,omitempty"`
	Date        Date     `yaml:"date"`
	Title       string   `yaml:"title"`
	Entities    []string `yaml:"entities,omitempty"`
	Sources     []string `yaml:"sources,omitempty"`
	Credibility int      `yaml:"credibility,omitempty"`
	Note        string   `yaml:"note,omitempty"`
}

// Hypothesis is a statement the investigation tests; Evidence lists ids of
// any case items that bear on it.
type Hypothesis struct {
	ID        string   `yaml:"id"`
	Statement string   `yaml:"statement"`
	Status    string   `yaml:"status,omitempty"`
	Evidence  []string `yaml:"evidence,omitempty"`
	Note      string   `yaml:"note,omitempty"`
}

// Truth is the known answer of a synthetic case: what a complete
// investigation should have flagged. The analysis never reads it to find
// anything; it only scores its findings against it.
type Truth struct {
	Typologies   []string `yaml:"typologies,omitempty"`
	Entities     []string `yaml:"entities,omitempty"`
	Transactions []string `yaml:"transactions,omitempty"`
	Note         string   `yaml:"note,omitempty"`
}

// Vocabularies. Entity types, account kinds, hypothesis statuses and grades
// are closed; relation types and source kinds are open (an unknown one is a
// warning), because real cases need words this list does not have.
var (
	entityTypes     = []string{"person", "organization", "asset", "location", "document", "digital", "other"}
	accountKinds    = []string{"bank", "crypto", "cash", "emoney", "other"}
	hypothesisState = []string{"open", "supported", "refuted"}
	relationTypes   = []string{"owns", "controls", "director", "officer", "agent", "employee", "family", "associate",
		"registered_at", "located_at", "uses", "mentions", "same_as",
		"founder", "invests_in", "partner", "customer", "supplier", "competitor", "product_of", "part_of", "other"}
	sourceKinds = []string{"registry", "court", "sanctions", "corporate", "bank", "blockchain", "media", "social",
		"leak", "human", "synthetic", "other"}
)

// controlTypes are relations that give a person a say over an organization
// without owning it.
var controlTypes = map[string]bool{"controls": true, "director": true, "officer": true, "agent": true}

// anchorTypes link an entity to a shared address or identifier.
var anchorTypes = map[string]bool{"registered_at": true, "located_at": true, "uses": true}

// metricRe is the shape of a metric's name.
var metricRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

var idRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

// LoadCase parses a case file. Unknown keys are errors: a typo in a key would
// otherwise drop a claim silently.
func LoadCase(src []byte) (*Case, error) {
	if len(src) > maxCaseBytes {
		return nil, fmt.Errorf("case is %d bytes; the limit is %d", len(src), maxCaseBytes)
	}
	dec := yaml.NewDecoder(bytes.NewReader(src))
	dec.KnownFields(true)
	c := &Case{}
	if err := dec.Decode(c); err != nil {
		return nil, fmt.Errorf("case YAML: %w", err)
	}
	for name, n := range map[string][2]int{
		"entities":     {len(c.Entities), maxEntities},
		"relations":    {len(c.Relations), maxRelations},
		"accounts":     {len(c.Accounts), maxAccounts},
		"transactions": {len(c.Transactions), maxTransactions},
	} {
		if n[0] > n[1] {
			return nil, fmt.Errorf("case has %d %s; the limit is %d", n[0], name, n[1])
		}
	}
	return c, nil
}

// MarshalCase renders a case as YAML.
func MarshalCase(c *Case) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(c); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ---- Decimal -----------------------------------------------------------------

var decimalRe = regexp.MustCompile(`^-?\d+(\.\d+)?$`)

// Decimal is an exact amount: money is never held in a float.
type Decimal struct{ r *big.Rat }

func newDecimal(s string) (Decimal, bool) {
	s = strings.ReplaceAll(strings.TrimSpace(s), "_", "")
	if !decimalRe.MatchString(s) {
		return Decimal{}, false
	}
	r, ok := new(big.Rat).SetString(s)
	return Decimal{r}, ok
}

func mustDecimal(s string) Decimal {
	d, ok := newDecimal(s)
	if !ok {
		panic("osint: bad decimal " + s)
	}
	return d
}

// Rat returns the value, zero when unset.
func (d Decimal) Rat() *big.Rat {
	if d.r == nil {
		return new(big.Rat)
	}
	return d.r
}

// IsSet reports whether a value was given.
func (d Decimal) IsSet() bool { return d.r != nil }

func (d Decimal) String() string { return ratString(d.Rat()) }

// IsZero lets `omitempty` drop an unset amount.
func (d Decimal) IsZero() bool { return d.r == nil }

func (d *Decimal) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return fmt.Errorf("line %d: expected a number", n.Line)
	}
	v, ok := newDecimal(n.Value)
	if !ok {
		return fmt.Errorf("line %d: %q is not a decimal number", n.Line, n.Value)
	}
	*d = v
	return nil
}

func (d Decimal) MarshalYAML() (any, error) {
	tag := "!!float"
	if d.Rat().IsInt() {
		tag = "!!int"
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: d.String()}, nil
}

// ratString renders a rational as a decimal with up to 8 places, trailing
// zeros removed.
func ratString(r *big.Rat) string {
	if r.IsInt() {
		return r.Num().String()
	}
	s := r.FloatString(8)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// ---- Date --------------------------------------------------------------------

var dateLayouts = []string{"2006-01-02", time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02 15:04"}

// Date is a calendar day or a moment; the zero value means "not known".
type Date struct {
	t   time.Time
	day bool
}

func parseDate(s string) (Date, bool) {
	s = strings.TrimSpace(s)
	for i, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return Date{t: t.UTC(), day: i == 0}, true
		}
	}
	return Date{}, false
}

func dayDate(t time.Time) Date { return Date{t: t.UTC(), day: true} }

// IsZero lets `omitempty` drop an unknown date.
func (d Date) IsZero() bool { return d.t.IsZero() }

// Time returns the moment (midnight UTC for a day).
func (d Date) Time() time.Time { return d.t }

func (d Date) String() string {
	switch {
	case d.IsZero():
		return ""
	case d.day:
		return d.t.Format("2006-01-02")
	}
	return d.t.Format(time.RFC3339)
}

func (d *Date) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return fmt.Errorf("line %d: expected a date", n.Line)
	}
	if n.Tag == "!!null" || n.Value == "" {
		*d = Date{}
		return nil
	}
	v, ok := parseDate(n.Value)
	if !ok {
		return fmt.Errorf("line %d: %q is not a date (use YYYY-MM-DD or RFC 3339)", n.Line, n.Value)
	}
	*d = v
	return nil
}

func (d Date) MarshalYAML() (any, error) {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: d.String()}, nil
}
