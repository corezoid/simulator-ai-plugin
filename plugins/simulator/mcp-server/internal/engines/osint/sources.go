package osint

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Source kinds. The first three are built in and need no configuration; an
// http_json source is described by the user in the sources file.
const (
	KindTelegram = "telegram_channel" // public preview of a Telegram channel, t.me/s/<channel>
	KindWebPage  = "web_page"         // one page: its title, description and picture
	KindRSS      = "rss"              // an RSS or Atom feed
	KindHTTPJSON = "http_json"        // any JSON API, mapped by the sources file
)

// SourcesFile is the default name of the user's sources file.
const SourcesFile = "osint-sources.yaml"

// sourceInfo describes a source to the user.
type sourceInfo struct {
	ID          string
	Kind        string
	Title       string
	SourceKind  string // the `kind` its case source entry gets
	Reliability string // a starting grade; the investigator sets the real one
}

var builtinSources = []sourceInfo{
	{ID: KindTelegram, Kind: KindTelegram, Title: "Public Telegram channel (t.me/s/<channel>, no login)", SourceKind: "social", Reliability: "B"},
	{ID: KindWebPage, Kind: KindWebPage, Title: "A web page: title, description, picture", SourceKind: "other", Reliability: "C"},
	{ID: KindRSS, Kind: KindRSS, Title: "RSS or Atom feed", SourceKind: "media", Reliability: "C"},
}

// SourceConfig is one source the user has set up. Only http_json sources are
// configured; the built-in kinds are always there.
type SourceConfig struct {
	ID          string            `yaml:"id"`
	Kind        string            `yaml:"kind"`
	Title       string            `yaml:"title,omitempty"`
	URL         string            `yaml:"url"`
	Headers     map[string]string `yaml:"headers,omitempty"`
	Items       string            `yaml:"items,omitempty"`
	Fields      map[string]string `yaml:"fields,omitempty"`
	Cursor      string            `yaml:"cursor,omitempty"`
	FirstPage   *int              `yaml:"first_page,omitempty"`
	SourceKind  string            `yaml:"source_kind,omitempty"`
	Reliability string            `yaml:"reliability,omitempty"`
}

// Sources is the parsed sources file.
type Sources struct {
	Sources []SourceConfig `yaml:"sources"`
}

// itemFields are the keys of `fields`: the parts of an item a source can fill.
var itemFields = []string{"id", "date", "url", "author", "title", "text"}

var (
	sourceIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)
	// A secret is read from an environment variable named for this purpose
	// only. Without the prefix a sources file could name any variable of the
	// process — a cloud key, the Simulator token — and send it to its URL.
	envRefRe    = regexp.MustCompile(`\$\{([^}]*)\}`)
	envNameRe   = regexp.MustCompile(`^OSINT_[A-Z0-9_]{1,60}$`)
	placeholder = regexp.MustCompile(`\{(target|page|cursor)\}`)
)

// LoadSources parses a sources file and checks it.
func LoadSources(src []byte) (*Sources, error) {
	dec := yaml.NewDecoder(bytes.NewReader(src))
	dec.KnownFields(true)
	s := &Sources{}
	if err := dec.Decode(s); err != nil {
		return nil, fmt.Errorf("sources YAML: %w", err)
	}
	seen := map[string]bool{}
	for i := range s.Sources {
		c := &s.Sources[i]
		where := fmt.Sprintf("sources[%d] %s", i, c.ID)
		switch {
		case !sourceIDRe.MatchString(c.ID):
			return nil, fmt.Errorf("%s: id must be lower-case letters, digits, '_' or '-' (1-40 chars)", where)
		case seen[c.ID] || slices.ContainsFunc(builtinSources, func(b sourceInfo) bool { return b.ID == c.ID }):
			return nil, fmt.Errorf("%s: id is already taken", where)
		case c.Kind != KindHTTPJSON:
			return nil, fmt.Errorf("%s: kind %q — only %s sources are configured; %s, %s and %s are built in", where, c.Kind, KindHTTPJSON, KindTelegram, KindWebPage, KindRSS)
		}
		seen[c.ID] = true
		u, err := url.Parse(envRefRe.ReplaceAllString(placeholder.ReplaceAllString(c.URL, "x"), "x"))
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
			return nil, fmt.Errorf("%s: url must be an http(s) URL", where)
		}
		secrets := false
		for _, v := range append(sortedValues(c.Headers), c.URL) {
			for _, m := range envRefRe.FindAllStringSubmatch(v, -1) {
				if !envNameRe.MatchString(m[1]) {
					return nil, fmt.Errorf("%s: ${%s} — a key is read only from an environment variable named OSINT_…", where, m[1])
				}
				secrets = true
			}
		}
		if secrets && u.Scheme != "https" {
			return nil, fmt.Errorf("%s: a source that sends a key must use https", where)
		}
		if c.Items == "" {
			return nil, fmt.Errorf("%s: items (the path to the list in the reply) is missing", where)
		}
		for k := range c.Fields {
			if !slices.Contains(itemFields, k) {
				return nil, fmt.Errorf("%s: fields.%s — the item fields are %s", where, k, strings.Join(itemFields, ", "))
			}
		}
		if c.Reliability != "" && !slices.Contains([]string{"A", "B", "C", "D", "E", "F"}, c.Reliability) {
			return nil, fmt.Errorf("%s: reliability %q is not A-F", where, c.Reliability)
		}
		if c.Cursor != "" && !strings.Contains(c.URL, "{cursor}") {
			return nil, fmt.Errorf("%s: cursor is set but the url has no {cursor}", where)
		}
	}
	return s, nil
}

func sortedValues(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = m[k]
	}
	return out
}

// envNames lists the environment variables a source reads.
func (c *SourceConfig) envNames() []string {
	set := map[string]bool{}
	for _, v := range append(sortedValues(c.Headers), c.URL) {
		for _, m := range envRefRe.FindAllStringSubmatch(v, -1) {
			set[m[1]] = true
		}
	}
	return sortedKeys(set)
}

// expandEnv replaces ${OSINT_…} with the variable's value. A variable that is
// not set is an error that names it, never its value.
func expandEnv(s string) (string, error) {
	var missing string
	out := envRefRe.ReplaceAllStringFunc(s, func(ref string) string {
		name := ref[2 : len(ref)-1]
		v, ok := os.LookupEnv(name)
		if !ok || v == "" {
			missing = name
			return ""
		}
		return v
	})
	if missing != "" {
		return "", fmt.Errorf("environment variable %s is not set", missing)
	}
	return out, nil
}

// find returns the configured source with the given id.
func (s *Sources) find(id string) *SourceConfig {
	if s == nil {
		return nil
	}
	for i := range s.Sources {
		if s.Sources[i].ID == id {
			return &s.Sources[i]
		}
	}
	return nil
}
