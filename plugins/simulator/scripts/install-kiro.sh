#!/bin/sh
# install-kiro.sh — set up the simulator plugin inside an AWS Kiro workspace.
#
# Usage:
#   plugins/simulator/scripts/install-kiro.sh [workspace-dir]
#
# If workspace-dir is omitted, defaults to $KIRO_WORKSPACE_DIR or $PWD.
# Creates the following under <workspace>/.kiro/:
#   settings/mcp.json        ← copy of .mcp.kiro.json
#   steering/<name>.md       ← symlinked from this plugin's steering/
#   skills/<name>/SKILL.md   ← symlinked from this plugin's skills/<name>/
#
# Exports KIRO_PLUGIN_ROOT (printed for the user to add to their shell rc if
# they want it persistent — the MCP wrapper resolves it on its own).
#
# Windows: symlinks are not available; falls back to hard-copy.

set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PLUGIN_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
WORKSPACE="${1:-${KIRO_WORKSPACE_DIR:-$PWD}}"
KIRO_DIR="$WORKSPACE/.kiro"

if [ ! -d "$WORKSPACE" ]; then
  echo "ERROR: workspace dir not found: $WORKSPACE" >&2
  exit 1
fi

mkdir -p "$KIRO_DIR/settings" "$KIRO_DIR/steering" "$KIRO_DIR/skills"

# Copy the MCP entry (never symlink — workspace-local edits must not leak
# back into the plugin source).
cp "$PLUGIN_ROOT/.mcp.kiro.json" "$KIRO_DIR/settings/mcp.json"

# Pick the right linker: symlink on POSIX, hard-copy on Windows shells.
case "$(uname -s 2>/dev/null || echo Unknown)" in
  MINGW*|CYGWIN*|MSYS*) LINK_CMD="cp -R" ;;
  *)                    LINK_CMD="ln -sfn" ;;
esac

# Link steering files.
for f in "$PLUGIN_ROOT"/steering/*.md; do
  [ -f "$f" ] || continue
  $LINK_CMD "$f" "$KIRO_DIR/steering/$(basename "$f")"
done

# Link each skill directory under .kiro/skills/<name>/. Refresh any stale
# entry so re-running this script is idempotent.
for d in "$PLUGIN_ROOT"/skills/*/; do
  [ -d "$d" ] || continue
  name="$(basename "$d")"
  rm -rf "$KIRO_DIR/skills/$name"
  $LINK_CMD "$d" "$KIRO_DIR/skills/$name"
done

echo "Installed simulator plugin into: $KIRO_DIR"
echo "Open this workspace in Kiro and the simulator MCP server, skills, and steering will be picked up."
echo
echo "If your shell does not already set KIRO_PLUGIN_ROOT, add:"
echo "  export KIRO_PLUGIN_ROOT=\"$PLUGIN_ROOT\""
