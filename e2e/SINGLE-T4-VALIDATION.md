# Single-T4 model validation — 2026-10-22

**Bonsai 2 27B runs with vLLM on one T4 after a custom SM75 backend adaptation.** The unmodified community plugin fails the GPU capability check. This proves runtime feasibility, not equivalent quality to Qwen3.8-27B or a validated replacement for the full workshop.

## Setup

- Existing classic ARO environment `vcdhr`, OpenShift 4.21.22, East US.
- Existing `Standard_NC64as_T4_v3` worker. Each candidate pod requests **one GPU**; no tensor parallelism or GPU sharing.
- Red Hat image `registry.redhat.io/rhaii/vllm-cuda-rhel9:3.5.0`, digest `sha256:5d4434598dc702b89c8d63b9fa6e2378130abf649c95f4211fb93e325cfd7698`, vLLM `0.24.0+rhaiv.9`.
- FP16 activations and KV cache, 16,384-token context, one sequence, eager execution, text only. CPU offload is disabled except in the explicitly named Qwen offload trial.
- The AWS comparison uses the existing Qwen3.8-27B deployment on one L40S 48 GB, with its existing BF16/FP8 configuration.

The normal Foundry-backed workshop and its credentials were not changed. Trials ran on separate GPUs in the same VM and shared CPU/storage. End-to-end timings include prefill and occasional JIT compilation; they are not a controlled throughput benchmark.

## Candidates

| Model | Revision | GPU model-loading memory |
|---|---|---:|
| `fraserprice/Ternary-Bonsai-2-27B-vllm` | `1e1d7c0f5cb02b71d81b0574c54ddd1dc9088495` | 9.05 GiB |
| `cyankiwi/Qwen3.5-9B-AWQ-4bit` | `e4d2d39cede8c5d1716d824cd843e2889c648083` | 7.56 GiB |
| `cyankiwi/MiMo-V2.6-Distill-Qwen-9B-AWQ-BF16-INT4` | `30dc70c25a937c453b8503936c4959cff073e029` | 7.67 GiB |
| `RedHatAI/Qwen3.8-27B-INT4`, with 8 GiB CPU offload | `91bd022d5b49442a868bc35008f6c21e1860edfa` | 8.66 GiB |

These numbers exclude the separately allocated KV cache and runtime buffers. They are not total GPU usage.

The cyankiwi catalog also contains `Qwen3.8-27B-AWQ-INT4`, but its text tensors alone occupy **19,246,830,832 bytes**, excluding vision and MTP. This is not a fully GPU-resident T4 candidate. Sizes were calculated from the five safetensors shard headers, not inferred from the INT4 label. See [size evidence](results/single-t4/cyankiwi-qwen38-sizes.json).

## Bonsai runtime

The upstream plugin at `e5ed2454a6b06a857a2146ba8cc60b7814c54bdf` requires SM80 and uses BF16 CUDA matrix instructions. T4 is SM75. The real startup attempt failed before loading weights; [failure evidence](results/single-t4/upstream-bonsai.json).

The [experimental backend](../experiments/gpu/single-t4/bonsai) keeps the original packed 2-bit codes, FP16 group scales and signed Hadamard rotation. It uses FP32 packed GEMV for decode and temporary FP16 dequantization with cuBLAS for prefill. Dense model weights are not cached. MTP is disabled. Simply lowering the upstream capability check is insufficient.

The first Triton-only prefill implementation lowered `dot` to SIMT on SM75; its emitted PTX contained no `mma.sync`. The final implementation uses cuBLAS instead. Numerical checks against an FP32 dense reference passed for 1, 4, 17 and 256 rows; maximum absolute error was 0.00143 or less.

The committed ConfigMap and pod manifest were also deployed independently, without the diagnostic runner's preinstalled package. Startup and a streaming tool call passed on a T4 exposing **14,912 MiB to PyTorch**. That deployment allocated a 3.28 GiB KV cache and reported capacity for 46,421 tokens, while the configured request limit remained 16,384.

## Functional results

All initial probes use temperature 0 and explicitly disable thinking. Three repeated short generations, JSON-only instructions and two 12,527-token inputs were tested. A per-case cache salt prevents the existing AWS prefix cache from shortcutting this comparison.

| Check | Bonsai 2 27B, one T4 | Qwen 9B, one T4 | MiMo 9B, one T4 | Qwen3.8 reference, L40S |
|---|---:|---:|---:|---:|
| Short generation | Pass, 30.8–33.2 s | Pass, 5.0–9.1 s | Pass, 4.1–8.2 s | Pass, 3.5 s |
| Automatic tool call | Pass, 7.7 s | Pass, 1.8 s | Fail: empty arguments | Pass, 0.9 s |
| Tool result round trip | Pass, 3.8 s | Pass, 0.8 s | Not reached | Pass, 0.3 s |
| First 12,527-token input | Pass, 138.3 s | Pass, 29.0 s | Pass, 29.1 s | Pass, 4.1 s |
| Second input, different prefix | Pass, 139.6 s | Pass, 29.3 s | Pass, 29.4 s | Pass, 4.1 s |
| JSON instructions | Pass | Pass | Pass | Pass |
| Streaming tool call | Pass, 11.2 s | Pass, 1.9 s | Not validated | Not repeated |

Short generations have different output lengths, so their elapsed times should not be read as a throughput ranking. Bonsai generated 211 tokens in the repeated short case, versus 116 for Qwen 9B and 151 for the reference. Raw responses and measured token counts are in [results/single-t4](results/single-t4).

MiMo's returned call had the right function name but `{}` arguments. The installed `mimo` parser resolves to the same Qwen3 engine parser used by `qwen3_coder`; switching that name alone does not provide a different implementation. The failed call is retained, not treated as successful tool use.

## Generated code

Two Go functions were generated from the same prompts and compiled against the [fixtures](model-code-tests). The Go compiler was 1.24.4. Source collection is recorded separately from passing a compile/test run.

| Model | Initial tasks passed | After one compiler/test-feedback correction |
|---|---:|---:|
| Qwen3.8-27B reference | 1/2 | 2/2 |
| Bonsai 2 27B | 1/2 | 1/2 |
| Qwen3.5-9B INT4 | 1/2 | 1/2 |
| MiMo distilled 9B INT4 | 0/2 | 1/2 |

Bonsai invented `time.MaxDuration`, then failed integer-overflow cases after correction. Qwen 9B's retry-after implementation remained truncated. MiMo's initial answers included invalid Go syntax, wrong clock handling and overflow/sign errors. A separate MiMo trial with thinking enabled, temperature 0.6 and 4,096 output tokens still produced a compile error. A Bonsai trial with thinking enabled, `reasoning_effort=low`, temperature 1.0 and a 4,096-token ceiling did not return a complete response before the 360-second HTTP timeout; no code-accuracy result is assigned to that unfinished response. The reference also failed initially, then corrected its duplicate declaration.

These are two small tasks and one sample per configuration. They expose problems worth testing in the real agent workflow; they do not establish a general accuracy ranking. See [initial compile results](results/single-t4/go-initial.json), [corrections](results/single-t4/go-repair.json) and [MiMo thinking trial](results/single-t4/mimo-thinking.json) and [bounded Bonsai thinking trial](results/single-t4/bonsai-thinking.json).

## The exact Qwen3.8 checkpoint with CPU offload

The same `RedHatAI/Qwen3.8-27B-INT4` revision used by the workshop also ran on **one T4** with the standard image and `--cpu-offload-gb 8`. vLLM reported 8.02 GiB of offloaded parameters and 8.66 GiB of GPU model-loading memory. This retains the checkpoint and moves some weights to host RAM; it is not fully GPU-resident inference. The test used FP16, a 16k context and one sequence.

| Probe | Result | End-to-end time |
|---|---|---:|
| Exact short marker, 7 generated tokens | Pass | 9.440 s |
| Automatic tool call | Pass | 28.635 s |
| Tool result round trip | Pass | 11.533 s |
| 12,527-token input | Pass | 217.215 s |

Observed decode throughput in vLLM logs was around 1–1.5 tokens/s. This is a way to keep the reference checkpoint on one GPU, with a substantial latency cost. The Go code comparison above used the existing AWS reference, not another code evaluation of the offload deployment. See [offload responses](results/single-t4/qwen27b-offload.json) and [manifest](../experiments/gpu/single-t4/qwen27b-offload.yaml).

## Reproduce

Use the [single-T4 instructions and pinned manifests](../experiments/gpu/single-t4/README.md). The main participant workshop continues to use Foundry.

Using one GPU in this four-GPU VM does **not** lower its price. These trials do not validate the smallest single-T4 Azure VM or full RHOAI/MaaS/agent integration. No claim of equivalent Qwen3.8 quality is made from startup or HTTP 200 alone.

## Cleanup

The temporary candidate pods and the Bonsai ConfigMap were removed. The original four-GPU reference manifest was reapplied. The shared model cache and GPU worker remain available for follow-up and continue to incur charges. The application health endpoint returned HTTP 200 and all 33 cluster operators were available and not degraded during verification.

## Sources

- [Bonsai vLLM checkpoint](https://huggingface.co/fraserprice/Ternary-Bonsai-2-27B-vllm) and [upstream plugin](https://github.com/fraserprice/bonsai-vllm).
- [Prism ML's original Bonsai 2 model and benchmark methodology](https://huggingface.co/prism-ml/Ternary-Bonsai-2-27B-gguf).
- [cyankiwi Qwen 9B](https://huggingface.co/cyankiwi/Qwen3.5-9B-AWQ-4bit), [MiMo 9B](https://huggingface.co/cyankiwi/MiMo-V2.6-Distill-Qwen-9B-AWQ-BF16-INT4) and [Qwen3.8-27B](https://huggingface.co/cyankiwi/Qwen3.8-27B-AWQ-INT4).
