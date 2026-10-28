# Signal Desk validation

Experiment: `feature/briefing-desk`, based on the archived Sky Control experiment.
The published workshop and ROSA path remain unchanged.

The English app keeps the technical name `live-transcript`. Public BBC/France 24
RSS feeds supply news; AFP access is not required. The configured native OpenShift
AI Responses service and its MaaS DeepSeek model handle retrieval, summarization
and on-demand research. The existing local d3-mini on the single T4 classifies
financial relevance against editable fund mandates.

## Actual integration findings

The installed native API requires MCP credentials in `authorization`, not a custom
Authorization header. Its MCP path then stalled in this lab. The final reference
uses the same native Responses API's standard function-call protocol: the backend
executes only the declared read-only tools, passes actual results back to OpenShift
AI, and shows each execution. It exposes no extra public or internal tool port.

The provider sometimes returns prose followed by a JSON code block. The adapter
accepts one explicit JSON object/block, rejects ambiguous or invalid output, and
then validates its contents. It does not manufacture a successful response.

The model also changed characters when copying news URLs. Headline selection now
uses opaque IDs resolved by the backend; article reads use `original` or a source
ID returned by a search tool. The model cannot supply arbitrary URLs to its tools.

## Automated checks

- 33 workshop Python checks and all five Go modules pass with the race detector.
- The new module passes Go vet; all 17 template security/image/storage checks pass.
- Meaningful tests cover SSRF/reserved addresses, bounded source extraction,
  Markdown escaping, persistence and restart recovery, stale fund mandates,
  missing actual source reads, native function-call continuation, stage/source
  scoping, unexpected tool arguments, complete classification coverage,
  financial investigation gating, CSRF and structured response envelopes.
- Chromium always uses `--use-mock-keychain`.

## Live workflow

A real browser test through a local frontend connected to the lab's native
OpenShift AI service and T4 passed: current RSS selection, actual article reads,
English summaries, financial triage, multi-tool investigation, evidence references,
Markdown export, filters, mobile layout, mandate invalidation and stale-result
rejection. The source data was not mocked. The research agent used article reads,
current-feed searches and ECB FX reference data.

The final deployed-browser run and persisted MLflow/storage evidence are recorded
in `results/briefing-desk/` when validation completes.

## Practical limits

News search covers the latest 12 items from each of three configured feeds, not
the whole web. SEC tools return submission metadata for a documented list of US
companies, not filing contents or prices. ECB data is dated reference FX data,
not executable quotes. Source pages may be unavailable, incomplete or paywalled;
failed retrieval remains an explicit error. Financial hypotheses and missing data
are separate from source facts. Confidence is a model score, not a calibrated
probability of a market move. Research drafts require human review.

Articles, mandates and notes persist on a 2 Gi PVC. In-flight document jobs become
retryable failures after a restart. The MaaS runtime key lasts 24 hours; rerun the
facilitator preparation helper and restart the app to renew it. No extra GPU,
privileged SCC or cluster-monitoring permission was added.
