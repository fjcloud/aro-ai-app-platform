# GPU comparison on the assigned classic ARO cluster

This is an isolated GPU experiment. The normal workshop continues to use Azure AI Foundry. See [the live test report](../../e2e/GPU-VALIDATION.md) before provisioning anything.

For single-card alternatives, see the [one-T4 experiments](single-t4/README.md).

The AWS reference uses **one L40S 48 GB** (`g6e.xlarge`), `RedHatAI/Qwen3.8-27B-INT4`, Red Hat vLLM image `3.5.0`, and a 262,144-token context.

| Azure VM | GPU memory | Comparison |
|---|---|---|
| `Standard_NC24ads_A100_v4` | One A100 80 GB | Preferred single-GPU candidate; different architecture and performance from L40S. |
| `Standard_NV72ads_A10_v5` | Two A10, 24 GB each | Requires tensor parallelism; 48 GB aggregate is not one 48 GB GPU. Also requires checking the Azure driver configuration. |
| `Standard_NC64as_T4_v3` | Four T4, 16 GB each | Experimental fallback permitted in this RHDP lab. T4 does not support BF16; runtime and model compatibility must be tested. |

In environment `vcdhr`, the inherited policy **Deny expensive GPU VM SKUs (allow T4)** blocks NC, ND and NV GPUs except `Standard_NC*_T4_v3`. An A100 quota increase alone will not remove this restriction. Do not change or bypass the lab policy.

## Plan a worker

Run on the authenticated RHDP bastion, using the existing cluster administrator login:

```bash
python3 scripts/gpu-classic.py
```

The helper checks that Azure CLI and OpenShift use the same subscription, regional and GPU-family quotas, a compatible availability zone, and the existing ARO generation 2 image. It prints a manifest without changing the cluster. Quota and SKU listings do not establish permission or physical capacity.

For the T4 experiment:

```bash
python3 scripts/gpu-classic.py --vm-size Standard_NC64as_T4_v3 --apply
oc get machines -n openshift-machine-api
oc get nodes
```

This adds one separate tainted worker and preserves all CPU workers, networking and cluster credentials. Do not run `make deploy`: that creates a separate ARO HCP cluster.

## Test the runtime

Install only NFD and NVIDIA, not the entire dormant GPU platform:

```bash
for operator in nfd nvidia; do
  for kind in namespace operatorgroup subscription; do
    oc apply -f deploy/operators/$operator-$kind.yaml
  done
done
```

Wait for both subscriptions' current CSVs to reach `Succeeded`, then:

```bash
oc apply -f deploy/instances/nfd-instance.yaml
oc apply -f deploy/instances/nvidia-clusterpolicy.yaml
oc get pods -n nvidia-gpu-operator
oc get nodes -l nvidia.com/gpu.present=true
```

Wait for `nvidia.com/gpu: 4` in the GPU node's allocatable resources before testing:

```bash
oc apply -f experiments/gpu/qwen-t4.yaml
oc logs -n gpu-comparison qwen-t4-auto -f
```

The pod checks CUDA device names, VRAM, compute capability and BF16 support, then attempts the AWS model with tensor parallelism across four T4s. It is internal only, has no public route, and does not change Dev Spaces credentials. A Running pod does not prove successful model loading; require a healthy inference API and successful completions/tool calls.

After the test, delete the experimental pod/PVC and its MachineSet; wait for the cloud VM, disks and NIC to disappear. Keep the three original CPU MachineSets.

## Sources

- [Microsoft: GPU workloads on ARO](https://learn.microsoft.com/en-us/azure/openshift/howto-gpu-workloads)
- [Microsoft: A100 VM sizes](https://learn.microsoft.com/en-us/azure/virtual-machines/sizes/gpu-accelerated/nca100v4-series)
- [Microsoft: A10 VM sizes](https://learn.microsoft.com/en-us/azure/virtual-machines/sizes/gpu-accelerated/nvadsa10v5-series)
- [Microsoft: GPU drivers](https://learn.microsoft.com/en-us/azure/virtual-machines/linux/n-series-driver-setup)

If the API becomes healthy, run the same functional probe as on AWS:

```bash
oc exec -i -n gpu-comparison qwen-t4-auto -- \
  env BENCH_BASE=http://127.0.0.1:8000/v1 BENCH_LONG=1 python - < e2e/gpu-benchmark.py
```

Require generation, a tool call and its result round trip, and both 12,527-token prompts to pass. The T4 manifest uses an FP16 KV cache because the checkpoint's default FP8 cache is unsupported by the T4 Triton backend. The main model dtype also falls back to FP16 automatically. These differences prevent an identical-parameter performance comparison with the L40S.

The live test passed on four T4s, with substantially slower long-prompt processing than the L40S. The GPU worker remains active for follow-up. To stop this experiment, remove only its resources:

```bash
oc delete namespace gpu-comparison
oc delete machineset -n openshift-machine-api -l workshop.fjcloud.io/experiment=t4
```

Wait for the experimental VM, OS disk, data disk and NIC to disappear before considering cleanup complete. The NFD and NVIDIA operators remain installed for later GPU tests.
