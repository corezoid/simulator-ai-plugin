package tools

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestWebBaseURL(t *testing.T) {
	cases := map[string]string{
		"https://sim.simulator.company/papi/1.0":  "https://sim.simulator.company",
		"https://sim.simulator.company/papi/1.0/": "https://sim.simulator.company",
		"http://localhost:9000/papi/1.0":          "http://localhost:9000",
		"https://mw.simulator.company":            "https://mw.simulator.company",
	}
	for in, want := range cases {
		if got := webBaseURL(in); got != want {
			t.Errorf("webBaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShortAcc(t *testing.T) {
	cases := map[string]string{
		"af451011-2763-4bce-8b1b-c10bddd207ca": "af451011", // full UUID → 8-char form
		"af451011":                             "af451011", // already short, unchanged
		"shortish":                             "shortish", // non-UUID, unchanged
	}
	for in, want := range cases {
		if got := shortAcc(in); got != want {
			t.Errorf("shortAcc(%q) = %q, want %q", in, got, want)
		}
	}
}

// buildLink end-to-end: a real client (base incl. /papi/1.0, workspace = full
// UUID) produces an absolute web URL with the web base and short acc.
func TestBuildLinkHandler(t *testing.T) {
	c := apiclient.New("https://sim.simulator.company/papi/1.0",
		"af451011-2763-4bce-8b1b-c10bddd207ca",
		func() (string, error) { return "Simulator t", nil }, false)

	const actor = "a4a7f284-2763-4bce-8b1b-c10bddd207ca"
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"actor", map[string]any{"entity": "actor", "id": actor},
			"https://sim.simulator.company/actors_graph/af451011/view/" + actor},
		{"layer root", map[string]any{"entity": "layer", "id": "0"},
			"https://sim.simulator.company/actors_graph/af451011/graph/0/layers"},
		{"layer mode+focus", map[string]any{"entity": "layer", "id": "L1", "mode": "actors", "focusId": actor},
			"https://sim.simulator.company/actors_graph/af451011/graph/L1/actors/" + actor},
		{"event stream", map[string]any{"entity": "event", "streamId": "S1", "secondaryId": "S2"},
			"https://sim.simulator.company/events/af451011/list/S1/S2"},
		{"chat conversation (default stream)", map[string]any{"entity": "chat", "id": "C1"},
			"https://sim.simulator.company/chats/af451011/list/chats/C1?tab=chat"},
		{"chat conversation (explicit stream)", map[string]any{"entity": "chat", "streamId": "S1", "id": "C1"},
			"https://sim.simulator.company/chats/af451011/list/S1/C1?tab=chat"},
		{"chat list (no id)", map[string]any{"entity": "chat"},
			"https://sim.simulator.company/chats/af451011"},
		{"form new", map[string]any{"entity": "form"},
			"https://sim.simulator.company/form/af451011/edit"},
		{"explicit acc", map[string]any{"entity": "settings", "accId": "bbbb2222"},
			"https://sim.simulator.company/settings/bbbb2222"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := callBuildLink(t, c, tc.args); got != tc.want {
				t.Errorf("buildLink(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

// buildLink reports a clear error when a required id is missing.
func TestBuildLinkMissingID(t *testing.T) {
	c := apiclient.New("https://sim.simulator.company/papi/1.0", "af451011",
		func() (string, error) { return "t", nil }, false)
	res := invokeBuildLink(t, c, map[string]any{"entity": "actor"})
	if !res.IsError {
		t.Fatal("expected an error result when actor id is missing")
	}
}

// buildLink picks up hostOrigin / workspaceId / activeActor / activeLayer from the
// UI context (control-events-context) so "link to the open actor/layer" needs no ids.
func TestBuildLinkUsesUIContext(t *testing.T) {
	// Client base is localhost; the UI context must take precedence for the web base.
	c := apiclient.New("http://localhost:9000/papi/1.0", "",
		func() (string, error) { return "t", nil }, false)
	ui := apiclient.UIContext{
		HostOrigin:  "https://mw.simulator.company",
		WorkspaceID: "a58d969b-4b2f-42ce-add5-0972c4f45421", // full UUID → short "a58d969b"
		ActiveActor: "11111111-1111-1111-1111-111111111111",
		ActiveLayer: "21a7c6b6-bba1-4b19-9fcd-7e94f29a9e8a",
	}
	ctx := apiclient.WithUIContext(context.Background(), ui)

	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"open actor (no id)", map[string]any{"entity": "actor"},
			"https://mw.simulator.company/actors_graph/a58d969b/view/11111111-1111-1111-1111-111111111111"},
		{"open layer (no id)", map[string]any{"entity": "layer"},
			"https://mw.simulator.company/actors_graph/a58d969b/graph/21a7c6b6-bba1-4b19-9fcd-7e94f29a9e8a/layers"},
		{"explicit args override context", map[string]any{"entity": "actor", "accId": "bbbb2222", "id": "ffffffff-2222-3333-4444-555555555555"},
			"https://mw.simulator.company/actors_graph/bbbb2222/view/ffffffff-2222-3333-4444-555555555555"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := callBuildLinkCtx(t, c, ctx, tc.args); got != tc.want {
				t.Errorf("buildLink(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

// ParseUIContext decodes the base64-JSON header value pong-server sends.
func TestParseUIContext(t *testing.T) {
	raw := `{"hostOrigin":"https://mw.simulator.company","workspaceId":"ws-1","activeActor":"act-1","activeReaction":"rx-1","activeLayer":"lay-1","activeGraph":"gr-1"}`
	b64 := base64.StdEncoding.EncodeToString([]byte(raw))
	ui := apiclient.ParseUIContext(b64)
	if ui.HostOrigin != "https://mw.simulator.company" || ui.ActiveActor != "act-1" || ui.ActiveReaction != "rx-1" || ui.ActiveLayer != "lay-1" || ui.ActiveGraph != "gr-1" || ui.WorkspaceID != "ws-1" {
		t.Errorf("ParseUIContext(base64) = %+v", ui)
	}
	if got := apiclient.ParseUIContext(""); got != (apiclient.UIContext{}) {
		t.Errorf("ParseUIContext(\"\") = %+v, want zero", got)
	}
	if got := apiclient.ParseUIContext("!!!not-base64-or-json!!!"); got != (apiclient.UIContext{}) {
		t.Errorf("ParseUIContext(garbage) = %+v, want zero", got)
	}
}

// callBuildLinkCtx invokes the handler with a caller-supplied ctx and returns its text.
func callBuildLinkCtx(t *testing.T, c *apiclient.Client, ctx context.Context, args map[string]any) string {
	t.Helper()
	var req mcp.CallToolRequest
	req.Params.Name = "buildLink"
	req.Params.Arguments = args
	res, err := buildLinkHandler(c)(ctx, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %+v", res.Content)
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("result content is not text: %+v", res.Content[0])
	}
	return tc.Text
}

// callBuildLink invokes the tool and returns its text result, failing on error.
func callBuildLink(t *testing.T, c *apiclient.Client, args map[string]any) string {
	t.Helper()
	res := invokeBuildLink(t, c, args)
	if res.IsError {
		t.Fatalf("unexpected error result: %+v", res.Content)
	}
	if len(res.Content) == 0 {
		t.Fatal("empty result content")
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("result content is not text: %+v", res.Content[0])
	}
	return tc.Text
}

// invokeBuildLink registers buildLink on a throwaway server and calls it.
func invokeBuildLink(t *testing.T, c *apiclient.Client, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	h := buildLinkHandler(c)
	var req mcp.CallToolRequest
	req.Params.Name = "buildLink"
	req.Params.Arguments = args
	res, err := h(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	return res
}
