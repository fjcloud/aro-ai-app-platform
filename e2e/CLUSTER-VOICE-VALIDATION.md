# Cluster Voice validation — 2026-10-27

**Status: implementation and local integration verified; live ARO acceptance pending.** The previous lab API no longer resolves. A replacement environment is being provisioned. No new model, GPU, native Playground or MLflow UI success is claimed in this record.

Changes are isolated in `feature/cluster-voice-agentops` in the workshop and app-template repositories. The published workshop and the existing template branch remain unchanged until coordinated validation/merge.

## Passed locally

- 28 Python workshop helper tests, including runtime identity, monitoring Role binding, separate MaaS key, workspace experiment and non-destructive MCP catalog registration.
- 13 Go test functions with race detection; `go vet ./...` passes. Controlled HTTP servers exercise Whisper framing, upload limits, MaaS tool loops, history validation, tool-call budget, catalog-only PromQL, bounded trends, missing/partial data, TLS verification, redirect rejection and token rotation.
- A real MCP SDK client discovers and calls the reference server's tools over Streamable HTTP; extra tool arguments are rejected.
- Real OTLP protobuf requests reach a local test receiver with the workspace/experiment headers and refreshed bearer authentication. This validates exporter protocol, not ingestion by the native Red Hat MLflow deployment.
- Chromium browser integration uses `--use-mock-keychain` and a synthetic spoken fixture. The real Go app captures microphone clips, displays editable text, waits for explicit submission, executes an agent/tool round trip, renders evidence and a trace ID, accepts a typed follow-up, handles an upstream outage and releases microphone tracks. Upstream Whisper, MaaS and Thanos are simulated. Last run: 2 speech requests, 5 model requests, 2 monitoring queries.
- 16 ARO template checks pass, including the app-image guard with the OAuth sidecar, restricted-v2, loopback listener, secret mounts and HTTPS probes.
- Antora guide builds; 13 instruction pages, 43 shell blocks and 367 local rendered links validate.

## Live acceptance still required

1. Run platform setup on the new lab, including native MLflow, then developer and Cluster Voice preparation. Confirm the cluster's OAuth proxy ImageStream digest imports successfully and the actual MLflow endpoint/experiment APIs match this release.
2. Build the same personal app repository with Tekton and deliver the matching Git revision through personal Argo CD. Check restricted-v2 admission, image digests and both container readiness states.
3. Verify unauthenticated users cannot read the app's health questions or MCP results; log in through OpenShift and verify an allowed participant can use both.
4. Run `python3 e2e/check-cluster-voice.py` for real MaaS tool calling and internal monitoring evidence. Check DeepSeek's `reasoning_effort=none` plus required first tool choice through the deployed external provider.
5. Run browser microphone checks against the deployed app using private authenticated browser state. Compare returned evidence with Observe → Metrics at the same query time.
6. Find the displayed trace ID in the native MLflow experiment and inspect model/tool children and the linked audio clip. No raw audio, prompts or credentials should appear.
7. In the native Gen AI Playground, configure DeepSeek via MaaS, authorize the OpenShift-Health MCP connector and observe a real query_metric call. Verify custom endpoint and token handling on this installed OpenShift AI version.
8. Evaluate the Lab 3.2 failure/missing/stale-data and mutation-refusal cases. Local protocol fixtures do not assess real LLM behavior.

## Implementation references

- OpenShift 4.21 monitoring access is enforced against `prometheuses/api` on `openshift-monitoring/k8s`. `cluster-monitoring-metrics-api` is a **Role** in that namespace: [release-4.21 monitoring operator role](https://github.com/openshift/cluster-monitoring-operator/blob/release-4.21/assets/cluster-monitoring-operator/cluster-monitoring-api-role.yaml), [Thanos authorization](https://github.com/openshift/cluster-monitoring-operator/blob/release-4.21/assets/thanos-querier/kube-rbac-proxy-web-secret.yaml).
- The Playground supports MCP tools and custom endpoints as Technology Preview: [Red Hat prerequisites and MCP configuration](https://docs.redhat.com/en/documentation/red_hat_openshift_ai_self-managed/3.5/html/experimenting_with_models_in_the_gen_ai_playground/playground-prerequisites_rhoai-user).
- Native MLflow has namespace-scoped integration roles; the small SQLite/PVC configuration is a workshop deployment: [Red Hat MLflow setup](https://docs.redhat.com/en/documentation/red_hat_openshift_ai_self-managed/3.5/html/working_with_mlflow/installing-mlflow_mlflow).
- Go traces use MLflow's OTLP endpoint and experiment/workspace headers: [MLflow OpenTelemetry integration](https://mlflow.org/docs/latest/genai/tracing/app-instrumentation/opentelemetry/).
- DeepSeek V4 thinking mode rejects required tool choice; the app explicitly requests non-thinking mode: [DeepSeek API](https://api-docs.deepseek.com/api/create-chat-completion/). Foundry compatibility remains a live gate.
