# ARO workshop validation

Follow Acts 0–3 in the assigned environment, including the real Dev Spaces session, OpenCode prompts, Tekton build and personal Argo CD. Then run from an authenticated bastion:

```sh
bash e2e/platform-run.sh
bash e2e/dev-run.sh
bash e2e/usage-run.sh
python3 e2e/check-observability.py
```

| Check | Evidence |
| --- | --- |
| Platform | Workers and operators, MaaS provider/model/catalog, separate workspace key and real Foundry completion |
| Developer | Latest Tekton build, matching image, Argo CD Synced/Healthy, public HTTPS health endpoint, resources, probes and restricted security |
| Usage | Real completion with input/output token usage through MaaS |
| Observability | All nine workshop dashboard expressions through OpenShift Thanos |

These checks create no cluster resources or credentials. Platform and usage checks each make one billable coding-model call. Generate coding and speech activity before checking metrics and allow scrape propagation; p95 can be NaN without recent completions.

`APP_NAME` defaults to `live-transcript`. `EXPECT_UPDATED_APP=true` checks the CPU over 1h shortcut exercise. Also pass `E2E_NS=<workspace-namespace> DEV_POD=<workspace-pod> DEV_CONTAINER=dev` to verify the real workspace’s source, local Go build and pushed Git revision. The checks leave the environment available until RHDP expiration.

## Local checks

```sh
make test
make docs
```

Helper regressions cover Azure resource-group discovery, scoped developer permissions, operator readiness, provider configuration and telemetry failures. The reference Go app has HTTP, upload, speech-proxy, bounded agent/tool, MCP protocol, monitoring-failure, credential-rotation and OTLP export tests.

## Speech in the browser

```sh
python3 e2e/check.py speech
npm install --no-save @playwright/test@1.56.1
npx playwright install chromium
BROWSER_STATE=/private/path/login-state.json APP_URL=https://your-app-route AUDIO_FIXTURE=/absolute/french-speech.wav \
  node e2e/speech-browser.cjs
```

Use synthetic mono 16 kHz PCM16 WAV audio. For English, set `SPEECH_LANGUAGE=en` and supply an English fixture. The check captures real browser microphone audio against the Go backend and Whisper, and verifies text before Stop, independent clips, export, microphone release and failure handling. Every Chromium launch uses `--use-mock-keychain`.

## MaaS protocol and quota

```sh
python3 e2e/maas-protocol.py
python3 e2e/maas-quota.py
```

The protocol check makes billable tool-use and streaming calls, including a prompt larger than 16 KiB, and checks model discovery and invalid/missing-key rejection. The quota check requires an OpenShift user token. It creates its own temporary subscription and short-lived key, then revokes/deletes both in `finally`. It refuses to overwrite an existing validation subscription. Enforcement is asynchronous and is not a financial spending cap.

## Validation records

- [Cluster Voice: local evidence and pending live gates](CLUSTER-VOICE-VALIDATION.md)
- [Observability, scoped delivery and browser speech](OBSERVABILITY-SECURITY-VALIDATION.md)
- [External Foundry model through MaaS](MAAS-FOUNDRY-VALIDATION.md)
- [RedHatAI Whisper INT8 on one T4](WHISPER-INT8-VALIDATION.md)
- [Deployment image guard regression](IMAGE-CONTRACT-VALIDATION.md)

Retired GPU comparisons, FP16 baselines and terminal/provisioning experiments remain in [the pre-cleanup revision](https://github.com/fjcloud/aro-ai-app-platform/tree/e333ecdf5aef0946cf747d7a970b6b386ce87835).

## Cluster Voice local browser checks

```sh
AUDIO_FIXTURE=/absolute/spoken.wav node e2e/cluster-voice-browser.cjs
```

Uses the actual Go app and Chromium with mock-keychain. Whisper, MaaS and Prometheus are local protocol fixtures: this proves browser/backend integration, not live GPU/model/cluster compatibility. Run `python3 e2e/check-cluster-voice.py` for live app/model/monitoring checks (billable calls). Live acceptance also requires Lab 3.2, authenticated app access, real evidence and a visible MLflow trace. Native MCP Playground and runtime provider behavior remain separate live checks.
