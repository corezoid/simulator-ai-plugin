package sim

import (
	"testing"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/internal/engines/ecore"
)

func TestHostedRunSlots(t *testing.T) {
	prev := ecore.IsStateless()
	t.Cleanup(func() { ecore.SetStateless(prev); configureRunSlots() })

	ecore.SetStateless(true)
	t.Setenv("SIMULATOR_MAX_CONCURRENT_SIMULATIONS", "")
	configureRunSlots()
	var releases []func()
	for i := 0; i < defaultHostedRunSlots; i++ {
		rel, ok := acquireRunSlot()
		if !ok {
			t.Fatalf("slot %d refused", i)
		}
		releases = append(releases, rel)
	}
	if _, ok := acquireRunSlot(); ok {
		t.Fatal("run beyond the hosted cap was accepted")
	}
	releases[0]()
	if _, ok := acquireRunSlot(); !ok {
		t.Fatal("freed slot not reusable")
	}

	ecore.SetStateless(false)
	configureRunSlots()
	for i := 0; i < 10; i++ {
		if _, ok := acquireRunSlot(); !ok {
			t.Fatal("local server must not cap simulations")
		}
	}
}
