# Example investigation cases

Every case here is invented: the people, companies, addresses, accounts, documents and
payments do not exist. Each file is a complete case in the
[case format](../case-format.md) and passes `osintCaseCheck` with no errors and no warnings.
The engine's tests load these files and assert the findings listed below, so the examples
cannot drift from what the analysis does.

| Case | Story | Size | What `osintCaseAnalyze` finds |
|---|---|---|---|
| [`minimal.yaml`](minimal.yaml) | A buyer pays a supplier through an intermediary that belongs, through a holding, to the buyer's own director. | 5 entities, 4 accounts, 3 payments | `indirect_beneficial_owner` (80 %, resting on a D-grade source), `pass_through`, `shared_attribute` |
| [`procurement-kickback.yaml`](procurement-kickback.yaml) | A road authority pays a contractor; 30 % of each stage goes to a "consultant" owned, through a family holding, by the spouse of the official who ran the tender, and ends in a villa. | 15 entities of 5 types, 9 accounts, 15 payments | `conflict_of_interest`, `layering_chain`, `indirect_beneficial_owner`, `related_party_payments`, shared address and agent |
| [`investment-fraud-cashout.yaml`](investment-fraud-cashout.yaml) | Victims of a fake investment site pay three money mules in amounts under 10 000; the mules buy a stablecoin, the coins move through three wallets to an OTC desk, and the desk pays for a car. | 18 entities of 5 types, 17 accounts in USD and USDT, 22 payments | `structuring` ×3, `pass_through` ×3, `layering_chain` over the wallets, shared phone and domain |
| [`trade-overinvoicing.yaml`](trade-overinvoicing.yaml) | An importer pays wheat invoices 50 % above the market; the excess goes through two offshore companies and comes back as a "loan". The trader and the lender own each other. | 12 entities of 4 types, 6 accounts, 12 payments | `round_trip` (one finding for all the trips), `circular_ownership`, `layering_chain`, `indirect_beneficial_owner` (28 % through the loop), `related_party_payments` |

## What each one is for

- **minimal** — the smallest case that still has a finding on weak evidence. Start here to
  learn the format.
- **procurement-kickback** — a conflict of interest: no single payment is odd, the pattern is
  in who is related to whom. Ask `osintCaseAnalyze(from: official, to: consultant)` for the
  chain that connects them. The key ownership records come from a leak (D), and the finding
  says so: the hypotheses in the file show how to record what would raise the grade.
- **investment-fraud-cashout** — following money across a currency change. A conversion is
  two payments, one in each currency, with the exchange in the middle; the analysis follows
  each currency on its own. Re-run with another `threshold` to see `structuring` disappear.
- **trade-overinvoicing** — the same money going round. The price evidence is in the
  documents and events, not in the payments: the analysis shows the loop, the investigator
  supplies why the invoices are inflated.

## Try them

    osintCaseCheck(casePath="…/examples/procurement-kickback.yaml")
    osintCaseAnalyze(casePath="…/examples/procurement-kickback.yaml", from="official", to="consultant")
    osintCasePlan(casePath="…/examples/procurement-kickback.yaml")
    osintCaseBuild(casePath="…/examples/procurement-kickback.yaml")   # writes to the workspace

For a larger case with ordinary background activity around the schemes, generate one:
`osintCaseGenerate(seed=11, noise=6)`.
