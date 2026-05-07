#!/usr/bin/env bash
# Start the Simulator.Company MCP server via go run (no build step required).
# Optional env vars:
#   SIMULATOR_TOKEN    - Bearer token for API authentication (if set, skips OAuth login)
#   SIMULATOR_URL      - Override base API URL (defaults to spec value)
#   CLAUDE_PLUGIN_ROOT - Set automatically by Claude Code

set -e

# Resolve plugin root from the script location (works regardless of working directory)
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PLUGIN_ROOT="${CLAUDE_PLUGIN_ROOT:-$(cd "$SCRIPT_DIR/.." && pwd)}"

EXTRA_ARGS=()

if [ -n "$SIMULATOR_TOKEN" ]; then
  EXTRA_ARGS+=(--authorization "Simulator $SIMULATOR_TOKEN")
else
  CREDS_FILE="$HOME/.simulator/credentials.json"
  if [ ! -f "$CREDS_FILE" ]; then
    echo "[simulator-mcp] NOTICE: No SIMULATOR_TOKEN set and no saved credentials found." >&2
    echo "[simulator-mcp] Run the 'login' tool to authenticate via OAuth2." >&2
  fi
fi

if [ -n "$SIMULATOR_URL" ]; then
  EXTRA_ARGS+=(--url "$SIMULATOR_URL")
fi

# cd into the plugin root so that 'go run .' resolves the module correctly
cd "$PLUGIN_ROOT"
exec go run . --spec simulator "${EXTRA_ARGS[@]}"
