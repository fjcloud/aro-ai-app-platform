# ARO configuration review — vmffl

Validated November 5, 2026 against a fresh RHDP classic ARO lab. This is a
configuration validation, not a claim that the entire generated-application
workshop was rerun.

Follow-up: the [fresh 7vxhk participant run](../aro-7vxhk/README.md) found that
Dev Spaces derives invalid component names when creating a workspace from the
digest-pinned OpenCode injector. The current manifest uses a versioned tag for
the same image. The earlier image-pull check below did not establish fresh
factory creation with that digest reference.

## Environment

- OpenShift 4.21.22; OpenShift AI 3.5.2; OGX 1.2.5.1+rhaiv.0.
- Dev Spaces 3.31.2; OpenShift Pipelines 1.24.3; GitOps 1.22.3;
  Connectivity Link 1.4.3; NVIDIA GPU Operator 26.7.2.
- Three original D4 workers plus one D8 CPU worker and one NC8as T4 v3 worker.
- Foundry DeepSeek-V4-Pro 2026-04-23, GlobalStandard in East US 2.
- Isolated kubeconfig and named workshop-admin identity. Browser tests used
  Chromium with `--use-mock-keychain`. Credentials are excluded from this record.

## Validated changes

| Hypothesis | Live evidence | Instruction/configuration change |
| --- | --- | --- |
| Gateway dependencies can be prepared before Connectivity Link | GatewayClass and EnvoyFilter CRD established before subscriptions; Kuadrant operator restart count remained zero; actual MaaS key mint and completions succeeded | Prepare GatewayClass in Lab 1.1; remove automatic controller recycling from installation and key minting |
| Named identity works for the platform | workshop-admin CLI cluster-admin check, Dev Spaces browser login and OpenShift AI Playground login succeeded | Add preserving HTPasswd setup in Act 0 and use that identity in Lab 2.1 |
| MaaS key state can survive pod replacement | PostgreSQL 1 GiB managed-csi PVC Bound; rollout restart succeeded; subsequent native research completed with the previously prepared runtime key; rerunning credential helper left both Secrets unchanged | Generated/reused credentials, PVC and Recreate strategy; no in-place migration from an old ephemeral database |
| OGX can use native startup and verified outbound TLS | No command/args override or VLLM_TLS_VERIFY=false; native SSL_CERT_FILE injection; DeepSeek plus actual get_card_market_data MCP call succeeded from platform, Dev Spaces and Playground | Reference odh-trusted-ca-bundle through spec.tls.trust.caCertificates; label the watched bundle; keep normal image startup and operator-managed 1 GiB storage |
| Unneeded nested-container and GPU UI settings can be removed | Simplified CheCluster Active; real workspace Running; OpenCode 1.18.14 and gitpop available; UI Deployments available without GPU tolerations | Remove unused Che nested-container configuration and GPU tolerations; dashboard helper patches only supported dashboard flags |
| Installed Red Hat tasks can replace handwritten clone/build scripts | operator-context-probe-gr7tl and canonical-tasks-8rzcb succeeded: real template clone, fixture Dockerfile build and internal-registry push | Derive workshop tasks from installed git-clone/buildah, changing only workspace mount and initial workingDir; pass CONTEXT=/source in ARO template |
| OpenCode injector can be fixed to a reproducible image | Real workspace injector pulled and started with multiarch digest 11be643a79b29935ca0b7a8835201fce165dbe6aca646cec8dd05c742d43ee5b | Pin injector image digest |

The real Dev Spaces CLI identity was
`system:serviceaccount:card-scout-dev:workshop-developer`: it could create app
Deployments and PipelineRuns, but could not use privileged SCCs, create
ClusterRoleBindings, read model Secrets or patch the inference service.

All four Lab 2.2 shell blocks ran verbatim in that workspace. Five JSON responses
were verified: CUDA/FP16 model discovery, Charizard/star, Basic Energy/no symbol,
DeepSeek Chat Completions and native Responses with exact catalogue/source MCP
evidence. Full prepared batch: **105 completed, 0 failed**. This is execution
success, not a claim of 100% visual classification accuracy.

The Playground selected the runtime subscription and completed a real Charizard
MCP lookup without a token for the public MCP service. Its result is recorded in
`../card-scout/playground-current.json`.

A new research after PostgreSQL replacement returned trace
`tr-b9dfba406265d13293b21860097c51bd`. The exact MLflow link uses experiment 1
and workspace card-scout-dev. A real browser opened that exact trace and verified
`card.research`, `agent.responses` and `tool.get_card_market_data` in the native
MLflow graph. No model response or credential is needed in this record.

## Rejected hypotheses and retained adaptations

- MLflow `serveArtifacts: false` was rejected by the installed 3.5.2 CRD CEL
  validation for both local file storage variants tested. Keep the working
  SQLite/PVC profile with `serveArtifacts: true`. This is a single-instance
  workshop profile, not a highly available production database design.
- Removing vector_io/storage providers caused builtin Responses startup to fail.
  They remain required runtime dependencies even though this app does not use RAG.
- Typed native provider generation produced `inline::responses`, rejected by this
  installed runtime; automatic OCI base-config discovery returned registry 401.
  Keep the explicit working overrideConfig rather than shipping these failures.
- Unmodified vendor task workspace/CWD failed to persist or locate files on this
  ARO user-namespace setup. The small task adapter is retained. With workingDir=/,
  CONTEXT=. is also wrong; explicit /source fixes the build. Red Hat images,
  scripts, params and security settings remain unchanged.

## Scope limits

The build probes used a small, explicitly test-only Dockerfile, not an app
copied into the learner repository. Card Scout frontend generation and full Argo CD
app delivery were not repeated in this configuration review. Immutable app image
tags and ROSA-specific Qwen/L40S changes were not validated or introduced here.
The app and D3 auth choices remain the existing workshop profile.

Local validation: 46 workshop helper tests, 14 prepared-platform tests, 18 ARO
template tests; all passed. Kustomize rendered operators/instances/agent; instances
also passed server dry-run. All documented shell blocks parsed and Antora built.

## Documentation consulted

- [OGX deployment and security](https://docs.redhat.com/en/documentation/red_hat_openshift_ai_self-managed/3.5/html/working_with_ogx/ogx-security-guide_rag)
- [Pipelines tasks and cluster resolver](https://docs.redhat.com/en/documentation/red_hat_openshift_pipelines/1.24/html/creating_cicd_pipelines/creating-applications-with-cicd-pipelines)
- [OpenShift authentication](https://docs.redhat.com/en/documentation/openshift_container_platform/4.21/html-single/authentication_and_authorization/authentication_and_authorization)

Version-specific conclusions above were checked against actual installed CRD
schemas and real reconciliations, including the negative MLflow result.
