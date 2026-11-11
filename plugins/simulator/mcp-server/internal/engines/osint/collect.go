package osint

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"
)

// Limits of one collection.
const (
	defaultCollectItems = 200
	maxCollectItems     = 5000
	maxCollectPages     = 300
	maxPageBytes        = 5 << 20
	maxItemText         = 2000
	collectUserAgent    = "simulator-ai-plugin/osintCollect (+https://github.com/corezoid/simulator-ai-plugin)"
)

// Item is one thing a source returned: a post, an article, a page.
type Item struct {
	ID       string             `json:"id"`
	URL      string             `json:"url,omitempty"`
	Date     string             `json:"date,omitempty"` // RFC 3339, or as the source gave it
	Author   string             `json:"author,omitempty"`
	Title    string             `json:"title,omitempty"`
	Text     string             `json:"text,omitempty"`
	Links    []string           `json:"links,omitempty"`
	Mentions []string           `json:"mentions,omitempty"`
	Tags     []string           `json:"tags,omitempty"`
	Metrics  map[string]float64 `json:"metrics,omitempty"`
	Image    string             `json:"image,omitempty"`
}

// Collection is what one osintCollect call brought back.
type Collection struct {
	Source      string `json:"source"`
	Kind        string `json:"kind"`
	Target      string `json:"target"`
	URL         string `json:"url,omitempty"`
	Retrieved   string `json:"retrieved"`
	SourceKind  string `json:"sourceKind,omitempty"`
	Reliability string `json:"reliability,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"` // the source had more than the limit
	Items       []Item `json:"items"`
}

// CollectOptions bound a collection.
type CollectOptions struct {
	Limit int
	Since time.Time // items older than this end the collection
}

// fetcher gets one URL. Tests replace it; the real one is httpFetch.
type fetcher func(ctx context.Context, rawURL string, headers map[string]string) ([]byte, error)

// httpFetch gets a URL with the plugin's client for user-given URLs. A
// redirect to another host is refused: the request may carry the user's key,
// and Go forwards custom headers such as X-Api-Key wherever a redirect points.
func httpFetch(ctx context.Context, rawURL string, headers map[string]string) ([]byte, error) {
	if err := ecore.CheckUserURL(rawURL); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", collectUserAgent)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := *ecore.UserURLClient()
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		if next.URL.Host != via[0].URL.Host {
			return fmt.Errorf("redirect to another host (%s) is not followed", next.URL.Host)
		}
		return ecore.CheckUserURL(next.URL.String())
	}
	resp, err := client.Do(req)
	if err != nil {
		// a transport error may quote the URL, which may hold a key
		return nil, fmt.Errorf("request to %s failed", hostOf(rawURL))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading the reply of %s failed", hostOf(rawURL))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered HTTP %d", hostOf(rawURL), resp.StatusCode)
	}
	if len(body) > maxPageBytes {
		return nil, fmt.Errorf("the reply of %s is larger than %d bytes", hostOf(rawURL), maxPageBytes)
	}
	return body, nil
}

func hostOf(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil && u.Host != "" {
		return u.Host
	}
	return "the source"
}

// Collect runs one source against a target.
func Collect(ctx context.Context, sources *Sources, source, target string, opt CollectOptions, fetch fetcher) (*Collection, error) {
	if fetch == nil {
		fetch = httpFetch
	}
	if opt.Limit <= 0 {
		opt.Limit = defaultCollectItems
	}
	opt.Limit = min(opt.Limit, maxCollectItems)
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, fmt.Errorf("target is empty")
	}
	c := &Collection{Source: source, Target: target, Retrieved: time.Now().UTC().Format(time.RFC3339), Items: []Item{}}
	for _, b := range builtinSources {
		if b.ID == source {
			c.Kind, c.SourceKind, c.Reliability = b.Kind, b.SourceKind, b.Reliability
		}
	}
	var err error
	switch source {
	case KindTelegram:
		err = collectTelegram(ctx, c, opt, fetch)
	case KindWebPage:
		err = collectWebPage(ctx, c, fetch)
	case KindRSS:
		err = collectRSS(ctx, c, opt, fetch)
	default:
		cfg := sources.find(source)
		if cfg == nil {
			return nil, fmt.Errorf("no source %q: use %s, %s, %s, or one from the sources file", source, KindTelegram, KindWebPage, KindRSS)
		}
		c.Kind, c.SourceKind, c.Reliability = cfg.Kind, cfg.SourceKind, cfg.Reliability
		err = collectHTTPJSON(ctx, c, cfg, opt, fetch)
	}
	if err != nil {
		return nil, err
	}
	for i := range c.Items {
		enrich(&c.Items[i])
	}
	return c, nil
}

// collectPause is the wait between two requests to one source.
var collectPause = 400 * time.Millisecond

// pause waits between two requests to the same source.
func pause(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(collectPause):
		return nil
	}
}

// ---- text helpers ------------------------------------------------------------------

var (
	tagRe     = regexp.MustCompile(`(?s)<[^>]*>`)
	spaceRe   = regexp.MustCompile(`\s+`)
	mentionRe = regexp.MustCompile(`(?:^|[^\w/@.])@([A-Za-z][A-Za-z0-9_.]{2,31})`)
	hashtagRe = regexp.MustCompile(`#([\p{L}\p{N}_]{2,50})`)
	urlRe     = regexp.MustCompile(`https?://[^\s<>"')\]]+`)
	hrefRe    = regexp.MustCompile(`href="(https?://[^"]+)"`)
)

// plain turns a fragment of HTML into one line of text.
func plain(fragment string) string {
	return strings.TrimSpace(spaceRe.ReplaceAllString(html.UnescapeString(tagRe.ReplaceAllString(fragment, " ")), " "))
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// enrich reads the mentions, hashtags and links out of an item's text.
func enrich(it *Item) {
	it.Text = clip(it.Text, maxItemText)
	text := it.Title + " " + it.Text
	for _, m := range mentionRe.FindAllStringSubmatch(text, -1) {
		it.Mentions = append(it.Mentions, strings.ToLower(strings.TrimRight(m[1], ".")))
	}
	for _, m := range hashtagRe.FindAllStringSubmatch(text, -1) {
		it.Tags = append(it.Tags, strings.ToLower(m[1]))
	}
	for _, u := range urlRe.FindAllString(text, -1) {
		it.Links = append(it.Links, strings.TrimRight(u, ".,;:!?"))
	}
	it.Mentions, it.Tags, it.Links = uniq(it.Mentions), uniq(it.Tags), uniq(it.Links)
}

// ---- Telegram ----------------------------------------------------------------------

var (
	tgNameRe    = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{3,31}$`)
	tgPostRe    = regexp.MustCompile(`data-post="[^"/]+/(\d+)"`)
	tgTextRe    = regexp.MustCompile(`(?s)<div class="tgme_widget_message_text[^"]*"[^>]*>(.*?)</div>\s*(?:<div class="tgme_widget_message_(?:footer|reactions)|<a class="tgme_widget_message_link_preview|</div>)`)
	tgTimeRe    = regexp.MustCompile(`<time[^>]*datetime="([^"]+)"`)
	tgViewsRe   = regexp.MustCompile(`tgme_widget_message_views">([^<]+)<`)
	tgCounterRe = regexp.MustCompile(`<span class="counter_value">([^<]*)</span>\s*<span class="counter_type">([^<]*)</span>`)
)

// telegramChannel reads a channel name out of "name", "@name" or a t.me link.
func telegramChannel(target string) (string, error) {
	t := strings.TrimPrefix(strings.TrimSpace(target), "@")
	if u, err := url.Parse(t); err == nil && (u.Host == "t.me" || u.Host == "telegram.me") {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) > 0 && parts[0] == "s" {
			parts = parts[1:]
		}
		if len(parts) > 0 {
			t = parts[0]
		}
	}
	if !tgNameRe.MatchString(t) {
		return "", fmt.Errorf("target %q is not a Telegram channel name (give the name, @name or a t.me link)", target)
	}
	return t, nil
}

// figure reads a number such as "13.1K" or "2 428".
func figure(s string) (float64, bool) {
	s = strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), " ", ""), " ", "")
	mult := 1.0
	switch {
	case strings.HasSuffix(s, "K"):
		mult, s = 1e3, strings.TrimSuffix(s, "K")
	case strings.HasSuffix(s, "M"):
		mult, s = 1e6, strings.TrimSuffix(s, "M")
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	return v * mult, err == nil
}

// collectTelegram pages back through the public preview of a channel, newest
// posts first. Only what Telegram shows without a login is read.
func collectTelegram(ctx context.Context, c *Collection, opt CollectOptions, fetch fetcher) error {
	name, err := telegramChannel(c.Target)
	if err != nil {
		return err
	}
	c.Target, c.URL = name, "https://t.me/s/"+name
	seen := map[int]bool{}
	before := 0
	for page := 0; page < maxCollectPages && len(c.Items) < opt.Limit; page++ {
		pageURL := c.URL
		if before > 0 {
			pageURL += "?before=" + strconv.Itoa(before)
		}
		body, err := fetch(ctx, pageURL, nil)
		if err != nil {
			return err
		}
		blocks := strings.Split(string(body), `<div class="tgme_widget_message_wrap`)[1:]
		if len(blocks) == 0 {
			if page == 0 {
				return fmt.Errorf("%s shows no posts: the channel does not exist, is private, or has its preview turned off", c.URL)
			}
			break
		}
		lowest, old := 0, false
		// newest first within the page too, so the limit keeps the latest posts
		for i := len(blocks) - 1; i >= 0; i-- {
			b := blocks[i]
			m := tgPostRe.FindStringSubmatch(b)
			if m == nil {
				continue
			}
			id, _ := strconv.Atoi(m[1])
			if lowest == 0 || id < lowest {
				lowest = id
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			it := Item{ID: m[1], URL: "https://t.me/" + name + "/" + m[1]}
			if t := tgTimeRe.FindStringSubmatch(b); t != nil {
				it.Date = t[1]
				if when, err := time.Parse(time.RFC3339, t[1]); err == nil && !opt.Since.IsZero() && when.Before(opt.Since) {
					old = true
					continue
				}
			}
			if t := tgTextRe.FindStringSubmatch(b); t != nil {
				it.Text = plain(t[1])
				for _, h := range hrefRe.FindAllStringSubmatch(t[1], -1) {
					it.Links = append(it.Links, html.UnescapeString(h[1]))
				}
			}
			if v := tgViewsRe.FindStringSubmatch(b); v != nil {
				if n, ok := figure(v[1]); ok {
					it.Metrics = map[string]float64{"views": n}
				}
			}
			if len(c.Items) < opt.Limit {
				c.Items = append(c.Items, it)
			} else {
				c.Truncated = true
			}
		}
		if old || lowest <= 1 || lowest == before {
			break
		}
		before = lowest
		if len(c.Items) >= opt.Limit {
			c.Truncated = true
			break
		}
		if err := pause(ctx); err != nil {
			return err
		}
	}
	sort.SliceStable(c.Items, func(i, j int) bool { return c.Items[i].Date > c.Items[j].Date })
	return nil
}

// ---- web page ----------------------------------------------------------------------

var (
	metaRe  = regexp.MustCompile(`(?is)<meta\s+[^>]*>`)
	attrRe  = regexp.MustCompile(`(?is)([a-z:-]+)\s*=\s*"([^"]*)"`)
	titleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
)

// collectWebPage reads what a page says about itself: its title, description
// and picture. One item.
func collectWebPage(ctx context.Context, c *Collection, fetch fetcher) error {
	u, err := url.Parse(c.Target)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("target %q is not an http(s) URL", c.Target)
	}
	c.URL = c.Target
	body, err := fetch(ctx, c.Target, nil)
	if err != nil {
		return err
	}
	meta := map[string]string{}
	for _, tag := range metaRe.FindAllString(string(body), -1) {
		attrs := map[string]string{}
		for _, a := range attrRe.FindAllStringSubmatch(tag, -1) {
			attrs[strings.ToLower(a[1])] = html.UnescapeString(a[2])
		}
		key := attrs["property"]
		if key == "" {
			key = attrs["name"]
		}
		if key != "" && attrs["content"] != "" {
			meta[strings.ToLower(key)] = attrs["content"]
		}
	}
	it := Item{ID: c.Target, URL: c.Target, Title: meta["og:title"], Text: meta["og:description"], Image: meta["og:image"],
		Author: meta["author"], Date: meta["article:published_time"]}
	if it.Title == "" {
		if t := titleRe.FindStringSubmatch(string(body)); t != nil {
			it.Title = plain(t[1])
		}
	}
	if it.Text == "" {
		it.Text = meta["description"]
	}
	if !strings.HasPrefix(it.Image, "https://") {
		it.Image = ""
	}
	c.Items = append(c.Items, it)
	return nil
}

// ---- RSS / Atom ---------------------------------------------------------------------

type feed struct {
	Items   []feedItem `xml:"channel>item"`
	Entries []feedItem `xml:"entry"`
}

type feedItem struct {
	GUID      string   `xml:"guid"`
	ID        string   `xml:"id"`
	Title     string   `xml:"title"`
	Links     []feedLn `xml:"link"`
	PubDate   string   `xml:"pubDate"`
	Published string   `xml:"published"`
	Updated   string   `xml:"updated"`
	Desc      string   `xml:"description"`
	Summary   string   `xml:"summary"`
	Author    struct {
		Name string `xml:"name"`
		Text string `xml:",chardata"`
	} `xml:"author"`
	Creator  string   `xml:"http://purl.org/dc/elements/1.1/ creator"`
	Category []string `xml:"category"`
}

// feedLn is a link of either dialect: RSS puts the address in the element,
// Atom in its href attribute.
type feedLn struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Text string `xml:",chardata"`
}

var feedDates = []string{time.RFC1123Z, time.RFC1123, time.RFC3339, "Mon, 2 Jan 2006 15:04:05 -0700", "2006-01-02T15:04:05Z07:00", "2006-01-02"}

func feedDate(s string) (string, time.Time) {
	s = strings.TrimSpace(s)
	for _, layout := range feedDates {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format(time.RFC3339), t
		}
	}
	return s, time.Time{}
}

func collectRSS(ctx context.Context, c *Collection, opt CollectOptions, fetch fetcher) error {
	u, err := url.Parse(c.Target)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("target %q is not an http(s) URL of a feed", c.Target)
	}
	c.URL = c.Target
	body, err := fetch(ctx, c.Target, nil)
	if err != nil {
		return err
	}
	var f feed
	dec := xml.NewDecoder(strings.NewReader(string(body)))
	dec.Strict = false
	if err := dec.Decode(&f); err != nil {
		return fmt.Errorf("%s is not an RSS or Atom feed", hostOf(c.Target))
	}
	for _, e := range append(f.Items, f.Entries...) {
		link := ""
		for _, l := range e.Links {
			if addr := firstNonEmpty(l.Href, l.Text); addr != "" && (link == "" || l.Rel == "alternate") {
				link = addr
			}
		}
		date, when := feedDate(firstNonEmpty(e.PubDate, e.Published, e.Updated))
		if !opt.Since.IsZero() && !when.IsZero() && when.Before(opt.Since) {
			continue
		}
		it := Item{ID: firstNonEmpty(e.GUID, e.ID, link), URL: link, Date: date, Title: plain(e.Title),
			Text: plain(firstNonEmpty(e.Desc, e.Summary)), Author: plain(firstNonEmpty(e.Creator, e.Author.Name, e.Author.Text))}
		for _, cat := range e.Category {
			if cat = plain(cat); cat != "" {
				it.Tags = append(it.Tags, strings.ToLower(cat))
			}
		}
		if len(c.Items) >= opt.Limit {
			c.Truncated = true
			break
		}
		c.Items = append(c.Items, it)
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// ---- any JSON API -------------------------------------------------------------------

// dig follows a dotted path ("data.items", "author.name") into decoded JSON.
func dig(v any, path string) any {
	if path == "" {
		return v
	}
	for _, key := range strings.Split(path, ".") {
		switch x := v.(type) {
		case map[string]any:
			v = x[key]
		case []any:
			i, err := strconv.Atoi(key)
			if err != nil || i < 0 || i >= len(x) {
				return nil
			}
			v = x[i]
		default:
			return nil
		}
	}
	return v
}

func scalar(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return x.String()
	case bool:
		return strconv.FormatBool(x)
	}
	return ""
}

// collectHTTPJSON calls a JSON API the user described: the URL with {target},
// {page} and {cursor} filled in, the list at `items`, each item's parts at the
// paths in `fields`.
func collectHTTPJSON(ctx context.Context, c *Collection, cfg *SourceConfig, opt CollectOptions, fetch fetcher) error {
	headers := map[string]string{"Accept": "application/json"}
	for k, v := range cfg.Headers {
		expanded, err := expandEnv(v)
		if err != nil {
			return fmt.Errorf("source %s: %w", cfg.ID, err)
		}
		headers[k] = expanded
	}
	base, err := expandEnv(cfg.URL)
	if err != nil {
		return fmt.Errorf("source %s: %w", cfg.ID, err)
	}
	paged := strings.Contains(cfg.URL, "{page}") || strings.Contains(cfg.URL, "{cursor}")
	page, cursor := 1, ""
	if cfg.FirstPage != nil {
		page = *cfg.FirstPage
	}
	seen := map[string]bool{}
	for n := 0; n < maxCollectPages && len(c.Items) < opt.Limit; n++ {
		pageURL := strings.NewReplacer("{target}", url.QueryEscape(c.Target), "{page}", strconv.Itoa(page), "{cursor}", url.QueryEscape(cursor)).Replace(base)
		body, err := fetch(ctx, pageURL, headers)
		if err != nil {
			return fmt.Errorf("source %s: %w", cfg.ID, err)
		}
		var reply any
		dec := json.NewDecoder(strings.NewReader(string(body)))
		dec.UseNumber()
		if err := dec.Decode(&reply); err != nil {
			return fmt.Errorf("source %s: the reply is not JSON", cfg.ID)
		}
		itemsPath := cfg.Items
		if itemsPath == "." {
			itemsPath = "" // the reply itself is the list
		}
		list, ok := dig(reply, itemsPath).([]any)
		if !ok {
			return fmt.Errorf("source %s: the reply has no list at %q", cfg.ID, cfg.Items)
		}
		fresh := 0
		for _, raw := range list {
			it := Item{}
			for field, path := range cfg.Fields {
				v := scalar(dig(raw, path))
				switch field {
				case "id":
					it.ID = v
				case "date":
					it.Date = v
				case "url":
					it.URL = v
				case "author":
					it.Author = plain(v)
				case "title":
					it.Title = plain(v)
				case "text":
					it.Text = plain(v)
				}
			}
			for name, path := range cfg.Metrics {
				if n, ok := dig(raw, path).(json.Number); ok {
					if v, err := n.Float64(); err == nil {
						if it.Metrics == nil {
							it.Metrics = map[string]float64{}
						}
						it.Metrics[name] = v
					}
				}
			}
			if it.ID == "" {
				it.ID = firstNonEmpty(it.URL, fmt.Sprintf("%s-%d", cfg.ID, len(c.Items)+1))
			}
			if seen[it.ID] {
				continue
			}
			seen[it.ID] = true
			fresh++
			if when, err := time.Parse(time.RFC3339, it.Date); err == nil && !opt.Since.IsZero() && when.Before(opt.Since) {
				continue
			}
			if len(c.Items) >= opt.Limit {
				c.Truncated = true
				break
			}
			c.Items = append(c.Items, it)
		}
		if !paged || fresh == 0 {
			break
		}
		if cfg.Cursor != "" {
			if cursor = scalar(dig(reply, cfg.Cursor)); cursor == "" {
				break
			}
		}
		page++
		if err := pause(ctx); err != nil {
			return err
		}
	}
	return nil
}
