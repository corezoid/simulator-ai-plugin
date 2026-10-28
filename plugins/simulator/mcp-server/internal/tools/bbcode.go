package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/apiclient"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// getBbcodeTags is a local tool that fetches the environment's published BBCode
// vocabulary from <web-base>/bbcode-tags.json — the tags allowed in description
// text (chips like [actor=…], formatting like [b]/[color], pictures, and [md] for
// a markdown block), each with the places it is allowed in (`context`). The spec
// is per-environment (served at the web host root, like routes.json), so it is
// fetched live rather than embedded.
//
// Registered alongside buildLink (workspace mode only — see registerBuildLink).

// registerBbcodeTags adds the getBbcodeTags reference tool to s.
func registerBbcodeTags(s *server.MCPServer, c *apiclient.Client) {
	tool := mcp.NewTool("getBbcodeTags",
		mcp.WithDescription(
			"Fetch the BBCode tag vocabulary for the current environment (from <web-base>/bbcode-tags.json) — "+
				"the tags you may use in description text to make it render nicely in the UI: chips "+
				"([actor=<uuid>]…[/actor], [user=…], [application=<smartFormId>]…[/application], …), formatting, "+
				"lists, code, pictures and [md]…[/md] for a markdown block. Each tag entry has its attributes, an "+
				"example and `context` — the places the tag is allowed in: actor-description, reaction, event-card "+
				"(Events actors: chats, meetings, tasks), layer-comment, form-label. Use only tags whose `context` "+
				"lists the place you are writing to; anything else shows as literal text or is dropped. "+
				"Call it before composing a rich description so you use real tags. "+
				"IMPORTANT: BBCode tags are processed only OUTSIDE [md] blocks — inside [md]…[/md] the content is "+
				"markdown, so put chips/BBCode outside the [md] section. A tag closes only with its exact [/tag]. "+
				"Pictures: [img=<fileName>] takes the fileName an upload returned, [imgSrc=…] only an http(s):// "+
				"address or a /path; inside [md] write ![alt](<fileName>#width=W;height=H). A reaction's text "+
				"cannot hold pictures — attach the file to the reaction instead."),
	)
	s.AddTool(tool, bbcodeTagsHandler(c))
}

// bbcodeMaxBytes caps the bbcode-tags.json body we read — the spec is ~17KB; the
// cap guards against a misconfigured/hostile web root streaming an unbounded body.
const bbcodeMaxBytes = 2 << 20 // 2 MiB

// bbcodeTagsHandler fetches and returns the env's bbcode-tags.json. Split out for
// testability. It fetches from the CONFIGURED environment base (per-request base
// override, else the client's base) with the /papi/1.0 suffix dropped — NOT from a
// client-supplied UI-context origin: the vocabulary belongs to the environment this
// server talks to, and using a header-supplied origin as a server-side fetch target
// would be an SSRF risk.
func bbcodeTagsHandler(c *apiclient.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		apiBase := apiclient.BaseURLFromContext(ctx)
		if apiBase == "" {
			apiBase = c.BaseURL()
		}
		base := webBaseURL(apiBase)
		if base == "" {
			return mcp.NewToolResultError("[Error] getBbcodeTags: no environment set — run set-environment first"), nil
		}
		url := base + "/bbcode-tags.json"

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] getBbcodeTags: %v", err)), nil
		}
		resp, err := c.HTTP.Do(httpReq)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] getBbcodeTags: fetch %s: %v", url, err)), nil
		}
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(io.LimitReader(resp.Body, bbcodeMaxBytes))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] getBbcodeTags: read %s: %v", url, err)), nil
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] getBbcodeTags: %s returned %d", url, resp.StatusCode)), nil
		}
		return mcp.NewToolResultText(string(body)), nil
	}
}
