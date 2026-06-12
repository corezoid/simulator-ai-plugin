package mcpserver_test

import (
	"context"
	"errors"
	"testing"

	mcpserver "github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/app/mcpserver"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// One stateless server: tools/list switches between the full curated catalogue
// (no actor on ctx) and the per-actor subset (WithActorID on ctx) based on the
// per-request value alone. Same server instance, two views.
func TestUnifiedServerSwitchesModeOnCtxActor(t *testing.T) {
	srv, _, err := mcpserver.New(mcpserver.Options{
		Stateless:  true,
		AuthHeader: func() (string, error) { return "", errors.New("not used") },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cli, err := client.NewInProcessClient(srv)
	if err != nil {
		t.Fatalf("new in-process client: %v", err)
	}
	defer func() { _ = cli.Close() }()

	ctx := context.Background()
	if err := cli.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}

	var init mcp.InitializeRequest
	init.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	init.Params.ClientInfo = mcp.Implementation{Name: "unified-test", Version: "0"}
	if _, err := cli.Initialize(ctx, init); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	// Full mode: workspace-wide tools and engine tools are present.
	fullList, err := cli.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("tools/list (full): %v", err)
	}
	full := toolMap(fullList.Tools)
	for _, want := range []string{"getCurrencies", "getWorkspaces", "createForm"} {
		if _, ok := full[want]; !ok {
			t.Errorf("full mode is missing workspace-wide tool %q", want)
		}
	}
	if _, ok := full["getActor"]; !ok {
		t.Error("full mode is missing getActor")
	}
	// In full mode the actor-scoped tools still expose actorId — the model
	// supplies it explicitly.
	if !schemaContains(t, full["getActor"], `"actorId"`) {
		t.Error("full mode getActor should expose actorId in its schema")
	}

	// Actor mode: same server, ctx carries WithActorID.
	actorCtx := mcpserver.WithActorID(ctx, "act-XYZ")
	actorList, err := cli.ListTools(actorCtx, mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("tools/list (actor): %v", err)
	}
	actor := toolMap(actorList.Tools)

	// Same set as the standalone per-actor server: actor-scoped reads + CRUD.
	for _, want := range []string{
		"getActor", "getAccounts", "createAccount", "updateAccount", "deleteAccount",
		"getReactions", "createReaction", "updateReaction", "deleteReaction",
		"getRelatedActors", "getLinkedActors", "getActorLinks",
		"createLink", "existLink", "updateEdge", "deleteEdge",
		"getActorAttachments", "uploadBase64", "addAttachments", "removeAttachments",
		"getAccessRules", "saveAccessRules",
	} {
		if _, ok := actor[want]; !ok {
			t.Errorf("actor mode is missing tool %q", want)
		}
	}
	// Workspace-wide tools and engine tools are filtered out.
	for _, notWant := range []string{"getCurrencies", "getWorkspaces", "createForm", "login", "set-workspace", "set-environment"} {
		if _, ok := actor[notWant]; ok {
			t.Errorf("actor mode must not expose %q", notWant)
		}
	}
	// Bound identity is stripped from every actor-mode schema.
	for name, tool := range actor {
		if schemaContains(t, tool, `"actorId"`) {
			t.Errorf("actor mode leaks the bound actorId in %q", name)
		}
	}
	for _, name := range []string{"getAccessRules", "saveAccessRules"} {
		for _, hidden := range []string{`"objType"`, `"objId"`} {
			if schemaContains(t, actor[name], hidden) {
				t.Errorf("actor mode leaks bound %s in %q", hidden, name)
			}
		}
	}
	for _, name := range []string{"createLink", "existLink"} {
		if schemaContains(t, actor[name], `"source"`) {
			t.Errorf("actor mode leaks the bound source in %q", name)
		}
	}

	// Flipping the same client back to full mode (no actor on the next call's
	// ctx) restores the full catalogue — the switch is per request, not per
	// session, so the test for that is just symmetric coverage.
	again, err := cli.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("tools/list (full, second call): %v", err)
	}
	if got := len(again.Tools); got <= len(actor) {
		t.Errorf("full-mode catalogue smaller than actor-mode (%d vs %d) — filter is leaking state", got, len(actor))
	}
}

func toolMap(list []mcp.Tool) map[string]mcp.Tool {
	out := make(map[string]mcp.Tool, len(list))
	for _, t := range list {
		out[t.Name] = t
	}
	return out
}

