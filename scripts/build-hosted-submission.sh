#!/usr/bin/env bash
# Build the OpenAI plugin-directory submission ZIP for the HOSTED Simulator MCP
# server (https://mcp.simulator.company/mcp). The repository's own .mcp.json
# keeps the local (stdio) server for existing installs; this script only
# produces a separate artifact:
#   - .mcp.json points at the hosted URL (Codex format: {"url": ...});
#   - the Go server sources, install scripts and Kiro files are left out;
#   - skills built on local workflows (environment/login setup, smart-form file
#     sync, app generation into local files) are left out;
#   - the main skill gains a "Hosted connector" section.
# Usage: scripts/build-hosted-submission.sh [output-dir]   (default: dist/)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="$ROOT/plugins/simulator"
OUT="${1:-$ROOT/dist}"
URL="https://mcp.simulator.company/mcp"
EXCLUDE_SKILLS=(simulator-init simulator-smart-forms simulator-app-generator)

VERSION="$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["version"])' "$SRC/.codex-plugin/plugin.json")"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
PKG="$WORK/simulator"
mkdir -p "$PKG" "$OUT"

cp -R "$SRC/.codex-plugin" "$SRC/assets" "$SRC/docs" "$SRC/skills" "$PKG/"
for s in "${EXCLUDE_SKILLS[@]}"; do rm -rf "$PKG/skills/$s"; done

cat > "$PKG/.mcp.json" <<EOF
{
  "mcpServers": {
    "simulator": {
      "url": "$URL"
    }
  }
}
EOF

python3 - "$PKG/skills/simulator/SKILL.md" <<'PY'
import sys
p = sys.argv[1]
s = open(p).read()
note = """
## Hosted connector (read this first)

This plugin talks to the hosted Simulator MCP server. Differences from the local plugin:

- **No login, set-workspace or set-environment.** The connection itself is authenticated (OAuth, account.corezoid.com). Pick the workspace with `getWorkspaces` and pass its id as `accId` where a tool asks for it.
- **No local files.** `pullGraphFile` / `pushGraphFile`, `pullSmartForm` / `pushSmartForm` and `simulationSnapshot` are not available. Read layers with `getAllLayerPlacements` / `getLayerActors`, and change them with `createActor`, `createLink`, `manageLayerActors` and the other API tools. Simulations take the model as YAML text plus a live `layerId`.
- **Images** for `uploadActorPicture` come from a public `https://` URL or `base64`; `localPath` is not available.
- Ignore any step below that tells you to write or read a file, a `.env`, or a `<layerId>.yaml`.
"""
if s.startswith("---"):
    end = s.index("\n---", 3) + 4
    s = s[:end] + "\n" + note + s[end:]
else:
    s = note + s
open(p, "w").write(s)
PY

ZIP="$OUT/simulator-plugin-hosted-v$VERSION.zip"
rm -f "$ZIP"
(cd "$PKG" && zip -qr "$ZIP" . -x '.DS_Store')
echo "$ZIP"
