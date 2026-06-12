package mcpserver

import (
	"errors"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/config"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"
	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/tools"
	"github.com/mark3labs/mcp-go/server"
)

// NewActorServer builds a per-actor MCP server: the curated tools scoped to one
// actor, with the actor's identity ({actorId}, link source, access-rule object)
// pre-bound and hidden from the tool schemas — the model cannot target a
// different actor. It is always stateless: Authorization, workspace id and API
// base URL arrive per request via ctx (WithAuthorization / WithWorkspaceID /
// WithBaseURL), and no .env / login helpers are registered. Serve it over any
// transport; the hosting gateway routes /mcp/actors/{actorId} to one of these.
//
// Use this when the URL itself selects the actor (one server instance per
// actor). For a single server that switches between full and per-actor mode
// based on a per-request ctx value, use New(Options{Stateless: true}) and
// attach WithActorID(ctx, id) from your transport instead.
func NewActorServer(actorID string, opts Options) (*server.MCPServer, Info, error) {
	prof, err := config.Resolve(opts.Profile)
	if err != nil {
		return nil, Info{}, err
	}
	// Stateless: auth/workspace/baseURL come from request ctx. A non-nil authHeader
	// surfaces misuse if a request arrives without an Authorization on the ctx.
	authHeader := func() (string, error) {
		return "", errors.New("stateless mode: Authorization header missing on request")
	}
	client := apiclient.New(prof.APIBaseURL, "", authHeader, opts.Insecure)

	name := opts.Name
	if name == "" {
		name = defaultName
	}
	version := opts.Version
	if version == "" {
		version = defaultVersion
	}

	s := server.NewMCPServer(name, version)
	ecore.SetStateless(true)
	tools.BuildActorScoped(s, client, actorID)
	return s, Info{
		Profile:    prof.Name,
		APIBaseURL: prof.APIBaseURL,
		AccountURL: prof.AccountURL,
	}, nil
}
