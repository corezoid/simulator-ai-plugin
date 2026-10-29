package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// getBbcodeTags is a local tool that fetches the environment's published BBCode
// vocabulary from <web-base>/bbcode-tags.json — the tags allowed in description
// text (chips like [actor=…], formatting like [b]/[color], pictures, and [md] for
// a markdown block), each with the places it is allowed in (`context`). The spec
// is per-environment (served at the web host root, like routes.json), so it is
// fetched live rather than embedded, then cached per environment for a while.
//
// The answer is a compact projection (no per-tag prose, no editor-only
// attributes) and can be narrowed to one context, so a skill can say "ask the
// tool for your context" instead of carrying its own copy of the per-place lists.
//
// Exposed in workspace mode and in actor mode (see actorBindings): it takes no
// actor arguments and fetches only from the configured environment.

// registerBbcodeTags adds the getBbcodeTags reference tool to s.
func registerBbcodeTags(s *server.MCPServer, c *apiclient.Client) {
	tool := mcp.NewTool("getBbcodeTags",
		mcp.WithDescription(
			"Fetch the BBCode tag vocabulary of the current environment — the tags you may use in description "+
				"text (chips like [actor=<uuid>]…[/actor], formatting, lists, code, pictures, [md]…[/md] for a "+
				"markdown block). Pass `context` for the place you write to (actor-description, reaction, "+
				"event-card for Events actors, layer-comment, form-label) to get only the tags allowed there. "+
				"Each entry has its attributes (in the order they must be written), an example and, without "+
				"`context`, the places it is allowed in. Use only the tags and attributes it returns. "+
				"Pass detail=true for each tag's full prose description. "+
				"IMPORTANT: BBCode tags are processed only OUTSIDE [md] blocks — inside [md]…[/md] the content is "+
				"markdown, so put chips/BBCode outside the [md] section."),
		mcp.WithString("context", mcp.Description("Place you write to, e.g. reaction or actor-description. Omit for every tag.")),
		mcp.WithBoolean("detail", mcp.Description("Include each tag's prose description (larger answer).")),
	)
	s.AddTool(tool, bbcodeTagsHandler(c))
}

// bbcodeMaxBytes caps the bbcode-tags.json body we read — the spec is ~20KB; the
// cap guards against a misconfigured/hostile web root streaming an unbounded body.
const bbcodeMaxBytes = 2 << 20 // 2 MiB

// bbcodeCacheTTL bounds how stale a cached vocabulary may get: an environment
// changes it only on a frontend deploy.
const bbcodeCacheTTL = 10 * time.Minute

// bbcodeVocabulary is the shape of bbcode-tags.json (pong-front-end
// bin/scripts/generateBbcodeTags.js, from packages/utils/modules/bbTagDefs.js).
type bbcodeVocabulary struct {
	Version string      `json:"version"`
	Tags    []bbcodeTag `json:"tags"`
}

type bbcodeTag struct {
	Tag         string            `json:"tag"`
	Description string            `json:"description,omitempty"`
	Attributes  []bbcodeAttribute `json:"attributes,omitempty"`
	SelfClosing bool              `json:"selfClosing,omitempty"`
	Context     []string          `json:"context,omitempty"`
	Example     string            `json:"example,omitempty"`
}

type bbcodeAttribute struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Description string `json:"description,omitempty"`
}

// bbcodeEditorOnlyAttrs are attributes the web editor writes for itself; an agent
// never needs them.
var bbcodeEditorOnlyAttrs = map[string]bool{"elementId": true}

type bbcodeCacheEntry struct {
	vocab   bbcodeVocabulary
	fetched time.Time
}

// bbcodeCache memoizes the parsed vocabulary per web base.
var bbcodeCache sync.Map // web base(string) -> bbcodeCacheEntry

// bbcodeTagsHandler serves the env's vocabulary, narrowed and compacted. Split out
// for testability. It fetches from the CONFIGURED environment base (per-request
// base override, else the client's base) with the /papi/1.0 suffix dropped — NOT
// from a client-supplied UI-context origin: the vocabulary belongs to the
// environment this server talks to, and using a header-supplied origin as a
// server-side fetch target would be an SSRF risk.
func bbcodeTagsHandler(c *apiclient.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		apiBase := apiclient.BaseURLFromContext(ctx)
		if apiBase == "" {
			apiBase = c.BaseURL()
		}
		base := webBaseURL(apiBase)
		if base == "" {
			return mcp.NewToolResultError("[Error] getBbcodeTags: no environment set — run set-environment first"), nil
		}
		vocab, err := loadBbcodeVocabulary(ctx, c, base)
		if err != nil {
			return mcp.NewToolResultError("[Error] getBbcodeTags: " + err.Error()), nil
		}
		args := req.GetArguments()
		place, _ := args["context"].(string)
		detail, _ := args["detail"].(bool)
		out, err := projectBbcodeVocabulary(vocab, strings.TrimSpace(place), detail)
		if err != nil {
			return mcp.NewToolResultError("[Error] getBbcodeTags: " + err.Error()), nil
		}
		body, err := json.Marshal(out)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] getBbcodeTags: %v", err)), nil
		}
		return mcp.NewToolResultText(string(body)), nil
	}
}

// loadBbcodeVocabulary returns the cached vocabulary for base, fetching it when
// absent or older than bbcodeCacheTTL. A failed fetch is not cached.
func loadBbcodeVocabulary(ctx context.Context, c *apiclient.Client, base string) (bbcodeVocabulary, error) {
	if e, ok := bbcodeCache.Load(base); ok {
		entry := e.(bbcodeCacheEntry)
		if time.Since(entry.fetched) < bbcodeCacheTTL {
			return entry.vocab, nil
		}
	}
	url := base + "/bbcode-tags.json"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return bbcodeVocabulary{}, err
	}
	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return bbcodeVocabulary{}, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, bbcodeMaxBytes))
	if err != nil {
		return bbcodeVocabulary{}, fmt.Errorf("read %s: %w", url, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return bbcodeVocabulary{}, fmt.Errorf("%s returned %d", url, resp.StatusCode)
	}
	var vocab bbcodeVocabulary
	if err := json.Unmarshal(body, &vocab); err != nil {
		return bbcodeVocabulary{}, fmt.Errorf("parse %s: %w", url, err)
	}
	bbcodeCache.Store(base, bbcodeCacheEntry{vocab: vocab, fetched: time.Now()})
	return vocab, nil
}

// bbcodeAnswer is what the tool returns: the vocabulary version, the context it
// was narrowed to (if any), a note when the narrowing could not be applied, and
// the tags.
type bbcodeAnswer struct {
	Version string      `json:"version,omitempty"`
	Context string      `json:"context,omitempty"`
	Note    string      `json:"note,omitempty"`
	Tags    []bbcodeTag `json:"tags"`
}

// projectBbcodeVocabulary keeps the tags allowed in place (all when empty), drops
// editor-only attributes, and drops prose descriptions unless detail is set.
// Within a single context the per-tag context list is redundant and dropped too.
// An unknown place is an error naming the places the vocabulary knows. An older
// vocabulary that lists no places at all cannot be narrowed: every tag comes back
// with a note saying so, rather than an error naming no places.
func projectBbcodeVocabulary(v bbcodeVocabulary, place string, detail bool) (bbcodeAnswer, error) {
	if place != "" && !slices.ContainsFunc(v.Tags, func(t bbcodeTag) bool { return len(t.Context) > 0 }) {
		out, err := projectBbcodeVocabulary(v, "", detail)
		out.Note = fmt.Sprintf("this environment's vocabulary does not say where each tag is allowed, so it "+
			"cannot be narrowed to %q; every tag is returned", place)
		return out, err
	}
	out := bbcodeAnswer{Version: v.Version, Context: place, Tags: []bbcodeTag{}}
	known := map[string]bool{}
	for _, t := range v.Tags {
		allowed := place == ""
		for _, c := range t.Context {
			known[c] = true
			if c == place {
				allowed = true
			}
		}
		if !allowed {
			continue
		}
		p := bbcodeTag{Tag: t.Tag, SelfClosing: t.SelfClosing, Example: t.Example}
		if detail {
			p.Description = t.Description
		}
		if place == "" {
			p.Context = t.Context
		}
		for _, a := range t.Attributes {
			if bbcodeEditorOnlyAttrs[a.Name] {
				continue
			}
			p.Attributes = append(p.Attributes, a)
		}
		out.Tags = append(out.Tags, p)
	}
	if place != "" && !known[place] {
		names := make([]string, 0, len(known))
		for k := range known {
			names = append(names, k)
		}
		sort.Strings(names)
		return bbcodeAnswer{}, fmt.Errorf("unknown context %q; this environment knows: %s", place, strings.Join(names, ", "))
	}
	return out, nil
}
