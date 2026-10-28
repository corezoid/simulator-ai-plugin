package tools

import (
	"context"
	"testing"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
)

func TestWorkspaceArgContext(t *testing.T) {
	const ws = "8be56e1c-248a-44f5-86e4-d33c7177eb4c"
	ctx, err := WorkspaceArgContext(context.Background(), map[string]any{"accId": ws})
	if err != nil || apiclient.WorkspaceIDFromContext(ctx) != ws {
		t.Fatalf("accId not carried onto ctx: %q %v", apiclient.WorkspaceIDFromContext(ctx), err)
	}
	const conn = "11111111-1111-1111-1111-111111111111"
	ctx, _ = WorkspaceArgContext(apiclient.WithWorkspaceID(context.Background(), conn), map[string]any{"accId": ws})
	if got := apiclient.WorkspaceIDFromContext(ctx); got != conn {
		t.Fatalf("connection workspace overridden by an argument: %q", got)
	}
	if _, err := WorkspaceArgContext(context.Background(), map[string]any{"accId": "../../etc"}); err == nil {
		t.Fatal("non-UUID accId must be refused")
	}
}

// On stdio the composite tools take accId too: it overrides the configured
// workspace (here none) for the form lookup.
func TestCompositeToolsTakeAccID(t *testing.T) {
	const ws = "8be56e1c-248a-44f5-86e4-d33c7177eb4c"
	var last recordedReq
	srv := newSkillsServer(t, true, &last)
	defer srv.Close()
	c := apiclient.New(srv.URL, "", func() (string, error) { return "Simulator t", nil }, false)

	if res := callFindSkill(t, c, map[string]any{}); !res.IsError {
		t.Fatal("findSkill without any workspace must error")
	}
	if res := callFindSkill(t, c, map[string]any{"accId": ws}); res.IsError {
		t.Fatalf("findSkill with accId: %+v", res.Content)
	}
	if res := callGetSkill(t, c, map[string]any{"ref": "s", "accId": ws}); res.IsError || last.path != "/actors/ref/42/s" {
		t.Fatalf("getSkill(ref) with accId: path %q, %+v", last.path, res.Content)
	}
}
