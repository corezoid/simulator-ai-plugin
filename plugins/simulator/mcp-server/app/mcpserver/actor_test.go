package mcpserver_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	mcpserver "github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/app/mcpserver"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// End-to-end over the in-process transport: a real MCP client performs the
// initialize + tools/list handshake against NewActorServer and receives exactly
// the per-actor specification — actor-scoped tools (read + CRUD), with the actor's
// identity ({actorId}, link source, access-rule object) hidden from every schema,
// and no .env/login helpers (stateless). tools/list issues no HTTP.
func TestNewActorServerServesPerActorSpec(t *testing.T) {
	srv, info, err := mcpserver.NewActorServer("act-XYZ", mcpserver.Options{})
	if err != nil {
		t.Fatalf("NewActorServer: %v", err)
	}
	if info.APIBaseURL == "" {
		t.Error("Info.APIBaseURL should be resolved")
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
	init.Params.ClientInfo = mcp.Implementation{Name: "spec-test", Version: "0"}
	if _, err := cli.Initialize(ctx, init); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	lt, err := cli.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	byName := make(map[string]mcp.Tool, len(lt.Tools))
	for _, tool := range lt.Tools {
		byName[tool.Name] = tool
	}

	// actor-scoped tools are served — reads, finance/reaction CRUD, graph
	// relationship tools, link CRUD, attachments (read + manage), access rules.
	for _, want := range []string{
		"getActor", "getAccounts", "createAccount", "updateAccount", "deleteAccount",
		"getReactions", "createReaction", "updateReaction", "deleteReaction",
		"getRelatedActors", "getLinkedActors", "getActorLinks",
		"createLink", "existLink", "updateEdge", "deleteEdge",
		"getActorAttachments", "uploadBase64", "addAttachments", "removeAttachments",
		"getAccessRules", "saveAccessRules",
	} {
		if _, ok := byName[want]; !ok {
			t.Errorf("per-actor spec is missing tool %q", want)
		}
	}
	// workspace-wide tools and .env/login helpers are not served.
	for _, notWant := range []string{
		"getCurrencies", "getWorkspaces", "createForm",
		"login", "set-workspace", "set-environment",
	} {
		if _, ok := byName[notWant]; ok {
			t.Errorf("per-actor spec must not expose %q", notWant)
		}
	}

	// the bound actorId must not appear in any served tool's input schema.
	for name, tool := range byName {
		if schemaContains(t, tool, `"actorId"`) {
			t.Errorf("tool %q leaks the bound actorId in its schema", name)
		}
	}
	// access-rule tools are pinned to the actor: objType/objId are bound + hidden.
	for _, name := range []string{"getAccessRules", "saveAccessRules"} {
		for _, hidden := range []string{`"objType"`, `"objId"`} {
			if schemaContains(t, byName[name], hidden) {
				t.Errorf("tool %q leaks bound %s in its schema", name, hidden)
			}
		}
	}
	// link create/check is pinned to the actor as source.
	for _, name := range []string{"createLink", "existLink"} {
		if schemaContains(t, byName[name], `"source"`) {
			t.Errorf("tool %q leaks the bound source in its schema", name)
		}
	}
}

func schemaContains(t *testing.T, tool mcp.Tool, needle string) bool {
	t.Helper()
	raw, err := json.Marshal(tool.InputSchema)
	if err != nil {
		t.Fatalf("marshal %s schema: %v", tool.Name, err)
	}
	return strings.Contains(string(raw), needle)
}
