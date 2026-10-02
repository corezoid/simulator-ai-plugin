# sim-morrow model format, version 1

This is the contract between behaviour models and the simulation engine of this plugin (`simulationCheck`, `simulationRun`, `simulationSnapshot`; Go package `internal/engines/sim`). The same format is implemented by the Python reference engine sim-morrow; both must reproduce the conformance cases in `mcp-server/internal/engines/sim/testdata/conformance` (see "Conformance" at the end).

A simulation takes three inputs:

| File | What it holds |
|---|---|
| graph file | actors, links and accounts: the Simulator plugin's layer YAML with `sim:` sections |
| `model.yaml` | value types, references, parameters, initial events, behaviours, metrics, goals |
| `scenarios.yaml` | a list of scenarios: parameter overrides and field changes applied to the snapshot |

## 1. Graph file

The layer YAML of `pullGraphFile` / `pushGraphFile`, extended with `sim:` sections that the plugin ignores.

```yaml
layerId: <layer uuid or empty>
actors:
  - id: <id>                 # uuid for Simulator actors, any string for synthetic graphs
    title: Client A
    formId: 123              # optional
    formName: client         # actor type when sim.type is absent
    data: {budget: 120}      # public fields
    position: {x: 0, y: 0}
    sim:                     # optional
      type: client           # actor type used to pick behaviours (default: formName, then formId)
      origin_id: <id>        # id in Simulator, if the actor came from a snapshot
      created_by: null
      state: {}              # internal fields (names start with "_"), merged into data
edges:
  - {source: <id>, target: <id>, sim: {id: <link id>, edgeType: hierarchy, mediator: null}}
sim:
  format: sim-morrow/1
  source: {kind: simulator, workspace: <accId>, layer: <id>, period_seconds: 2592000}
  valueTypes: {UAH: {kind: decimal, scale: 2, conserved: true, min: null, max: null}}
  accounts:
    - {actor_id: <id>, name: cash, value_type: UAH, value: "120"}
```

Rules:
- Actors keep their order from the file. Every "for each actor" in this spec uses this order.
- A snapshot of a live layer lists actors and links sorted by id, so the result does not depend on the order the Simulator API returns them in. An actor or edge placed twice appears once.
- A snapshot reads layer pages until an empty page (a page can be short because deleted elements are dropped from it) and account values with high precision. An actor whose accounts the caller may not read stays in the graph without accounts and is listed in `source.accounts_not_readable`.
- A file without `sim:` sections (straight from `pullGraphFile`) is valid: types come from `formName` or `formId`, there are no accounts.
- An edge whose source or target is not an actor in the file is dropped.

## 2. Values

| Rule | Detail |
|---|---|
| Numbers | exact decimals; no binary floating point anywhere. Literals like `0.1` are decimal `0.1`. |
| Value type | `kind: integer` (scale 0) or `decimal` with `scale` places; `conserved`; optional `min`, `max`. |
| Storing | a value written to an account is rounded to the type's scale, half to even. An integer type rejects a non-integer value. |
| Bounds | a stored value outside `min`/`max` fails the step. |
| Conserved | a conserved type only moves with `transfer`. At the end of every step the sum of each conserved type over all accounts must equal the sum at the start of the step, otherwise the step fails. `add` and `set_account` on a conserved type fail. |
| Unknown type | an account whose type is not declared uses decimal, scale 2, not conserved. The model's `value_types` override types from the graph file. |

## 3. Time and the event queue

- Model time is integer seconds from 0. Durations are written as numbers (seconds) or `<n><unit>` with units `ms`, `s`, `m`, `min`, `h`, `d` (`15m` = 900).
- `horizon` is inclusive: events at exactly the horizon are processed; later events stay in the queue and are reported as `pending_events`.
- The queue is ordered by `(time, priority, seq)`. `seq` is a counter that increases by one every time an event is queued. Lower priority numbers run first.
- Initial events are queued first, in model order; for `for_type`, targets are taken in graph order. With `every`, the event occurs at `at`, `at + every`, … up to the horizon. Occurrences are numbered as if all of them were queued at the start, in (initial event, target, occurrence) order, so they run before any event a step schedules for the same time and priority. An engine queues only the next occurrence of each chain; `pending_events` still counts the occurrences not run.
- Events scheduled by a step are queued after the step commits, in the order they were scheduled.
- An event whose target type has no handler for its kind is logged as skipped and counts as a step.
- The run stops with `failed` on the first step that fails, with `stopped_by_limit` after `max_steps` (default 1 000 000), and otherwise with `completed`.

## 4. A step

1. Take the first event of the queue. The step's actor is the event target (`self`).
2. Run the handler `behaviors.<type of self>.<event kind>` action by action.
3. If any action fails, undo every change of the step and stop the run with `failed`.
4. Otherwise check conserved totals, commit, and queue the scheduled events.

The type of an actor is `data[type_field]` when the model sets `type_field` and the actor has that field, otherwise the actor type.

## 5. Model file

```yaml
value_types: {<name>: {kind, scale, conserved, min, max}}
refs:        {<name>: <actor>}          # actor = id, origin id or title (string), or {id}, {title}, {type}, {title, type}
params:      {<name>: <value>}          # scenario params override these
horizon:     20m
type_field:  null
actors:                                  # helper actors added to the graph (not in Simulator)
  - {title: Loan officers, type: team, data: {}, accounts: [{name: slots, value_type: slots, value: "=params.officers"}]}
initial_events:
  - {event: offer, for_type: client, at: 0, priority: 30, payload: {}}
  - {event: sell, target: shop, at: 30m, every: 30m}      # target = ref name or actor
behaviors:
  <actor type>:
    <event kind>: [<action>, ...]
metrics: {<name>: <expression>}
goals:   {<name>: <expression over metric names>}
```

- A reference must match exactly one actor, otherwise loading fails.
- Model actors get the id `model:<title>` unless `id` is given, and are appended after the graph's actors. Their account values may be `=expressions` over `params`.
- `at` and `every` of initial events may be `=expressions` over `params`.

## 6. Actions

Every action is a one-key mapping. Unless noted, values of `data`, `payload`, `fields` and `set` are literals, and a string starting with `=` is an expression. The fields `if`, `rule`, `amount`, `value`, `target`, `actor`, `resource`, `from`/`to` of `link` are always expressions. `after` and `at` are a duration literal (`15m`, `30`) or an expression that yields one.

| Action | Fields | Effect |
|---|---|---|
| `if` | `if`, `then`, `else` | runs `then` when the expression is true, else `else` |
| `set` | `{field: value}` | sets fields of `self` |
| `set_on` | `actor`, `fields` | sets fields of another actor |
| `create` | `type`, `title`, `data`, `as`, `link_from`, `edge_type`, `accounts` | creates an actor (see ids below); `as` names it for later actions; `link_from` links it from that actor; `accounts` = `[{name, value_type}]` created at 0 |
| `link` | `from`, `to`, `edge_type` | creates a link (default type `hierarchy`) |
| `transfer` | `from: {actor, account}`, `to: {actor, account}`, `amount` | moves a non-negative amount; a zero amount does nothing; the destination account is created with the source's type if missing; types must match |
| `add` | `actor`, `account`, `amount`, `value_type` | adds to a non-conserved account, creating it with `value_type` |
| `set_account` | `actor`, `account`, `value`, `value_type` | sets a non-conserved account |
| `schedule` | `event`, `target` (default self), `after` or `at`, `priority` (default 30), `payload` | queues an event; a time before now fails |
| `enqueue` | `resource`, `account` (default `capacity`), `target`, `event`, `amount` (default 1), `priority` (default 40), `payload` | puts `target` in the FIFO queue of `resource`, then dispatches |
| `release` | `target` | returns what `target` holds to its resource, then dispatches that resource |
| `dequeue` | `resource`, `target` | removes `target` from the queue |
| `decide` | `options`, `rule`, `var` (default `choice`), `jev` | picks one option (see decisions) and stores it in the variable |
| `log` | text | adds a note to the step |

Resource queue: dispatch gives free capacity to waiting jobs in FIFO order while the first job fits. A job that fits gets `amount` transferred from the resource's capacity account to the same account of the job target, the target's `_holding` field records it, and its `event` is queued now with the job's priority. Queue state lives in the resource's `_queue` field, the capacity account name in `_capacity_account`.

Created actors:
- ids are `sim-1`, `sim-2`, … in creation order over the whole run, skipping ids already in the graph;
- link ids are `simlink-<number of links + 1>`, with `x` appended while that id exists;
- `data._logical_id` is `<event key>:<as or type>#<n>`, where `n` counts the actors created earlier in the same step.

Event keys (used by created actors and random numbers): an initial event is `init<i>:<actor origin id or id>#<n>`, where `i` is its index in `initial_events` and `n` the occurrence number. A scheduled event is `<key of the event that scheduled it>><kind>#<n>`, where `n` is its index among the events scheduled by that step.

## 7. Decisions and random numbers

A uniform number is `U(parts) = first 8 bytes of SHA-256("|".join(parts)) as a big-endian integer / 2^64`.

- `decide` with a `rule` takes the rule's value, which must be one of `options`.
- `decide` without a rule and without Jev takes `options[floor(U(seed, "decide", self logical id, event key, var) × n)]`.
- `rand()` returns `U(seed, "rand", self logical id, event key, k)`, where `k` counts calls to `rand()` in the step, from 1.
- `pick(list)` uses `rand()`: `list[floor(u × len)]`; an empty list gives null.
- `chance(p)` is `rand() < p`.

The logical id of an actor is `data._logical_id`, else its origin id, else its id. `seed` is the scenario seed, default `morrow`. Many-run mode uses seeds `<seed>#0` … `<seed>#<N-1>`.

A `jev:` block (`question`, `criteria`, optional `state`, `params`) asks TypeSafe Jev instead of using the rule, in engines that support it. Without Jev a `decide` behaves as if the block were absent: the rule, else a uniform choice. With Jev enabled but unavailable, a `decide` without a rule fails. Jev is not part of conformance.

## 8. Expressions

A subset of Python expression syntax.

| Construct | Notes |
|---|---|
| literals | numbers (exact decimals), `'strings'` and `"strings"`, `True`, `False`, `None`, lists, dicts, tuples |
| names | `self`, `event` (payload), `params`, `refs`, `now` (seconds), variables from `create … as` and `decide … var` |
| operators | `+ - * / // %`, unary `- + not`, `== != < <= > >=` (chained), `in`, `not in`, `is`, `is not`, `and`, `or` (return an operand, like Python), `x if c else y`, `a[i]`, `a.b` |
| functions | `actor(x)`, `actors(type=None, **fields)`, `count(type=None, **fields)`, `total(account, type=None, **fields)`, `dur(x)`, `dec(x)`, `min`, `max`, `abs`, `len`, `round(x, n=0)`, `str`, `rand()`, `pick(list)`, `where(list, title_has=, title_not=, leaf=)`, `chance(p)` |
| actor view | `.id .type .title .data`, `.acc(name)` (0 if missing), `.sum_accounts(value_type=None)`, `.has(field)`, `.get(field, default)`, `.children(type=None, edge_type=None)`, `.parents(…)`, `.parent`, and any data field as an attribute |
| dicts and namespaces | `.get(key, default)`, `.keys() .values() .items()`; strings: `.lower() .upper() .startswith() .endswith() .count()` |

- `+` on two strings concatenates; `str(x)` of a number prints it in plain notation, keeping its digits (`str(dec('1.50'))` is `1.50`, never `1.5E+0`).
- `round(x, n)` rounds half to even.
- `where` filters actor views: `title_has`/`title_not` are case-insensitive substrings of the title; `leaf=True` keeps actors without children.
- `actors`/`count`/`total` filter by type (with `type_field`) and by equality of data fields, in graph order.
- Unknown names, functions, fields and attributes starting with `_` fail the step.
- Division by zero fails the step.
- Numbers in data coming from JSON or YAML are read as decimals.

Scenarios: `{name, params, set: [{actor, fields}], horizon, seed}`. `set` changes fields before the first event; `actor` is a ref name or an actor as in `refs`; field values starting with `=` are expressions over `params`.

## 9. Metrics, goals and many runs

- Metrics are evaluated after the run, with `self` = the first actor of the graph and `now` = the horizon. A failed run has no metrics.
- A goal is a boolean expression whose names are metric names.
- Many-run mode runs a scenario N times with seeds `<seed>#i`. Only completed runs reached the horizon, so only they feed statistics: failed and stopped (`stopped_by_limit`, `stopped_by_time`) runs are counted separately and excluded. For each numeric metric: `n`, `mean`, `min`, `max` and nearest-rank percentiles `p10`, `p50`, `p90` (`k = ceil(p × n / 100)`, 1-based). For each goal: `[held, evaluated]`.

## 10. Conformance

Each directory in `testdata/conformance/` holds `graph.yaml`, `model.yaml`, `scenarios.yaml` and `expected.json`. A scenario may carry `runs: N` to add a many-run check. For every scenario an engine must match `status`, `steps`, `pending_events`, `metrics` and `events` (`[time, priority, kind, target]` per processed event); for a failed scenario the error message may differ but must be present. Numbers are compared after rounding to 12 decimal places, half to even, without trailing zeros.

`expected.json` is produced by the Python reference engine and checked here by `TestConformance`. A change of semantics changes this document, the cases and both engines together.

## 11. Differences of this engine

- Jev decisions (`jev:` blocks) are not available here: a `decide` uses its `rule`, or a uniform choice without one.
- Publishing results to Simulator and the slow mode (a live copy updated step by step) are not available yet; `simulationRun` only reads.
- A `graphPath` file straight from `pullGraphFile` names forms only by `formId`. When logged in, `simulationRun` and `simulationCheck` replace such types with the form titles from Simulator, so a model can say `type: Shops` for either input.
- `simulationRun` has a wall-clock budget (`timeLimit`, default 2 minutes, at most 15): a single run that exceeds it ends with status `stopped_by_time`, and many runs report the runs made so far with a `note`. The event log in a result is capped at 10,000 entries per scenario.
- `simulationSnapshot` does not replace an existing `<layerId>.sim.yaml` unless `overwrite: true` is passed, so hand edits are not lost.
