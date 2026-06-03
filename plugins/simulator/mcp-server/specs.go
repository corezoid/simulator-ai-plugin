package main

import (
	_ "embed"
	"fmt"
)

// sim-public-swagger.full.json is the full /papi/1.0 surface (185 ops) generated
// from the live spec (https://mw.simulator.company/api/1.0/doc/json) by
// scripts/enrich-spec.py, which back-fills the operationId/summary the live spec
// lacks. The hand-curated sim-public-swagger.json (48) / -all.json (80) are kept
// as the reuse source for that script (they carry the canonical operationIds the
// server special-cases depend on). Regenerate via `make enrich-spec`.
//
//go:embed swagger/sim-public-swagger.full.json
var simPublicSwagger []byte

var builtinSpecs = map[string][]byte{
	"simulator": simPublicSwagger,
}

func getBuiltinSpec(name string) ([]byte, error) {
	data, ok := builtinSpecs[name]
	if !ok {
		names := make([]string, 0, len(builtinSpecs))
		for k := range builtinSpecs {
			names = append(names, k)
		}
		return nil, fmt.Errorf("unknown built-in spec %q, available: %v", name, names)
	}
	return data, nil
}
