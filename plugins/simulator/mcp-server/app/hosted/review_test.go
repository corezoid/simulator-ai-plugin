package hosted

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/corezoid/simulator-ai-plugin/plugins/simulator/mcp-server/app/mcpserver"
)

// The hosted tool list as a connector review sees it: the real stateless
// server, wrapped by NewHandler.
func hostedTools(t *testing.T) map[string]struct {
	desc   string
	schema string
	ro, de bool
} {
	t.Helper()
	s, _, err := mcpserver.New(mcpserver.Options{Profile: "prod", Stateless: true})
	if err != nil {
		t.Fatal(err)
	}
	NewHandler(s, prodLike)
	out := map[string]struct {
		desc   string
		schema string
		ro, de bool
	}{}
	for name, st := range s.ListTools() {
		a := st.Tool.Annotations
		if a.ReadOnlyHint == nil || a.DestructiveHint == nil || a.OpenWorldHint == nil {
			t.Errorf("%s: hints unset", name)
			continue
		}
		schema, _ := json.Marshal(st.Tool.InputSchema)
		out[name] = struct {
			desc   string
			schema string
			ro, de bool
		}{st.Tool.Description, string(schema), *a.ReadOnlyHint, *a.DestructiveHint}
	}
	if len(out) < 100 {
		t.Fatalf("only %d hosted tools", len(out))
	}
	return out
}

// Reads say they are reads, deletes say they are destructive.
func TestHostedToolHintsMatchBehaviour(t *testing.T) {
	read := regexp.MustCompile(`^(get|search|list|filter|exist|find)[A-Z]|^(layerStats|buildLink|diffReleases|readAttachment|simulationCheck|simulationRun)$`)
	destructive := regexp.MustCompile(`^(delete|remove|revoke|clean|prune|rollback|update|set|save|bulkSave|move|deploy|import|finalize)`)
	for name, tl := range hostedTools(t) {
		switch {
		case name == "getAgent" || name == "getSystemActor":
			// get-or-creates a twin: not read-only, not destructive
			if tl.ro || tl.de {
				t.Errorf("%s: ro=%v de=%v, want additive", name, tl.ro, tl.de)
			}
		case read.MatchString(name):
			if !tl.ro || tl.de {
				t.Errorf("%s: ro=%v de=%v, want read-only", name, tl.ro, tl.de)
			}
		case destructive.MatchString(name):
			if tl.ro || !tl.de {
				t.Errorf("%s: ro=%v de=%v, want destructive", name, tl.ro, tl.de)
			}
		}
	}
}

// Descriptions and schemas must not offer files, login or workspace switching:
// none exists on the hosted server.
func TestHostedToolsOfferNoLocalFeatures(t *testing.T) {
	for _, banned := range []string{"createSmartForm", "updateSmartFormEnv", "appGetPage", "appSendForm"} {
		if _, ok := hostedTools(t)[banned]; ok {
			t.Errorf("%s must not be hosted", banned)
		}
	}
	local := []string{"apiSecret", "localPath", "modelPath", "scenariosPath", "graphPath", "working directory", "set-workspace", "local file", "Call after login"}
	for name, tl := range hostedTools(t) {
		for _, l := range local {
			if strings.Contains(tl.desc, l) || strings.Contains(tl.schema, l) {
				t.Errorf("%s offers %q", name, l)
			}
		}
	}
}
