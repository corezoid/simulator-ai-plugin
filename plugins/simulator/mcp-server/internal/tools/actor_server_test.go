package tools

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// actorScopedOps must contain only operations that carry an {actorId} path
// placeholder, include the obvious per-actor reads + the actor's attachments, and
// exclude workspace-wide tools (which take accId, not actorId).
func TestActorScopedOps(t *testing.T) {
	ops := actorScopedOps()
	if len(ops) == 0 {
		t.Fatal("actorScopedOps() is empty")
	}

	byName := map[string]Operation{}
	for _, op := range ops {
		byName[op.Name] = op
		ok := false
		for _, p := range op.Params {
			if p.In == InPath && p.Name == "actorId" {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf("%s selected but has no {actorId} path param", op.Name)
		}
	}

	for _, want := range []string{
		"getActor", "getAccounts", "createAccount", "updateAccount", "deleteAccount",
		"getReactions", "createReaction",
		"getRelatedActors", "getLinkedActors", "getActorLinks",
		"getActorAttachments",
	} {
		if _, ok := byName[want]; !ok {
			t.Errorf("expected actor-scoped op %q to be included", want)
		}
	}
	for _, notWant := range []string{"getCurrencies", "getWorkspaces"} {
		if _, ok := byName[notWant]; ok {
			t.Errorf("workspace-wide op %q must not be actor-scoped", notWant)
		}
	}
}

// BuildActorScoped registers the per-actor set onto a server without panicking.
func TestBuildActorScoped(t *testing.T) {
	c, _ := setup(t)
	s := server.NewMCPServer("simulator", "test")
	BuildActorScoped(s, c, "act-1") // must not panic / double-register
}

// The per-actor link/unlink handler injects the session actor into every items[]
// element, so the model supplies only attachId and the request targets this actor.
func TestActorItemsInjectsActorID(t *testing.T) {
	c, rec := setup(t)
	op := opByName(t, "addAttachments")
	h := makeHandlerRewrite(c, op, nil, func(args map[string]any) {
		injectActorIntoItems(args, "act-7")
	})

	var req mcp.CallToolRequest
	req.Params.Name = op.Name
	req.Params.Arguments = map[string]any{
		"items": []any{map[string]any{"attachId": float64(5521)}}, // model sends only attachId
	}
	res, err := h(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %+v", res.Content)
	}

	arr, ok := rec.body.([]any)
	if !ok || len(arr) != 1 {
		t.Fatalf("request body is not a 1-element array: %#v", rec.body)
	}
	item, ok := arr[0].(map[string]any)
	if !ok {
		t.Fatalf("item is not an object: %#v", arr[0])
	}
	if item["actorId"] != "act-7" {
		t.Errorf("items[0].actorId = %v, want act-7 (injected)", item["actorId"])
	}
	if item["attachId"] != float64(5521) {
		t.Errorf("items[0].attachId = %v, want 5521 (preserved)", item["attachId"])
	}
}
