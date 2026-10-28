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

## Live local models

The active development backend uses **Azure DeepSeek-V4-Pro**, version
`2026-04-23`, matching the workshop, and the actual pinned `vllm-sr/d3-mini`
on Apple Metal/MPS in FP16. CPU fallback is disabled. MPS is
marked **unvalidated upstream**; these are local functional checks, not numerical
parity or financial-performance validation.

- [Azure deployment](results/techmeme-terminal/azure-deployment.json): Global
  Standard, deployment `deepseekv4pro`, provisioned on the principal Azure account
  using `az`. Its key was obtained through the CLI and stays in a private file.
- [Azure live browser run](results/techmeme-terminal/azure-live.json) and
  [screen](results/techmeme-terminal/azure-live.png): a new, empty feed through
  actual DeepSeek summaries and Metal classifications. All three stories returned
  NO SIGNAL and remained gated. Techmeme's headline link is preferred over author
  attribution links, including X profile links.
- [Azure research probe](results/techmeme-terminal/azure-research.json): real
  article/excerpt reading, Techmeme search and market snapshot, followed by a
  validated neutral report. Azure's Chat Completions endpoint explicitly disables
  thinking; the adapter preserves the common source/tool/evidence validation.
- [Metal probes](results/techmeme-terminal/local-metal.json): three explicitly
  synthetic directional inputs, complete input coverage and actual model answers.
- Earlier optional Groq validation: [live browser run](results/techmeme-terminal/local-live.json) and
  [screen](results/techmeme-terminal/local-live.png): public Techmeme stories,
  Groq summaries and real local classifications, English/mobile UI, no fixture
  providers. The sample initially returned NO SIGNAL for all three stories, so
  the UI correctly refused investigation. Publisher paywalls were disclosed as
  RSS excerpt coverage. The linked evidence records the latest run.
- [Direct research probe](results/techmeme-terminal/local-research.json): Qwen
  called `read_article`, `search_news` and `market_snapshot` on a real imported
  article and returned a schema/evidence-validated report. This independent
  backend check did **not** change a classification or bypass the app's NO SIGNAL
  gate. A transient Groq cooldown was honored on the same model and account.
- Regression tests cover Azure/Groq authentication, tool continuation, final
  JSON mode, provider/model mismatch, quota errors, cooldown cancellation and
  credential redaction, Azure truncation rejection and RSS attribution links.
  The OpenShift AI transport remains the deployment default.

With the live local app and Metal server running, repeat the browser check using:

```sh
node e2e/briefing-desk-live-local.cjs
```

It fetches live news if the feed is empty and accepts the model's actual signals;
it never requires a fabricated BUY/SHORT. Model setup and credential-file
configuration are in [the app README](../examples/briefing-desk/README.md).

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
