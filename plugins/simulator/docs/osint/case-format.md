# Investigation case format

An investigation is kept in one YAML file, the **case**. It holds what is known (entities,
relations, accounts, transactions, events), where each claim comes from (sources and
grades), and what is being tested (hypotheses). The `osintCase*` tools read this file; they
collect nothing themselves and write nothing to Simulator.

Complete cases, each with what the analysis finds in it: [`examples/`](examples/README.md).
A walkthrough from question to digital twin:
[`../user-flows/osint-investigation.md`](../user-flows/osint-investigation.md). Collecting
from open sources into a case: [`sources.md`](sources.md).

## Rules that hold everywhere

- **One id namespace.** Every `id` is unique across the whole file, whatever it names, so a
  hypothesis can cite an entity, a transaction or a source alike. An id is 1–64 characters:
  letters, digits, `_`, `.`, `-`.
- **Unknown keys are errors.** A typo in a key would otherwise drop a claim silently.
- **Amounts are exact decimals** (`1250.50`), never floats, and always positive: direction is
  `from` → `to`.
- **Dates** are `YYYY-MM-DD` or RFC 3339 (`2025-03-01T14:30:00Z`). A missing date means "not
  known", and the item is left out of time-based patterns.
- **Every claim can carry provenance**: `sources` (ids from `sources`) and `credibility`.

## Grading (Admiralty system)

| On | Field | Scale |
|---|---|---|
| a source | `reliability` | `A` completely reliable · `B` usually reliable · `C` fairly reliable · `D` not usually reliable · `E` unreliable · `F` cannot be judged |
| a claim | `credibility` | `1` confirmed by other sources · `2` probably true · `3` possibly true · `4` doubtful · `5` improbable · `6` cannot be judged |

A claim's grade is the best reliability among its sources plus its credibility, e.g. `B2`.
A source with no `reliability` counts as `F`; a claim with no `credibility` counts as `6`; a
claim with no source is `unsourced`. A finding is as strong as its weakest evidence.

## Sections

### `case` (required)

| Key | Meaning |
|---|---|
| `id` | Case slug. Becomes part of every ref when the case is built in Simulator. Required. |
| `title` | Required. |
| `question` | What the investigation has to answer. |
| `synthetic` | `true` for a generated case with invented data. |
| `currency` | Default currency for accounts and transactions that name none. |
| `note` | Free text. |

### `sources`

`id`, `title` (required); `kind` — `registry`, `court`, `sanctions`, `corporate`, `bank`,
`blockchain`, `media`, `social`, `leak`, `human`, `synthetic`, `other` (another word is a
warning); `url`; `retrieved` (date); `reliability`; `note`.

### `entities`

`id`, `title` (required); `type` (required) — `person`, `organization`, `asset`, `location`,
`document`, `digital`, `other`; `aliases` (list); `attrs` (map of text values); `picture`;
`sources`; `credibility`; `note`.

`picture` is an `https://` URL or a path to an image file (`.png`, `.jpg`, `.jpeg`, `.gif`,
`.webp`, `.svg`) relative to the working directory. Use only a picture you may lawfully use.
An entity without one gets a generated placeholder in Simulator — a glyph for its type on a
colour of its own, not a likeness.

Attributes with the keys below become **fields of the actor** when the case is built; any
other attribute is kept in the actor's description only.

| Type | Attribute keys with a field of their own |
|---|---|
| `person` | `dob`, `nationality`, `country`, `occupation`, `phone`, `email`, `address`, `passport`, `tax_id` |
| `organization` | `legal_form`, `reg_number`, `tax_id`, `country`, `incorporated`, `status`, `industry`, `address`, `registered_address`, `registered_agent`, `website`, `phone`, `email` |
| `asset` | `kind`, `identifier`, `value`, `acquired`, `country`, `address` |
| `location` | `address`, `city`, `country` |
| `document` | `kind`, `number`, `date`, `issuer` |
| `digital` | `kind`, `value`, `registered`, `provider` |
| `other` | `kind` |

Attributes that identify something are compared across entities, and a shared value is
reported: `address`, `registered_address`, `phone`, `email`, `website`, `domain`, `ip`,
`registered_agent`, `tax_id`, `passport`, `wallet`. Other attributes are kept but not
compared. Model an address or identifier that several entities use as its own `location` or
`digital` entity linked with `registered_at` / `located_at` / `uses` when its own history
matters; use an attribute when it is just a value.

### `relations`

`from`, `to` (entity ids, required); `type` (required); `id` (optional — give one to cite the
relation as evidence); `share` (percent, on `owns` only); `since`, `until`; `sources`;
`credibility`; `note`.

| Type | Reads as | Used by the analysis for |
|---|---|---|
| `owns` | from owns `share` % of to | beneficial ownership, circular ownership, related parties |
| `controls`, `director`, `officer`, `agent` | from has a say over to | one person fronting many organizations |
| `registered_at`, `located_at`, `uses` | from is at / uses to | shared addresses and identifiers |
| `employee`, `family`, `associate`, `mentions`, `same_as`, `other` | plain link | connections between entities |

Any other word is accepted with a warning and treated as a plain link.

### `accounts`

`id`, `owner` (entity id) (required); `kind` — `bank`, `crypto`, `cash`, `emoney`, `other`;
`title`; `number`; `institution`; `currency`; `opened`, `closed`; `sources`; `credibility`;
`note`.

### `transactions`

`id`, `from`, `to` (account ids), `amount` (required); `currency`; `date`; `purpose`;
`sources`; `credibility`; `note`. The currency is the transaction's own, else its accounts',
else the case's.

### `events`

`date`, `title` (required); `id`; `entities` (list of entity ids); `sources`; `credibility`;
`note`. The chronology of the case.

### `hypotheses`

`id`, `statement` (required); `status` — `open`, `supported`, `refuted`; `evidence` (ids of
any items in the case); `note`.

### `truth` (synthetic cases only)

`typologies`, `entities`, `transactions`, `note` — the known answer of a generated case. The
analysis never reads it to find anything; it only scores its findings against it.

## What the check reports

Errors (the case is inconsistent; it is not analysed or planned until they are fixed): an
id that is malformed, repeated or points nowhere; an unknown entity type, account kind or
hypothesis status; a picture that is neither an https URL nor a relative image path; a relation or transaction from something to itself; a person being
owned; a share outside (0, 100]; an amount missing or not positive; an end date before a
start date; an event without a date.

Warnings (weak spots): claims with no source; sources cited by nothing; a source with no
grade; entities linked to nothing; entities with the same type and title; stakes in one
entity adding up to more than 100 %; `owns` without a share; transactions outside the life
of their account, without a date, or in another currency than their account; transactions
with the same accounts, amount and date.

## What the analysis reports

Every finding has a `kind`, a `severity` (`high`, `medium`, `low`), the `subjects` it is
about, the `evidence` (case ids) it rests on and `weakestEvidence`. **A finding is a lead to
check, not a conclusion**: each pattern has innocent explanations too, and the analysis sees
only what is in the case. The `assumptions` list states the rule behind each pattern found.

| Kind | Pattern |
|---|---|
| `indirect_beneficial_owner` | An ultimate owner whose effective stake — stakes multiplied along each chain, chains added up — reaches `minShare` (default 25 %) through at least one intermediate company. `high` from two layers. |
| `circular_ownership` | Entities that own each other in a loop. |
| `unknown_owner` | Organizations with no recorded owner (one finding for all). |
| `multiple_directorships` | A person with a controlling role in 3 or more organizations. |
| `shared_anchor` | 2 or more entities registered at, located at or using the same entity. |
| `shared_attribute` | 2 or more entities with the same identifying attribute value. |
| `related_party_payments` | Payments between two entities with the same ultimate owner. |
| `conflict_of_interest` | Money that leaves an organization and, within 4 payments, reaches a person with a controlling role in it (other than an owner), someone linked to that person as `family`, `associate` or `same_as`, or a company such a person ultimately owns. A payment straight to the officer is taken as pay. |
| `layering_chain` | 2 or more pass-through accounts that pay each other. |
| `pass_through` | An account that sent on at least 90 % of what it received within `windowDays` (default 7), paid out no more than 110 % of it, and deals with at most 3 payers and 3 payees. |
| `round_trip` | Up to 6 payments returning to the entity they started from within 120 days, each no earlier than the one before and between half and double its size. Every way round the same set of entities is one finding: it shows the quickest trip and says how many there are. |
| `structuring` | 3 or more payments of 80–100 % of `threshold` (default 10000) to or from one entity within `windowDays`. Set the threshold of the jurisdiction in question. |
| `funnel` | An entity paid by 5 or more payers that passes 80–120 % of it to at most 2 payees. |
| `fan_out` | The reverse: at most 2 payers, 5 or more payees. `low` — a payroll looks the same. |

Beside the findings: `beneficialOwnership` (every ultimate owner of every owned entity),
`central` (the most connected entities), `flows` (money in and out per entity and
currency), `connection` (the shortest chains between `from` and `to`, when given),
`timeline`, `evidence` (how well the case is sourced) and, for a synthetic case, `truth`
(what share of the planted scheme the findings reached, what was missed, what was flagged
outside it).

## Building the case in Simulator

`osintCaseBuild` builds the case as a digital twin; `osintCasePlan` shows what it will create
without writing anything.

| Case | Simulator |
|---|---|
| the case | a graph and a layer titled `OSINT · <case title>`, refs `osint:<caseId>:graph` / `:layer` |
| entity | an actor of the form `OSINT <Type>` (form ref `osint-<type>`), ref `osint:<caseId>:<entityId>`, placed on the layer in a column per type. Its fields (`item_osint_<key>`) hold the case id, the id in the case, the evidence grade, aliases, source titles and the attributes listed under `entities`; its description holds all attributes, the sources with their grades and the note. A `location` also gets its title as the actor's place name |
| picture | the actor's picture: the entity's `picture`, or a generated placeholder (`pictures: auto`, the default; `given` sets only the case's pictures, `none` leaves pictures alone). The actor remembers which picture it has, so a repeated build uploads nothing |
| relation | a link named after the relation type (`owns 60%`). Simulator keeps one link between two actors, so several relations between the same two become one link named after all of them (`owns 100% · director`) |
| account | an account on its owner's actor, named `OSINT <kind>` (`#2` for a second one of the same kind and currency). In Simulator it has a debit side and a credit side |
| transaction | a transfer from the payer's debit side to the payee's credit side, ref `osint:<caseId>:<transactionId>`, dated with the payment's own date (the transfer's original date), with the date, purpose, id and grade also in its comment |
| event, finding | a comment reaction on the layer or on the case's event actor (posted by hand, not by the build) |

- **Balances.** An account's balance is its credit side minus its debit side — what it
  received minus what it paid. Nothing has to be funded first.
- **Dates.** A transfer carries the date of the original payment as its original date, so
  turnover by period in Simulator follows the case: a payment of March 2025 counts in March
  2025 whenever the case is built. A payment with no date is dated when it is built.
- **Repeatable.** Everything is found again by its ref. Building the same case twice creates
  nothing the second time; a build that stopped half-way is finished by running it again;
  after the case file changes, a new build adds what is new and brings the existing actors'
  titles and descriptions up to date. It never deletes: an entity removed from the case
  stays in Simulator until removed there.
- **Currencies.** A transaction between accounts in different currencies is not recorded;
  the build reports it.
- **Forms.** A form that already exists keeps its own fields and gets the ones it lacks.
- **Limits.** One call builds up to 1000 entities and 3000 transactions.

## Analysing a live layer

`osintCaseAnalyze(layerId=…)` reads a layer of the workspace as a case and analyses it — no
file. Its actors become entities, its links relations, the accounts on its actors accounts,
and the transfers between those accounts transactions.

| On the layer | Read as |
|---|---|
| an actor of an `OSINT <Type>` form | an entity of that type, with its id in the case, attributes and grade |
| an actor of a form named Human, User, Person, Employee | a `person` |
| an actor of a form named Company, Organization | an `organization` |
| any other actor | an entity of type `other`, with the form's name as its `kind` |
| a link named after relations (`owns 60% · director`) | those relations |
| a link with any other name, or none | a plain link that keeps the name |
| an account (not one of the platform's own tallies) | an account of its actor; its debit and credit sides are one account |
| a transfer between two accounts on the layer | a transaction, dated by its original date |

It works on any layer, built from a case or by hand. What a layer cannot tell is said in the
result's `layer.notes`:

- A layer does not record where a claim came from. Entities and transfers built from a case
  keep their grade; relations do not, so findings that rest on them are graded `F6`. For
  graded findings analyse the case file.
- Rules that need to know a person from an organization (beneficial owners, directorships,
  conflicts of interest) do not see actors read as `other`.
- A transfer with one side outside the layer is left out.
