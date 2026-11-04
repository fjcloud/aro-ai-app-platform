# Card Scout multi-era baseline — 4 November 2026

This is the multi-era snapshot tested before migration to the complete English
Base Set later on 4 November. Its old item IDs and model metrics are historical.
Card Scout was operational on ARO lab `54w5q`. The public application,
filters, pagination, saved filter, mobile layout, image serving and native
agent research passed the existing browser acceptance suite. Isolated browser
fixtures also passed. Lab URLs are temporary.

- [Application](https://signal-desk-signal-desk-dev.apps.ijtxiwk6.eastus.aroapp.io)
- [Fresh research trace](https://rh-ai.apps.ijtxiwk6.eastus.aroapp.io/mlflow/#/experiments/1/traces?selectedEvaluationId=tr-3b625e1604dab38aad65bd514d398e2e&workspace=signal-desk-dev)

## LLM access

The existing runtime key reproduced HTTP 403 directly through MaaS. The native
agent wrapped that failure as HTTP 500 with `Error code: 403`. The developer
secret dated from 2 November; both preparation scripts mint keys lasting 24 hours.
The old key's expiry timestamp was not retrieved from the management API.

After renewing both keys and restarting `card-scout-platform`, a real developer
Chat Completions request returned HTTP 200 and `OK`. A fresh agent request for
`item-039` returned HTTP 200, matching catalogue identity, dated Cardmarket values
and an MLflow trace. Existing classifications and persistent state survived.
The guide now gives the renewal commands and explains that a Dev Space must
stop and start to load a new environment key.

## Fresh T4 benchmark

All 57 images were sent again to the live `vllm-sr/d3-mini` endpoint, using the
current prompt. No reference labels, card names or image URLs entered inference.
Reference labels were read after the batch for evaluation.

| Check | Current prompt | Exploratory symbol prompt |
|---|---:|---:|
| Image requests completed | 57/57 | 57/57 |
| Card-kind identification | 57/57 | 57/57 |
| Common / uncommon / rare-or-higher correct | 42/53 | 43/53 |
| Actual rare cards detected | 16/20 | 18/20 |
| Non-rare cards incorrectly flagged rare | 1 | 4 |
| Mean request time, including local port-forward | 1.225 s | 1.322 s |

There are 54 Pokemon cards and three non-Pokemon controls. Pikachu Illustrator
has no standard printed rarity in the reference and is excluded from the
53-card rarity denominator; both prompts correctly abstain on it. Both prompts
also correctly classify all three controls. These are clean catalogue images,
not smartphone photographs. Timing is one serial run, not a load benchmark.

The current prompt misses Umbreon Gold Star and Professor's Research as Common,
and abstains on Shining Charizard and Lapras ex. The symbol prompt recovers the
first two, but introduces more false rare classifications. It was **not deployed**.

## Recommended next step

1. Route ambiguous decisions to **Needs review**, while retaining the raw D3
   choice and probabilities. On this batch, marking a decision for review when
   its largest rarity probability is below 0.40, or its choice is `uncertain`,
   sends 13 of the 53 evaluable cards for review. The remaining 40 are all correct.
   This threshold was inspected after seeing this dataset's errors: it is an
   exploratory result, **not** independent calibration or a guarantee. Test it
   on new editions and real photographs before making it the default.
2. Let the existing agent verify those cases against their exact catalogue
   references. Display **model assessment** and **catalogue verification**
   separately, rather than replacing a prediction and counting it as D3 accuracy.
3. Keep printed rarity separate from collector value. Illustrator belongs in
   review; a collectible Birthday Pikachu can have a Common printed rarity.
   A rarity-first gallery is not a price or investment ranking.
4. Evaluate a full-card image plus a sharp close-up of the rarity symbol on a
   separate test set. This is a proposed experiment; no image preprocessing or
   smartphone-image benchmark was performed here.

The default prompt and production review rule remain unchanged.

## Evidence

- `fresh-analysis.json`: current prompt predictions, probabilities and errors.
- `symbol-prompt.json`, `symbol-analysis.json`: tested alternative and predictions.
- `summary.json`: metrics, exploratory review policy and LLM recovery.
- `app-live.json`, `app-fixtures.json`: live and isolated browser checks.

Reproduce the baseline with `examples/card-scout/prepare_assets.py`, then
`examples/card-scout/analyze_batch.py --url http://127.0.0.1:18094 --output /tmp/card-analysis.json`
while port-forwarding `service/d3-mini` port `18094:8080` in `decision-serving`.
Use `e2e/card-scout-browser.cjs` with `APP_URL` set to the application route for
the browser checks. The browser uses `--use-mock-keychain`.
