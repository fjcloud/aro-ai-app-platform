# Cluster Voice validation — 2026-10-27

**Status: passed on the live ARO lab `tfclk`.** The same `live-transcript` Go application now supports voice questions, read-only cluster metrics and MLflow traces. Its Git → Tekton → personal Argo CD delivery path remains intact.

## Environment and delivery

- ARO 4.21.22; OpenShift AI 3.5.2; MLflow 3.14.0; Dev Spaces 3.31.0; Pipelines 1.24.2; GitOps 1.22.2.
- One `Standard_NC8as_T4_v3` worker, one physical Tesla T4. RedHatAI Whisper INT8 serves speech through an internal OpenShift AI endpoint. Observed GPU memory: 7.949 GiB.
- DeepSeek-V4 Pro on Azure AI Foundry is exposed through OpenShift AI MaaS. Coding and runtime have separate subscriptions and keys. The deployed provider accepts required tool choice with `reasoning_effort=none`.
- OpenCode 1.18.14 ran in the actual Dev Space through MaaS and implemented the CPU-over-one-hour shortcut exercise.
- Final personal app revision: `2782b6b7f001c2db6be9f1c10ba3fc7cb1464de8`. The workspace was clean and matched remote Git and Argo CD.
- PipelineRun `build-app-cz87n`: Succeeded. Application: Synced / Healthy. Running app image: `sha256:d5c33d2d17416983550c686f21a1533fe593f4168627ac807600ee26cde0897c`.
- Both app containers Ready. Runtime SCC: `restricted-v2`. Builds use `pipelines-scc` and Linux user namespaces (`hostUsers: false`), without privileged containers.

## Live acceptance

| Check | Observed result |
| --- | --- |
| Platform | Operators and workers ready; real Foundry completion through MaaS; invalid key rejected; coding/provider keys separate; effective provider capacity 500,000 TPM |
| Speech | RedHatAI model ready on exactly one T4; no public speech Route; network isolation and real transcription pass |
| Delivery | Actual Dev Space identity and scoped permissions, clean pushed source, successful Tekton build, matching Argo revision and running image, OAuth, probes and restricted admission pass |
| Runtime agent | Real node-readiness, CPU one-hour trend and GPU-memory questions produce monitoring evidence with units and timestamps |
| Browser voice | English and French microphone capture → real Whisper → explicit question submission → real MaaS tool calling → cluster evidence pass |
| Access control | Unauthenticated app/chat access denied; authorized OpenShift login works; MCP uses a dedicated Service-reader token; that identity cannot read Secrets or create pods |
| Native Playground | Existing MaaS catalog model, runtime subscription and OpenShift-Health connector work together; three tools discovered; actual `query_metric` CPU trend result shown |
| Native MLflow | Application launcher opens MLflow; workspace/experiment overview displays trace/token metrics; persisted Go trace contains AGENT, LLM and TOOL spans and a link to the speech trace |
| Observability | All nine dashboard expressions return series, including separate coding/runtime usage, Whisper latency and GPU memory |
| Behavioral evaluation | A request to delete pods is refused; an unsupported metric is identified without inventing its value. Actual lab alerts are reported rather than hidden behind a blanket healthy answer |

The final French voice test used prompt `cluster-health-v2`: first transcript in 4.309 seconds, two Whisper clip responses in 340 ms and 188 ms. The English test returned its first text in 3.303 seconds. These are single-run observations with synthetic microphone fixtures, not performance guarantees. Permission denial, upstream speech failure and microphone release also passed.

The French voice trace `98dadf6f639b73f9b5f03c9a01208d69` is stored in MLflow as `tr-98dadf6f639b73f9b5f03c9a01208d69`. The native browser check reads its actual stored spans: `cluster.question`, `llm.chat`, `tool.cluster_health`, `llm.chat`, plus the linked speech trace. Trace payloads omit audio, question/answer text, tool results and credentials. Model token usage and the prompt version are retained.

## Corrections validated on this lab

- Wait for the Authorino Deployment on fresh installations before setting its TLS environment.
- Use the native MLflow integration-role name, wait for the runtime MaaS subscription, and enable artifact serving as required by the operator's `file://` configuration.
- Enable the TrustyAI component so the installed native Playground backend can discover its guardrail CRDs. No guardrail inference instance or extra GPU is created.
- Reuse the existing MaaS model in the Playground; no custom endpoint or provider key copy is needed.
- Give the Playground a dedicated short-lived identity restricted to reading the app Service.
- Replace the MCP SDK's localhost-only check behind the loopback OAuth proxy with exact public Host and Origin checks. Requests with an unrelated Host or Origin are rejected.
- Require a clean, committed and pushed app source before Tekton, and verify its revision through Argo CD. Historical trend samples are distinguished from stale current data.

## Local regression checks

31 Python workshop tests, 14 Go test functions with race detection, `go vet ./...`, and 16 ARO template checks pass. Controlled HTTP tests cover bounded agent/tool loops, metrics failures, missing data, TLS verification, token rotation, OTLP export and MCP Host/Origin handling. A separate Chromium/Go integration test uses simulated upstreams; it is distinct from the live checks above. Every Chromium launch uses `--use-mock-keychain`.

Antora builds successfully; static validation covers 13 instruction pages, 43 shell blocks, 17 generated HTML files and 367 local rendered references.

## Limits

- Short speech clips provide near-real-time transcription, not native Whisper WebSocket streaming.
- Playground/external MaaS functionality is Technology Preview in this OpenShift AI release. Its MCP call was verified in the native UI; its internal backend traces are not claimed to be exported to the app's MLflow experiment.
- The single-replica SQLite/PVC MLflow deployment is for this disposable workshop, not production HA. MLflow's “No cost data available” / `$0.00` display does not establish inference cost; inspect MaaS usage and Foundry billing separately.
- The model can phrase an answer imperfectly. Returned measurements, timestamps, errors and gaps remain the authoritative evidence. Local failure/stale-data fixtures and the two live behavioral cases do not establish correctness for every possible prompt.
