package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/mark3labs/mcp-go/mcp"
)

const bbcodeFixture = `{"version":"1","generatedAt":"2026-10-28T00:00:00Z","tags":[
 {"tag":"actor","description":"Actor chip.","attributes":[{"name":"id","type":"uuid","required":true,"description":"Actor UUID."},{"name":"elementId","type":"string","description":"Editor only."}],"context":["actor-description","reaction"],"example":"[actor=u]A[/actor]"},
 {"tag":"img","description":"Stored picture.","attributes":[{"name":"ref","type":"string","required":true,"description":"File path."},{"name":"width","type":"integer"},{"name":"height","type":"integer"}],"context":["actor-description"],"example":"[img=a/b width=1 height=2]x[/img]"},
 {"tag":"br","description":"Line break.","selfClosing":true,"context":["actor-description","reaction"],"example":"a[br]b"}
]}`

// newBbcodeServer serves the fixture at /bbcode-tags.json and counts the fetches.
func newBbcodeServer(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bbcode-tags.json" {
			http.NotFound(w, r)
			return
		}
		hits.Add(1)
		_, _ = w.Write([]byte(bbcodeFixture))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func callBbcodeTags(t *testing.T, c *apiclient.Client, args map[string]any) (string, bool) {
	t.Helper()
	var req mcp.CallToolRequest
	req.Params.Arguments = args
	res, err := bbcodeTagsHandler(c)(context.Background(), req)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	return res.Content[0].(mcp.TextContent).Text, res.IsError
}

func TestGetBbcodeTagsProjection(t *testing.T) {
	var hits atomic.Int32
	srv := newBbcodeServer(t, &hits)
	c := apiclient.New(srv.URL+"/papi/1.0", "ws", func() (string, error) { return "Simulator t", nil }, false)

	cases := []struct {
		name     string
		args     map[string]any
		wantTags []string
		wantErr  string
	}{
		{name: "all tags", args: nil, wantTags: []string{"actor", "img", "br"}},
		{name: "one context", args: map[string]any{"context": "reaction"}, wantTags: []string{"actor", "br"}},
		{name: "unknown context", args: map[string]any{"context": "nowhere"}, wantErr: "knows: actor-description, reaction"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text, isErr := callBbcodeTags(t, c, tc.args)
			if tc.wantErr != "" {
				if !isErr || !strings.Contains(text, tc.wantErr) {
					t.Fatalf("want error containing %q, got %q", tc.wantErr, text)
				}
				return
			}
			var got bbcodeAnswer
			if err := json.Unmarshal([]byte(text), &got); err != nil {
				t.Fatalf("decode %q: %v", text, err)
			}
			var names []string
			for _, tag := range got.Tags {
				names = append(names, tag.Tag)
				if tag.Description != "" {
					t.Errorf("%s: prose kept without detail", tag.Tag)
				}
				if (tc.args["context"] != nil) == (len(tag.Context) > 0) {
					t.Errorf("%s: context list %v with narrowing %v", tag.Tag, tag.Context, tc.args["context"])
				}
			}
			if strings.Join(names, ",") != strings.Join(tc.wantTags, ",") {
				t.Fatalf("tags = %v, want %v", names, tc.wantTags)
			}
			if strings.Contains(text, "elementId") {
				t.Error("editor-only attribute leaked")
			}
		})
	}

	text, _ := callBbcodeTags(t, c, map[string]any{"detail": true})
	if !strings.Contains(text, "Stored picture.") || !strings.Contains(text, `"width"`) {
		t.Errorf("detail answer lacks prose or attribute order: %s", text)
	}
	// Attributes keep the vocabulary's order: width before height.
	if strings.Index(text, `"width"`) > strings.Index(text, `"height"`) {
		t.Error("attribute order changed")
	}
	if hits.Load() != 1 {
		t.Errorf("vocabulary fetched %d times, want 1 (cached per environment)", hits.Load())
	}
}

func TestProjectBbcodeVocabularyWithoutContexts(t *testing.T) {
	v := bbcodeVocabulary{Tags: []bbcodeTag{{Tag: "b"}, {Tag: "i"}}}
	got, err := projectBbcodeVocabulary(v, "reaction", false)
	if err != nil {
		t.Fatalf("want every tag for a vocabulary that lists no places, got %v", err)
	}
	if len(got.Tags) != 2 || got.Context != "" || !strings.Contains(got.Note, `"reaction"`) {
		t.Errorf("got %+v, want both tags, no context and a note naming the place", got)
	}
}
