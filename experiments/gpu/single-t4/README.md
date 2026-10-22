# One T4 with vLLM

Experiments on the existing ARO GPU worker. The participant workshop still uses Foundry. See the [measured results](../../../e2e/SINGLE-T4-VALIDATION.md) before choosing a model.

| Candidate | Runtime | Purpose |
|---|---|---|
| Reference Qwen3.8-27B + 8 GiB CPU offload | Standard Red Hat vLLM image | Keep the same checkpoint; much slower, with extra host RAM usage. |
| Bonsai 2 27B | Experimental SM75 plugin adaptation | Keep a Qwen3.8-derived 27B model in one T4's memory. |
| cyankiwi Qwen3.5-9B INT4 | Standard Red Hat vLLM image | Simpler, faster single-card candidate; smaller model. |
| cyankiwi MiMo-V2.6-Distill-Qwen-9B INT4 | Standard Red Hat vLLM image | Alternative under investigation; tool arguments failed in this stack. |

These trials use FP16 activations and KV cache, one GPU, one sequence and a 16,384-token context. They do not establish equivalent quality to Qwen3.8-27B or validate the full agent workshop. Only the explicitly named offload variant moves weights to host RAM.

## Prepare

Use the NVIDIA-enabled worker from the [parent experiment](../README.md). The node must have the `workshop.fjcloud.io/gpu=t4` label. The manifests share a 60 GiB model cache and use the cluster's existing Red Hat registry access.

Stop the previous four-GPU comparison pod if it is running:

```bash
oc delete pod -n gpu-comparison qwen-t4-auto --ignore-not-found
oc apply -f experiments/gpu/single-t4/cache.yaml
```

Run one candidate at a time on a single-GPU worker. First startup downloads its weights and compiles kernels; follow the logs until the API is ready.

## Qwen 9B

```bash
oc apply -f experiments/gpu/single-t4/cyankiwi9b.yaml
oc logs -n gpu-comparison cyankiwi-9b-t4 -f
```

## Bonsai 2 27B

The upstream plugin rejects SM75 and uses BF16 CUDA kernels. This experiment preserves its packed ternary weights and Hadamard rotation, adds an FP32 packed GEMV for decode, and uses temporary FP16 dequantization with cuBLAS for prefill. Dense model weights are not retained. MTP stays disabled. This is a custom backend, not stock vLLM or an upstream-supported T4 release.

```bash
oc create configmap bonsai-t4-backend -n gpu-comparison \
  --from-file=experiments/gpu/single-t4/bonsai/kernels_t4.py \
  --from-file=experiments/gpu/single-t4/bonsai/patch_plugin.py \
  --dry-run=client -o yaml | oc apply -f -
oc apply -f experiments/gpu/single-t4/bonsai2.yaml
oc logs -n gpu-comparison bonsai2-t4 -f
```

The manifest pins the image digest, upstream plugin commit and model revision. Plugin installation and the T4 patch apply inside this experimental pod.

## Keep the reference Qwen3.8 model

This variant uses the same checkpoint as the workshop, with 8 GiB of parameters moved to host RAM. It passed generation, tool calls and a 12k-token input, but decoded around 1–1.5 tokens/s in this lab. Allow enough host RAM as well as GPU memory.

```bash
oc apply -f experiments/gpu/single-t4/qwen27b-offload.yaml
oc logs -n gpu-comparison qwen27b-t4-offload -f
oc exec -i -n gpu-comparison qwen27b-t4-offload -- \
  python - < e2e/single-t4-offload-probes.py
```

## Verify

The API binds only to localhost and has no public route:

```bash
oc exec -n gpu-comparison bonsai2-t4 -- curl -fsS http://127.0.0.1:8000/health
oc exec -i -n gpu-comparison bonsai2-t4 -- \
  env BENCH_MODEL=bonsai2 python - < e2e/single-t4-probes.py
```

For Qwen 9B, use pod `cyankiwi-9b-t4` and `BENCH_MODEL=cyankiwi9b`. The probes cover generation, tool round trips, JSON and two 12k-token inputs. Streaming tool parsing can also be checked with `e2e/single-t4-stream.py` and the same `BENCH_MODEL`. The Go probes only collect source; compile it against the fixtures in `e2e/model-code-tests` before assessing correctness.

Delete only the candidate pod when finished; keep the shared cache for another test. Restore the four-GPU reference with `oc apply -f experiments/gpu/qwen-t4.yaml` if needed. The VM and disks continue to incur charges while they exist; using one GPU in a four-GPU VM does not reduce its price.
