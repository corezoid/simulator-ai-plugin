MCP   := plugins/simulator/mcp-server
LIVE  := https://mw.simulator.company/api/1.0/doc/json
# Paths below are relative to the mcp-server module dir (recipes cd into $(MCP)).
REUSE := swagger/sim-public-swagger-all.json,swagger/sim-public-swagger.json
FULL  := swagger/sim-public-swagger.full.json

.PHONY: help enrich-spec check-spec discovery build vet test

help:
	@echo "Targets:"
	@echo "  enrich-spec  Regenerate the embedded full spec ($(MCP)/$(FULL)) from the live API doc."
	@echo "  check-spec   Drift gate: fail if the live API has ops missing from the embedded spec."
	@echo "  discovery    Regenerate public/.well-known/skills/index.json and public/llms.txt."
	@echo "  build/vet/test  Go build / vet / test the MCP server."

# Pull the live OpenAPI doc, back-fill operationId/summary (reusing curated specs),
# and write the embedded full spec consumed by specs.go.
enrich-spec:
	cd $(MCP) && go run ./cmd/enrichspec --input "$(LIVE)" --reuse "$(REUSE)" --output "$(FULL)" --report

# CI gate: fail if upstream added /papi ops not present in the committed full spec.
check-spec:
	cd $(MCP) && go run ./cmd/enrichspec --input "$(LIVE)" --check "$(FULL)"

# Regenerate AI-discovery artifacts (public/) from the plugin SKILL.md files.
discovery:
	cd $(MCP) && go run ./cmd/gendiscovery --root ../../..

build:
	cd $(MCP) && go build ./...

vet:
	cd $(MCP) && go vet ./...

test:
	cd $(MCP) && go test ./...
