package sim

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const inputsDesc = "Model: pass `model` (YAML text) or `modelPath` (file in the working directory). " +
	"Scenarios: `scenarios` (YAML list) or `scenariosPath`; without them one scenario `base` runs. " +
	"Graph: `layerId` (read live from Simulator: actors, links, account values) or `graphPath` " +
	"(a layer YAML from pullGraphFile / simulationSnapshot)."

// Register adds the simulation tools (read-only: nothing is written to Simulator).
func Register(s *server.MCPServer) {
	common := func(opts ...mcp.ToolOption) []mcp.ToolOption {
		return append([]mcp.ToolOption{
			mcp.WithString("model", mcp.Description("Model YAML text (sim-morrow model format v1; see docs/simulation/model-format.md).")),
			mcp.WithString("modelPath", mcp.Description("Model YAML file, relative to the working directory.")),
			mcp.WithString("scenarios", mcp.Description("Scenarios YAML text: a list of {name, params, set, horizon, seed}.")),
			mcp.WithString("scenariosPath", mcp.Description("Scenarios YAML file, relative to the working directory.")),
			mcp.WithString("layerId", mcp.Description("Layer actor UUID to read live.")),
			mcp.WithString("graphPath", mcp.Description("Graph file (plugin layer YAML), relative to the working directory.")),
			mcp.WithString("period", mcp.Description("With layerId: account values are the turnover of this last period (e.g. 30d) instead of the balance.")),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
		}, opts...)
	}
	s.AddTool(mcp.NewTool("simulationCheck", common(
		mcp.WithDescription("Check a behaviour model before running it: unknown actions or functions, missing fields, "+
			"typos in params, refs that match no actor, events nobody handles, handlers never triggered, add on a "+
			"conserved account, goals over unknown metrics. Returns errors (a run would fail) and warnings. "+inputsDesc),
		mcp.WithIdempotentHintAnnotation(true),
	)...), handleCheck)

	s.AddTool(mcp.NewTool("simulationRun", common(
		mcp.WithDescription("Simulate a Simulator graph: run behaviour rules in model time for each scenario and "+
			"compare metrics. Fast mode, in memory: nothing is written to Simulator. The model is checked first; a "+
			"model with errors does not run. With `runs` > 1 each scenario runs in that many random worlds and the "+
			"result is the median with the 10–90 % range, plus the share of runs meeting each goal, over completed runs "+
			"only: runs that failed or stopped before the horizon are counted apart and must be reported. "+
			"Report a single run only when the model has no randomness. "+inputsDesc),
		mcp.WithString("scenario", mcp.Description("Comma-separated scenario names to run (default: all).")),
		mcp.WithNumber("runs", mcp.Description("Runs per scenario with different random seeds (default 1, max 1000).")),
		mcp.WithString("goals", mcp.Description("Extra goals as YAML/JSON map name -> condition over metrics, e.g. {no_leaves: 'leaves == 0'}.")),
		mcp.WithNumber("logEvents", mcp.Description(fmt.Sprintf("Include the first N processed events of each scenario (default 0, max %d).", maxLogEvents))),
		mcp.WithString("timeLimit", mcp.Description(fmt.Sprintf("Wall-clock budget for the whole call, e.g. 30s or 5m (default %s, max %s). "+
			"When it runs out, a single run ends with status stopped_by_time and many runs report the runs made so far.", defaultTimeLimit, maxTimeLimit))),
		mcp.WithIdempotentHintAnnotation(true),
	)...), handleRun)

	// Snapshot writes a file to the server's working directory, which a
	// hosted (stateless) server shares between callers: not offered there.
	if ecore.IsStateless() {
		return
	}
	s.AddTool(mcp.NewTool("simulationSnapshot",
		mcp.WithDescription("Read a layer with its actors, links and account values and write it to "+
			"<layerId>.sim.yaml in the working directory (plugin layer YAML plus sim: sections for accounts and "+
			"value types). Use it to inspect what a simulation will see, or to rerun offline with graphPath."),
		mcp.WithString("layerId", mcp.Description("Layer actor UUID."), mcp.Required()),
		mcp.WithString("period", mcp.Description("Account values = turnover of this last period (e.g. 30d) instead of the balance.")),
		mcp.WithBoolean("overwrite", mcp.Description("Replace an existing <layerId>.sim.yaml (default false: the call refuses, so hand edits are not lost).")),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
	), handleSnapshot)
}

// localPath resolves a user-given relative path inside the working directory.
func localPath(p string) (string, error) {
	if ecore.IsStateless() {
		return "", fmt.Errorf("file paths are not available on the hosted server; pass the YAML as text or use layerId")
	}
	if p == "" || filepath.IsAbs(p) {
		return "", fmt.Errorf("path must be relative to the working directory: %q", p)
	}
	clean := filepath.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path must stay inside the working directory: %q", p)
	}
	return ecore.ResolvePath(clean), nil
}

func textOrFile(args map[string]any, textKey, pathKey string) ([]byte, error) {
	if t, _ := args[textKey].(string); t != "" {
		return []byte(t), nil
	}
	if p, _ := args[pathKey].(string); p != "" {
		path, err := localPath(p)
		if err != nil {
			return nil, err
		}
		return os.ReadFile(path)
	}
	return nil, nil
}

const (
	maxLogEvents     = 10_000
	defaultTimeLimit = 2 * time.Minute
	maxTimeLimit     = 15 * time.Minute
)

func parseTimeLimit(args map[string]any) (time.Duration, error) {
	p, _ := args["timeLimit"].(string)
	if p == "" {
		return defaultTimeLimit, nil
	}
	sec, err := parseDuration(p)
	if err != nil {
		return 0, err
	}
	d := time.Duration(sec) * time.Second
	if d <= 0 {
		return 0, fmt.Errorf("must be positive")
	}
	return min(d, maxTimeLimit), nil
}

func parsePeriod(args map[string]any) (time.Duration, error) {
	p, _ := args["period"].(string)
	if p == "" {
		return 0, nil
	}
	sec, err := parseDuration(p)
	if err != nil {
		return 0, err
	}
	return time.Duration(sec) * time.Second, nil
}

type inputs struct {
	modelSrc  []byte
	model     *Model
	graph     *Graph
	scenarios []Scenario
	rawScen   []*OMap
}

func loadInputs(ctx context.Context, args map[string]any, needGraph bool) (*inputs, *mcp.CallToolResult) {
	fail := func(format string, a ...any) (*inputs, *mcp.CallToolResult) {
		return nil, mcp.NewToolResultError("[Error] " + fmt.Sprintf(format, a...))
	}
	in := &inputs{}
	var err error
	if in.modelSrc, err = textOrFile(args, "model", "modelPath"); err != nil {
		return fail("model: %v", err)
	}
	if in.modelSrc == nil {
		return fail("pass model (YAML text) or modelPath")
	}
	if in.model, err = LoadModel(in.modelSrc); err != nil {
		return fail("%v", err)
	}
	scen, err := textOrFile(args, "scenarios", "scenariosPath")
	if err != nil {
		return fail("scenarios: %v", err)
	}
	if scen != nil {
		if in.scenarios, in.rawScen, err = LoadScenarios(scen); err != nil {
			return fail("%v", err)
		}
	}
	if len(in.scenarios) == 0 {
		in.scenarios = []Scenario{ScenarioFrom(NewOMap())}
		in.rawScen = []*OMap{NewOMap()}
	}
	layerID, _ := args["layerId"].(string)
	graphPath, _ := args["graphPath"].(string)
	switch {
	case layerID != "":
		if r := ecore.RequireUUID("layerId", layerID); r != nil {
			return nil, r
		}
		if r := ecore.EnsureAuth(ctx); r != nil {
			return nil, r
		}
		period, err := parsePeriod(args)
		if err != nil {
			return fail("period: %v", err)
		}
		if in.graph, err = Snapshot(ctx, layerID, period); err != nil {
			return fail("%v", err)
		}
	case graphPath != "":
		path, err := localPath(graphPath)
		if err != nil {
			return fail("graphPath: %v", err)
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return fail("graphPath: %v", err)
		}
		if in.graph, err = LoadGraph(src); err != nil {
			return fail("%v", err)
		}
		// a pullGraphFile output names forms only by id; use their titles when logged in
		if len(unnamedFormTypes(in.graph)) > 0 && ecore.EnsureAuth(ctx) == nil {
			nameFormTypes(ctx, in.graph)
		}
	case needGraph:
		return fail("pass layerId (live) or graphPath (layer YAML file)")
	}
	return in, nil
}

func jsonText(v any) *mcp.CallToolResult {
	b, _ := json.MarshalIndent(v, "", " ")
	return mcp.NewToolResultText(string(b))
}

func handleCheck(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in, res := loadInputs(ctx, req.GetArguments(), false)
	if res != nil {
		return res, nil
	}
	rep := Check(in.modelSrc, in.graph, in.rawScen)
	return jsonText(map[string]any{"ok": rep.OK(), "errors": rep.Errors, "warnings": rep.Warnings}), nil
}

func handleRun(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	in, res := loadInputs(ctx, args, true)
	if res != nil {
		return res, nil
	}
	rep := Check(in.modelSrc, in.graph, in.rawScen)
	if !rep.OK() {
		return jsonText(map[string]any{"ok": false, "message": "model check failed: fix the errors, nothing was run",
			"errors": rep.Errors, "warnings": rep.Warnings}), nil
	}
	selected := in.scenarios
	if names, _ := args["scenario"].(string); names != "" {
		want := map[string]bool{}
		for _, n := range strings.Split(names, ",") {
			want[strings.TrimSpace(n)] = true
		}
		selected = nil
		for _, sc := range in.scenarios {
			if want[sc.Name] {
				selected = append(selected, sc)
			}
		}
		if len(selected) == 0 {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] no scenarios named %s", names)), nil
		}
	}
	runs := 1
	if r, ok := args["runs"].(float64); ok && r > 1 {
		runs = int(r)
		if runs > 1000 {
			runs = 1000
		}
	}
	goals := NewOMap()
	if gsrc, _ := args["goals"].(string); gsrc != "" {
		v, err := parseYAML([]byte(gsrc))
		if err != nil {
			return mcp.NewToolResultError("[Error] goals: " + err.Error()), nil
		}
		if m, ok := v.(*OMap); ok {
			goals = m
		}
	}
	logN := 0
	if n, ok := args["logEvents"].(float64); ok && n > 0 {
		logN = min(int(n), maxLogEvents)
	}
	limit, err := parseTimeLimit(args)
	if err != nil {
		return mcp.NewToolResultError("[Error] timeLimit: " + err.Error()), nil
	}
	runCtx, cancel := context.WithTimeoutCause(ctx, limit, fmt.Errorf("time limit %s reached", limit))
	defer cancel()
	out := map[string]any{"ok": true, "graph": in.graph.summary(), "source": toJSON(in.graph.Source)}
	if len(rep.Warnings) > 0 {
		out["warnings"] = rep.Warnings
	}
	if runs > 1 {
		var summaries []*Summary
		for _, sc := range selected {
			summaries = append(summaries, RunMany(runCtx, in.graph, in.model, sc, runs, nil, goals))
		}
		if ctx.Err() != nil {
			return mcp.NewToolResultError("[Error] cancelled"), nil
		}
		out["runs"] = runs
		out["table"] = SummaryTable(summaries)
		out["scenarios"] = summariesJSON(summaries)
		return jsonText(out), nil
	}
	var results []*RunResult
	for _, sc := range selected {
		results = append(results, RunScenario(in.graph, in.model, sc, nil, RunOptions{Ctx: runCtx, LogLimit: logN}))
	}
	if ctx.Err() != nil {
		return mcp.NewToolResultError("[Error] cancelled"), nil
	}
	out["table"] = CompareTable(results)
	out["scenarios"] = resultsJSON(results, logN)
	return jsonText(out), nil
}

func handleSnapshot(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	layerID, _ := args["layerId"].(string)
	if r := ecore.RequireUUID("layerId", layerID); r != nil {
		return r, nil
	}
	if r := ecore.EnsureAuth(ctx); r != nil {
		return r, nil
	}
	period, err := parsePeriod(args)
	if err != nil {
		return mcp.NewToolResultError("[Error] period: " + err.Error()), nil
	}
	path := ecore.ResolvePath(layerID + ".sim.yaml")
	if overwrite, _ := args["overwrite"].(bool); !overwrite {
		if _, err := os.Stat(path); err == nil {
			return mcp.NewToolResultError("[Error] " + path + " already exists (it may hold hand edits); pass overwrite: true to replace it"), nil
		}
	}
	g, err := Snapshot(ctx, layerID, period)
	if err != nil {
		return mcp.NewToolResultError("[Error] " + err.Error()), nil
	}
	b, err := GraphFile(g, layerID)
	if err != nil {
		return mcp.NewToolResultError("[Error] " + err.Error()), nil
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return mcp.NewToolResultError("[Error] write snapshot: " + err.Error()), nil
	}
	return jsonText(map[string]any{"file": path, "graph": g.summary(), "source": toJSON(g.Source)}), nil
}

// ---- output ------------------------------------------------------------------

func metricNames[T any](items []T, get func(T) []string) []string {
	set := map[string]bool{}
	for _, it := range items {
		for _, n := range get(it) {
			set[n] = true
		}
	}
	return sortedKeys(set)
}

// CompareTable renders one row per scenario.
func CompareTable(results []*RunResult) string {
	names := metricNames(results, func(r *RunResult) []string {
		out := make([]string, 0, len(r.Metrics))
		for k := range r.Metrics {
			out = append(out, k)
		}
		return out
	})
	var b strings.Builder
	b.WriteString("| scenario | status | steps | " + strings.Join(names, " | ") + " |\n|" + strings.Repeat("---|", 3+len(names)) + "\n")
	var errs []string
	for _, r := range results {
		cells := []string{r.Scenario, r.Status, fmt.Sprint(r.Steps)}
		for _, n := range names {
			v, ok := r.Metrics[n]
			if !ok {
				cells = append(cells, "")
				continue
			}
			cells = append(cells, MetricText(v))
		}
		b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
		if r.Error != "" {
			errs = append(errs, "- "+r.Scenario+": "+r.Error)
		}
	}
	if len(errs) > 0 {
		b.WriteString("\nErrors:\n" + strings.Join(errs, "\n") + "\n")
	}
	return b.String()
}

// SummaryTable renders medians with ranges and goal shares.
func SummaryTable(summaries []*Summary) string {
	metrics := metricNames(summaries, func(s *Summary) []string {
		out := make([]string, 0, len(s.Metrics))
		for k := range s.Metrics {
			out = append(out, k)
		}
		return out
	})
	goals := metricNames(summaries, func(s *Summary) []string {
		out := make([]string, 0, len(s.Goals))
		for k := range s.Goals {
			out = append(out, k)
		}
		return out
	})
	head := append([]string{"scenario", "runs completed/total"}, metrics...)
	for _, g := range goals {
		head = append(head, "goal: "+g)
	}
	var b strings.Builder
	b.WriteString("| " + strings.Join(head, " | ") + " |\n|" + strings.Repeat("---|", len(head)) + "\n")
	for _, s := range summaries {
		ok := fmt.Sprintf("%d/%d", s.Completed, s.Runs)
		var notOK []string
		if s.Stopped > 0 {
			notOK = append(notOK, fmt.Sprintf("%d stopped", s.Stopped))
		}
		if s.Failed > 0 {
			notOK = append(notOK, fmt.Sprintf("%d failed", s.Failed))
		}
		if len(notOK) > 0 {
			ok += " (" + strings.Join(notOK, ", ") + ")"
		}
		cells := []string{s.Scenario, ok}
		for _, m := range metrics {
			if st := s.Metrics[m]; st != nil {
				cells = append(cells, st.Short())
			} else {
				cells = append(cells, "")
			}
		}
		for _, g := range goals {
			v := s.Goals[g]
			if v[1] == 0 {
				cells = append(cells, "no data")
			} else {
				cells = append(cells, fmt.Sprintf("%d%% (%d/%d)", (100*v[0]+v[1]/2)/v[1], v[0], v[1]))
			}
		}
		b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
	}
	b.WriteString("\nNumbers: median (10th–90th percentile) over completed runs. Goal: share of completed runs where it holds; failed runs and runs stopped before the horizon are not counted.\n")
	for _, s := range summaries {
		for i, e := range s.Errors {
			if i == 3 {
				break
			}
			b.WriteString("- " + s.Scenario + ": " + e + "\n")
		}
	}
	return b.String()
}

func resultsJSON(results []*RunResult, logN int) []map[string]any {
	var out []map[string]any
	for _, r := range results {
		metrics := map[string]any{}
		for k, v := range r.Metrics {
			metrics[k] = toJSON(v)
		}
		entry := map[string]any{"scenario": r.Scenario, "status": r.Status, "steps": r.Steps, "model_time": r.ModelTime,
			"pending_events": r.PendingEvents, "metrics": metrics}
		if r.Error != "" {
			entry["error"] = r.Error
		}
		if logN > 0 {
			n := logN
			if n > len(r.Log) {
				n = len(r.Log)
			}
			entry["events"] = r.Log[:n]
		}
		out = append(out, entry)
	}
	return out
}

func summariesJSON(summaries []*Summary) []map[string]any {
	var out []map[string]any
	for _, s := range summaries {
		metrics := map[string]any{}
		for k, st := range s.Metrics {
			metrics[k] = map[string]any{"n": st.N, "mean": numString(st.Mean), "p10": numString(st.P10),
				"p50": numString(st.P50), "p90": numString(st.P90), "min": numString(st.Min), "max": numString(st.Max)}
		}
		goals := map[string]any{}
		keys := make([]string, 0, len(s.Goals))
		for k := range s.Goals {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			goals[k] = map[string]int{"held": s.Goals[k][0], "evaluated": s.Goals[k][1]}
		}
		entry := map[string]any{"scenario": s.Scenario, "runs": s.Runs, "completed": s.Completed,
			"failed": s.Failed, "stopped": s.Stopped, "metrics": metrics, "goals": goals, "errors": s.Errors}
		if s.Note != "" {
			entry["note"] = s.Note
		}
		out = append(out, entry)
	}
	return out
}
