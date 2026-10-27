# Cluster Voice reference

One Go application: microphone → Whisper → editable question → DeepSeek via MaaS → read-only OpenShift metrics → answer with evidence. The existing `live-transcript` app name, Tekton image and Argo CD delivery stay in place.

Copy **all** `.go` files, `go.mod`, `go.sum`, `Dockerfile` and `web/` into the application template. The template already contains deployment, OAuth, pipeline and GitOps configuration. Do not copy the workshop's platform scripts into the student app.

| Component | Responsibility |
| --- | --- |
| `main.go`, `web/` | Bounded WAV transcription, question review, text entry and conversation |
| `agent.go` | MaaS tool-calling loop, up to three tool rounds and four model calls |
| `metrics.go` | Catalog-only PromQL, rotating service-account token, verified TLS, bounded results |
| `tools.go` | Shared tools for the runtime agent and MCP Streamable HTTP endpoint `/mcp` |
| `telemetry.go` | OpenTelemetry model/tool spans exported to the project's MLflow experiment |

The model request explicitly sets `reasoning_effort=none`: DeepSeek V4 thinking mode does not support required tool choice. The first call requires a tool; later calls use auto selection. Confirm this combination through the deployed MaaS provider during live validation.

The runtime calls the same Go tool implementations that MCP exposes. It does not need a second connector deployment or another network hop. MCP clients, including the Playground, exercise those same tools over HTTP.

## Local verification

```sh
go test -race ./...
```

Tests use local HTTP servers for Whisper, MaaS, Prometheus and the MCP client. They validate protocols and failure handling; they do **not** prove a model, GPU or OpenShift deployment works. `e2e/cluster-voice-browser.cjs` in the workshop repository checks the browser against these local fixtures.

Running `go run .` with no agent configuration retains the microphone UI; questions return a clear configuration error. For a real cluster, follow Lab 1.6. Do not expose an unprotected local server on the internet.

| Environment variable | Purpose |
| --- | --- |
| `LISTEN_ADDR` | `127.0.0.1:8080` behind the deployed OAuth proxy; local default `:8080` |
| `WHISPER_URL` | Internal OpenShift AI endpoint, default `http://whisper-predictor.speech-serving.svc.cluster.local/v1` |
| `AGENT_BASE_URL`, `AGENT_MODEL` | MaaS `/v1` endpoint and `deepseekv4pro` |
| `AGENT_KEY_FILE` | Mounted runtime MaaS key; default `/etc/agent/OPENAI_API_KEY`; read for each call |
| `PROMETHEUS_URL` | Internal Thanos URL; no browser access or public monitoring endpoint needed |
| `SERVICE_ACCOUNT_TOKEN_FILE` | Rotating projected service-account token |
| `SERVICE_CA_FILE` | OpenShift service CA bundle; certificates are always verified |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | MLflow `/v1/traces`; absent means local trace IDs without export |
| `MLFLOW_EXPERIMENT_ID`, `MLFLOW_WORKSPACE` | Tracking experiment and project namespace |

The platform prepares `cluster-voice` with metrics API access in `openshift-monitoring`, namespace-scoped MLflow integration and OAuth delegated authentication. The application itself listens on loopback; its Service exposes only the OAuth proxy. The proxy requires permission to read the application Service. The runtime cannot run shell commands or mutate cluster resources. Metrics can contain cluster-wide names, so access to this workshop app must remain limited to its participants.

## Tools and evidence

- `list_metrics`: discover supported metric IDs, units and expressions.
- `cluster_health`: readiness, operator conditions, CPU, memory and firing alerts at one query timestamp.
- `query_metric`: one catalog metric, either now or a `15m`, `1h` or `6h` trend (at most 31 samples per series).

Prometheus queries have an 8-second server timeout, 10-second client deadline, at most 40 returned series and a 256 KiB response budget. Tool context is limited to 48 KiB. Monitoring warnings are treated as partial-data errors. Catalog expressions state fixed lookbacks: for example, `pod_restarts` remains a rolling one-hour increase even in a six-hour trend.

Each answer displays the actual query, unit, query time and Prometheus sample timestamps. Empty results are `missing`; errors remain errors. The model is instructed to flag samples older than five minutes. The evidence panel is authoritative; language-model wording still requires review. To support another metric, add a bounded expression to the server catalog and test it, then build through the same pipeline.

## Tracing and AgentOps

Each question produces an `AGENT` root span with `LLM` and `TOOL` children. Transcription is a separate trace; the question links to the last audio clip's real trace/span IDs. This makes the two stages navigable without holding a trace open while a user edits their question. Earlier clips keep their own traces.

Traces contain prompt version `cluster-health-v1`, model ID, token usage, tool names, metric IDs, status and timing. Audio, question/answer text, query result labels and credentials are not recorded. Authentication is refreshed for every MLflow export. Trace delivery is best effort and does not turn a monitoring failure into success; check the MLflow UI during the live acceptance test. The SQLite/PVC tracking configuration is for this disposable workshop, without artifact serving.

The Gen AI Playground and MaaS external model path are Technology Preview in OpenShift AI 3.5. MCP tool compatibility, OpenShift authentication, MLflow export, GPU inference and actual DeepSeek behavior require the live acceptance checks in Lab 3.2; local tests alone are insufficient.
