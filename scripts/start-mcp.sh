#!/usr/bin/env bash
# Start the Simulator.Company MCP server via go run (no build step required).
# Required env vars:
#   SIMULATOR_TOKEN    - Bearer token for API authentication
# Optional env vars:
#   SIMULATOR_URL      - Override base API URL (defaults to spec value)
#   CLAUDE_PLUGIN_ROOT - Set automatically by Claude Code

set -e

# Resolve plugin root from the script location (works regardless of working directory)
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PLUGIN_ROOT="${CLAUDE_PLUGIN_ROOT:-$(cd "$SCRIPT_DIR/.." && pwd)}"

if [ -z "$SIMULATOR_TOKEN" ]; then
  echo "[simulator-mcp] ERROR: SIMULATOR_TOKEN is not set." >&2
  echo "[simulator-mcp] Add to your shell profile: export SIMULATOR_TOKEN=your_token" >&2
  exit 1
fi

AUTH="Simulator $SIMULATOR_TOKEN"
EXTRA_ARGS=""
if [ -n "$SIMULATOR_URL" ]; then
  EXTRA_ARGS="--url $SIMULATOR_URL"
fi

# cd into the plugin root so that 'go run .' resolves the module correctly
cd "$PLUGIN_ROOT"
exec go run . --spec simulator --authorization "$AUTH" $EXTRA_ARGS
