package tools

import (
	"context"
	"fmt"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"
)

// accIDArgDesc is the accId argument of the composite tools (findAgent,
// getAgent, findSkill, getSkill) that resolve a workspace-scoped form or
// endpoint themselves. Hosted mode rewrites it for the bare /mcp route.
const accIDArgDesc = "Workspace id. Defaults to the configured workspace if omitted."

// OptionalAccIDPrefix opens the description of an accId that only feeds a
// convenience lookup (createActor's formName, getRelatedActors' default edge
// type) and is never required. Hosted mode keys on it to keep that wording
// instead of its "Required unless the connector URL names a workspace" text,
// so a required accId must not start with it.
const OptionalAccIDPrefix = "Workspace id — used only to "

// WorkspaceArgContext returns ctx carrying the accId argument as the request's
// workspace, so helpers that read c.WorkspaceIDForContext(ctx) see it. A
// workspace already on ctx (one named by a hosted connection URL) wins, exactly
// as for the stateless workspaceFromArgs middleware that calls this; on stdio
// ctx carries none, so accId overrides the configured default. An empty accId
// leaves ctx unchanged; a malformed one is an error because it would be
// interpolated into API paths.
func WorkspaceArgContext(ctx context.Context, args map[string]any) (context.Context, error) {
	if apiclient.WorkspaceIDFromContext(ctx) != "" {
		return ctx, nil
	}
	acc, _ := args["accId"].(string)
	if acc == "" {
		return ctx, nil
	}
	if !ecore.IsUUID(acc) {
		return ctx, fmt.Errorf("accId must be a valid UUID, got %q", acc)
	}
	return apiclient.WithWorkspaceID(ctx, acc), nil
}
