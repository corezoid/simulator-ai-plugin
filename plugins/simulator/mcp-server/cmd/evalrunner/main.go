// Command evalrunner is the behavioural half of the eval harness: it drives a
// Claude model through the natural-language prompts in eval-scenarios.json and
// checks the model reaches for the expected tools.
//
// It spawns the real MCP server (`go run ./cmd/server`), reads the live tool
// schemas via the MCP `tools/list` handshake, then for each scenario runs a
// bounded tool-use loop against the Anthropic Messages API.
//
//   - Default (dry): tool calls are answered with small canned fixtures (read/list
//     tools get realistic ids so scenarios can chain; mutating tools get "{}") — no
//     backend, no workspace needed. Verifies the model reaches for the right tools
//     and (via argChecks) produces the right argument shapes.
//   - --execute (live): tool calls are forwarded to the MCP server and run against
//     the real backend (the server's profile/.env). Entities the model creates are
//     tracked and best-effort deleted at the end. Use a THROWAWAY workspace.
//   - --skills: inject the plugin SKILL.md files as the system prompt, approximating
//     how a host (Claude Code) loads skills. Without it the model sees only the tool
//     schemas, so the eval validates tool descriptions but not the skill prose.
//
// Scenarios (testdata/eval-scenarios.json) may carry argChecks — per-tool assertions
// on the model's arguments: mustContain / mustNotContain (regression guards) /
// mustMatch (regexp), matched over the tool's canonical-compact args.
//
// Opt-in: with no ANTHROPIC_API_KEY it prints "skipped" and exits 0.
//
//	ANTHROPIC_API_KEY=… [ANTHROPIC_MODEL=…] go run ./cmd/evalrunner [--execute] [--skills] [scenarios.json]
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	defaultModel = "claude-sonnet-4-5"
	maxTurns     = 10
)

type scenario struct {
	Name   string   `json:"name"`
	Prompt string   `json:"prompt"`
	Tools  []string `json:"tools"`
	// LiveOnly scenarios need real backend state (ids returned by earlier tools)
	// to complete, so they only run under --execute; dry mode skips them.
	LiveOnly bool `json:"liveOnly,omitempty"`
	// DryOnlyTools are required only in dry mode. Use for the second step of a
	// real-dependency flow (e.g. finalizeTransaction after authorize): dry
	// fixtures make step 1 "succeed" so the model proceeds, but live step 1 hits
	// a 404 on the placeholder id and the model correctly stops — so don't require
	// the follow-up tool live. They still validate the full flow in dry.
	DryOnlyTools []string `json:"dryOnlyTools,omitempty"`
	// ArgChecks optionally assert on the JSON arguments the model passed to a
	// tool: every substring must appear in that tool's canonical-compact args
	// (across all of its calls in the scenario). Use it to verify data shapes
	// (e.g. multiform "__form__<id>:" keys, value-object discriminators).
	ArgChecks []argCheck `json:"argChecks,omitempty"`
}

type argCheck struct {
	Tool           string   `json:"tool"`
	MustContain    []string `json:"mustContain,omitempty"`    // substrings that must appear in the tool's args
	MustNotContain []string `json:"mustNotContain,omitempty"` // substrings that must NOT appear (regression guards)
	MustMatch      []string `json:"mustMatch,omitempty"`      // regexp patterns the args must match
	// DryOnly: only enforce this check in dry mode (e.g. it asserts on the args of
	// a follow-up call that is only reached when dry fixtures let step 1 succeed).
	DryOnly bool `json:"dryOnly,omitempty"`
}

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

var (
	execute    = flag.Bool("execute", false, "Live mode: run tool calls against the real backend (use a throwaway workspace)")
	withSkills = flag.Bool("skills", false, "Inject the plugin SKILL.md files as the system prompt (approximates Claude Code skill loading; otherwise only tool schemas are seen)")
)

func main() {
	flag.Parse()
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Println("evalrunner: ANTHROPIC_API_KEY not set — skipped (behavioural eval is opt-in).")
		return
	}
	model := envOr("ANTHROPIC_MODEL", defaultModel)

	scenPath := "internal/tools/testdata/eval-scenarios.json"
	if args := flag.Args(); len(args) > 0 {
		scenPath = args[0]
	}
	scenarios, err := loadScenarios(scenPath)
	if err != nil {
		fatal("load scenarios: %v", err)
	}

	mc, err := startMCP()
	if err != nil {
		fatal("start MCP server: %v", err)
	}
	defer mc.close()
	tools, err := mc.listTools()
	if err != nil {
		fatal("tools/list: %v", err)
	}
	anthropicTools := toAnthropicTools(tools)

	var system string
	if *withSkills {
		s, n, err := loadSkillsPrompt()
		if err != nil {
			fatal("load skills: %v", err)
		}
		system = s
		fmt.Printf("evalrunner: injecting %d SKILL.md files as system prompt (%d chars)\n", n, len(system))
	}

	mode := "dry (canned fixtures for read tools)"
	if *execute {
		mode = "LIVE (executing tools against the backend — best-effort cleanup)"
	}
	skillsMode := "off"
	if *withSkills {
		skillsMode = "on"
	}
	fmt.Printf("evalrunner: %d tools, %d scenarios, model=%s, mode=%s, skills=%s\n\n", len(tools), len(scenarios), model, mode, skillsMode)

	passed, ran, skipped := 0, 0, 0
	var cleanups []cleanup
	for _, sc := range scenarios {
		if sc.LiveOnly && !*execute {
			skipped++
			fmt.Printf("– %s — skipped (live-only; run with --execute)\n", sc.Name)
			continue
		}
		ran++
		called, argsByTool, created, err := runScenario(apiKey, model, system, anthropicTools, sc.Prompt, mc)
		cleanups = append(cleanups, created...)
		if err != nil {
			fmt.Printf("✗ %s — error: %v\n", sc.Name, err)
			continue
		}
		expectedTools := sc.Tools
		argChecks := sc.ArgChecks
		if !*execute {
			expectedTools = append(append([]string{}, sc.Tools...), sc.DryOnlyTools...)
		} else {
			// Live: drop dry-only argChecks (they assert on follow-up calls that
			// a placeholder-id 404 prevents the model from reaching).
			argChecks = nil
			for _, ac := range sc.ArgChecks {
				if !ac.DryOnly {
					argChecks = append(argChecks, ac)
				}
			}
		}
		missing := missingTools(expectedTools, called)
		argFails := failedArgChecks(argChecks, argsByTool)
		if len(missing) == 0 && len(argFails) == 0 {
			passed++
			fmt.Printf("✓ %s — called: %v\n", sc.Name, keys(called))
		} else {
			fmt.Printf("✗ %s — missing tools %v, arg failures %v (called: %v)\n", sc.Name, missing, argFails, keys(called))
		}
	}

	if *execute && len(cleanups) > 0 {
		fmt.Printf("\nevalrunner: cleaning up %d created entities…\n", len(cleanups))
		runCleanups(mc, cleanups)
	}

	fmt.Printf("\nevalrunner: %d/%d scenarios passed (%d skipped as live-only)\n", passed, ran, skipped)
	if passed < ran {
		os.Exit(1)
	}
}

// ---- scenario loop ----

type cleanup struct {
	tool string
	args map[string]any
}

func runScenario(apiKey, model, system string, tools []map[string]any, prompt string, mc *mcpClient) (map[string]bool, map[string]string, []cleanup, error) {
	called := map[string]bool{}
	argsByTool := map[string]string{}
	var created []cleanup
	messages := []map[string]any{{"role": "user", "content": prompt}}

	for turn := 0; turn < maxTurns; turn++ {
		resp, err := callAnthropic(apiKey, model, system, tools, messages)
		if err != nil {
			return called, argsByTool, created, err
		}
		var blocks []contentBlock
		if err := json.Unmarshal(resp.Content, &blocks); err != nil {
			return called, argsByTool, created, fmt.Errorf("parse content: %w", err)
		}

		var results []map[string]any
		for _, b := range blocks {
			if b.Type != "tool_use" {
				continue
			}
			called[b.Name] = true
			argsByTool[b.Name] += canonicalJSON(b.Input) + "\n"
			resultText := dryResult(b.Name)
			if *execute {
				text, _, callErr := mc.callTool(b.Name, b.Input)
				if callErr != nil {
					resultText = fmt.Sprintf(`{"error":%q}`, callErr.Error())
				} else {
					resultText = truncate(text, 4000)
					if c, ok := cleanupFor(b.Name, text); ok {
						created = append(created, c)
					}
				}
			}
			results = append(results, map[string]any{
				"type": "tool_result", "tool_use_id": b.ID, "content": resultText,
			})
		}
		if len(results) == 0 {
			break // model stopped calling tools
		}
		messages = append(messages,
			map[string]any{"role": "assistant", "content": resp.Content},
			map[string]any{"role": "user", "content": results},
		)
	}
	return called, argsByTool, created, nil
}

// canonicalJSON re-marshals the model's raw tool input to compact JSON so
// substring argChecks are insensitive to whitespace formatting. On parse
// failure it falls back to the raw bytes.
func canonicalJSON(raw json.RawMessage) string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return string(raw)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return string(raw)
	}
	return string(b)
}

// failedArgChecks returns a human-readable list of unmet argument expectations:
// for each argCheck, every mustContain substring must appear somewhere in that
// tool's accumulated (canonical) arguments across the scenario.
func failedArgChecks(checks []argCheck, argsByTool map[string]string) []string {
	var fails []string
	for _, ac := range checks {
		args := argsByTool[ac.Tool]
		for _, want := range ac.MustContain {
			if !strings.Contains(args, want) {
				fails = append(fails, fmt.Sprintf("%s missing %q", ac.Tool, want))
			}
		}
		for _, bad := range ac.MustNotContain {
			if strings.Contains(args, bad) {
				fails = append(fails, fmt.Sprintf("%s must NOT contain %q", ac.Tool, bad))
			}
		}
		for _, pat := range ac.MustMatch {
			re, err := regexp.Compile(pat)
			if err != nil {
				fails = append(fails, fmt.Sprintf("%s bad regex %q: %v", ac.Tool, pat, err))
				continue
			}
			if !re.MatchString(args) {
				fails = append(fails, fmt.Sprintf("%s no match for /%s/", ac.Tool, pat))
			}
		}
	}
	return fails
}

// ---- dry-mode fixtures ----

// dryResult answers a tool call in dry mode. Read/list tools get a small canned
// payload (instead of "{}") so the model has realistic ids to chain into later
// calls — e.g. getForm returns a form with known item_<id> fields, so a downstream
// createActor scenario can be scored on whether it keys `data` by those ids.
// Unknown / mutating tools fall back to "{}".
func dryResult(tool string) string {
	if r, ok := dryFixtures[tool]; ok {
		return r
	}
	return "{}"
}

// dryFixtures: a small, internally-consistent world. Form 42 "Vehicle" has fields
// item_make/item_year/item_owner; form 16951 "Position" is a second (multiform)
// template; one actor, one account name, USD+Km currencies.
var dryFixtures = map[string]string{
	"getForm": `{"data":{"id":42,"title":"Vehicle","sections":[{"title":"Basics","content":[` +
		`{"id":"item_make","class":"edit","title":"Make"},` +
		`{"id":"item_year","type":"int","class":"edit","title":"Year"},` +
		`{"id":"item_owner","class":"select","title":"Owner","extra":{"optionsSource":{"type":"workspaceMembers"}}}]}]}}`,
	"getForms":              `{"data":[{"id":42,"title":"Vehicle"},{"id":16951,"title":"Position"}]}`,
	"searchForms":           `{"data":[{"id":42,"title":"Vehicle"}]}`,
	"getActor":              `{"data":{"id":"11111111-1111-1111-1111-111111111111","title":"Camry","formId":42,"data":{"item_make":"Toyota"}}}`,
	"getActorByRef":         `{"data":{"id":"11111111-1111-1111-1111-111111111111","title":"Camry","formId":42}}`,
	"searchActors":          `{"data":[{"id":"11111111-1111-1111-1111-111111111111","title":"Camry","formId":42}]}`,
	"searchLayerActors":     `{"data":[{"id":"11111111-1111-1111-1111-111111111111","title":"Camry"}]}`,
	"filterActors":          `{"data":[{"id":"11111111-1111-1111-1111-111111111111","title":"Camry"}]}`,
	"searchAll":             `{"data":{"actors":[{"id":"11111111-1111-1111-1111-111111111111","title":"Camry"}]}}`,
	"getAccountNames":       `{"data":[{"id":"aaaaaaaa-0000-0000-0000-000000000001","name":"Maintenance"}]}`,
	"getCurrencies":         `{"data":[{"id":1,"name":"USD","symbol":"$"},{"id":2,"name":"Km"}]}`,
	"getAccounts":           `{"data":[{"id":"acc-1","nameId":"aaaaaaaa-0000-0000-0000-000000000001","currencyId":1,"amount":1500}]}`,
	"getBalance":            `{"data":{"id":"acc-1","amount":1500}}`,
	"getEdgeTypes":          `{"data":[{"id":1,"name":"hierarchy"},{"id":2,"name":"link"}]}`,
	"getEdge":               `{"data":{"id":"eeeeeeee-0000-0000-0000-000000000001","source":"11111111-1111-1111-1111-111111111111","target":"22222222-2222-2222-2222-222222222222","edgeTypeId":1,"name":"link"}}`,
	"existLink":             `{"data":[{"id":"eeeeeeee-0000-0000-0000-000000000001","source":"11111111-1111-1111-1111-111111111111","target":"22222222-2222-2222-2222-222222222222","edgeTypeId":1}]}`,
	"getRelatedActors":      `{"data":[{"id":"22222222-2222-2222-2222-222222222222","title":"Wheel"}]}`,
	"getLayerActors":        `{"data":{"nodes":[{"id":"22222222-2222-2222-2222-222222222222","title":"Wheel"}]}}`,
	"getWorkspaces":         `{"data":[{"id":"ws-demo","title":"Demo workspace"}]}`,
	"getTransactions":       `{"data":[{"id":"tx-1","amount":450}]}`,
	"getTransfer":           `{"data":{"id":"tr-1","amount":200}}`,
	"getAllLayerPlacements": `{"data":[{"layerId":"33333333-3333-3333-3333-333333333333"}]}`,
	// create/setup tools: return an id so multi-step dry scenarios can chain.
	"createForm":        `{"data":{"id":42,"title":"Vehicle"}}`,
	"createActor":       `{"data":{"id":"11111111-1111-1111-1111-111111111111","formId":42}}`,
	"createAccountName": `{"data":{"id":"aaaaaaaa-0000-0000-0000-000000000002","name":"Mileage"}}`,
	"createCurrency":    `{"data":{"id":3,"name":"Km","symbol":"km"}}`,
	"createAccount":     `{"data":{"id":"acc-1"}}`,

	// New-domain fixtures so multi-step chains resolve in dry mode.
	"searchUsers":            `{"data":[{"id":4210,"nick":"Olena"},{"id":4310,"nick":"Petro"}]}`,
	"getUsers":               `{"data":[{"id":4210,"nick":"Olena"},{"id":4310,"nick":"Petro"}]}`,
	"getUser":                `{"data":{"id":4210,"nick":"Olena"}}`,
	"getSystemActor":         `{"data":{"id":"99999999-0000-0000-0000-000000000001","title":"Olena (user)","formId":1}}`,
	"getAttachments":         `{"data":[{"id":5521,"title":"report.pdf"}]}`,
	"getFormAccounts":        `{"data":[{"id":7788,"nameId":"aaaaaaaa-0000-0000-0000-000000000001","currencyId":1,"accountType":"fact"}]}`,
	"getFormsTree":           `{"data":[{"id":16950,"title":"People"},{"id":16951,"title":"Position"}]}`,
	"getLinkedForms":         `{"data":[{"id":16951,"title":"Position"}]}`,
	"getLinkedActors":        `{"data":[{"id":"22222222-2222-2222-2222-222222222222","title":"Wheel"}]}`,
	"getActorLinks":          `{"data":[{"id":"eeeeeeee-0000-0000-0000-000000000001","source":"11111111-1111-1111-1111-111111111111","target":"22222222-2222-2222-2222-222222222222","edgeTypeId":1}]}`,
	"getReactions":           `{"data":[{"id":"rx-100","description":"Looks good"}]}`,
	"getPinnedReactions":     `{"data":[{"id":"rx-100","pinned":true}]}`,
	"getReactionsStats":      `{"data":{"comment":3,"total":3}}`,
	"getTransferByRef":       `{"data":{"id":"tr-1","amount":200}}`,
	"getAccountTransactions": `{"data":[{"id":"tx-1","amount":450}]}`,
	"getTransactionByRef":    `{"data":{"id":"tx-1","amount":450}}`,
	"getCounters":            `{"data":[{"actorRef":"car-camry","accountName":"mileage","amount":45000}]}`,
	"getChildAccounts":       `{"data":[{"id":"acc-2","amount":100}]}`,
	"searchCurrencies":       `{"data":[{"id":1,"name":"USD","symbol":"$"}]}`,
	"searchAccountNames":     `{"data":[{"id":"aaaaaaaa-0000-0000-0000-000000000001","name":"Maintenance"}]}`,
	"getAccount":             `{"data":{"id":"acc-1","amount":1500,"currencyId":1,"nameId":"aaaaaaaa-0000-0000-0000-000000000001"}}`,
	// create/mutating tools that scenarios chain off of.
	"createReaction":        `{"data":{"id":"rx-100"}}`,
	"uploadBase64":          `{"data":{"attachId":5521,"fileName":"report.pdf"}}`,
	"createFormAccount":     `{"data":{"id":7788}}`,
	"createTransfer":        `{"data":{"id":"tr-1"}}`,
	"createTransferTwoStep": `{"data":{"id":"tr-1","status":"authorized"}}`,
	"createTransaction":     `{"data":{"id":"tx-1","status":"completed"}}`,
	"atomCreateTransaction": `{"data":[{"id":"tx-1"},{"id":"tx-2"}]}`,
	"saveAccessRules":       `{"data":[],"taskId":"task-1"}`,
}

// ---- skill injection ----

// loadSkillsPrompt concatenates the plugin SKILL.md files into a system prompt,
// approximating how a host (Claude Code) injects skill guidance. Without it the
// behavioural eval sees only tool schemas. Returns the prompt and file count.
func loadSkillsPrompt() (string, int, error) {
	matches, err := filepath.Glob("../skills/*/SKILL.md")
	if err != nil {
		return "", 0, err
	}
	if len(matches) == 0 {
		return "", 0, fmt.Errorf("no SKILL.md under ../skills/*/ (run evalrunner from the mcp-server dir)")
	}
	var b strings.Builder
	b.WriteString("You are using the Simulator.Company plugin. Below are its skill instructions; follow them when choosing and calling tools.\n\n")
	for _, m := range matches {
		data, err := os.ReadFile(m) // #nosec G304 -- fixed glob under the repo
		if err != nil {
			return "", 0, fmt.Errorf("read %s: %w", m, err)
		}
		b.WriteString("===== " + m + " =====\n")
		b.Write(data)
		b.WriteString("\n\n")
	}
	return b.String(), len(matches), nil
}

// cleanupFor derives a delete action from a successful create response so the
// throwaway workspace can be tidied up afterwards.
func cleanupFor(tool, resultText string) (cleanup, bool) {
	var r struct {
		Data struct {
			ID    json.RawMessage `json:"id"`
			AccID string          `json:"accId"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(resultText), &r) != nil || len(r.Data.ID) == 0 {
		return cleanup{}, false
	}
	switch tool {
	case "createActor":
		var id string
		_ = json.Unmarshal(r.Data.ID, &id)
		if id != "" {
			return cleanup{tool: "deleteActor", args: map[string]any{"actorId": id}}, true
		}
	case "createForm":
		var id float64
		_ = json.Unmarshal(r.Data.ID, &id)
		if id != 0 {
			return cleanup{tool: "deleteForm", args: map[string]any{"formId": id}}, true
		}
	}
	return cleanup{}, false
}

func runCleanups(mc *mcpClient, cleanups []cleanup) {
	// reverse order: delete most-recently created first
	for i := len(cleanups) - 1; i >= 0; i-- {
		c := cleanups[i]
		args, _ := json.Marshal(c.args)
		if _, _, err := mc.callTool(c.tool, args); err != nil {
			fmt.Printf("  cleanup %s %v: %v\n", c.tool, c.args, err)
		}
	}
}

// ---- Anthropic ----

type contentBlock struct {
	Type  string          `json:"type"`
	Name  string          `json:"name"`
	ID    string          `json:"id"`
	Input json.RawMessage `json:"input"`
}

type anthropicResp struct {
	Content json.RawMessage `json:"content"`
}

func callAnthropic(apiKey, model, system string, tools []map[string]any, messages []map[string]any) (*anthropicResp, error) {
	payload := map[string]any{
		"model": model, "max_tokens": 1024, "tools": tools, "messages": messages,
	}
	if system != "" {
		payload["system"] = system
	}
	body, _ := json.Marshal(payload)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("anthropic HTTP %d: %.300s", resp.StatusCode, raw)
	}
	var out anthropicResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse anthropic response: %w", err)
	}
	return &out, nil
}

func toAnthropicTools(tools []mcpTool) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		schema := t.InputSchema
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, map[string]any{"name": t.Name, "description": t.Description, "input_schema": schema})
	}
	return out
}

// ---- MCP stdio client ----

type mcpClient struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	r     *bufio.Reader
	id    int
}

func startMCP() (*mcpClient, error) {
	cmd := exec.Command("go", "run", "./cmd/server")
	cmd.Stderr = os.Stderr // server logs go to our stderr; inherits env (SIMULATOR_PROFILE, .env via cwd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c := &mcpClient{cmd: cmd, stdin: stdin, r: bufio.NewReader(stdout)}
	if _, err := c.rpc("initialize", map[string]any{
		"protocolVersion": "2025-03-26", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "evalrunner", "version": "1"},
	}); err != nil {
		c.close()
		return nil, fmt.Errorf("initialize: %w", err)
	}
	_ = c.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	return c, nil
}

func (c *mcpClient) send(v any) error {
	b, _ := json.Marshal(v)
	_, err := c.stdin.Write(append(b, '\n'))
	return err
}

// rpc sends a request and returns the result for the matching id, skipping
// notifications and any non-JSON log lines on stdout.
func (c *mcpClient) rpc(method string, params any) (json.RawMessage, error) {
	c.id++
	id := c.id
	if err := c.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	for {
		line, err := c.r.ReadBytes('\n')
		if err != nil {
			return nil, err
		}
		var msg struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if json.Unmarshal(line, &msg) != nil || msg.ID != id {
			continue
		}
		if msg.Error != nil {
			return nil, fmt.Errorf("rpc error: %s", msg.Error)
		}
		return msg.Result, nil
	}
}

func (c *mcpClient) listTools() ([]mcpTool, error) {
	res, err := c.rpc("tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var out struct {
		Tools []mcpTool `json:"tools"`
	}
	return out.Tools, json.Unmarshal(res, &out)
}

// callTool invokes an MCP tool and returns its text content. arguments is the
// raw JSON object the model produced (or marshalled cleanup args).
func (c *mcpClient) callTool(name string, arguments json.RawMessage) (string, bool, error) {
	if len(arguments) == 0 {
		arguments = json.RawMessage("{}")
	}
	res, err := c.rpc("tools/call", map[string]any{"name": name, "arguments": arguments})
	if err != nil {
		return "", true, err
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(res, &out); err != nil {
		return "", true, err
	}
	var text string
	for _, b := range out.Content {
		if b.Type == "text" {
			text += b.Text
		}
	}
	return text, out.IsError, nil
}

func (c *mcpClient) close() {
	_ = c.stdin.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	_ = c.cmd.Wait()
}

// ---- helpers ----

func loadScenarios(path string) ([]scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s []scenario
	return s, json.Unmarshal(data, &s)
}

// missingTools reports expected tools the model did not call. An expected entry
// may be an any-of group written as "a|b|c": it is satisfied if ANY of the
// alternatives was called — used for genuinely interchangeable tools (e.g.
// "getForms|searchForms", "getRelatedActors|getLinkedActors").
func missingTools(expected []string, called map[string]bool) []string {
	var missing []string
	for _, e := range expected {
		satisfied := false
		for _, alt := range strings.Split(e, "|") {
			if called[strings.TrimSpace(alt)] {
				satisfied = true
				break
			}
		}
		if !satisfied {
			missing = append(missing, e)
		}
	}
	return missing
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…(truncated)"
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "evalrunner: "+format+"\n", args...)
	os.Exit(2)
}
