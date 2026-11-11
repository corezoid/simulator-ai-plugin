# Investigation as a digital twin (OSINT)

This flow takes an investigation from a question to a graph in Simulator: who the people
and companies are, how they are connected, which accounts they hold and how the money moved
between them — with the source of every claim kept beside it.

It uses the `simulator-osint` skill and five tools: `osintCaseCheck`, `osintCaseAnalyze`,
`osintCasePlan`, `osintCaseBuild` and `osintCaseGenerate`. The reference for the file format
and for every finding is [`../osint/case-format.md`](../osint/case-format.md); complete
cases are in [`../osint/examples/`](../osint/examples/README.md).

## The idea

```
   question
      │
      ▼
   case.yaml  ◀── sources, entities, relations, accounts, transactions, events, hypotheses
      │            (written by the investigator; every claim cites its source)
      ├─▶ osintCaseCheck     is the case consistent? where is it weak?
      ├─▶ osintCaseAnalyze   ownership, money flows, connections → findings with evidence
      ├─▶ osintCasePlan      what would be created in Simulator
      └─▶ osintCaseBuild     graph + layer, actors, links, accounts, transfers
                                   │
                                   ▼
                          Simulator workspace: the digital twin
```

The case file is the source of truth. The engine never collects anything: facts come from
the investigator — from documents they have, from the host's own search tools, or, for
training, from `osintCaseGenerate`. Only `osintCaseBuild` writes to Simulator.

## 1. Frame it

Write the question first; it decides what belongs in the case.

```yaml
case:
  id: procurement-kickback
  title: Road contract RA-2025-014
  question: Did anyone connected to the road authority's procurement office benefit from contract RA-2025-014?
  currency: EUR
```

Decide whether the case is real or synthetic (`synthetic: true`). For a real case, work only
from sources you may lawfully use, and collect about a person only what the question needs.

## 2. Record sources, then claims

A source comes first, with a reliability grade (A–F); each claim then cites it and carries a
credibility grade (1–6). See "Grading" in the format reference.

```yaml
sources:
  - {id: s-registry, title: Company registry extracts, kind: registry, reliability: B, retrieved: 2025-06-02}
  - {id: s-leak, title: Leaked corporate-services files, kind: leak, reliability: D}

entities:
  - id: consultant
    type: organization
    title: Greywater Consulting LP
    attrs: {legal_form: LP, reg_number: NH-880231, country: Nordhaven Union, incorporated: 2024-11-18}
    sources: [s-registry]
    credibility: 1

relations:
  - {id: r-holding, from: holding, to: consultant, type: owns, share: 100, sources: [s-leak], credibility: 3}
```

Money is recorded as accounts and transactions. Direction is always `from` → `to`, amounts
are exact decimals:

```yaml
accounts:
  - {id: acc-contractor, owner: contractor, kind: bank, sources: [s-bank]}
  - {id: acc-consultant, owner: consultant, kind: bank, sources: [s-bank]}

transactions:
  - {id: t06, from: acc-contractor, to: acc-consultant, amount: 360000, date: 2025-03-05,
     purpose: project management consulting, sources: [s-bank], credibility: 1}
```

What you infer rather than read in a source goes in as a hypothesis with its evidence, not
as a relation:

```yaml
hypotheses:
  - id: h2
    statement: Marenna Halvik is the beneficial owner of Greywater Consulting through Halden Family Holdings.
    status: open
    evidence: [r-holding, r-spouse]
```

## 3. Check

```
osintCaseCheck(casePath="cases/procurement-kickback.yaml")
→ { "ok": true, "errors": [], "warnings": [] }
```

Errors (an id that points nowhere, an amount that is not positive, a person being owned)
must be fixed: a case with errors is not analysed or built. Warnings are the weak spots —
claims with no source, sources with no grade, entities linked to nothing.

## 4. Analyse

```
osintCaseAnalyze(casePath="cases/procurement-kickback.yaml", from="official", to="consultant")
```

For the kickback example it returns, among others:

| Severity | Finding | Weakest evidence |
|---|---|---|
| high | `conflict_of_interest` — money paid by the road authority reaches Greywater Consulting, Halden Family Holdings and Marenna Halvik, who is family of Tobren Halvik, an officer of the authority | D3 |
| high | `layering_chain` — the consultant's and the holding's accounts forward what they receive within days; the money comes from the contractor and ends with Marenna Halvik | A1 |
| medium | `indirect_beneficial_owner` — Marenna Halvik ultimately holds 100 % of Greywater Consulting through one layer | D3 |
| medium | `related_party_payments` — the consultant and the holding pay each other and have the same ultimate owner | D3 |

and the `connection` asked for: official — family → spouse — owns 100 % → holding — owns
100 % → consultant.

Read a finding as a lead:

- `evidence` lists the case items it rests on; `weakestEvidence` grades the weakest of them.
  The conflict of interest above is documented in bank statements (A) but connected to the
  official's family only by a leak (D): the payments are certain, the ownership is not yet.
- `assumptions` states the rule behind each pattern found, with its thresholds.
- Every pattern has an innocent explanation too, and the finding names it. Report what the
  data shows and what would confirm or refute it, not a verdict.

Then iterate: turn a finding into a hypothesis, collect what would test it, add it to the
case, check and analyse again.

## 5. Build the twin

Look at what will be created, then build:

```
osintCasePlan(casePath="cases/procurement-kickback.yaml")
→ counts: 5 forms, 15 actors, 14 links, 9 accounts, 15 transfers

osintCaseBuild(casePath="cases/procurement-kickback.yaml")
→ graphId, layerId, actors {created: 15}, pictures {created: 15}, links {created: 13},
  accounts: 9, transfers {created: 15}
```

Confirm the workspace with the user before building: it writes many objects, and transfers
cannot be undone. What appears in the workspace:

- a graph and a layer titled `OSINT · Road contract RA-2025-014`;
- one actor per entity, of the form `OSINT Person`, `OSINT Organization`, `OSINT Asset`,
  `OSINT Location`, `OSINT Document` or `OSINT Digital`, with its fields filled from the
  case (date of birth, registration number, address, …), a picture, and a description that
  lists every attribute and source with its grade;
- one named link per pair of related entities (`owns 100% · director`);
- an account on each owner's actor, and a transfer for each transaction, from the payer's
  debit side to the payee's credit side, with the original date and purpose in its comment.

Open it with `buildLink(entity="layer", id=<graphId>, focusId=<layerId>)`.

The build is repeatable. Everything carries a ref (`osint:<caseId>:<id>`), so:

- running it again creates nothing new;
- a build that stopped with an error is finished by running it again;
- after the case file changes, a new build adds what is new and updates the existing actors.

It never deletes. Keep editing the file, not the twin.

## 6. Read the twin

The usual tools work on it:

```
getAccounts(actorId=<spouse actor>, query="OSINT bank")
→ credit 575000 EUR (received), debit 540000 EUR (paid) — balance 35000

filterActors(formId=<OSINT Person form>, q="item_osint_case=procurement-kickback")
getLayerActorsPaginated(actorId=<layerId>, type="edges")
```

Two things to keep in mind when reading money in Simulator:

- an account has a debit side and a credit side; its balance is credit minus debit;
- a transfer is dated when it is made, so turnover-by-period reflects the day of the build.
  The original date of each payment is in the transfer's comment.

## 7. Keep the log

`events` in the case file are the chronology of the *subject* (the tender was announced, the
villa was bought). The chronology of the *investigation* — what was collected, what was
found, what changed — belongs in comment reactions on the case's layer or on an event actor
for the case, one dated entry per step.

## Synthetic cases

```
osintCaseGenerate(seed=11, noise=6, outPath="cases/synthetic-11.yaml")
→ 37 entities, 31 relations, 26 accounts, 48 transactions; typologies: layering, round_trip, smurfing, nominee
```

A generated case hides the chosen schemes in ordinary background activity and records the
answer in `truth`. `osintCaseAnalyze` then also reports how much of the planted scheme its
findings reached. Use it to train an investigator (give them the file without `truth`), to
demo the flow, or to test a change to the analysis. The same seed always gives the same
case; every name in it is invented.

## Limits to know

- The analysis sees only what is in the case. No finding means "not in this data", not
  "did not happen".
- Thresholds are rules of thumb: set `threshold` to the reporting threshold of the
  jurisdiction in question, `windowDays` and `minShare` to what the case needs.
- A payment between accounts in different currencies is analysed (in the transaction's
  currency) but not recorded as a transfer in Simulator; model a conversion as two payments
  with the exchange in the middle, as the fraud example does.
- One build call takes up to 1000 entities and 3000 transactions.
