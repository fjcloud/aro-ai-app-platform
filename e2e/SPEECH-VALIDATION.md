# ARO speech workshop validation

Completed on 2026-10-22 against the assigned classic ARO 4.21.22 lab. This records the initial FP16 transcription baseline. The active workshop now uses the RedHatAI INT8 model: see [INT8 validation](WHISPER-INT8-VALIDATION.md). `VALIDATION.md` and the GPU LLM comparisons describe earlier Fortune Cookie/model experiments.

Application: **https://live-transcript-live-transcript-dev.apps.ytlbb232.eastus.aroapp.io/**

## Deployed stack

| Component | Verified configuration |
|---|---|
| Coding model | DeepSeek-V4 Pro on Foundry; real completion and token usage, effective 500,000 tokens/minute, invalid key rejected |
| Workspace | New `aro-transcription` template factory opened the actual Dev Spaces VS Code editor; OpenCode 1.18.14 called Foundry for plan, implementation, iteration and manifest audit |
| OpenShift AI | 3.5.2, cert-manager 1.20.1, DataScienceCluster Ready; dashboard SSO and speech project visible |
| GPU worker | One `Standard_NC8as_T4_v3`, one physical 16 GB NVIDIA T4; original three CPU workers preserved |
| CPU headroom | One additional D8s_v3 worker in the existing workspace PVC's zone; three small D4 workers had insufficient schedulable CPU after adding OpenShift AI |
| Driver/operators | NVIDIA GPU Operator 26.7.2, driver 595.91.07, CUDA 13.0; NFD 4.21 |
| Model serving | KServe InferenceService Ready, all three pod containers Ready; Red Hat AI Inference 3.5.0 / vLLM 0.24.0+rhaiv.9; pinned image digest in the manifest |
| Model | `openai/whisper-large-v3-turbo`, revision `41f01f3fe87f28c78e2fbf8b568835947dd65ed9`, FP16, one GPU, no added packages or custom kernels |
| Memory | `nvidia-smi` reported 8,221 MiB including vLLM cache/runtime reservation; the model checkpoint is about 1.51 GiB |
| Application | Go backend with embedded HTML/JS/CSS, independent mono 16 kHz PCM16 WAV clips, no browser speech-recognition or Foundry audio calls |
| Delivery | Two successful Tekton builds, `build-app-jnjp8` and `build-app-zqhz8`; personal Argo CD Synced / Healthy; browser administrator login verified |
| Git | Final workspace HEAD, pushed personal `main` and Argo CD revision matched `187b5c6ebe3189bd1ab289284972bc2750210f56`; no unstaged changes. The last commit updates only the runbook; application code was built from `a80ee21` |

[Runtime, image and build evidence](results/speech/runtime.json).

## Real audio/browser results

Synthetic spoken WAV files were generated with macOS `say` and converted to mono 16 kHz PCM16 WAV. Chromium used the WAV file as its microphone device with `%noloop`. Every browser launch used `--use-mock-keychain`.

| Path | Result |
|---|---|
| Direct vLLM, French + English | Both accepted real audio and returned transcripts; initial concurrent calls took ~5.5 s for ~8 s of audio |
| Public HTTPS app, French | Text appeared while recording, two sequential clips, ~5.3 s to first text including capture, 250–282 ms inference per clip |
| Public HTTPS app, English | Text appeared while recording, two sequential clips, ~5.3 s to first text including capture, 256–301 ms inference per clip |
| Updated app | Second real build deployed Copy transcript; browser verified clipboard text, download, Clear, Stop and microphone release; 248–302 ms inference per clip |
| Automatic language | A French WAV sent through the public Go API without a language hint returned French text, HTTP 200 |
| Failure/permission | Browser model-outage response displayed an error and released the microphone; browser Permissions Policy denial produced the expected permission message |
| Network isolation | Dev Spaces reached the internal model service; an unrelated application namespace resolved its DNS but its TCP connection timed out. Temporary test pod removed. No public model Route exists |

[French](results/speech/browser-fr.json), [English](results/speech/browser-en.json), [updated app with clipboard](results/speech/browser-updated-fr.json).

These are short synthetic samples, not an accuracy benchmark or a concurrency/load test. Whisper misheard the name “Red Hat” in French. The application uses near-real-time clips, not a native Whisper continuous WebSocket stream. Long utterances can be split at clip boundaries.

## Checks and corrections

- Go reference and generated-source tests passed with `go test -race ./...`; `go vet ./...` passed. Coverage includes WAV/rate/body limits, upstream forwarding, timeout/error redaction, denied origins, concurrency bounds and health/UI.
- `e2e/check.py platform`, `speech`, and `developer` passed. The developer check ran against the real workspace, compiled its source, compared local/pushed/Argo revisions, and checked the updated public app, actual image, resource limits, probes, restricted security context and HTTPS redirect.
- OpenCode audited the deployment's resource requests/limits, probes, pod security and Route TLS: no fixes needed. Independent resource assertions also passed. All 33 cluster operators remained Available and nondegraded.
- Antora generated without warnings; all 43 documented shell blocks passed `bash -n`; eight GPU preflight/isolation tests passed.
- Added the missing workspace CLI login step: browser SSO alone leaves `oc` using a service account that cannot create lab namespaces/build permissions. The guide now explicitly requires the assigned lab administrator login before prompting deployment and explicitly says not to paste credentials into OpenCode.
- Clarified embedded assets: the binary-only Dockerfile cannot serve source-tree HTML/JS. The template now points to the tested embedded reference and forbids serving the project directory.
- Explicitly set `HF_HUB_OFFLINE=0` and `TRANSFORMERS_OFFLINE=0` for first model download; the runtime image otherwise starts offline.
- Clarified personal `main` branch creation and the complete ship-a-change prompt. “Then ship it” alone caused the coding model to stop after a local build; the guide now names commit, Tekton and GitOps completion.

## Support scope and final state

The [OpenShift AI matrix](https://access.redhat.com/articles/rhoai-supported-configs) includes ARO and OpenShift 4.21 with OpenShift AI 3.5. The [Red Hat AI hardware matrix](https://docs.redhat.com/en/documentation/red_hat_ai/3/html-single/supported_product_and_hardware_configurations/index) includes T4 with the CUDA runtime but excludes T4 from supported INT4 configurations. The [Microsoft ARO GPU guide](https://learn.microsoft.com/en-us/azure/openshift/howto-gpu-workloads) lists NC8as T4 v3. This path therefore uses upstream Whisper FP16 weights with the Red Hat runtime. It uses neither the Technology Preview multimodal playground nor vLLM-Omni. Runtime/operator support does not certify the generated application or every model/configuration; production support requires the appropriate subscriptions and supported configurations.

The application, Foundry deployment, Whisper service, one-T4 VM and additional CPU worker are left available for manual testing. The previous four-T4 comparison VM is removed and its absence verified in Azure. Its historical Qwen cache PVC remains in `gpu-comparison`; it is not used by Whisper and continues to incur disk storage cost until removed. No RHDP cluster or original CPU worker was deleted. Lab 3.1 documents scoped facilitator cleanup; that destructive cleanup was not executed at the end of this validation so the demo stays usable.
