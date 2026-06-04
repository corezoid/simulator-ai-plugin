// Command gendiscovery regenerates the AI-discovery artifacts
// (public/.well-known/skills/index.json and public/llms.txt) from the plugin
// SKILL.md files. It is the Go port of the former scripts/generate-discovery.py.
//
// Run from the mcp-server module directory; --root points at the repo root:
//
//	go run ./cmd/gendiscovery --root ../../..
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const repoRaw = "https://raw.githubusercontent.com/corezoid/simulator-ai-plugin/main"

var (
	skillsRaw = repoRaw + "/plugins/simulator/skills"
	docsRaw   = repoRaw + "/plugins/simulator/docs"

	fmBlockRe  = regexp.MustCompile(`(?s)^---\n(.*?)\n---`)
	nameRe     = regexp.MustCompile(`(?m)^name:\s*(.+)$`)
	descFolded = regexp.MustCompile(`(?m)^description:\s*[>|]\s*\n((?:[ \t]+[^\n]*\n?)+)`)
	descDQ     = regexp.MustCompile(`(?m)^description:\s*"(.*)"`)
	descSQ     = regexp.MustCompile(`(?m)^description:\s*'(.*)'`)
	descPlain  = regexp.MustCompile(`(?m)^description:\s*(.+)$`)
)

// mcpTools is a curated highlight list for llms.txt. llms.txt is meant to be a
// concise overview, not an exhaustive dump of all 185 generated tools, so this
// stays a hand-picked set of the most useful tools.
var mcpTools = [][2]string{
	{"login", "Authenticate with Simulator.Company via OAuth2 browser flow"},
	{"set-workspace", "Save workspace ID to .env for subsequent API calls"},
	{"pullGraphFile", "Export a layer to a local YAML file for editing"},
	{"pushGraphFile", "Sync a local YAML graph file back to a Simulator layer"},
	{"createActor", "Create a single actor (node) in a layer"},
	{"createActors", "Bulk-create up to 50 actors in a single call"},
	{"updateActor", "Update actor properties (title, description, status)"},
	{"deleteActor", "Delete an actor from a layer"},
	{"getActor", "Get actor details by ID"},
	{"searchActors", "Search actors by title or custom fields"},
	{"createLink", "Create a link (edge) between two actors"},
	{"massLink", "Create multiple links in one call"},
	{"deleteLink", "Delete a link between actors"},
	{"manageLayer", "Create, update, or delete a layer"},
	{"moveElements", "Move actors on a layer canvas"},
	{"compactGraphLayout", "Auto-layout actors with domain-clustering strategy"},
	{"pruneLongEdges", "Delete links that exceed a Manhattan distance threshold"},
	{"getAllLayerPlacements", "Enumerate all actors on a layer in one call"},
	{"createForm", "Create a new form template"},
	{"updateForm", "Update an existing form template"},
	{"getForms", "List all available form templates"},
	{"createAccountName", "Define an account name (asset, liability, expense, income)"},
	{"createCurrency", "Create a currency for financial tracking"},
	{"addFormAccount", "Add an account to a form template"},
	{"uploadActorPicture", "Set an actor's avatar from URL, local file, or base64 (auto SVG→PNG)"},
	{"uploadActorPictureBulk", "Bulk-upload actor pictures with SHA-256 deduplication"},
	{"createChart", "Create a chart/dashboard actor on a layer"},
}

type skill struct {
	Name        string
	Description string
	Dir         string
	Files       []string
}

func parseDescription(fm string) string {
	if m := descFolded.FindStringSubmatch(fm); m != nil {
		var parts []string
		for _, ln := range strings.Split(m[1], "\n") {
			if s := strings.TrimSpace(ln); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " ")
	}
	if m := descDQ.FindStringSubmatch(fm); m != nil {
		return strings.TrimSpace(strings.ReplaceAll(m[1], `\"`, `"`))
	}
	if m := descSQ.FindStringSubmatch(fm); m != nil {
		return strings.TrimSpace(strings.ReplaceAll(m[1], "''", "'"))
	}
	if m := descPlain.FindStringSubmatch(fm); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func parseFrontmatter(path string) (name, desc string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	block := fmBlockRe.FindSubmatch(data)
	if block == nil {
		return "", ""
	}
	fm := string(block[1])
	if m := nameRe.FindStringSubmatch(fm); m != nil {
		name = strings.TrimSpace(m[1])
	}
	return name, parseDescription(fm)
}

func collectSkills(skillsDir string) ([]skill, error) {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, err
	}
	var skills []skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillPath := filepath.Join(skillsDir, e.Name())
		skillMD := filepath.Join(skillPath, "SKILL.md")
		if _, err := os.Stat(skillMD); err != nil {
			continue
		}
		name, desc := parseFrontmatter(skillMD)
		if name == "" || desc == "" {
			fmt.Fprintf(os.Stderr, "WARN: skipping %s — missing name or description\n", e.Name())
			continue
		}
		var mdFiles []string
		_ = filepath.WalkDir(skillPath, func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(p, ".md") {
				rel, _ := filepath.Rel(skillPath, p)
				mdFiles = append(mdFiles, rel)
			}
			return nil
		})
		sort.Strings(mdFiles)
		skills = append(skills, skill{Name: name, Description: desc, Dir: e.Name(), Files: mdFiles})
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Dir < skills[j].Dir })
	return skills, nil
}

func indexJSON(skills []skill) map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(skills))
	for _, s := range skills {
		files := make([]string, len(s.Files))
		for i, f := range s.Files {
			files[i] = fmt.Sprintf("%s/%s/%s", skillsRaw, s.Dir, f)
		}
		out = append(out, map[string]interface{}{"name": s.Name, "description": s.Description, "files": files})
	}
	return map[string]interface{}{"skills": out}
}

func llmsTxt(skills []skill) string {
	var b strings.Builder
	b.WriteString("# Simulator.Company AI Plugin\n\n")
	b.WriteString("> Official Claude Code plugin for Simulator.Company platform. " +
		"Provides skills and MCP tools for managing actors, graph-based business " +
		"processes, form templates, financial accounts, and visualisations " +
		"directly from the IDE.\n\n")
	b.WriteString("## Skills\n\n")
	for _, s := range skills {
		teaser := strings.TrimRight(strings.SplitN(s.Description, ". ", 2)[0], ".")
		b.WriteString(fmt.Sprintf("- [%s](%s/%s/SKILL.md): %s\n", s.Name, skillsRaw, s.Dir, teaser))
	}
	b.WriteString("\n## MCP Tools\n\n")
	b.WriteString("The plugin bundles a Go MCP server that wraps the Simulator REST API:\n\n")
	for _, t := range mcpTools {
		b.WriteString(fmt.Sprintf("- **%s**: %s\n", t[0], t[1]))
	}
	b.WriteString("\n## Documentation\n\n")
	docs := [][2]string{
		{"Actors", "/entities/actors.md): Actor model, fields, status lifecycle"},
		{"Forms", "/entities/forms.md): Form templates and field definitions"},
		{"Links", "/entities/links.md): Link model and directionality"},
		{"Layers", "/entities/layers.md): Layer types and canvas operations"},
		{"Accounts", "/entities/accounts.md): Financial account types"},
		{"Transactions", "/entities/transactions.md): Transaction lifecycle"},
		{"Graph Management", "/user-flows/actor-graph-management.md): End-to-end graph build and sync workflow"},
	}
	for _, d := range docs {
		b.WriteString(fmt.Sprintf("- [%s](%s%s\n", d[0], docsRaw, d[1]))
	}
	b.WriteString("\n## Optional\n\n")
	b.WriteString(fmt.Sprintf("- [Skills Index](%s/public/.well-known/skills/index.json): Machine-readable agent discovery index\n", repoRaw))
	b.WriteString(fmt.Sprintf("- [Changelog](%s/CHANGELOG.md): Release history\n", repoRaw))
	return b.String()
}

func main() {
	root := flag.String("root", "../../..", "repo root, relative to the mcp-server module dir")
	flag.Parse()

	skillsDir := filepath.Join(*root, "plugins", "simulator", "skills")
	publicDir := filepath.Join(*root, "public")

	skills, err := collectSkills(skillsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: skills dir not found: %s (%v)\n", skillsDir, err)
		os.Exit(1)
	}
	if len(skills) == 0 {
		fmt.Fprintln(os.Stderr, "ERROR: no skills found")
		os.Exit(1)
	}
	names := make([]string, len(skills))
	for i, s := range skills {
		names[i] = s.Name
	}
	fmt.Printf("Found %d skills: %v\n", len(skills), names)

	skillsOut := filepath.Join(publicDir, ".well-known", "skills")
	if err := os.MkdirAll(skillsOut, 0o750); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	idx, _ := json.MarshalIndent(indexJSON(skills), "", "  ")
	idx = append(idx, '\n')
	indexPath := filepath.Join(skillsOut, "index.json")
	if err := os.WriteFile(indexPath, idx, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("Written: %s\n", indexPath)

	llmsPath := filepath.Join(publicDir, "llms.txt")
	if err := os.WriteFile(llmsPath, []byte(llmsTxt(skills)), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("Written: %s\n", llmsPath)
}
