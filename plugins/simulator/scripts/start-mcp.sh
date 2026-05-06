#!/usr/bin/env bash
# Codex wrapper: re-roots CLAUDE_PLUGIN_ROOT to the repo root, then delegates
# to the real MCP start script where the Go module lives.
#
# When Codex installs this plugin from plugins/simulator/, it sets
# CLAUDE_PLUGIN_ROOT to that subdirectory. The actual Go module lives at the
# repo root (three levels up), so we redirect before exec.
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# plugins/simulator/scripts/ → ../../.. → repo root
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

export CLAUDE_PLUGIN_ROOT="$REPO_ROOT"
exec "$REPO_ROOT/scripts/start-mcp.sh"
