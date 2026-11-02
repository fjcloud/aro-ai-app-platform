# Fresh ARO workshop review — 2 November 2026

**Passed after corrections.** The Card Scout participant journey was exercised
on [ARO lab 54w5q](https://bastion.54w5q.azure.redhatworkshops.io/showroom/),
starting with the base cluster, using the guide's CLI commands and a real Dev
Space. OpenShift was 4.21.22. These are dated results; lab URLs expire.

- [Generated application](https://signal-desk-signal-desk-dev.apps.ijtxiwk6.eastus.aroapp.io)
- [Generated source](https://github.com/fjcloud/go-app-template/tree/generated/card-scout-54w5q)
- [Verified MLflow trace](https://rh-ai.apps.ijtxiwk6.eastus.aroapp.io/mlflow/#/experiments/1/traces?selectedEvaluationId=tr-cf43924d96ff76ce6c83d21f25087ba5&workspace=signal-desk-dev)

## What ran

| Participant stage | Actual validation |
|---|---|
| Sign in and operators | Bastion onboarding and operator CLI commands; RHOAI 3.5.2, Dev Spaces 3.31.1, Pipelines 1.24.3, GitOps 1.22.2, NFD and GPU Operator ready. |
| Capacity | Added one Standard_D8s_v3 CPU worker and one Standard_NC8as_T4_v3 GPU worker. Both Ready; one allocatable GPU. Worker setup took about 12 minutes. |
| AI platform | DSCI/DSC, MLflow, MaaS, gateway and native Playground backend ready. Explicit MLflow Available wait verified. |
| DeepSeek | Created Azure Foundry DeepSeek-V4-Pro, version 2026-04-23, GlobalStandard capacity 500; verified real completions through MaaS. Coding and runtime subscriptions have distinct keys and counters. |
| D3 | Real InferenceService on one T4, pinned native runtime image and checkpoint; cold readiness about six minutes. No public model Route or participant model key. |
| Developer APIs | All four Lab 2.2 command blocks executed in the actual Dev Space: model metadata, Pokémon image, non-Pokémon control, MaaS Chat and native Responses with a matching MCP tool result. JSON was inspected with jq. |
| Generate and deliver | OpenCode 1.18.14 and DeepSeek generated the Go app from the empty starter and contract. Plan, build, filter-persistence exercise and review corrections ran in the real workspace. Four successful Tekton deliveries and Argo CD reconciliation; final workspace, pushed main and Argo revision match. |
| Application | Public UI/API, complete 51-image batch, filters, pagination, saved filter, real price research, exact structured prices/source/date, trace link and 390px mobile view passed. |
| Agent Playground | Actual OpenShift AI UI, real DeepSeek response and one read-only get_card_market_data tool call; no connector authentication. |
| Traces | Persisted parent/child spans include model and MCP inputs/outputs; the MLflow graph opens in the browser. No credentials or image-byte payloads detected in the saved trace. |
| Observability | All nine dashboard queries returned series; dashboard selected and rendered through the OpenShift console UI. Fresh image and agent requests verified rather than relying on an already completed batch. |
| Quotas | Temporary probe returned 200 then 429; its key was revoked and temporary subscription removed. Missing/invalid credentials and native streaming protocol also checked. |

`execution.json` records command-stage results. The 18-second platform entry is
an **idempotent replay**, excluding initial provisioning and image pulls. Dev
Spaces' initial multi-GB images took several minutes; a starting screen alone
was not treated as a failure. Native Playground backend creation was performed
by CLI; its UI was used to test the agent.

The earlier CPU experiment was removed: its namespace, MachineSet, machine,
node and experiment files. The CPU worker used above is the workshop's separate
worker and remains deployed.

## Corrections made during the review

The original sequence did not pass unchanged. The guide and starter now wait
for MLflow and Argo CD availability, cover the actual Dev Spaces consent/trust
screens, select DeepSeek explicitly and clear the template Git upstream before
initializing the personal repository. The generated application's race tests
need CGO enabled; production builds remain CGO-free.

Actual application review found an HTTP client initialization race, unintended
redirect following, overly permissive routing, tests that duplicated routing
instead of exercising it, hidden selected results and fragile inline click
handlers for names containing apostrophes. DeepSeek corrected these through
OpenCode. A final array-lookup regression was caught by the error-state browser
test and corrected before acceptance. Sixteen generated-app Go tests, race
detection, vet and build passed. The contract now makes these behaviors explicit;
it still contains no completed application source.

Extended generation and repeated review exhausted the previous 5-million-token
coding allowance. The workshop coding policy is now **10 million tokens per
24 hours**, and the existing key resumed working. The runtime remains at
1 million per 24 hours; Foundry capacity remains 500,000 tokens/minute. This is
a cap, not measured consumption or a guarantee every generation fits it.

Old finance-era acceptance checks and wording were aligned with Card Scout.
Python test dependencies are explicit in CI and local setup. Browser assertions
were adapted to this newly generated UI without replacing live services with
fixtures. The real app and image checks were rerun after the final delivery.

## Model and data limits

| D3 result | Observed |
|---|---:|
| Images completed / failed | 51 / 0 |
| Card-kind decisions correct | 51 / 51 |
| Rare/non-rare decisions correct | 45 / 48 Pokémon cards |
| Common/uncommon/rare-or-higher decisions correct | 40 / 48 Pokémon cards |

Held-out labels were read after inference. Catalogue identity and market prices
come from the prepared dataset's exact TCGdex reference, not D3 visual identity.
This small set of clean catalogue images does not establish accuracy on phone
photographs. `rare_or_higher` includes ordinary Rare; the agent's explanatory
prose can misinterpret that category even when its structured catalogue fields
and prices are correct. Treat those fields and the source as evidence, and the
prose as model output. Market references are dated aggregates, not sale offers.

## Evidence

- `generated-app.json`: coding model, final revision, successful pipelines, image digest and security checks.
- `d3-models.json`, `analysis-summary.json`: actual model metadata and batch results.
- `app-live.json`, `app-desktop.png`, `app-mobile.png`: real deployed browser flow.
- `app-fixtures.json`: isolated browser error/empty/zero/null/escaping/link tests. These fixtures never entered the live platform or dataset.
- `platform-research.json`, `persisted-trace.json`, `trace-link.json`, `mlflow-graph.png`: real research, stored spans and native graph.
- `playground/playground.json`, `playground-current.json`: live model and native MCP UI checks.
- `metrics.json`, `console-dashboard.png`: observed dashboard data and UI.
- `devspaces-starting.png`, `devspaces-opencode.png`: actual workspace startup and OpenCode session.

Local regression checks passed: 45 helper tests, six prepared-service tests,
five retained Go example suites with race detection, terminal browser fixtures,
Antora build and navigation through all 16 guide pages. This does not claim
the retired finance, speech or game exercises were deployed in this lab.

The application and builds use restricted settings: no extra privileged SCC
grants, no model key or Kubernetes token mounted into the generated UI pod,
and build tasks run in Linux user namespaces. No credentials or raw terminal
logs are published with this evidence.
