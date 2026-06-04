// Command evalrunner is the behavioural half of the eval harness: it drives a
// Claude model through the natural-language prompts in eval-scenarios.json and
// checks the model reaches for the expected tools.
//
// It spawns the real MCP server (`go run ./cmd/server`), reads the live tool
// schemas via the MCP `tools/list` handshake, then for each scenario runs a
// bounded tool-use loop against the Anthropic Messages API (feeding back stub
// tool results) and records which tools the model called. A scenario passes when
// every expected tool was called.
//
// It is opt-in: with no ANTHROPIC_API_KEY it prints "skipped" and exits 0, so it
// never blocks unit CI. Tool calls are NOT executed against a live backend — the
// stub results keep the run read-only and workspace-free.
//
//	ANTHROPIC_API_KEY=… [ANTHROPIC_MODEL=…] go run ./cmd/evalrunner [scenarios.json]
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"
)

const (
	defaultModel = "claude-sonnet-4-5"
	maxTurns     = 8
)

type scenario struct {
	Name   string   `json:"name"`
	Prompt string   `json:"prompt"`
	Tools  []string `json:"tools"`
}

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func main() {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Println("evalrunner: ANTHROPIC_API_KEY not set — skipped (behavioural eval is opt-in).")
		return
	}
	model := envOr("ANTHROPIC_MODEL", defaultModel)

	scenPath := "internal/tools/testdata/eval-scenarios.json"
	if len(os.Args) > 1 {
		scenPath = os.Args[1]
	}
	scenarios, err := loadScenarios(scenPath)
	if err != nil {
		fatal("load scenarios: %v", err)
	}

	tools, stop, err := startServerAndListTools()
	if err != nil {
		fatal("start server / list tools: %v", err)
	}
	defer stop()
	anthropicTools := toAnthropicTools(tools)
	fmt.Printf("evalrunner: %d tools, %d scenarios, model=%s\n\n", len(tools), len(scenarios), model)

	passed := 0
	for _, sc := range scenarios {
		called, err := runScenario(apiKey, model, anthropicTools, sc.Prompt)
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

	fmt.Printf("\nevalrunner: %d/%d scenarios passed\n", passed, len(scenarios))
	if passed < len(scenarios) {
		os.Exit(1)
	}
}

func loadScenarios(path string) ([]scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s []scenario
	return s, json.Unmarshal(data, &s)
}

// startServerAndListTools spawns the MCP server over stdio, performs the
// initialize + tools/list handshake, and returns the advertised tools plus a
// stop function.
func startServerAndListTools() ([]mcpTool, func(), error) {
	cmd := exec.Command("go", "run", "./cmd/server")
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	stop := func() { _ = stdin.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() }

	r := bufio.NewReader(stdout)
	send := func(v any) error {
		b, _ := json.Marshal(v)
		_, err := stdin.Write(append(b, '\n'))
		return err
	}
	readResult := func() (json.RawMessage, error) {
		for {
			line, err := r.ReadBytes('\n')
			if err != nil {
				return nil, err
			}
			var msg struct {
				ID     int             `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			if json.Unmarshal(line, &msg) != nil || msg.ID == 0 {
				continue // skip notifications / log lines
			}
			if msg.Error != nil {
				return nil, fmt.Errorf("rpc error: %s", msg.Error)
			}
			return msg.Result, nil
		}
	}

	_ = send(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{},
			"clientInfo": map[string]any{"name": "evalrunner", "version": "1"}}})
	if _, err := readResult(); err != nil {
		stop()
		return nil, nil, fmt.Errorf("initialize: %w", err)
	}
	_ = send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	_ = send(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": map[string]any{}})
	res, err := readResult()
	if err != nil {
		stop()
		return nil, nil, fmt.Errorf("tools/list: %w", err)
	}
	var listed struct {
		Tools []mcpTool `json:"tools"`
	}
	if err := json.Unmarshal(res, &listed); err != nil {
		stop()
		return nil, nil, fmt.Errorf("parse tools/list: %w", err)
	}
	return listed.Tools, stop, nil
}

func toAnthropicTools(tools []mcpTool) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		schema := t.InputSchema
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, map[string]any{
			"name": t.Name, "description": t.Description, "input_schema": schema,
		})
	}
	return out
}

// runScenario runs a bounded tool-use loop and returns the set of tool names the
// model called. Tool results are stubbed ("{}") so nothing hits a real backend.
func runScenario(apiKey, model string, tools []map[string]any, prompt string) (map[string]bool, error) {
	called := map[string]bool{}
	messages := []map[string]any{{"role": "user", "content": prompt}}

	for turn := 0; turn < maxTurns; turn++ {
		resp, err := callAnthropic(apiKey, model, tools, messages)
		if err != nil {
			return called, err
		}
		var toolUses []map[string]any
		for _, block := range resp.Content {
			if block.Type == "tool_use" {
				called[block.Name] = true
				toolUses = append(toolUses, map[string]any{
					"type": "tool_result", "tool_use_id": block.ID, "content": "{}",
				})
			}
		}
		if len(toolUses) == 0 {
			break // model stopped calling tools
		}
		messages = append(messages,
			map[string]any{"role": "assistant", "content": resp.Content},
			map[string]any{"role": "user", "content": toolUses},
		)
	}
	return called, nil
}

type anthropicResp struct {
	Content []struct {
		Type string `json:"type"`
		Name string `json:"name"`
		ID   string `json:"id"`
	} `json:"content"`
}

func callAnthropic(apiKey, model string, tools []map[string]any, messages []map[string]any) (*anthropicResp, error) {
	body, _ := json.Marshal(map[string]any{
		"model": model, "max_tokens": 1024, "tools": tools, "messages": messages,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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
