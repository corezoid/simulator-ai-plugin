package sim

import (
	"context"
	"fmt"
	"math/big"
	"sort"
)

// Stats of one numeric metric over many runs (spec §9).
type Stats struct {
	N                             int
	Mean, P10, P50, P90, Min, Max *big.Rat
}

type Summary struct {
	Scenario  string
	Runs      int
	Completed int
	Failed    int
	Stopped   int // stopped_by_time / stopped_by_limit: the horizon was not reached
	Metrics   map[string]*Stats
	Goals     map[string][2]int // held, evaluated
	Errors    []string
	Requested int    // runs asked for; Runs is lower when the time budget ran out
	Note      string // why fewer runs were made

	goals  *OMap
	values map[string][]*big.Rat
}

func percentile(sorted []*big.Rat, p int) *big.Rat {
	n := len(sorted)
	k := (p*n+99)/100 - 1 // nearest rank: ceil(p*n/100), 1-based
	if k < 0 {
		k = 0
	}
	if k > n-1 {
		k = n - 1
	}
	return sorted[k]
}

func statsOf(vals []*big.Rat) *Stats {
	v := append([]*big.Rat(nil), vals...)
	sort.Slice(v, func(i, j int) bool { return v[i].Cmp(v[j]) < 0 })
	sum := new(big.Rat)
	for _, x := range v {
		sum.Add(sum, x)
	}
	return &Stats{N: len(v), Mean: new(big.Rat).Quo(sum, ratInt(int64(len(v)))),
		P10: percentile(v, 10), P50: percentile(v, 50), P90: percentile(v, 90), Min: v[0], Max: v[len(v)-1]}
}

// RunMany runs a scenario n times with seeds "<seed>#i" (see record for what is counted).
// A cancelled ctx stops after the current run; the summary covers the runs made.
func RunMany(ctx context.Context, g *Graph, model *Model, sc Scenario, n int, decider Decider, extraGoals *OMap) *Summary {
	goals := NewOMap()
	for _, k := range model.Goals.Keys() {
		goals.Set(k, model.Goals.m[k])
	}
	for _, k := range extraGoals.Keys() {
		goals.Set(k, extraGoals.m[k])
	}
	s := newSummary(sc.Name, n, goals)
	for i := 0; i < n; i++ {
		if ctx != nil && ctx.Err() != nil {
			s.Runs, s.Note = i, fmt.Sprintf("stopped after %d of %d runs: %v", i, n, context.Cause(ctx))
			break
		}
		run := sc
		run.Seed = fmt.Sprintf("%s#%d", sc.Seed, i)
		s.record(RunScenario(g, model, run, decider, RunOptions{Ctx: ctx}))
	}
	for k, v := range s.values {
		s.Metrics[k] = statsOf(v)
	}
	return s
}

// newSummary lists every goal at [0, 0], so a goal no run could evaluate still shows.
func newSummary(scenario string, n int, goals *OMap) *Summary {
	s := &Summary{Scenario: scenario, Runs: n, Requested: n, Metrics: map[string]*Stats{}, Goals: map[string][2]int{},
		goals: goals, values: map[string][]*big.Rat{}}
	for _, k := range goals.Keys() {
		s.Goals[k] = [2]int{}
	}
	return s
}

// record adds one run. Only a completed run reached the horizon, so only it feeds metrics
// and goals; failed and stopped runs are counted, with their error, and not averaged.
func (s *Summary) record(r *RunResult) {
	if r.Status != "completed" {
		if r.Status == "failed" {
			s.Failed++
		} else {
			s.Stopped++
		}
		if r.Error != "" && !contains(s.Errors, r.Error) {
			s.Errors = append(s.Errors, r.Error)
		}
		return
	}
	s.Completed++
	names := map[string]any{}
	for k, v := range r.Metrics {
		if num, err := toNum(v); err == nil {
			s.values[k] = append(s.values[k], num)
			names[k] = num
		} else {
			names[k] = v
		}
	}
	for _, gname := range s.goals.Keys() {
		ok, err := evaluate(s.goals.m[gname], names, map[string]Func{})
		if err != nil {
			msg := fmt.Sprintf("goal %s: %v", gname, err)
			if !contains(s.Errors, msg) {
				s.Errors = append(s.Errors, msg)
			}
			continue
		}
		held := s.Goals[gname]
		if truthy(ok) {
			held[0]++
		}
		held[1]++
		s.Goals[gname] = held
	}
}

func (st *Stats) Short() string {
	if st.Min.Cmp(st.Max) == 0 {
		return fmtShort(st.P50)
	}
	return fmt.Sprintf("%s (%s–%s)", fmtShort(st.P50), fmtShort(st.P10), fmtShort(st.P90))
}

func fmtShort(r *big.Rat) string { return trimZeros(quantize(r, 2).FloatString(2)) }
