# Packs: what an analysis looks for

A **pack** tells `osintCaseAnalyze` what to look for. It sets the thresholds of the built-in
patterns, chooses which of them to report and how severe each is, and adds **your own
rules** — conditions written as data. The engine stays the same; the pack makes it an
investigation, a counterparty check, or whatever your field needs.

```
osintCaseAnalyze(casePath="cases/acme.yaml", pack="due-diligence")      # a pack that comes with the plugin
osintCaseAnalyze(layerId="…", packPath="packs/my-policy.yaml")           # your own, from a file
osintCaseAnalyze(case="…", pack="pack: {id: mine}\nrules: …")            # your own, as text
```

Without a pack the analysis reports every built-in pattern with the default thresholds.

## Packs that come with the plugin

| Pack | For | What it changes |
|---|---|---|
| `financial-investigation` | following money | Every built-in pattern, thresholds stated (`threshold: 10000`, `window_days: 7`, `min_share: 25`). Rules: `large-payment-weak-evidence` (a payment of 100 000 or more resting on a D–F or missing source), `money-to-unowned-organization` (an organization that received 100 000 or more and has no recorded owner), `recently-incorporated-recipient` (an organization paid 100 000 or more within about a year of being incorporated). |
| `due-diligence` | checking a counterparty | Ownership from 10 % (`min_share: 10`). Leaves out `structuring`, `funnel`, `fan_out`, `pass_through`, which need a full set of statements; raises `unknown_owner` and `shared_anchor` to medium. Rules: `owner-not-evidenced` (a stake resting on weak evidence), `offshore-owner` (an owner of 10 % or more registered in a listed jurisdiction — edit the list to your policy's), `director-of-many`, `entity-without-sources`. |

| `market-map` | a company and the market around it | Leaves out the money-movement findings (a market map has no statements behind it). Rules: `claim-from-one-side-only` (a customer, partner or supplier stated by a single source), `relationship-not-evidenced` (resting on a D–F or missing source), `larger-competitor` (five times the headcount, from the `employees` metric), `more-visible-competitor` (twice the `github_stars`), `key-account` (an organization linked to three or more others), `organization-without-sources`. |

Their files are in the plugin source, `mcp-server/internal/engines/osint/packs/`; copy one
as the start of your own.

## Format

```yaml
pack:
  id: my-policy            # required
  title: Our counterparty policy
  note: free text

settings:                  # thresholds of the built-in patterns
  threshold: 10000         # structuring stays under this
  window_days: 7           # money moving on within this many days is one move
  min_share: 25            # effective stake, in percent, that makes a beneficial owner

builtin:                   # built-in findings: off, or the severity to report them with
  fan_out: "off"
  unknown_owner: medium

rules:                     # your own
  - id: large-unowned-recipient
    for: entity            # entity | relation | account | transaction
    when: "type == 'organization' and owners == 0 and received >= 500000"
    severity: high         # high | medium (default) | low
    title: "{title} received {received} {currency} and has no recorded owner"
    detail: "Paid by {payers} payer(s)."
```

Unknown keys are errors, and so is a condition that does not parse or that names a field
the item does not have: a pack is checked in full before anything runs. A threshold given
to `osintCaseAnalyze` itself (`threshold`, `windowDays`, `minShare`) wins over the pack's.

The built-in findings a pack can switch: `circular_ownership`, `conflict_of_interest`,
`fan_out`, `funnel`, `indirect_beneficial_owner`, `layering_chain`, `multiple_directorships`,
`pass_through`, `related_party_payments`, `round_trip`, `shared_anchor`, `shared_attribute`,
`structuring`, `unknown_owner`. What each one means: [`case-format.md`](case-format.md).

## Rules

A rule is tried on every item of the kind named by `for`. Each item its `when` holds for
becomes a finding of kind `<rule id>`, with the item as its evidence — graded like any other
finding by the weakest source behind it.

`title` and `detail` are templates: `{field}` is replaced by the item's field, `{payer.title}`
by a field of a record in it, `{owner.attrs.country}` by an attribute.

### Conditions

`when` is an expression in the plugin's expression language — the one the simulation models
use: numbers (exact decimals), texts in quotes, `==` `!=` `<` `<=` `>` `>=`, `+ - * /`,
`and` `or` `not`, `in` with a list (`type in ['person', 'organization']`), brackets.

| | |
|---|---|
| a field of a record | `payer.type`, `owner.title` |
| an attribute, with a default | `attrs.get('country', '')`, `payee.attrs.get('incorporated', '')` |
| text methods | `.lower()`, `.upper()`, `.startswith('x')`, `.endswith('x')` |
| `weak(grade)` | the evidence is unsourced, or its best source is graded D, E or F |
| `year(date)` | the year of a `YYYY-MM-DD` date; 0 when there is none |
| `int(x)`, `str(x)` | a text as a whole number; a number as text |

A condition that fails on an item — a division by zero, say — does not stop the analysis:
that item is skipped and the result's `notes` say so.

### What a rule sees

**`for: entity`** — also the record a relation, an account or a transaction carries for the
entities at its ends (`source`, `target`, `owner`, `payer`, `payee`).

| Field | |
|---|---|
| `id`, `type`, `title`, `note` | as in the case |
| `attrs` | the entity's attributes: `attrs.get('country', '')` |
| `metrics` | the entity's metrics, as numbers: `metrics.get('employees', 0)` |
| `sources`, `grade` | number of sources; the grade, e.g. `B2` or `unsourced` |
| `owners`, `owned` | `owns` relations pointing at it; pointing from it |
| `controllers`, `controls` | `controls` / `director` / `officer` / `agent` relations pointing at it; from it |
| `relations`, `neighbours` | all its relations; distinct entities it is linked to or pays |
| `accounts` | number of its accounts |
| `received`, `sent`, `payments_in`, `payments_out`, `payers`, `payees`, `currency` | its money, in its main currency (the one it moved most payments in); payments between its own accounts are not counted |

**`for: relation`** — `id`, `type`, `share` (0 when none), `since`, `until`, `note`,
`sources`, `grade`, `source`, `target`.

**`for: account`** — `id`, `kind`, `title`, `currency`, `sources`, `grade`, `owner`,
`received`, `sent`, `payments_in`, `payments_out`, `payers`, `payees`.

**`for: transaction`** — `id`, `amount`, `currency`, `date`, `purpose`, `sources`, `grade`,
`payer`, `payee`, `from_account`, `to_account`.

## Limits and what to keep in mind

- 100 rules in a pack. A rule reports its first 50 items and says how many more it holds
  for. An analysis evaluates at most 2 000 000 conditions.
- A rule says its condition holds, nothing more. Write the `detail` so that whoever reads
  the finding knows what to check next, and the innocent explanation.
- Rules work on what the case holds. On a layer read with `layerId`, entities of forms that
  are not people or organizations are `other`, and relations carry the layer's grade (`F6`),
  so a rule on `grade` or `type` sees that.
- A pack is data from whoever wrote it. Read a pack before using it; its conditions cannot
  run code or reach anything outside the case.
