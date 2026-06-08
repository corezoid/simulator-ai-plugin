// Command evalrunner is the behavioural half of the eval harness: it drives a
// Claude model through the natural-language prompts in eval-scenarios.json and
// checks the model reaches for the expected tools.
//
// It spawns the real MCP server (`go run ./cmd/server`), reads the live tool
// schemas via the MCP `tools/list` handshake, then for each scenario runs a
// bounded tool-use loop against the Anthropic Messages API.
//
//   - Default (dry): tool calls are answered with a stub "{}" — read-only, no
//     backend, no workspace needed. Verifies the model reaches for the right tools.
//   - --execute (live): tool calls are forwarded to the MCP server and run against
//     the real backend (the server's profile/.env). Entities the model creates are
//     tracked and best-effort deleted at the end. Use a THROWAWAY workspace.
//
// Opt-in: with no ANTHROPIC_API_KEY it prints "skipped" and exits 0.
//
//	ANTHROPIC_API_KEY=… [ANTHROPIC_MODEL=…] go run ./cmd/evalrunner [--execute] [scenarios.json]
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
}

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

var execute = flag.Bool("execute", false, "Live mode: run tool calls against the real backend (use a throwaway workspace)")

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

	mode := "dry (stubbed tool results)"
	if *execute {
		mode = "LIVE (executing tools against the backend — best-effort cleanup)"
	}
	fmt.Printf("evalrunner: %d tools, %d scenarios, model=%s, mode=%s\n\n", len(tools), len(scenarios), model, mode)

	passed, ran, skipped := 0, 0, 0
	var cleanups []cleanup
	for _, sc := range scenarios {
		if sc.LiveOnly && !*execute {
			skipped++
			fmt.Printf("– %s — skipped (live-only; run with --execute)\n", sc.Name)
			continue
		}
		ran++
		called, created, err := runScenario(apiKey, model, anthropicTools, sc.Prompt, mc)
		cleanups = append(cleanups, created...)
		if err != nil {
			fmt.Printf("✗ %s — error: %v\n", sc.Name, err)
			continue
		}
		missing := missingTools(sc.Tools, called)
		if len(missing) == 0 {
			passed++
			fmt.Printf("✓ %s — called: %v\n", sc.Name, keys(called))
		} else {
			fmt.Printf("✗ %s — missing %v (called: %v)\n", sc.Name, missing, keys(called))
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

func runScenario(apiKey, model string, tools []map[string]any, prompt string, mc *mcpClient) (map[string]bool, []cleanup, error) {
	called := map[string]bool{}
	var created []cleanup
	messages := []map[string]any{{"role": "user", "content": prompt}}

	for turn := 0; turn < maxTurns; turn++ {
		resp, err := callAnthropic(apiKey, model, tools, messages)
		if err != nil {
			return called, created, err
		}
		var blocks []contentBlock
		if err := json.Unmarshal(resp.Content, &blocks); err != nil {
			return called, created, fmt.Errorf("parse content: %w", err)
		}

		var results []map[string]any
		for _, b := range blocks {
			if b.Type != "tool_use" {
				continue
			}
			called[b.Name] = true
			resultText := "{}"
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
	return called, created, nil
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

func callAnthropic(apiKey, model string, tools []map[string]any, messages []map[string]any) (*anthropicResp, error) {
	body, _ := json.Marshal(map[string]any{
		"model": model, "max_tokens": 1024, "tools": tools, "messages": messages,
	})
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

func missingTools(expected []string, called map[string]bool) []string {
	var missing []string
	for _, e := range expected {
		if !called[e] {
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
