# Foundry through OpenShift AI MaaS — live validation

Validated on 26 October 2026 against the previously empty RHDP ARO environment `mnzq5`, OpenShift 4.21.22. This report covers the new platform installation, external coding-model publication and speech coexistence; earlier end-to-end app-delivery reports remain separate evidence.

## Result

DeepSeek-V4 Pro on Microsoft Foundry is registered as an `ExternalProvider` and `ExternalModel`, published through OpenShift AI Models-as-a-Service, and available through the cluster gateway with a MaaS key. The Azure account key stays in `llm-serving` and is not injected into workspaces.

External model support is a **Technology Preview in OpenShift AI 3.5**. This is a tested workshop configuration, not a production support claim. Coding inference and its data still go to Foundry.

| Check | Observed result |
|---|---|
| Platform installation | OpenShift AI 3.5.2, Dev Spaces 3.30.2, Pipelines 1.24.2, GitOps 1.22.2, Connectivity Link 1.4.3, NFD, NVIDIA, cert-manager and LeaderWorkerSet installed |
| Readiness | DataScienceCluster and default MaaS tenant Ready; Dev Spaces Active; gateway Programmed |
| Capacity | Original three D4 workers preserved; one D8 CPU worker and one single-T4 NC8 worker added |
| Repeat installation | Final `install-platform.sh` completed successfully without disabling the LWS operator; LWS remains Managed with two manager replicas |
| Provider | `foundry` / `azure`, real account hostname, HTTPS; provider Secret uses `inference.llm-d.ai/ipp-managed=true` |
| Model publication | `deepseekv4pro` ExternalModel and MaaSModelRef Ready; access policy and workshop subscription Active |
| Credential separation | Workspace key starts with `sk-oai-` and differs from the Azure key; provider Secret has no workspace-injection label |
| Real completion | 14 input tokens and 9 output tokens through MaaS; upstream effective capacity 500,000 tokens/minute |
| Model discovery | `deepseekv4pro` present in `/v1/models` |
| Dashboard | Browser login succeeded; External models → `llm-serving` lists DeepSeek-V4 Pro / foundry / Ready; the Dev Spaces browser IDE opens the actual template |
| Authentication | No key: HTTP 401; bogus key: HTTP 403 |
| Tool use | Forced `workshop_probe` call returned the expected marker |
| Streaming and large context | 73,987-byte request accepted; six SSE events, `[DONE]`, content marker and measured token usage received |
| Quota | Isolated one-token/hour subscription produced HTTP 200, 200, then 429; temporary key revoked and subscription deleted |
| Native OpenCode | Version 1.18.14 in the real browser-created Dev Spaces workspace; project `ff276d5`, DeepSeek model and MaaS key/base URL verified |
| Agent interaction | Plan prompt returned all three delivery phases and the two model roles; a second prompt completed two `read` tool calls and correctly reported the image, Always pull policy and clone → validate-image → build order |
| Speech coexistence | Whisper INT8 Ready on exactly one T4, internal service only; independent speech resource checks passed |
| Real speech inference | A five-second synthetic English WAV sent from the real workspace to the internal Whisper service returned the expected sentence |

The quota result demonstrates asynchronous enforcement. It is not an exact token ceiling or a financial spending cap; in-flight requests can overshoot.

## Installation corrections included

- All operators now belong to Lab 1.1; operator installation no longer includes Tekton Tasks before their CRD exists. Readiness failures stop before database or platform-instance application.
- One shared DataScienceCluster enables KServe and MaaS. Lab 1.4 does not apply the old speech-only DSC that disables the AI gateway.
- ARO ingress uses the certificate named by its actual default IngressController, not a ROSA certificate-name convention.
- The ARO wildcard domain resolves to the default router. A TLS-passthrough OpenShift Route publishes the MaaS Gateway Service; a Programmed internal gateway alone was insufficient for public access.
- The provider Secret carries the label read by the installed inference payload processor. A Ready provider alone does not prove that credential injection works; a real completion is checked.
- The supported `externalModels` dashboard flag is enabled so the external-model management view is visible.
- The ARO template pins DeepSeek through MaaS directly; older workspaces can synchronize the project config from the platform. The Azure `api-key` header is removed from client configuration.
- The participant OpenCode audit lab is removed from both ARO and ROSA. Automated resource checks remain independent safeguards.

## Reproduce

From the bastion with the assigned Azure identity and OpenShift administrator user token:

```sh
bash scripts/install-platform.sh
# Follow Lab 1.2 to create Foundry and state/foundry.env.
bash scripts/configure-coding-provider.sh
oc apply -k deploy/coding
bash deploy/coding/mint-workshop-key.sh
bash e2e/platform-run.sh
python3 e2e/maas-protocol.py
python3 e2e/maas-quota.py
python3 e2e/check.py speech
```

`maas-quota.py` creates only its named validation subscription and key; cleanup runs in `finally`. The protocol probes make small billable inference calls and print no credentials. Sanitized results: [protocol](results/maas-foundry/protocol.json), [quota](results/maas-foundry/quota.json), [native OpenCode](results/maas-foundry/opencode.json), [speech](results/maas-foundry/speech.json).

Local validation: 24 helper regression tests, 10 ARO template image-contract tests, both Antora builds, all published local references and 77 documented shell blocks passed. Chromium uses `--use-mock-keychain`.

## Environment retained

The assigned cluster, CPU/T4 workers, platform, Foundry deployment, coding-model publication and Whisper service remain available for participant testing. All 33 cluster operators remained Available and nondegraded. The project remained clean after read-only OpenCode prompts. Only quota-test fixtures are removed. Use Lab 3.1 for facilitator cleanup after all participants finish.

## Sources

- [OpenShift AI 3.5 MaaS and external models](https://docs.redhat.com/en/documentation/red_hat_openshift_ai_self-managed/3.5/html/govern_llm_access_with_models-as-a-service/deploy-and-manage-models-as-a-service)
- [Payload processor provider-key label](https://github.com/opendatahub-io/ai-gateway-payload-processing/blob/main/pkg/plugins/apikey-injection/reconciler.go)
