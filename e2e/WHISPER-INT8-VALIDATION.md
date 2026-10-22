# RedHatAI Whisper INT8 on one T4

Validated on the existing ARO lab on 2026-10-22. **This model is now active** at https://live-transcript-live-transcript-dev.apps.ytlbb232.eastus.aroapp.io/ . It replaces the initial OpenAI FP16 model; DeepSeek on Foundry remains the coding LLM.

## Exact configuration

- Hugging Face model: [`RedHatAI/whisper-large-v3-turbo-quantized.w8a8`](https://huggingface.co/RedHatAI/whisper-large-v3-turbo-quantized.w8a8).
- Pinned revision: `95991fd06170e03c8fa55595958972e6cb76c441`.
- Red Hat AI Inference 3.5.0, vLLM `0.24.0+rhaiv.9`; the existing pinned container image was used without added packages, patches or custom kernels.
- OpenShift AI 3.5.2 / KServe InferenceService `whisper`, three pod containers Ready, one `Standard_NC8as_T4_v3` worker with one 16 GB NVIDIA T4.
- Same settings as the FP16 baseline: `--dtype=half --max-model-len=448 --max-num-seqs=4 --gpu-memory-utilization=0.50 --enforce-eager`.
- `--dtype=half` controls non-quantized operations. vLLM detects the model's `compressed-tensors` W8A8 configuration and selected **`CutlassInt8ScaledMMLinearKernel for CompressedTensorsW8A8Int8`** on the T4.
- Checkpoint metadata contained 233 INT8 tensors and 588 FP16 tensors. Checkpoint size was 0.89 GiB; vLLM reported 0.83 GiB for model loading. `nvidia-smi` reported 8,141 MiB total GPU memory use including runtime and KV/cache reservations. Quantizing weights does not eliminate vLLM's cache reservation.

The actual manifest is [servingruntime.yaml](../deploy/speech/servingruntime.yaml). [Runtime evidence](results/whisper-int8/runtime.json) and [checkpoint metadata](results/whisper-int8/checkpoint.json).

## End-to-end results

Requests went through the **public HTTPS Go application**, which called the private model service. The model alias stayed `whisper`; no application code or image rebuild was required.

| Test | Result |
|---|---|
| French WAV, language `fr` | HTTP 200; first request took 2,758 ms of upstream processing |
| English WAV, language `en` | HTTP 200; 325 ms upstream processing |
| French WAV, automatic language | HTTP 200, French transcript; 406 ms upstream processing |
| French browser microphone | Text appeared while recording; two successive WAV clips, 289 / 246 ms processing |
| English browser microphone | Text appeared while recording; two successive WAV clips, 293 / 239 ms processing |
| Clipboard / download / Clear | Passed for both browser runs |
| Stop | Microphone tracks ended; controls became available after pending requests completed |
| Injected backend HTTP 502 | User-visible error and microphone release |
| Permission denial | Browser Permissions Policy denial produced the expected error |
| Platform | DataScienceCluster and InferenceService Ready; exactly one physical T4; RedHatAI model ID verified; no public model Route |

Every Chromium launch used `--use-mock-keychain`, a synthetic WAV microphone device and `%noloop`. Full browser capture-to-first-text timing is recorded separately from inference latency in the result files. These are short synthetic French/English samples, not a load or accuracy benchmark. The French sample still misheard the name “Red Hat”, as the FP16 baseline did. The app still uses short audio clips for near-real-time transcription rather than native continuous Whisper streaming.

Evidence: [API calls](results/whisper-int8/api.json), [French browser](results/whisper-int8/browser-fr.json), [English browser](results/whisper-int8/browser-en.json).

## Reproduce and support scope

Follow Lab 1.4 and apply `deploy/speech`. The model ID and revision are pinned. `python3 e2e/check.py speech` now checks that the deployed runtime uses this RedHatAI model. Run `e2e/speech-browser.cjs` with a spoken WAV fixture as described in [the E2E README](README.md).

The [Red Hat AI hardware matrix](https://docs.redhat.com/en/documentation/red_hat_ai/3/html-single/supported_product_and_hardware_configurations/index) lists the T4 for INT8 W8A8. The RedHatAI model card identifies this checkpoint as INT8 weights and activations for vLLM. This run establishes functional compatibility for the pinned model/runtime/T4 combination; a model's presence in the RedHatAI organization alone is not a certification of every configuration or of the workshop application.

The working FP16 configuration and cached weights were preserved locally for rollback during the test. No extra GPU VM was created. The single-T4 INT8 service remains active for manual testing.
