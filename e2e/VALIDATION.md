# Full live ARO workshop — 2026-10-22

**Acts 0–3 passed after the corrections below**, on the assigned RHDP environment `vcdhr`. This validates the normal workshop; the experimental OpenCode companion was not installed.

The cluster is **classic ARO 4.21.22**, with three `Standard_D4s_v3` CPU workers in East US. All six nodes are Ready; all 33 cluster operators are Available and none are Degraded. No cluster upgrade, additional worker, GPU, or new cloud identity was needed.

| Area | Live evidence |
|---|---|
| Cloud Terminal | Published GitHub Pages launcher retrieved the catalog assignment and embedded the existing Azure WeTTY terminal. `oc whoami` and `oc get nodes` worked inside the iframe. |
| Access | Assigned Azure service principal and existing cluster administrator login worked. Corrected `make onboard` passed on 4.21.22. |
| Foundry | Created `arodeepseekvcdhr` in the assigned resource group, East US 2; deployed `DeepSeek-V4-Pro`, version `2026-04-23`, as `deepseekv4pro`. Real completions succeeded and an invalid key was rejected. |
| Throughput | Capacity 1 produced HTTP 429 on longer prompts. After resizing and propagation, a 12,010-token prompt returned HTTP 200. Final inference headers confirmed 500,000 tokens/minute at capacity 500. |
| Operators | Dev Spaces 3.30.2, DevWorkspace 1.0.0, GitOps 1.22.2, Pipelines 1.24.1 and the two workshop Tasks installed successfully. |
| Workspace | Real Dev Spaces factory launch, repository trust, VS Code workspace trust, editor and browser terminal checked. OpenCode 1.18.14, authenticated `oc`, GitPop and injected Foundry credentials were available. |
| Native OpenCode | TUI displayed the Foundry model. Plan, build/deploy, ship-a-change and all three audit prompts ran against DeepSeek from the real workspace. `/exit` returned to the shell. |
| Initial delivery | Generated and compiled the Go app, pushed to a personal GitPop repository, built the container through Tekton, and deployed through the participant's own Argo CD. |
| Iteration | A second successful PipelineRun shipped the styled card and correct 🥠 emoji. Go AST inspection counted 15 initial fortunes and 25 after the change. The running image digest changed. |
| GitOps | Workspace HEAD, pushed `main` and Argo CD revision matched `d612c7647147545708e1998d1114b506fe8cb56f`. Application Synced / Healthy; source committed with no unstaged changes. |
| Audit | Requests 50m/64Mi, limits 200m/128Mi; readiness and liveness probes at `/healthz:8080`; non-root pod, RuntimeDefault seccomp, dropped capabilities, no privilege escalation; edge TLS with HTTP redirect. |
| Browser | Updated Fortune Cookie rendered correctly over public HTTPS. Personal Argo CD administrator login showed Synced / Healthy. Chromium always used `--use-mock-keychain`. |
| Act 3 | The guide's real request returned 10 input and 12 output tokens. Azure Retail Prices API confirmed the documented Global rates. Deleted the workshop Foundry deployment and verified its absence; the application still returned `{"status":"ok"}`. |
| Checks | `platform-run.sh`, `dev-run.sh` against the real workspace with `EXPECT_UPDATED_APP=true`, and `usage-run.sh` exited 0 before cleanup. All 35 documented Bash blocks passed syntax checks; Antora generated without warnings. |

Successful PipelineRuns: `build-app-bdfwk` and `build-app-2mkxs`.

## Corrections validated

- Accept assigned `*.azure.redhatworkshops.io` bastions in both catalog URL validation and the frontend frame policy. The relay and frontend fix were published and tested live.
- Allow the Foundry-backed ARO workshop on 4.21. Requiring 4.22 blocked a compatible environment; the three operators used here support this cluster.
- Allocate enough Foundry throughput for coding conversations. Azure control-plane provisioning can finish before inference rate limits change; allow up to 15 minutes for propagation.
- Increase workspace startup time from 300 to 900 seconds. The first cold launch hit the old deployment deadline; the restarted workspace used the corrected 900-second deadline and reached Running.
- Copy the injected platform configuration into the project and commit it. This replaces the long manual JSON generator and keeps the Foundry model in the personal repository.
- Explicitly ask the model to deploy through AGENTS.md. The original “Implement the plan” generated and compiled files but did not ship them.
- Wait for the pushed Git revision after requesting an Argo CD refresh. Immediately after a push, Synced / Healthy can still refer to the previous commit.
- Keep the platform shell in the workshop repository when reviewing AGENTS.md; explicitly return there before loading `state/foundry.env` in Act 3.
- Preserve literal shell variables in rendered explanations, use strict HTTPS curl checks, correct the OpenCode exit instruction, and replace the inherited Qwen/MaaS validation assumptions with Foundry checks.

## State left for follow-up tests

The cluster, platform, workspace, application and personal Argo CD remain. The Foundry account remains, but **`deepseekv4pro` was deleted as required by Lab 3.1**. Recreate the deployment with Lab 1.1 before making further model requests.

GPU checks were read-only: East US reports 512 vCPUs of NCASv3 T4 family quota with none used; A100 and H100 family quotas are zero. Quota does not prove VM capacity or a successful GPU launch. No GPU was provisioned, and no in-cluster model was tested. This classic ARO cluster uses GPU MachineSets; the existing `make gpu` helper targets separately provisioned ARO HCP clusters.
