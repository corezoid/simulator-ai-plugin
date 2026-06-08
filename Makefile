MCP := plugins/simulator/mcp-server

.PHONY: help discovery build vet lint test run-local run-prod eval eval-skills eval-live

help:
	@echo "Targets:"
	@echo "  build/vet/test  Go build / vet / test the MCP server."
	@echo "  lint            golangci-lint the MCP server (config: $(MCP)/.golangci.yml)."
	@echo "  discovery       Regenerate public/.well-known/skills/index.json and public/llms.txt."
	@echo "  run-local       Run the MCP server against a local pong-server (:9000)."
	@echo "  run-prod        Run the MCP server against the public gateway."
	@echo "  eval            Behavioural eval, dry — canned fixtures, no backend (needs ANTHROPIC_API_KEY; skips otherwise)."
	@echo "  eval-skills     Behavioural eval, dry, with the SKILL.md files injected as the system prompt."
	@echo "  eval-live       Behavioural eval executing tools against the backend (throwaway workspace)."

# Regenerate AI-discovery artifacts (public/) from the plugin SKILL.md files.
discovery:
	cd $(MCP) && go run ./cmd/gendiscovery --root ../../..

build:
	cd $(MCP) && go build ./...

vet:
	cd $(MCP) && go vet ./...

# Requires golangci-lint v2. Install:
#   curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $$(go env GOPATH)/bin v2.11.4
lint:
	cd $(MCP) && golangci-lint run ./...

test:
	cd $(MCP) && go test ./...

run-local:
	cd $(MCP) && go run ./cmd/server --profile local

run-prod:
	cd $(MCP) && go run ./cmd/server --profile prod

# Behavioural eval: drive a model through eval-scenarios.json and check it calls
# the expected tools (and, via argChecks, with the expected argument shapes).
# No-op without ANTHROPIC_API_KEY.
#   eval         dry — canned fixtures for read tools, no backend (skips live-only scenarios)
#   eval-skills  dry, plus the plugin SKILL.md files injected as the system prompt
#   eval-live    executes tool calls against the backend (login + THROWAWAY workspace first)
eval:
	cd $(MCP) && go run ./cmd/evalrunner

eval-skills:
	cd $(MCP) && go run ./cmd/evalrunner --skills

eval-live:
	cd $(MCP) && go run ./cmd/evalrunner --execute
