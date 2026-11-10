package osint

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const caseDesc = "Case: pass `case` (YAML text) or `casePath` (file in the working directory). " +
	"Format: docs/osint/case-format.md."

// hostedCaseDesc is caseDesc for the hosted (stateless) server, which has no
// working directory.
const hostedCaseDesc = "Case: pass `case` (YAML text). Format: docs/osint/case-format.md."

// Register adds the investigation tools. None of them reaches an outside
// source. Check, analyse, plan and generate read a case and compute; only
// osintCaseBuild writes to Simulator.
func Register(s *server.MCPServer) {
	desc := caseDesc
	if ecore.IsStateless() {
		desc = hostedCaseDesc
	}
	common := func(opts ...mcp.ToolOption) []mcp.ToolOption {
		base := []mcp.ToolOption{
			mcp.WithString("case", mcp.Description("Case YAML text.")),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
		}
		if !ecore.IsStateless() {
			base = append(base, mcp.WithString("casePath", mcp.Description("Case YAML file, relative to the working directory.")))
		}
		return append(base, opts...)
	}

	s.AddTool(mcp.NewTool("osintCaseCheck", common(
		mcp.WithDescription("Check an investigation case before analysing or building it: ids that point nowhere, "+
			"duplicate ids, amounts and dates that cannot be right, stakes above 100 %, claims with no source, "+
			"entities linked to nothing, likely duplicates. Returns errors (the case is inconsistent) and warnings "+
			"(weak spots to know about). "+desc),
	)...), handleCheck)

	var packFile []mcp.ToolOption
	if !ecore.IsStateless() {
		packFile = append(packFile, mcp.WithString("packPath", mcp.Description("A pack file, relative to the working directory.")))
	}
	s.AddTool(mcp.NewTool("osintCaseAnalyze", common(append(packFile,
		mcp.WithDescription("Analyse an investigation case: effective beneficial ownership through chains of companies, "+
			"circular ownership, one person fronting many companies, shared addresses and identifiers, pass-through "+
			"accounts and layering chains, round trips, structuring under a threshold, funnels and payout networks, "+
			"the most connected entities, money moved per entity, a chronology, and how well the case is sourced. "+
			"Each finding lists the case items it rests on and the grade of its weakest evidence. Findings are leads "+
			"to check, not conclusions. With `from` and `to` it also returns the shortest chains of relations and "+
			"payments between two entities. A case with errors is not analysed. A synthetic case is scored against "+
			"its known answer. Give a case, or `layerId` to analyse a live layer. "+desc),
		mcp.WithString("layerId", mcp.Description("Analyse a layer of the workspace as it stands, instead of a case file: its actors, links, accounts and the transfers between them. Works on any layer; one built by osintCaseBuild keeps its ids, dates and grades.")),
		mcp.WithString("pack", mcp.Description("What to look for: the id of a pack that comes with the plugin ("+strings.Join(ShippedPacks(), ", ")+"), or the YAML text of your own pack — thresholds, which built-in findings to report, and your own rules. Format: docs/osint/packs.md. Without one, every built-in pattern with the default thresholds.")),
		mcp.WithString("from", mcp.Description("Entity or account id: find how it is connected to `to`.")),
		mcp.WithString("to", mcp.Description("Entity or account id: the other end of the connection.")),
		mcp.WithString("threshold", mcp.Description(fmt.Sprintf("Reporting threshold that structuring stays under, as a number in the case's currency (default %d).", defaultThreshold))),
		mcp.WithNumber("windowDays", mcp.Description(fmt.Sprintf("Days within which money moving on counts as one move, for pass-through and structuring (default %d).", defaultWindowDays))),
		mcp.WithString("minShare", mcp.Description(fmt.Sprintf("Effective stake, in percent, from which an indirect owner is reported (default %d).", defaultMinShare))),
		mcp.WithNumber("top", mcp.Description(fmt.Sprintf("Rows in the ranked tables (default %d, max 100).", defaultTop))),
		mcp.WithNumber("timeline", mcp.Description(fmt.Sprintf("Chronology entries (default %d, max 500).", defaultTimeline))),
	)...)...), handleAnalyze)

	s.AddTool(mcp.NewTool("osintCasePlan", common(
		mcp.WithDescription("Plan how to build an investigation case as a digital twin in Simulator: forms, actors with "+
			"descriptions and positions, named links, accounts and transfers, each with a stable ref "+
			"so that building twice changes nothing, plus the ordered steps and the curated tools that carry them out. "+
			"The plan itself writes nothing; osintCaseBuild carries it out. A case with errors is not planned. Without `part` it returns the "+
			"summary (counts, forms, account pairs, steps, notes); read each list page by page with `part`. "+desc),
		mcp.WithString("part", mcp.Description("Which part to return: summary (default), actors, links, accounts, transfers, chronology.")),
		mcp.WithNumber("offset", mcp.Description("Items to skip in a list part (default 0).")),
		mcp.WithNumber("limit", mcp.Description(fmt.Sprintf("Items per page of a list part (default %d, max %d).", defaultPlanPage, maxPlanPage))),
	)...), handlePlan)

	buildOpts := []mcp.ToolOption{
		mcp.WithDescription("Build an investigation case in Simulator as a digital twin: a graph and a layer for the case, " +
			"one actor per entity (form OSINT <Type>) with its fields filled from the case, its sources and grades and a picture, " +
			"named links for the relations, " +
			"an account on its owner's actor for each account, and a transfer for each transaction. Everything is found again " +
			"by its ref (osint:<caseId>:<id>), so repeating the call creates nothing twice and finishes a build that stopped " +
			"half-way; existing actors are brought up to date with the case. WRITES to the workspace — transfers cannot be " +
			"undone: confirm the workspace and the counts of osintCasePlan with the user first. A case with errors is not " +
			"built. " + desc),
		mcp.WithString("case", mcp.Description("Case YAML text.")),
		mcp.WithString("accId", mcp.Description("Workspace id. Defaults to the configured workspace if omitted.")),
		mcp.WithString("layerId", mcp.Description("Layer actor UUID to build on. Omit to give the case its own graph and layer.")),
		mcp.WithBoolean("relayout", mcp.Description("Move the case's actors that are already on the layer to their planned places (default false: actors keep where they stand, since the user may have arranged them). Other things on the layer are not moved.")),
		mcp.WithString("pictures", mcp.Description("Actor pictures: auto (default: the case's pictures, a generated placeholder for the rest), given (only the case's pictures), none.")),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
	}
	if !ecore.IsStateless() {
		buildOpts = append(buildOpts, mcp.WithString("casePath", mcp.Description("Case YAML file, relative to the working directory.")))
	}
	s.AddTool(mcp.NewTool("osintCaseBuild", buildOpts...), handleBuild)

	returns := "Returns the case YAML, or writes it to `outPath`."
	if ecore.IsStateless() {
		returns = "Returns the case YAML."
	}
	genOpts := []mcp.ToolOption{
		mcp.WithDescription("Generate a synthetic investigation case for training and for testing an analysis: ordinary " +
			"background activity with schemes hidden in it (layering through shell companies, a round trip, smurfing " +
			"under a threshold, a nominee director) and the known answer in `truth`. Every name is invented. The same " +
			"seed gives the same case. " + returns),
		mcp.WithString("typologies", mcp.Description("Comma-separated schemes to plant: "+strings.Join(Typologies, ", ")+" (default: all).")),
		mcp.WithNumber("seed", mcp.Description("Random seed (default 1).")),
		mcp.WithNumber("noise", mcp.Description(fmt.Sprintf("Ordinary people and companies around the schemes (default %d, max %d).", defaultNoise, maxNoise))),
		mcp.WithString("currency", mcp.Description("Currency of the case (default USD).")),
		mcp.WithString("start", mcp.Description("First day of the case, YYYY-MM-DD (default 2025-01-06).")),
		mcp.WithReadOnlyHintAnnotation(ecore.IsStateless()),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
	}
	if !ecore.IsStateless() {
		genOpts = append(genOpts,
			mcp.WithString("outPath", mcp.Description("Write the case to this .yaml file, relative to the working directory, instead of returning its text.")),
			mcp.WithBoolean("overwrite", mcp.Description("Replace an existing file at outPath (default false).")),
		)
	}
	s.AddTool(mcp.NewTool("osintCaseGenerate", genOpts...), handleGenerate)
	registerCollect(s)
}

// localPath resolves a user-given relative path inside the working directory.
func localPath(p string) (string, error) {
	if ecore.IsStateless() {
		return "", fmt.Errorf("file paths are not available on the hosted server; pass the YAML as text")
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

func fail(format string, a ...any) *mcp.CallToolResult {
	return mcp.NewToolResultError("[Error] " + fmt.Sprintf(format, a...))
}

func jsonText(v any) *mcp.CallToolResult {
	b, _ := json.MarshalIndent(v, "", " ")
	return mcp.NewToolResultText(string(b))
}

func loadCase(args map[string]any) (*Case, *mcp.CallToolResult) {
	var src []byte
	if t, _ := args["case"].(string); t != "" {
		src = []byte(t)
	} else if p, _ := args["casePath"].(string); p != "" {
		path, err := localPath(p)
		if err != nil {
			return nil, fail("casePath: %v", err)
		}
		if info, err := os.Stat(path); err == nil && info.Size() > maxCaseBytes {
			return nil, fail("casePath: file is %d bytes; the limit is %d", info.Size(), maxCaseBytes)
		}
		if src, err = os.ReadFile(path); err != nil {
			return nil, fail("casePath: %v", err)
		}
	} else {
		return nil, fail("pass case (YAML text) or casePath")
	}
	c, err := LoadCase(src)
	if err != nil {
		return nil, fail("%v", err)
	}
	return c, nil
}

func handleCheck(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	c, res := loadCase(req.GetArguments())
	if res != nil {
		return res, nil
	}
	rep := Check(c)
	return jsonText(map[string]any{"ok": rep.OK(), "case": c.Meta.ID, "errors": rep.Errors, "warnings": rep.Warnings}), nil
}

// refused is the reply for a case that failed its check.
func refused(what string, rep *Report) *mcp.CallToolResult {
	return jsonText(map[string]any{"ok": false, "message": "case check failed: fix the errors, nothing was " + what,
		"errors": rep.Errors, "warnings": rep.Warnings})
}

// packArg loads the pack a call names: one that comes with the plugin, the
// text of the user's own, or a file of it.
func packArg(args map[string]any) (*Pack, error) {
	if text, _ := args["pack"].(string); text != "" {
		if idRe.MatchString(text) {
			return ShippedPack(text)
		}
		return LoadPack([]byte(text))
	}
	name, _ := args["packPath"].(string)
	if name == "" {
		return nil, nil
	}
	path, err := localPath(name)
	if err != nil {
		return nil, fmt.Errorf("packPath: %v", err)
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("packPath: %v", err)
	}
	return LoadPack(src)
}

func ratArg(args map[string]any, key string) (*big.Rat, error) {
	switch v := args[key].(type) {
	case nil:
		return nil, nil
	case float64:
		r, ok := new(big.Rat).SetString(fmt.Sprint(v))
		if !ok || r.Sign() <= 0 {
			return nil, fmt.Errorf("%s must be a positive number", key)
		}
		return r, nil
	case string:
		if v == "" {
			return nil, nil
		}
		d, ok := newDecimal(v)
		if !ok || d.Rat().Sign() <= 0 {
			return nil, fmt.Errorf("%s must be a positive number, got %q", key, v)
		}
		return d.Rat(), nil
	}
	return nil, fmt.Errorf("%s must be a number", key)
}

func intArg(args map[string]any, key string, limit int) int {
	n, _ := args[key].(float64)
	if n < 1 {
		return 0
	}
	return min(int(n), limit)
}

// caseOrLayer loads the case a call names: a case file or text, or a live
// layer read as a case. For a layer it also returns what the layer could not
// tell.
func caseOrLayer(ctx context.Context, args map[string]any) (*Case, []string, *mcp.CallToolResult) {
	layerID, _ := args["layerId"].(string)
	if layerID == "" {
		c, res := loadCase(args)
		return c, nil, res
	}
	if r := ecore.RequireUUID("layerId", layerID); r != nil {
		return nil, nil, r
	}
	if r := ecore.EnsureAuth(ctx); r != nil {
		return nil, nil, r
	}
	c, notes, err := FromLayer(ctx, layerID)
	if err != nil {
		return nil, nil, fail("%v", err)
	}
	return c, notes, nil
}

func handleAnalyze(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	c, layerNotes, res := caseOrLayer(ctx, args)
	if res != nil {
		return res, nil
	}
	rep := Check(c)
	if !rep.OK() {
		return refused("analysed", rep), nil
	}
	opt := Options{WindowDays: intArg(args, "windowDays", 365), Top: intArg(args, "top", 100), Timeline: intArg(args, "timeline", 500)}
	opt.From, _ = args["from"].(string)
	opt.To, _ = args["to"].(string)
	var err error
	if opt.Pack, err = packArg(args); err != nil {
		return fail("%v", err), nil
	}
	if opt.Threshold, err = ratArg(args, "threshold"); err != nil {
		return fail("%v", err), nil
	}
	if opt.MinShare, err = ratArg(args, "minShare"); err != nil {
		return fail("%v", err), nil
	}
	out := map[string]any{"ok": true, "analysis": Analyze(c, opt)}
	if opt.Pack != nil {
		out["pack"] = opt.Pack.Meta.ID
	}
	if len(rep.Warnings) > 0 {
		out["checkWarnings"] = rep.Warnings
	}
	if layerNotes != nil {
		out["layer"] = map[string]any{"notes": layerNotes, "entities": len(c.Entities), "relations": len(c.Relations),
			"accounts": len(c.Accounts), "transactions": len(c.Transactions)}
	}
	return jsonText(out), nil
}

func handlePlan(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	c, res := loadCase(args)
	if res != nil {
		return res, nil
	}
	rep := Check(c)
	if !rep.OK() {
		return refused("planned", rep), nil
	}
	p := BuildPlan(c)
	counts := map[string]int{"forms": len(p.Forms), "actors": len(p.Actors), "links": len(p.Links), "accounts": len(p.Accounts),
		"transfers": len(p.Transfers), "chronology": len(p.Chronology)}
	part, _ := args["part"].(string)
	offset := intArg(args, "offset", 1<<30)
	limit := intArg(args, "limit", maxPlanPage)
	if limit == 0 {
		limit = defaultPlanPage
	}
	out := map[string]any{"ok": true, "case": p.Case, "counts": counts}
	switch part {
	case "", "summary":
		out["plan"] = map[string]any{"synthetic": p.Synthetic, "refPrefix": p.RefPrefix, "layerTitle": p.Layer, "forms": p.Forms,
			"accountPairs": p.AccountPairs, "steps": p.Steps, "notes": p.Notes}
		out["next"] = "read the lists with part = actors | links | accounts | transfers | chronology (offset, limit)"
	case "actors":
		out["actors"], out["more"] = page(p.Actors, offset, limit)
	case "links":
		out["links"], out["more"] = page(p.Links, offset, limit)
	case "accounts":
		out["accounts"], out["more"] = page(p.Accounts, offset, limit)
	case "transfers":
		out["transfers"], out["more"] = page(p.Transfers, offset, limit)
	case "chronology":
		out["chronology"], out["more"] = page(p.Chronology, offset, limit)
	default:
		return fail("part %q is not one of summary, actors, links, accounts, transfers, chronology", part), nil
	}
	return jsonText(out), nil
}

const (
	defaultPlanPage = 50
	maxPlanPage     = 200
)

// page returns one page of a list and whether more follows it.
func page[T any](list []T, offset, limit int) ([]T, bool) {
	if offset >= len(list) {
		return []T{}, false
	}
	end := min(offset+limit, len(list))
	return list[offset:end], end < len(list)
}

func handleBuild(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	c, res := loadCase(args)
	if res != nil {
		return res, nil
	}
	rep := Check(c)
	if !rep.OK() {
		return refused("built", rep), nil
	}
	layerID, _ := args["layerId"].(string)
	if layerID != "" {
		if r := ecore.RequireUUID("layerId", layerID); r != nil {
			return r, nil
		}
	}
	if r := ecore.EnsureAuth(ctx); r != nil {
		return r, nil
	}
	ws := ecore.WorkspaceIDForContext(ctx)
	if acc, _ := args["accId"].(string); acc != "" && ws == "" {
		ws = acc
	}
	if ws == "" {
		return mcp.NewToolResultError(ecore.NoWorkspaceError), nil
	}
	if r := ecore.RequireUUID("accId", ws); r != nil {
		return r, nil
	}
	pictures, _ := args["pictures"].(string)
	if pictures != "" && pictures != PicturesAuto && pictures != PicturesGiven && pictures != PicturesNone {
		return fail("pictures %q is not one of auto, given, none", pictures), nil
	}
	relayout, _ := args["relayout"].(bool)
	result, err := Build(ctx, c, ws, BuildOptions{LayerID: layerID, Pictures: pictures, Relayout: relayout})
	out := map[string]any{"ok": err == nil, "build": result}
	if err != nil {
		out["error"] = err.Error()
		out["message"] = "the build stopped; what was created stays, and running it again continues from there"
	}
	return jsonText(out), nil
}

func handleGenerate(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	opt := GenOptions{Seed: 1}
	if list, _ := args["typologies"].(string); list != "" {
		for _, t := range strings.Split(list, ",") {
			if t = strings.TrimSpace(t); t != "" {
				opt.Typologies = append(opt.Typologies, t)
			}
		}
	}
	if seed, ok := args["seed"].(float64); ok {
		if seed < 0 || seed > 1<<53 {
			return fail("seed must be a whole number from 0 to 2^53"), nil
		}
		opt.Seed = uint64(seed)
	}
	if n, ok := args["noise"].(float64); ok {
		if n < 0 || n > maxNoise {
			return fail("noise must be 0-%d", maxNoise), nil
		}
		// 0 means "no background" here, while GenOptions reads 0 as "default"
		opt.Noise = int(n)
		if opt.Noise == 0 {
			opt.Noise = -1
		}
	}
	opt.Currency, _ = args["currency"].(string)
	if s, _ := args["start"].(string); s != "" {
		d, ok := parseDate(s)
		if !ok {
			return fail("start: %q is not a date (use YYYY-MM-DD)", s), nil
		}
		opt.Start = d
	}
	c, err := Generate(opt)
	if err != nil {
		return fail("%v", err), nil
	}
	src, err := MarshalCase(c)
	if err != nil {
		return fail("%v", err), nil
	}
	out := map[string]any{"ok": true, "case": c.Meta.ID, "seed": opt.Seed, "typologies": c.Truth.Typologies,
		"counts": map[string]int{"entities": len(c.Entities), "relations": len(c.Relations), "accounts": len(c.Accounts),
			"transactions": len(c.Transactions), "events": len(c.Events)}}
	outPath, _ := args["outPath"].(string)
	if outPath == "" {
		out["yaml"] = string(src)
		return jsonText(out), nil
	}
	if ext := strings.ToLower(filepath.Ext(outPath)); ext != ".yaml" && ext != ".yml" {
		return fail("outPath must end in .yaml or .yml"), nil
	}
	path, err := localPath(outPath)
	if err != nil {
		return fail("outPath: %v", err), nil
	}
	if overwrite, _ := args["overwrite"].(bool); !overwrite {
		if _, err := os.Stat(path); err == nil {
			return fail("%s already exists; pass overwrite: true to replace it", path), nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fail("outPath: %v", err), nil
	}
	if err := os.WriteFile(path, src, 0o600); err != nil {
		return fail("write case: %v", err), nil
	}
	out["file"] = path
	return jsonText(out), nil
}
