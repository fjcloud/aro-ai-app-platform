# Signal Desk / Techmeme terminal validation

The current experiment is a compact English terminal: fetch Techmeme, summarize,
rank BUY / SHORT / NO SIGNAL, then investigate an index-proxy thesis and a priced
paper scenario. The previous general briefing UI and its live validation are
historical; see commit `a74c28b` for that version.

## Verified locally

- Go race tests and vet pass. Existing workshop regression checks remain intact.
- Actual public-data checks retrieve 12 Techmeme items and timestamped USD
  reference quotes for QQQ, SPY, SMH and XLK. See
  [network-check output](results/techmeme-terminal/public-data.txt).
- Publisher URLs are extracted from RSS HTML. Techmeme fragment permalinks are
  normalized. Unreadable publishers use a disclosed RSS excerpt rather than a
  fabricated full-article summary.
- Quote identity, currency, finite positive price and timestamp are validated.
  Missing, future or stale references cannot produce priced scenarios. The model's
  own quote fields are discarded and rebound to actual tool evidence.
- Long and short target/stop arithmetic, invalid values, source-ID stability,
  preview isolation, native Responses function continuation and model-input
  coverage have meaningful regression tests.
- `make test-terminal` installs its pinned browser dependencies and runs a complete
  local HTTP + browser workflow. **Inference, news and quotes in this end-to-end
  harness are explicit fixtures.** It covers three signal categories, investigation,
  editable scenario math, matching Markdown export, neutral-signal gating, mobile
  layout and JavaScript errors. Chromium always uses mock-keychain.
- The separate local preview uses fictional data, labels it prominently and refuses
  live model execution. It does not fabricate a working OpenShift AI connection.

Evidence: [fixture workflow](results/techmeme-terminal/fixture-e2e.json),
[fixture screen](results/techmeme-terminal/fixture-e2e.png),
[preview checks](results/techmeme-terminal/preview.json), and
[preview screen](results/techmeme-terminal/preview.png).

## Financial interpretation

The app proposes generic research theses for ETF proxies, not directly tradable
indices. Entry is a dated provider reference, not an executable bid/ask. Target,
stop and horizon are hypothetical assumptions. Conditional gross P&L is computed
from those assumptions and the selected notional; it is not an estimated expected
return or probability-weighted forecast. Costs are excluded and the stop-case loss
is not a maximum-loss guarantee. The signal score is not a target-hit probability.
There are no trading or client-messaging tools.

## Live OpenShift validation pending

The previous tfclk ARO instance no longer resolves. The user is recreating the
cluster and requested local development meanwhile. This version has **not** been
validated against the replacement native OpenShift AI service or T4 and is not
claimed as deployed. When the new lab is ready, reconnect the prepared endpoints,
renew the MaaS credential and rerun real inference, traces, image/revision and
persistent-storage checks. Keep the existing restricted-v2, OAuth and Git → Tekton
→ Argo CD contract.
