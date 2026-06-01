#!/usr/bin/env python3
"""
Generate public/.well-known/skills/index.json and public/llms.txt
from plugin SKILL.md files.

Usage:
    python3 scripts/generate-discovery.py
"""

import json
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SKILLS_DIR = os.path.join(ROOT, "plugins", "simulator", "skills")
PUBLIC_DIR = os.path.join(ROOT, "public")
REPO_RAW = "https://raw.githubusercontent.com/corezoid/simulator-ai-plugin/main"
SKILLS_RAW = f"{REPO_RAW}/plugins/simulator/skills"
DOCS_RAW = f"{REPO_RAW}/plugins/simulator/docs"


# ---------------------------------------------------------------------------
# Frontmatter parsing (no external deps)
# ---------------------------------------------------------------------------

def _parse_description(fm):
    # Folded/literal scalar  (description: >\n  line1\n  line2)
    folded = re.search(r"^description:\s*[>|]\s*\n((?:[ \t]+[^\n]*\n?)+)", fm, re.MULTILINE)
    if folded:
        lines = folded.group(1).splitlines()
        return " ".join(ln.strip() for ln in lines if ln.strip())

    # Double-quoted inline
    dq = re.search(r'^description:\s*"(.*)"', fm, re.MULTILINE)
    if dq:
        return dq.group(1).replace('\\"', '"').strip()

    # Single-quoted inline
    sq = re.search(r"^description:\s*'(.*)'", fm, re.MULTILINE)
    if sq:
        return sq.group(1).replace("''", "'").strip()

    # Plain inline
    plain = re.search(r"^description:\s*(.+)$", fm, re.MULTILINE)
    if plain:
        return plain.group(1).strip()

    return None


def parse_frontmatter(path):
    with open(path, encoding="utf-8") as f:
        content = f.read()

    m = re.match(r"^---\n(.*?)\n---", content, re.DOTALL)
    if not m:
        return None
    fm = m.group(1)

    name_m = re.search(r"^name:\s*(.+)$", fm, re.MULTILINE)
    name = name_m.group(1).strip() if name_m else None

    return {"name": name, "description": _parse_description(fm)}


# ---------------------------------------------------------------------------
# Skills discovery
# ---------------------------------------------------------------------------

def collect_skills():
    skills = []
    for entry in sorted(os.listdir(SKILLS_DIR)):
        skill_path = os.path.join(SKILLS_DIR, entry)
        skill_md = os.path.join(skill_path, "SKILL.md")
        if not os.path.isfile(skill_md):
            continue

        fm = parse_frontmatter(skill_md)
        if not fm or not fm["name"] or not fm["description"]:
            print(f"WARN: skipping {entry} — missing name or description", file=sys.stderr)
            continue

        # List all .md files in the skill directory
        md_files = []
        for root, dirs, files in os.walk(skill_path):
            dirs.sort()
            for fname in sorted(files):
                if fname.endswith(".md"):
                    rel = os.path.relpath(os.path.join(root, fname), skill_path)
                    md_files.append(rel)

        skills.append({
            "name": fm["name"],
            "description": fm["description"],
            "dir": entry,
            "files": md_files,
        })
    return skills


# ---------------------------------------------------------------------------
# Generators
# ---------------------------------------------------------------------------

def generate_index_json(skills):
    return {
        "skills": [
            {
                "name": s["name"],
                "description": s["description"],
                "files": [f"{SKILLS_RAW}/{s['dir']}/{f}" for f in s["files"]],
            }
            for s in skills
        ]
    }


MCP_TOOLS = [
    # Auth & workspace
    ("login",                   "Authenticate with Simulator.Company via OAuth2 browser flow"),
    ("set-workspace",           "Save workspace ID to .env for subsequent API calls"),
    # Graph files
    ("pullGraphFile",           "Export a layer to a local YAML file for editing"),
    ("pushGraphFile",           "Sync a local YAML graph file back to a Simulator layer"),
    # Actors
    ("createActor",             "Create a single actor (node) in a layer"),
    ("createActors",            "Bulk-create up to 50 actors in a single call"),
    ("updateActor",             "Update actor properties (title, description, status)"),
    ("deleteActor",             "Delete an actor from a layer"),
    ("getActor",                "Get actor details by ID"),
    ("searchActors",            "Search actors by title or custom fields"),
    # Links
    ("createLink",              "Create a link (edge) between two actors"),
    ("massLink",                "Create multiple links in one call"),
    ("deleteLink",              "Delete a link between actors"),
    # Layers
    ("manageLayer",             "Create, update, or delete a layer"),
    ("moveElements",            "Move actors on a layer canvas"),
    ("compactGraphLayout",      "Auto-layout actors with domain-clustering strategy"),
    ("pruneLongEdges",          "Delete links that exceed a Manhattan distance threshold"),
    ("getAllLayerPlacements",   "Enumerate all actors on a layer in one call"),
    # Forms
    ("createForm",              "Create a new form template"),
    ("updateForm",              "Update an existing form template"),
    ("getForms",                "List all available form templates"),
    # Finance
    ("createAccountName",       "Define an account name (asset, liability, expense, income)"),
    ("createCurrency",          "Create a currency for financial tracking"),
    ("addFormAccount",          "Add an account to a form template"),
    # Pictures
    ("uploadActorPicture",      "Set an actor's avatar from URL, local file, or base64 (auto SVG→PNG)"),
    ("uploadActorPictureBulk",  "Bulk-upload actor pictures with SHA-256 deduplication"),
    # Charts
    ("createChart",             "Create a chart/dashboard actor on a layer"),
]


def generate_llms_txt(skills):
    lines = [
        "# Simulator.Company AI Plugin",
        "",
        "> Official Claude Code plugin for Simulator.Company platform. "
        "Provides skills and MCP tools for managing actors, graph-based business "
        "processes, form templates, financial accounts, and visualisations "
        "directly from the IDE.",
        "",
        "## Skills",
        "",
    ]

    for s in skills:
        url = f"{SKILLS_RAW}/{s['dir']}/SKILL.md"
        teaser = s["description"].split(". ")[0].rstrip(".")
        lines.append(f"- [{s['name']}]({url}): {teaser}")

    lines += [
        "",
        "## MCP Tools",
        "",
        "The plugin bundles a Go MCP server that wraps the Simulator REST API:",
        "",
    ]

    for name, desc in MCP_TOOLS:
        lines.append(f"- **{name}**: {desc}")

    lines += [
        "",
        "## Documentation",
        "",
        f"- [Actors]({DOCS_RAW}/entities/actors.md): Actor model, fields, status lifecycle",
        f"- [Forms]({DOCS_RAW}/entities/forms.md): Form templates and field definitions",
        f"- [Links]({DOCS_RAW}/entities/links.md): Link model and directionality",
        f"- [Layers]({DOCS_RAW}/entities/layers.md): Layer types and canvas operations",
        f"- [Accounts]({DOCS_RAW}/entities/accounts.md): Financial account types",
        f"- [Transactions]({DOCS_RAW}/entities/transactions.md): Transaction lifecycle",
        f"- [Graph Management]({DOCS_RAW}/user-flows/actor-graph-management.md): "
        "End-to-end graph build and sync workflow",
        "",
        "## Optional",
        "",
        f"- [Skills Index]({REPO_RAW}/public/.well-known/skills/index.json): "
        "Machine-readable agent discovery index",
        f"- [Changelog]({REPO_RAW}/CHANGELOG.md): Release history",
        "",
    ]

    return "\n".join(lines)


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def read_version():
    plugin_json = os.path.join(ROOT, "plugins", "simulator", ".claude-plugin", "plugin.json")
    try:
        with open(plugin_json) as f:
            return json.load(f).get("version", "unknown")
    except OSError:
        return "unknown"


def main():
    if not os.path.isdir(SKILLS_DIR):
        print(f"ERROR: skills dir not found: {SKILLS_DIR}", file=sys.stderr)
        sys.exit(1)

    skills = collect_skills()
    if not skills:
        print("ERROR: no skills found", file=sys.stderr)
        sys.exit(1)
    print(f"Found {len(skills)} skills: {[s['name'] for s in skills]}")

    # public/.well-known/skills/index.json
    skills_out_dir = os.path.join(PUBLIC_DIR, ".well-known", "skills")
    os.makedirs(skills_out_dir, exist_ok=True)
    index_path = os.path.join(skills_out_dir, "index.json")
    with open(index_path, "w", encoding="utf-8") as f:
        json.dump(generate_index_json(skills), f, indent=2, ensure_ascii=False)
        f.write("\n")
    print(f"Written: {os.path.relpath(index_path, ROOT)}")

    # public/llms.txt
    os.makedirs(PUBLIC_DIR, exist_ok=True)
    llms_path = os.path.join(PUBLIC_DIR, "llms.txt")
    with open(llms_path, "w", encoding="utf-8") as f:
        f.write(generate_llms_txt(skills))
    print(f"Written: {os.path.relpath(llms_path, ROOT)}")


if __name__ == "__main__":
    main()
