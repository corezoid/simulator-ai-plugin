package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
)

// workspaceOps — list the workspaces the authenticated user belongs to. Callable
// right after login (needs only a token, no workspace selected yet) so the user
// can pick one by name without knowing its accId.
var workspaceOps = []Operation{
	{
		Name: "getWorkspaces", Method: "GET", Path: "/workspaces",
		Summary: "List the workspaces you belong to (id + name). Call after login to choose a workspace by name without knowing its accId, then pass the id to set-workspace.",
		Params: []Param{
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset."},
			{Name: "withStats", In: InQuery, Type: "boolean", Desc: "Include totals."},
			fieldFilterParam("id,name"),
		},
	},
}

// userOps — workspace membership. Resolve the userId / groupId you need before
// granting access with the simulator-access tools (saveAccessRules expects a
// userId | saId | groupId grantee).
var userOps = []Operation{
	{
		Name: "getUsers", Method: "GET", Path: "/users/{accId}",
		Summary: "List the members (users) of a workspace — use to find a userId for sharing/access rules.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size (max 200)."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset."},
		},
	},
	{
		Name: "getUser", Method: "GET", Path: "/users/{accId}/{userId}",
		Summary: "Get one workspace member by id (user, SA user, or group).",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "userId", In: InPath, Type: "number", Required: true, Desc: "User id."},
			{Name: "type", In: InQuery, Type: "string", Enum: []string{"sa", "user", "group"}, Desc: "Resolve the id as an SA user, a workspace user, or a group."},
			{Name: "viewData", In: InQuery, Type: "boolean", Desc: "Include the member's profile data."},
		},
	},
	{
		Name: "searchUsers", Method: "GET", Path: "/users/search/{accId}/{query}",
		Summary: "Search workspace members by name/email — the quickest way to get a userId for sharing.",
		Params: []Param{
			{Name: "accId", In: InPath, Type: "string", Required: true, Desc: "Workspace id. Defaults to the configured workspace if omitted."},
			{Name: "query", In: InPath, Type: "string", Required: true, Desc: "Search text (name, email, or fragment)."},
			{Name: "groupIds", In: InQuery, Type: "string", Desc: "Comma-separated group ids to restrict the search to."},
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset."},
		},
	},
}

// workspace is one entry from getWorkspaces (id may be a string or number).
type workspace struct {
	ID   json.RawMessage `json:"id"`
	Name string          `json:"name"`
}

func (w workspace) id() string {
	var s string
	if json.Unmarshal(w.ID, &s) == nil && s != "" {
		return s
	}
	return strings.Trim(string(w.ID), `"`)
}

// parseWorkspaceList accepts the several envelopes the list endpoint may use:
// a bare array, {data: [...]}, or {list: [...]}.
func parseWorkspaceList(body []byte) ([]workspace, error) {
	var arr []workspace
	if json.Unmarshal(body, &arr) == nil && len(arr) > 0 {
		return arr, nil
	}
	var env struct {
		Data []workspace `json:"data"`
		List []workspace `json:"list"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse workspaces: %w", err)
	}
	if len(env.Data) > 0 {
		return env.Data, nil
	}
	return env.List, nil
}

// resolveWorkspaceName looks up a workspace id by (case-insensitive) name among
// the user's workspaces. Used by set-workspace so a name can be given instead of an id.
func resolveWorkspaceName(ctx context.Context, c *apiclient.Client, name string) (string, error) {
	resp, err := c.Do(ctx, "GET", "/workspaces", nil, nil)
	if err != nil {
		return "", err
	}
	items, err := parseWorkspaceList(resp)
	if err != nil {
		return "", err
	}
	var names []string
	for _, w := range items {
		if strings.EqualFold(w.Name, name) {
			return w.id(), nil
		}
		names = append(names, w.Name)
	}
	return "", fmt.Errorf("workspace %q not found; available: %s", name, strings.Join(names, ", "))
}
