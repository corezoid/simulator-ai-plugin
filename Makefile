MCP := plugins/simulator/mcp-server

.PHONY: help discovery build vet test run-local run-prod

help:
	@echo "Targets:"
	@echo "  build/vet/test  Go build / vet / test the MCP server."
	@echo "  discovery       Regenerate public/.well-known/skills/index.json and public/llms.txt."
	@echo "  run-local       Run the MCP server against a local pong-server (:9000)."
	@echo "  run-prod        Run the MCP server against the public gateway."
	@echo "  eval            Behavioural eval (needs ANTHROPIC_API_KEY; skips otherwise)."

# Regenerate AI-discovery artifacts (public/) from the plugin SKILL.md files.
discovery:
	cd $(MCP) && go run ./cmd/gendiscovery --root ../../..

build:
	cd $(MCP) && go build ./...

vet:
	cd $(MCP) && go vet ./...

test:
	cd $(MCP) && go test ./...

run-local:
	cd $(MCP) && go run ./cmd/server --profile local

run-prod:
	cd $(MCP) && go run ./cmd/server --profile prod

# Behavioural eval: drive a model through eval-scenarios.json and check it calls
# the expected tools. No-op without ANTHROPIC_API_KEY.
eval:
	cd $(MCP) && go run ./cmd/evalrunner
