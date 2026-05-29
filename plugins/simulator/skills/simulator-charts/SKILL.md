---
name: simulator-charts
description: >
  Simulator.Company chart and dashboard specialist. Use when the user wants to
  create, configure, or query charts (line, bar, area), dashboards, or financial
  time-series visualisations on a graph layer.

  Trigger on any of these intents:
  — Creating: "create chart", "add chart", "create dashboard", "add dashboard",
    "build visualisation", "chart на графе", "график с данными", "дашборд",
    "show metrics on graph", "visualise account data", "plot actor balances".
  — Configuring: "change chart type", "switch to bar chart", "update chart range",
    "use last day", "show turnover", "change time range", "filter chart actors".
  — Querying: "get chart data", "show dashboard", "what charts are on this layer",
    "fetch chart metrics", "dashboard data".
---

# Simulator.Company Chart Builder

You are a specialist in creating financial chart dashboards on Simulator.Company
graph layers using the `simulator` MCP server.

---

## Core Concepts

| Term               | Description                                                                               |
|--------------------|-------------------------------------------------------------------------------------------|
| **Dashboard actor** | An actor with `formName="Dashboards"` — renders a time-series chart on the layer.         |
| **ActorFilters actor** | An actor with `formName="ActorFilters"` — filters source actors by form/account/currency. |
| **actorFilter mode** | Chart data comes dynamically from the top-N actors matched by the filter.                 |
| **direct accounts mode** | Chart data comes from an explicit list of actor+account pairs.                       |
| **laId**           | Layer Actor ID returned when a dashboard is placed on a layer (needed for settings).      |

---

## Primary Tool — `createChart`

One call creates the full chart pipeline:
1. Creates an `ActorFilters` actor (or reuses an existing one).
2. Creates a `Dashboards` actor with the chart config.
3. Places the dashboard on the layer at the given position.
4. Sets account inheritance so the chart can read financial data.
5. Sets `expandType=chart` so the node renders as a chart widget.

Returns: `{ dashboardActorId, filterActorId, laId }`

---

## Mode 1 — actorFilter (dynamic source)

Use when you want the chart to automatically show the **top-N** actors from a
given form, ranked by their account balance.

```
createChart(
  layerId       = "<layerUUID>",        // required
  title         = "MCP API Calls",      // required
  description   = "Requests per tool",  // optional
  chartType     = "line",               // "line" | "bar" | "area" (default "line")
  counterType   = "amount",             // "amount" | "turnover" (default "amount")
  range         = "lastHour",           // "lastHour" | "lastDay" | "lastWeek" | "lastMonth"
  sourceFormId  = 580365,               // numeric formId of the source actors
  accountNameId = "b37efbf5-...",       // UUID of the account name
  currencyId    = 68933782,             // numeric currency ID
  top           = 20,                   // number of top actors to show (default 20)
  positionX     = -100,                 // x on layer (default -100)
  positionY     = 0                     // y on layer (default 0)
)
```

**Reuse an existing ActorFilters actor:**

```
createChart(
  layerId       = "<layerUUID>",
  title         = "My Chart",
  filterActorId = "<existingFilterUUID>",   // skip filter creation
  // sourceFormId / accountNameId / currencyId auto-extracted from the filter actor
  chartType     = "bar"
)
```

---

## Mode 2 — direct accounts (explicit series)

Use when you want to chart **specific actors** instead of the top-N.
Provide the `accounts` array — each item is one chart series.

```
createChart(
  layerId     = "<layerUUID>",
  title       = "Key Actors",
  chartType   = "bar",
  accounts    = [
    { actorId: "<uuid1>", currencyId: 68933782, nameId: "<nameUUID>", color: "#499894", incomeType: "total" },
    { actorId: "<uuid2>", currencyId: 68933782, nameId: "<nameUUID>", color: "#59A14F" },
    { actorId: "<uuid3>", currencyId: 68933782, nameId: "<nameUUID>" }
  ]
)
```

`color` and `incomeType` are optional per item; defaults are auto-assigned from
the Tableau palette and `"total"` respectively.

---

## Before Calling `createChart` — What You Need

### actorFilter mode prerequisites

1. **`layerId`** — the layer where the chart will appear. Use an existing layer
   or create one with `createActor(formName="Layers")`.

2. **`sourceFormId`** — the numeric formId of the actors you want to chart.
   Find it via `getForms()` or `getForm(formId=...)`.

3. **`accountNameId`** — UUID of the account name (e.g. "Requests", "Balance").
   Find it via `getAccountNames()`.

4. **`currencyId`** — numeric ID of the currency (e.g. "USD", "Requests").
   Find it via `getCurrencies()`.

### direct accounts mode prerequisites

1. **`layerId`** — same as above.
2. **`accounts`** array — each `actorId` must already exist; `nameId` and
   `currencyId` must correspond to accounts that actors actually have.

---

## Lookup Helpers

```
// Find formId for your source form
getForms()                          // list all user forms
getForm(formId="<formId>")          // get form details with fields

// Find accountNameId
getAccountNames()                   // list all account names in the workspace

// Find currencyId
getCurrencies()                     // list all currencies in the workspace

// Find layerId (if you have graph actor)
getLinkedActors(actorId="<graphId>") // find child Layers actor
```

---

## Complete Example: Build a Chart on an Existing Layer

```
// 1. Find required IDs
getAccountNames()  → locate "MCP API Calls" → nameId = "b37efbf5-..."
getCurrencies()    → locate "Requests"       → currencyId = 68933782

// 2. Create chart
createChart(
  layerId       = "01c447c3-9d15-4dda-9fd7-086b481c0465",
  title         = "MCP API Calls",
  description   = "Top tools by request count",
  chartType     = "line",
  range         = "lastHour",
  sourceFormId  = 580365,
  accountNameId = "b37efbf5-1b4876-9021-47639da2e093",
  currencyId    = 68933782,
  top           = 20,
  positionX     = -100,
  positionY     = 0
)
→ { dashboardActorId: "a643c9f1-...", filterActorId: "03f00f83-...", laId: 51944801 }
```

The chart appears immediately on the layer as a widget node.

---

## Parameter Reference

| Parameter       | Required | Default      | Description                                                  |
|-----------------|----------|--------------|--------------------------------------------------------------|
| `layerId`       | ✓        | —            | Layer actor UUID                                             |
| `title`         | ✓        | —            | Chart title                                                  |
| `description`   |          | `""`         | Chart description                                            |
| `chartType`     |          | `"line"`     | `"line"` \| `"bar"` \| `"area"`                              |
| `counterType`   |          | `"amount"`   | `"amount"` \| `"turnover"`                                   |
| `range`         |          | `"lastHour"` | `"lastHour"` \| `"lastDay"` \| `"lastWeek"` \| `"lastMonth"` |
| `positionX`     |          | `-100`       | X position on canvas                                         |
| `positionY`     |          | `0`          | Y position on canvas                                         |
| `filterActorId` |          | —            | Reuse existing ActorFilters actor                            |
| `filterTitle`   |          | = `title`    | Title for the new ActorFilters actor                         |
| `sourceFormId`  | *        | —            | Numeric formId of source actors (*required in actorFilter mode without `filterActorId`) |
| `accountNameId` | *        | —            | UUID of the account name (*same)                             |
| `currencyId`    | *        | —            | Numeric currency ID (*same)                                  |
| `top`           |          | `20`         | Top-N actors to show (actorFilter mode)                      |
| `accounts`      |          | —            | Explicit series array (direct accounts mode)                 |

---

## Key Rules

- **`createChart` is the only tool needed** — it handles filter creation, placement, inheritance, and expand-type in one call.
- **actorFilter mode** is the default when `accounts` is not provided. Needs `sourceFormId` + `accountNameId` + `currencyId` (unless reusing `filterActorId`).
- **direct accounts mode** activates automatically when `accounts` array is provided.
- If `filterActorId` is provided, the tool fetches the existing filter actor's data automatically — no need to re-specify `sourceFormId` / `accountNameId` / `currencyId`.
- Chart nodes appear as expanded widgets on the layer (`expandType=chart`).
- Reuse `filterActorId` to share a filter across multiple charts — avoids duplicating `ActorFilters` actors.
