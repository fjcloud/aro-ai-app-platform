# Signal Desk: memo scoring validation

The current app is a simple memo editor and news list sorted by opportunity score.
The default memo targets semiconductors and electronic components. BUY/SHORT
triage and the ticker dashboard belong to the previous revision (`84c756e`).

## Scoring contract

D3's native `score` question evaluates five ordered relevance/materiality levels.
The app validates complete input coverage, all five probabilities, their sum and
the agreement between the expected ordinal level and returned score. It normalizes
`0–4` to `0–100`. Model confidence is a separate field and is not displayed as the
opportunity score. The input contains the investment memo and source-summary facts;
generated market commentary is excluded.

Azure DeepSeek generates a concise memo-specific explanation and cites existing
source key-point IDs. It cannot change the D3 score. An explanation failure does
not discard a successfully computed score. High and low scores are research
priorities, not market direction, profit estimates or probabilities of success.
Any current scored article can launch read-only investigation.

## Verified

- Go race/vet checks and the workshop regression suite pass.
- The browser fixture workflow validates descending score order, simple English UI,
  preservation of an unsaved memo during refresh, memo edits triggering re-scoring,
  hiding old research after a memo change, research export and mobile layout.
- The active model configuration is Azure DeepSeek-V4-Pro `2026-04-23`, matching
  the workshop, with actual pinned d3-mini on Apple MPS/Metal in FP16. CPU fallback
  is disabled. MPS remains unvalidated upstream; this is functional validation.
- A live local browser run scored the saved real news, displayed memo-specific
  explanations and completed an investigation from a current scored article.
- A separate live D3 test evaluates the same real semiconductor and retail stories
  against two different memos, without changing the application's memo or scores.

Evidence:

- [Live memo-scored news](results/techmeme-terminal/memo-live.json)
- [Simple live UI](results/techmeme-terminal/memo-live.png)
- [Actual D3 sensitivity to two memos](results/techmeme-terminal/memo-sensitivity.json)
- [Fixture workflow](results/techmeme-terminal/fixture-e2e.json)
- [Fixture UI](results/techmeme-terminal/fixture-e2e.png)
- [Deployed Azure model/version](results/techmeme-terminal/azure-deployment.json)

Chromium always uses mock-keychain. Fixture providers are explicitly labeled and
are separate from live inference. The preceding BUY/SHORT and three-story records
are historical evidence of earlier versions, not the current scoring contract.

## Research and deployment limits

Publisher excerpts are labeled. Quote identity/currency/timestamps are validated;
invalid or >72-hour-old references cannot price a scenario. Detailed exported
research can contain hypothetical ETF scenarios whose gross P&L excludes costs;
no model-generated reference price is accepted and no order tools exist.

The former ARO lab is offline and its replacement is being recreated. The current
version has not been validated on replacement OpenShift AI/T4 endpoints. Keep
OAuth, restricted-v2, persistent storage and Git → Tekton → Argo CD deployment.
See [local setup](../examples/briefing-desk/README.md) for the working Mac/Azure path.
