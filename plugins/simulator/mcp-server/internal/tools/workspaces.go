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
