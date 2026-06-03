// Command enrichspec enriches the live Simulator public API OpenAPI spec with
// operationId + summary so the MCP server can derive good tool names.
//
// The live spec served by Fastify (https://mw.simulator.company/api/1.0/doc/json)
// describes the full /papi/1.0 surface but ships with no operationId and no
// summary on any operation. The MCP server derives each tool name from
// operationId (falling back to an ugly slug) and relies on a set of canonical
// operationIds for special-case logic in server.go (createActor, getForm, ...).
//
// This tool reuses operationId/summary/description from existing hand-curated
// specs wherever a (method, path) matches (param-name-insensitive fallback),
// generates a deterministic camelCase operationId for everything else, and
// guarantees the canonical operationIds survive (it fails otherwise). The live
// servers[].url + /papi/1.0 path prefix are preserved, so the URL the MCP
// server builds (servers[0].url + path) targets the right endpoint.
//
// Usage:
//
//	go run ./cmd/enrichspec --input <url|file> --reuse a.json,b.json --output out.json --report
//	go run ./cmd/enrichspec --input <url|file> --check committed.json   # CI drift gate
package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	liveURL = "https://mw.simulator.company/api/1.0/doc/json"
	prefix  = "/papi/1.0"
)

var (
	httpMethods = map[string]bool{"get": true, "post": true, "put": true, "delete": true, "patch": true}
	verb        = map[string]string{"get": "get", "post": "create", "put": "update", "delete": "delete", "patch": "update"}
	verbWord    = map[string]string{"get": "Get", "post": "Create", "put": "Update", "delete": "Delete", "patch": "Update"}
	// operationIds the MCP server special-cases (server.go: operationToolName(op)== / toolName==).
	// These MUST survive enrichment or tool-specific injection logic silently breaks.
	canonical  = []string{"createActor", "getActor", "getActorLinks", "getEdgeTypes", "getForm", "getLayer", "manageLayer", "createLink", "massLink"}
	paramRe    = regexp.MustCompile(`\{[^}]+\}`)
	segSplitRe = regexp.MustCompile(`[_\-]`)
)

type meta struct {
	operationID string
	summary     string
	description interface{}
	tags        interface{}
}

func load(src string) (map[string]interface{}, error) {
	var data []byte
	var err error
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		client := &http.Client{
			Timeout:   30 * time.Second,
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, //nolint:gosec // public doc endpoint; self-signed gateways tolerated
		}
		req, _ := http.NewRequest("GET", src, nil)
		req.Header.Set("Accept", "application/json")
		resp, e := client.Do(req)
		if e != nil {
			return nil, e
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("GET %s: HTTP %d", src, resp.StatusCode)
		}
		data, err = io.ReadAll(resp.Body)
	} else {
		data, err = os.ReadFile(src)
	}
	if err != nil {
		return nil, err
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", src, err)
	}
	return doc, nil
}

func isMethod(k string) bool { return httpMethods[strings.ToLower(k)] }

func normPath(path string) string { return paramRe.ReplaceAllString(path, "{}") }

func normSeg(seg string) string {
	var b strings.Builder
	for _, p := range segSplitRe.Split(seg, -1) {
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]) + p[1:])
	}
	return b.String()
}

func nonParamSegs(path string) []string {
	body := strings.TrimPrefix(path, prefix)
	var segs []string
	for _, s := range strings.Split(body, "/") {
		if s == "" || (strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}")) {
			continue
		}
		segs = append(segs, s)
	}
	return segs
}

func genOpID(method, path string) string {
	var resource strings.Builder
	for _, s := range nonParamSegs(path) {
		resource.WriteString(normSeg(s))
	}
	v := verb[strings.ToLower(method)]
	if v == "" {
		v = strings.ToLower(method)
	}
	return v + resource.String()
}

func paramsOf(path string) []string {
	var out []string
	for _, m := range paramRe.FindAllString(path, -1) {
		out = append(out, m[1:len(m)-1])
	}
	return out
}

func humanize(method, path string) string {
	var parts []string
	for _, s := range nonParamSegs(path) {
		parts = append(parts, strings.ReplaceAll(strings.ReplaceAll(s, "_", " "), "-", " "))
	}
	phrase := strings.Join(parts, " ")
	suffix := ""
	if ps := paramsOf(path); len(ps) > 0 {
		suffix = " by " + strings.Join(ps, ", ")
	}
	vw := verbWord[strings.ToLower(method)]
	if vw == "" {
		vw = strings.ToUpper(method)
	}
	return strings.TrimSpace(fmt.Sprintf("%s %s%s", vw, phrase, suffix))
}

// buildReuse fills exact (method,prefixed-path) and norm (param-insensitive)
// lookup maps from a curated spec. Curated specs drop the /papi/1.0 prefix.
func buildReuse(spec map[string]interface{}, exact map[string]meta, norm map[string]*meta) {
	paths, _ := spec["paths"].(map[string]interface{})
	for path, mv := range paths {
		keyPath := path
		if !strings.HasPrefix(keyPath, prefix) {
			keyPath = prefix + keyPath
		}
		ops, _ := mv.(map[string]interface{})
		for method, ov := range ops {
			if !isMethod(method) {
				continue
			}
			op, _ := ov.(map[string]interface{})
			oid, _ := op["operationId"].(string)
			if oid == "" {
				continue
			}
			summary, _ := op["summary"].(string)
			m := meta{operationID: oid, summary: summary, description: op["description"], tags: op["tags"]}
			exact[strings.ToLower(method)+"\x00"+keyPath] = m
			nk := strings.ToLower(method) + "\x00" + normPath(keyPath)
			if existing, ok := norm[nk]; ok {
				if existing != nil && existing.operationID != m.operationID {
					norm[nk] = nil // ambiguous: two curated ops collide on the fuzzy key
				}
			} else {
				mc := m
				norm[nk] = &mc
			}
		}
	}
}

func opKeys(spec map[string]interface{}) map[string]bool {
	keys := map[string]bool{}
	paths, _ := spec["paths"].(map[string]interface{})
	for path, mv := range paths {
		kp := path
		if !strings.HasPrefix(kp, prefix) {
			kp = prefix + kp
		}
		ops, _ := mv.(map[string]interface{})
		for method := range ops {
			if isMethod(method) {
				keys[strings.ToLower(method)+"\x00"+normPath(kp)] = true
			}
		}
	}
	return keys
}

func writeJSON(path string, doc map[string]interface{}) error {
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	if path == "" {
		_, err = os.Stdout.Write(out)
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

func main() {
	input := flag.String("input", liveURL, "live spec URL or file path")
	reuse := flag.String("reuse", "", "comma-separated curated specs to reuse operationId/summary from (later wins)")
	output := flag.String("output", "", "write enriched spec to this file (default stdout)")
	exclude := flag.String("exclude", "", "comma-separated regexes of paths to drop")
	report := flag.Bool("report", false, "print enrichment stats to stderr")
	check := flag.String("check", "", "drift gate: fail if live spec has ops missing from this committed spec")
	flag.Parse()

	spec, err := load(*input)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if *check != "" {
		committed, err := load(*check)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		live := opKeys(spec)
		have := opKeys(committed)
		var missing []string
		for k := range live {
			if !have[k] {
				parts := strings.SplitN(k, "\x00", 2)
				missing = append(missing, strings.ToUpper(parts[0])+" "+parts[1])
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			fmt.Fprintln(os.Stderr, "ERROR: live spec has operations missing from the embedded spec (run `make enrich-spec`):")
			for _, m := range missing {
				fmt.Fprintln(os.Stderr, "  "+m)
			}
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "OK: embedded spec covers all %d live operations.\n", len(live))
		return
	}

	exactReuse := map[string]meta{}
	normReuse := map[string]*meta{}
	if *reuse != "" {
		for _, ref := range strings.Split(*reuse, ",") {
			ref = strings.TrimSpace(ref)
			if ref == "" {
				continue
			}
			rspec, err := load(ref)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}
			buildReuse(rspec, exactReuse, normReuse)
		}
	}

	var excl []*regexp.Regexp
	if *exclude != "" {
		for _, x := range strings.Split(*exclude, ",") {
			if x = strings.TrimSpace(x); x != "" {
				excl = append(excl, regexp.MustCompile(x))
			}
		}
	}

	canonSet := map[string]bool{}
	for _, c := range canonical {
		canonSet[c] = true
	}
	seen := map[string]string{} // operationId -> method\x00path
	canonSeen := map[string]bool{}
	stats := map[string]int{}

	paths, _ := spec["paths"].(map[string]interface{})
	for path, mv := range paths {
		dropped := false
		for _, rx := range excl {
			if rx.MatchString(path) {
				dropped = true
				break
			}
		}
		ops, _ := mv.(map[string]interface{})
		for method, ov := range ops {
			if !isMethod(method) {
				continue
			}
			if dropped {
				stats["excluded"]++
				delete(ops, method)
				continue
			}
			op, _ := ov.(map[string]interface{})
			stats["total"]++
			mkey := strings.ToLower(method) + "\x00" + path
			var m *meta
			if e, ok := exactReuse[mkey]; ok {
				m = &e
				stats["reused_exact"]++
			} else if n := normReuse[strings.ToLower(method)+"\x00"+normPath(path)]; n != nil {
				m = n
				stats["reused_fuzzy"]++
			}
			var oid string
			if m != nil {
				oid = m.operationID
				if _, ok := op["summary"]; (!ok || op["summary"] == "") && m.summary != "" {
					op["summary"] = m.summary
				}
				if _, ok := op["description"]; !ok && m.description != nil {
					op["description"] = m.description
				}
				if _, ok := op["tags"]; !ok && m.tags != nil {
					op["tags"] = m.tags
				}
			} else {
				oid = genOpID(method, path)
				stats["generated"]++
			}

			base := oid
			attempt := 1
			for {
				prev, exists := seen[oid]
				if !exists || prev == mkey {
					break
				}
				if ps := paramsOf(path); len(ps) > 0 && attempt == 1 {
					oid = base + "By" + normSeg(ps[len(ps)-1])
				} else {
					attempt++
					oid = fmt.Sprintf("%s%d", base, attempt)
				}
			}
			if oid != base {
				stats["renamed_on_collision"]++
			}
			seen[oid] = mkey
			op["operationId"] = oid
			if s, _ := op["summary"].(string); s == "" {
				op["summary"] = humanize(method, path)
			}
			if canonSet[oid] {
				canonSeen[oid] = true
			}
		}
	}

	var missing []string
	for _, c := range canonical {
		if !canonSeen[c] {
			missing = append(missing, c)
		}
	}
	if *report {
		sort.Strings(missing)
		present := make([]string, 0, len(canonSeen))
		for k := range canonSeen {
			present = append(present, k)
		}
		sort.Strings(present)
		rep, _ := json.MarshalIndent(map[string]interface{}{
			"total": stats["total"], "reused_exact": stats["reused_exact"], "reused_fuzzy": stats["reused_fuzzy"],
			"generated": stats["generated"], "excluded": stats["excluded"], "renamed_on_collision": stats["renamed_on_collision"],
			"canonical_present": present, "canonical_missing": missing,
		}, "", "  ")
		fmt.Fprintln(os.Stderr, string(rep))
	}
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "ERROR: canonical operationIds missing after enrichment: %v\n", missing)
		os.Exit(2)
	}

	if err := writeJSON(*output, spec); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if *report && *output != "" {
		fmt.Fprintf(os.Stderr, "wrote %s\n", *output)
	}
}
