# ARO GPU comparison — 2026-10-22

**Qwen generation, tool use and the 12,527-token prompt passed on four T4s with an FP16 KV cache.** This is a separate experiment; the Foundry-backed Acts 0–3 remain the validated workshop.

## Candidate selection

The AWS reference is `g6e.xlarge`: **one NVIDIA L40S, 48 GB**. For the existing East US ARO cluster, the preferred single-GPU candidate is **`Standard_NC24ads_A100_v4`: one A100, 80 GB**. Microsoft documents adding this size to ARO with a MachineSet. It is a functional candidate, not a claim of identical architecture, throughput, or price. Its inference performance remains untested here.

| Azure option | Live result in the assigned lab |
|---|---|
| A100, `Standard_NC24ads_A100_v4` | Listed in East US zone 2, but family quota is zero. The API request for 24 vCPUs failed with `QuotaNotAvailableForResource`. The inherited GPU policy also denies it. No A100 was launched. |
| Two A10s, `Standard_NV72ads_A10_v5` | Family quota is 100 vCPUs and SKU restrictions are empty. Actual MachineSet provisioning failed with HTTP 403 `RequestDisallowedByPolicy`. The failed pool and its resources were removed. |
| Four T4s, `Standard_NC64as_T4_v3` | Family quota is 512 vCPUs. Created a real VM and Ready worker in zone 2, preserving the three CPU workers. All four GPUs were detected and the NVIDIA CUDA validator passed. |

The management group policy **Deny expensive GPU VM SKUs (allow T4)** denies `Standard_NC*`, `Standard_ND*` and `Standard_NV*`, except `Standard_NC*_T4_v3`. Quota availability and an unrestricted SKU listing do not override this policy. An A100 test needs both 24 vCPUs of family quota and an environment whose policy permits that SKU.

T4 is a fallback experiment, not an L40S equivalent. Four separate 16 GB GPUs require tensor parallelism. The checkpoint's weights alone total **19,452,788,048 bytes**; one T4 cannot hold them. T4 compute capability is 7.5 and native BF16 is unavailable.

## Runtime test

- Classic ARO **4.21.22**, East US, environment `vcdhr`.
- NVIDIA GPU Operator **26.7.2**; NFD **4.21.0-202610150640**.
- Driver **595.91.07**, CUDA **13.0**, PyTorch **2.11.0a0+git069d5d8**, vLLM **0.24.0+rhaiv.9**.
- Same model revision as AWS: `91bd022d5b49442a868bc35008f6c21e1860edfa`.
- Same image digest as AWS: `sha256:5d4434598dc702b89c8d63b9fa6e2378130abf649c95f4211fb93e325cfd7698`, tagged `registry.redhat.io/rhaii/vllm-cuda-rhel9:3.5.0`.
- Requested the same 262,144-token context, reasoning parser, tool-call parser, prefix caching and maximum 16 sequences. Added tensor parallelism across four T4s.
- `--dtype=auto` automatically falls back from BF16 to FP16 on T4. PyTorch's default BF16 check includes emulation; use `including_emulation=False` to check native support.
- The first configured runtime attempt failed because the checkpoint selects an FP8 KV cache, unsupported by Triton attention on T4. Retried with the runtime's recommended `--kv-cache-dtype=float16`.
- vLLM successfully loaded the model (5.17 GiB per GPU reported on rank 0), selected Marlin INT4 kernels and Triton attention, compiled the kernels, and started the API. It reported a 330,562-token KV cache and maximum concurrency of 1.26 for requests at the full 262,144-token context. The final configured cold start took about 11 minutes, including weight download and compilation.
- The test pod is internal only. No public route, model credentials, Foundry deployment, Dev Spaces provider, or existing application was changed. RHOAI and MaaS were not installed for this isolated runtime test.

## Inference checks

Both `/health` and `/v1/models` returned HTTP 200 on ARO; the listed model is `qwen3`. All seven probe cases passed.

| Check | AWS L40S | Azure four T4s |
|---|---|---|
| Short generation, first request | 151 tokens in 3.529 s | 164 tokens in 6.200 s, including first-use JIT |
| Short generation, next two requests | 151 tokens in 3.538 s | 164 tokens in 4.206–4.212 s |
| Automatic tool call | Correct, 0.927 s | Correct, 1.569 s |
| Tool result round trip | Correct result 56, 0.340 s | Correct result 56, 1.041 s |
| 12,527-token prompt | Correct marker, 4.034 s | Correct marker, 44.240 s |
| Same length, new prefix, warmed kernels | Correct marker, 3.957 s | Correct marker, 44.454 s |

T4 is usable for this model with the documented changes, but the longer prompt remained about 11 times slower after warming the kernels. Changing its first token prevented reuse of the previous prompt cache. These are end-to-end timings with different output lengths, numerical precision, attention backends and GPU counts; they are not a controlled GPU throughput benchmark. See [ARO results](results/gpu-aro-t4.json) and [AWS results](results/gpu-rosa-reference.json).

## Reproduction and isolation

[GPU experiment commands and manifest](../experiments/gpu/README.md) are separate from the normal participant instructions. The classic ARO helper defaults to printing a quota-checked manifest; `--apply` explicitly creates one tainted worker. Seven automated tests cover quota exhaustion, cross-subscription protection, zone restrictions, image generation, preservation of the CPU template, and the T4 core requirement. The generated zone 2 manifest matched the actual deployed T4 MachineSet. Re-running `--apply` against the live lab left exactly one GPU worker.

The existing Fortune Cookie application's `/healthz` remained successful and all 33 cluster operators remained Available with none Degraded during the GPU test.

## State left for follow-up

The tested four-T4 worker, internal Qwen pod, model-cache PVC, NFD and NVIDIA operators remain for further tests. The normal workshop's Foundry provider configuration and the existing application remain unchanged. The experimental pod is not the RHOAI/MaaS serving architecture and has no public endpoint.

For a closer single-GPU comparison, obtain an ARO environment that permits `Standard_NC24ads_A100_v4` and has at least 24 unused family vCPUs. The A100 remains a candidate requiring live inference validation; this report does not claim it was tested.
