# ARO speech workshop validation

Validation in progress on 2026-10-22, on the assigned classic ARO 4.21.22 lab. This report replaces the Fortune Cookie baseline for the current workshop path; historical LLM GPU experiments remain separate.

Verified so far:

- OpenShift AI 3.5.2 and cert-manager 1.20.1 installed; DataScienceCluster Ready.
- One `Standard_NC8as_T4_v3` worker, with one physical NVIDIA T4. Original three CPU workers preserved; one separate D8s_v3 CPU worker added for OpenShift AI / Dev Spaces / Tekton headroom in the workspace PVC's zone.
- Old four-T4 comparison worker removed. Qwen model cache retained separately in `gpu-comparison`; it is not used by Whisper.
- KServe `InferenceService/whisper` Ready, all three pod containers Ready, no external model Route. NetworkPolicy restricts model ingress.
- Red Hat AI Inference 3.5.0, vLLM 0.24.0+rhaiv.9, pinned image digest in `deploy/speech/servingruntime.yaml`.
- `openai/whisper-large-v3-turbo`, revision `41f01f3fe87f28c78e2fbf8b568835947dd65ed9`, FP16, no quantization or custom kernels. GPU memory reported 8,221 MiB including runtime/cache reservation.
- Real French and English WAV requests succeeded. Initial concurrent calls took about 5.5 seconds for ~8 seconds of audio; English matched the synthetic sentence, French misheard the name “Red Hat”. Transcription is not guaranteed verbatim.
- Go reference app tests (`go test -race ./...`, `go vet ./...`) passed: audio forwarding, WAV/rate/size validation, rejected origins, timeout/upstream error redaction, concurrency bound and health/UI.
- Local Chromium + real cluster Whisper passed: text visible before Stop, sequential clips, export, microphone release, model-outage handling and actual browser Permissions Policy denial. First text ~2.3 s; measured warm inference ~0.3–0.6 s/clip. Chromium always launched with `--use-mock-keychain`.
- New transcription Dev Spaces factory opened the actual VS Code editor. Foundry returned a real coding completion, 500,000 effective tokens/minute, and rejected an invalid key.
- All 33 cluster operators were available without degradation.

Remaining validation: complete the coding/deployment exercise, verify the public HTTPS app against the private model service, and record the final GitOps/build revisions and browser results.

## Support scope

The [OpenShift AI matrix](https://access.redhat.com/articles/rhoai-supported-configs) includes ARO and OpenShift 4.21 with OpenShift AI 3.5. The [Red Hat AI hardware matrix](https://docs.redhat.com/en/documentation/red_hat_ai/3/html-single/supported_product_and_hardware_configurations/index) includes T4 with the CUDA runtime but excludes T4 from supported INT4 configurations. Consequently this path uses FP16 upstream Whisper weights. The runtime/operator support matrix does not certify the workshop-generated application or every possible model/configuration. This path uses neither the Technology Preview multimodal playground nor vLLM-Omni.
