package osint

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Activity is what a collection adds up to: when its items appeared, how they
// were received, and what they point at. It carries counts and identifiers
// only — never the text of an item, which is whatever its author wrote.
type Activity struct {
	Source      string            `json:"source"`
	Target      string            `json:"target"`
	Items       int               `json:"items"`
	Dated       int               `json:"dated"`
	First       string            `json:"first,omitempty"`
	Last        string            `json:"last,omitempty"`
	Truncated   bool              `json:"truncated,omitempty"`
	PerMonth    []NameCount       `json:"perMonth,omitempty"`
	PerWeekday  []NameCount       `json:"perWeekday,omitempty"`
	PerHourUTC  []NameCount       `json:"perHourUTC,omitempty"`
	Metrics     map[string]Spread `json:"metrics,omitempty"`
	Hosts       []NameCount       `json:"publishedOn,omitempty"`
	Domains     []NameCount       `json:"linkedDomains,omitempty"`
	Mentions    []NameCount       `json:"mentions,omitempty"`
	Tags        []NameCount       `json:"tags,omitempty"`
	Authors     []NameCount       `json:"authors,omitempty"`
	TopItems    map[string][]Rank `json:"topItems,omitempty"`
	CaseDraft   string            `json:"caseDraft,omitempty"`
	Assumptions []string          `json:"assumptions"`
}

// NameCount is a name and how many items carry it.
type NameCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Spread summarises one metric over the items that have it.
type Spread struct {
	Items  int     `json:"items"`
	Median float64 `json:"median"`
	P90    float64 `json:"p90"`
	Max    float64 `json:"max"`
	Sum    float64 `json:"sum"`
}

// Rank is one of the highest items by a metric.
type Rank struct {
	ID    string  `json:"id"`
	URL   string  `json:"url,omitempty"`
	Date  string  `json:"date,omitempty"`
	Value float64 `json:"value"`
}

// safeName keeps a handle, a tag or a host name that came from a source to
// characters that cannot be read as anything else.
var safeName = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N}_.\-]{0,62}$`)

// itemTime reads an item's date.
func itemTime(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, strings.TrimSpace(s)); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

func ranked(counts map[string]int, top int) []NameCount {
	out := make([]NameCount, 0, len(counts))
	for name, n := range counts {
		if safeName.MatchString(name) {
			out = append(out, NameCount{name, n})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > top {
		out = out[:top]
	}
	return out
}

// selfDomains are hosts that say nothing about who an item points at: the
// platform's own links and link shorteners.
var selfDomains = map[string]bool{"t.me": true, "telegram.me": true, "bit.ly": true, "t.co": true, "goo.gl": true, "tinyurl.com": true}

// platformDomains are sites that host everyone's pages: a link there says
// nothing about an organization, so they are ranked but not drafted as one.
var platformDomains = map[string]bool{"instagram.com": true, "facebook.com": true, "youtube.com": true, "youtu.be": true,
	"x.com": true, "twitter.com": true, "threads.com": true, "threads.net": true, "tiktok.com": true, "linkedin.com": true,
	"goodreads.com": true, "telegra.ph": true, "forms.gle": true, "docs.google.com": true, "drive.google.com": true,
	"wikipedia.org": true, "medium.com": true, "github.com": true}

// Summarise adds a collection up. subject is the id, in the case, of the
// entity the collection is about: with it the result carries a draft of the
// case entries for the source and for what its items link to most.
func Summarise(c *Collection, subject string, top int) *Activity {
	if top <= 0 {
		top = 10
	}
	a := &Activity{Source: c.Source, Target: c.Target, Items: len(c.Items), Truncated: c.Truncated, Assumptions: []string{}}
	months, weekdays, hours := map[string]int{}, map[string]int{}, map[string]int{}
	domains, mentions, tags, authors := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	hosts := map[string]int{} // where the items themselves are published
	values := map[string][]Rank{}
	var first, last time.Time
	for _, it := range c.Items {
		if when, ok := itemTime(it.Date); ok {
			a.Dated++
			if first.IsZero() || when.Before(first) {
				first = when
			}
			if when.After(last) {
				last = when
			}
			months[when.Format("2006-01")]++
			weekdays[when.Weekday().String()]++
			hours[fmt.Sprintf("%02d", when.Hour())]++
		}
		if u, err := url.Parse(it.URL); err == nil && u.Hostname() != "" {
			hosts[strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")]++
		}
		seen := map[string]bool{}
		for _, l := range it.Links {
			u, err := url.Parse(l)
			if err != nil {
				continue
			}
			host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
			if host != "" && !selfDomains[host] && !seen[host] {
				seen[host] = true
				domains[host]++
			}
		}
		for _, m := range it.Mentions {
			mentions[m]++
		}
		for _, t := range it.Tags {
			tags[t]++
		}
		if it.Author != "" {
			authors[it.Author]++
		}
		for name, v := range it.Metrics {
			values[name] = append(values[name], Rank{it.ID, it.URL, it.Date, v})
		}
	}
	if !first.IsZero() {
		a.First, a.Last = first.Format("2006-01-02"), last.Format("2006-01-02")
	}
	inOrder := func(counts map[string]int, keys []string) []NameCount {
		var out []NameCount
		for _, k := range keys {
			if counts[k] > 0 {
				out = append(out, NameCount{k, counts[k]})
			}
		}
		return out
	}
	a.PerMonth = inOrder(months, sortedKeys(months))
	a.PerWeekday = inOrder(weekdays, []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"})
	a.PerHourUTC = inOrder(hours, sortedKeys(hours))
	a.Domains, a.Mentions, a.Tags = ranked(domains, top), ranked(mentions, top), ranked(tags, top)
	// one host for every item is the source itself and tells nothing
	if len(hosts) > 1 {
		a.Hosts = ranked(hosts, top)
	}
	// authors whose names are plain words; others are left out rather than passed on
	a.Authors = ranked(authors, top)
	if len(values) > 0 {
		a.Metrics, a.TopItems = map[string]Spread{}, map[string][]Rank{}
		for name, list := range values {
			if !safeName.MatchString(name) {
				continue
			}
			sort.Slice(list, func(i, j int) bool { return list[i].Value > list[j].Value })
			sp := Spread{Items: len(list), Max: list[0].Value, Median: list[len(list)/2].Value, P90: list[len(list)/10].Value}
			for _, r := range list {
				sp.Sum += r.Value
			}
			a.Metrics[name] = sp
			a.TopItems[name] = list[:min(5, len(list))]
		}
	}
	a.Assumptions = append(a.Assumptions,
		"Counts cover the collected items only; with truncated set, the source had more than the limit.",
		"A link, a mention or a tag is counted once per item. Hours are UTC.",
		"A link to a site shows the item pointed there, not what the relationship is: advertising, a review and a complaint look the same.")
	if subject != "" {
		a.CaseDraft = caseDraft(c, a, subject, min(top, 5))
	}
	return a
}

// caseDraft writes the case entries a collection suggests: the source, and an
// organization with a `mentions` relation for each of the sites its items link
// to most. It is a draft: the investigator names the organizations and decides
// what belongs in the case.
func caseDraft(c *Collection, a *Activity, subject string, top int) string {
	var b strings.Builder
	sid := "s-" + strings.ReplaceAll(c.Source, "_", "-") + "-" + slug(c.Target)
	retrieved := c.Retrieved
	if len(retrieved) >= 10 {
		retrieved = retrieved[:10]
	}
	reliability := c.Reliability
	if reliability == "" {
		reliability = "F"
	}
	kind := c.SourceKind
	if kind == "" {
		kind = "other"
	}
	period := ""
	if a.First != "" {
		period = fmt.Sprintf(", %s to %s", a.First, a.Last)
	}
	fmt.Fprintf(&b, "sources:\n  - {id: %s, title: %q, kind: %s, reliability: %s, retrieved: %s", sid,
		fmt.Sprintf("%s %s: %d items%s", c.Source, c.Target, a.Items, period), kind, reliability, retrieved)
	if strings.HasPrefix(c.URL, "https://") {
		fmt.Fprintf(&b, ", url: %q", c.URL)
	}
	b.WriteString("}\n")
	var sites []NameCount
	for _, d := range a.Domains {
		if !platformDomains[d.Name] && len(sites) < top {
			sites = append(sites, d)
		}
	}
	if len(sites) == 0 {
		return b.String()
	}
	b.WriteString("entities:\n")
	for _, d := range sites {
		fmt.Fprintf(&b, "  - {id: site-%s, type: organization, title: %q, attrs: {website: %q}, sources: [%s], credibility: 1}\n",
			slug(d.Name), d.Name, "https://"+d.Name, sid)
	}
	b.WriteString("relations:\n")
	for _, d := range sites {
		fmt.Fprintf(&b, "  - {from: %s, to: site-%s, type: mentions, sources: [%s], credibility: 1, note: %q}\n",
			subject, slug(d.Name), sid, fmt.Sprintf("Linked from %d of %d items.", d.Count, a.Items))
	}
	return b.String()
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// slug makes a string usable inside a case id.
func slug(s string) string {
	out := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(out) > 40 {
		out = strings.Trim(out[:40], "-")
	}
	if out == "" {
		return "x"
	}
	return out
}
