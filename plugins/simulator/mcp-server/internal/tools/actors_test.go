package tools

import "testing"

// TestActorHoleParam guards the `hole` flag exposed on createActor and
// updateActor. The backend declares `hole` (boolean) on the actor
// create/update request body; these tools must surface it so a model can create
// or clear a hole (an empty placeholder node) without falling back to a raw API
// call. See actors.go (holeDesc).
func TestActorHoleParam(t *testing.T) {
	byName := map[string]Operation{}
	for _, op := range allOps() {
		byName[op.Name] = op
	}

	for _, name := range []string{"createActor", "updateActor"} {
		op, ok := byName[name]
		if !ok {
			t.Fatalf("operation %q not found in allOps()", name)
		}

		var hole *Param
		for i := range op.Params {
			if op.Params[i].Name == "hole" {
				hole = &op.Params[i]
				break
			}
		}
		if hole == nil {
			t.Errorf("%s: missing `hole` param (backend accepts `hole` on the body)", name)
			continue
		}
		if hole.In != InBody {
			t.Errorf("%s: hole param In = %v, want InBody (it is a body field)", name, hole.In)
		}
		if hole.Type != "boolean" {
			t.Errorf("%s: hole param Type = %q, want %q", name, hole.Type, "boolean")
		}
	}
}
