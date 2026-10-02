---
name: simulator-simulate
description: >
  Simulator.Company behaviour simulation specialist ("what if" on a graph). Use when the
  user wants to simulate, forecast or compare scenarios on a layer: describe how actors
  behave, change parameters (price, capacity, delivery time, growth, AI share), play model
  time forward and compare metrics. Any graph works — a company, a process, infrastructure,
  a supply chain — and accounts can hold any quantity (money, items, hours, slots).

  Trigger on any of these intents:
  — "what if", "simulate", "forecast", "scenario", "compare scenarios", "how many will
    we finish", "where is the bottleneck", "what will it cost in 3 months".
  — "симуляция", "смоделируй", "прогони симуляцию", "что будет если", "прогноз",
    "сравни сценарии", "проверь модель", "симуляція", "змоделюй", "що буде якщо".
  — the user points at a behaviour model (`model.yaml`, `scenarios.yaml`) or a layer YAML
    file and asks to run or check it. Use these tools for that, not other simulation CLIs.
  Tools: simulationCheck, simulationRun, simulationSnapshot (read-only; nothing is written
  to Simulator).
---

# Simulate a Simulator graph

The simulation engine runs inside this MCP server. It reads a layer (actors, links, account
values), runs behaviour rules in model time for each scenario and returns metrics. It never
writes to Simulator: the source layer is only read.

Format of models and scenarios: `$CLAUDE_PLUGIN_ROOT/docs/simulation/model-format.md` — read
it before writing a model. Worked examples: `$CLAUDE_PLUGIN_ROOT/docs/simulation/examples/`.

## 1. Pick the graph — always ask

If a tool answers "not authenticated", call `login` and repeat the call.

- If the UI context has `activeLayer`, offer it first; otherwise help the user find a layer
  (`getLayerActorsPaginated`, `layerStats`) and confirm which one.
- Look at what is there before modelling: `simulationSnapshot(layerId, period?)` writes
  `<layerId>.sim.yaml` with every actor, link and account value the simulation will see.
- Account values that accumulate over time (bills, turnover) need `period` (e.g. `30d`):
  values become the turnover of the last period. Check for accounts that are breakdowns of
  the same total (by service, region, operation): summing them counts one bill many times.

## 2. Write the model with the user

Ask only what changes the result: which actors behave and how, what the scenarios change,
the horizon, what to measure. Show the rules in plain words before running, and label
assumptions (durations, probabilities, growth rates) as assumptions.

A model is YAML (spec §5): `value_types`, `refs`, `params`, `horizon`, `initial_events`,
`behaviors.<actor type>.<event>: [actions]`, `metrics`, `goals`, optional helper `actors`
(people or resources that are not on the graph). Actor types are form titles.

Rules of thumb:
- Money and other quantities that only move between actors: `conserved: true` and `transfer`.
  Things that only grow (hours worked, missed sales): not conserved, `add`.
- Capacity (people, machines, slots): an integer conserved account on a resource actor plus
  `enqueue` / `release`.
- Processes drawn as graphs: steps are actors, transitions are links; an application walks
  with `pick(where(actor(self.at).children(), …))` (see examples/process_flow).
- Never write a key named `on:` (YAML reads it as true); use `actor:`.

## 3. Check, then run

    simulationCheck(model, scenarios, layerId | graphPath)

`simulationRun` checks the model too and refuses a model with errors. Warnings (an event
nobody handles, a param typo) usually mean the model does not do what the user thinks.

    simulationRun(model, scenarios, layerId, period?, scenario?, runs?, goals?, logEvents?)

- The model has randomness (`rand()`, `pick`, `chance`, a `decide` without a `rule`): never
  report a single run. Use `runs: 100` and report the median, the 10–90 % range and the share
  of runs meeting each goal.
- These numbers cover completed runs only (`runs completed/total` in the table). When runs
  failed or stopped before the horizon (`stopped_by_time`, `stopped_by_limit`), say how many
  and why (the lines under the table): a scenario that stops early is not cheaper or faster,
  it is unfinished. Raise `timeLimit` or shorten the horizon rather than compare it.
- A warning that the snapshot was written by v2.9.0 means its numbers are text: re-take it
  with `simulationSnapshot(layerId, overwrite: true)` before trusting any result.
- No randomness: one run is exact; say so.
- `logEvents: 30` returns the first processed events — use it to explain why a result
  happened.

## 4. Explain

- Lead with the answer, then the comparison table. Name the scenario and metric behind
  every number; separate results from assumptions.
- A range from `runs` is spread over the model's random outcomes, not a confidence interval
  for the real world.
- The engine computes the consequences of the rules exactly (decimal arithmetic, conserved
  totals checked on every step); whether the rules match reality is a separate question,
  best answered by comparing with history.
