package main

import (
	_ "embed"
	"fmt"
)

//go:embed swagger/sim-public-swagger.json
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
