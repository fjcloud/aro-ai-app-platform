#!/usr/bin/env python3
"""Add one GPU worker to the currently selected classic ARO cluster."""
import argparse
import json
import subprocess
import sys


VM_SIZE = "Standard_NC8as_T4_v3"
GPU_SIZES = {"Standard_NC24ads_A100_v4": ("a100", 24),
             VM_SIZE: ("t4", 8), "Standard_NC4as_T4_v3": ("t4", 4), "Standard_NV72ads_A10_v5": ("a10", 72),
             "Standard_NC64as_T4_v3": ("t4", 64)}
NAMESPACE = "openshift-machine-api"
MACHINESET_LABEL = "machine.openshift.io/cluster-api-machineset"


def read(*args):
    return json.loads(subprocess.check_output(args, text=True))


def plan(machine_sets, sku, usage, resource_group_id, subscription):
    vm_size = sku["name"]
    gpu_name, required_cores = GPU_SIZES[vm_size]
    if resource_group_id.split("/")[2].lower() != subscription.lower():
        raise ValueError("Azure CLI and the selected OpenShift cluster use different subscriptions.")
    if sku.get("restrictions"):
        # A zone-specific restriction can leave another zone available.
        if any(r["type"] == "Location" for r in sku["restrictions"]):
            raise ValueError(f"{vm_size} is unavailable for this subscription.")
    zones = set(sku["locationInfo"][0].get("zones", []))
    for restriction in sku.get("restrictions", []):
        zones -= set(restriction.get("restrictionInfo", {}).get("zones", []))
    candidates = [m for m in machine_sets if
                  m["spec"]["template"]["spec"]["providerSpec"]["value"].get("zone") in zones]
    if not candidates:
        raise ValueError("No existing CPU MachineSet matches an available GPU zone.")
    family = next((q for q in usage if q["name"]["value"].lower() == sku["family"].lower()), None)
    cores = next((q for q in usage if q["name"]["value"] == "cores"), None)
    for label, quota in [(sku["family"], family), ("regional vCPUs", cores)]:
        if not quota or int(quota["limit"]) - int(quota["currentValue"]) < required_cores:
            raise ValueError(f"Need {required_cores} unused {label}; request quota before creating the worker.")
    machine = json.loads(json.dumps(sorted(candidates, key=lambda m: m["metadata"]["name"])[0]))
    provider = machine["spec"]["template"]["spec"]["providerSpec"]["value"]
    if "v2" not in provider["image"].get("sku", ""):
        raise ValueError("This helper requires a generation 2 ARO image; the selected template is not v2.")
    infra = machine["spec"]["selector"]["matchLabels"]["machine.openshift.io/cluster-api-cluster"]
    name = f"{infra}-gpu-{gpu_name}-{required_cores}-{provider['location']}{provider['zone']}"
    machine["metadata"] = {"name": name, "namespace": NAMESPACE,
                           "labels": {"machine.openshift.io/cluster-api-cluster": infra,
                                      "workshop.fjcloud.io/experiment": gpu_name}}
    machine.pop("status", None)
    machine["spec"]["replicas"] = 1
    machine["spec"]["selector"]["matchLabels"][MACHINESET_LABEL] = name
    template = machine["spec"]["template"]
    template["metadata"]["labels"][MACHINESET_LABEL] = name
    template["spec"].setdefault("metadata", {}).setdefault("labels", {})["workshop.fjcloud.io/gpu"] = gpu_name
    template["spec"]["taints"] = [{"key": "nvidia.com/gpu", "value": "present", "effect": "NoSchedule"}]
    provider["vmSize"] = vm_size
    return machine


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--apply", action="store_true", help="create the worker; otherwise print the checked manifest")
    parser.add_argument("--vm-size", choices=GPU_SIZES, default=VM_SIZE)
    args = parser.parse_args()
    subscription = read("az", "account", "show", "-o", "json")["id"]
    nodes = read("oc", "get", "nodes", "-o", "json")["items"]
    cluster_subscriptions = {n.get("spec", {}).get("providerID", "").lower().split("/subscriptions/", 1)[1].split("/")[0]
                             for n in nodes if "/subscriptions/" in n.get("spec", {}).get("providerID", "").lower()}
    if cluster_subscriptions != {subscription.lower()}:
        raise ValueError("Azure CLI and the selected cluster must use the same Azure subscription.")
    machines = read("oc", "get", "machinesets", "-n", NAMESPACE, "-o", "json")["items"]
    # Re-running must never add another GPU worker.
    existing = [m for m in machines if m.get("metadata", {}).get("labels", {}).get("workshop.fjcloud.io/experiment") in {v[0] for v in GPU_SIZES.values()}]
    if existing:
        current_size = existing[0]["spec"]["template"]["spec"]["providerSpec"]["value"]["vmSize"]
        if current_size != args.vm_size:
            raise ValueError(f"A GPU worker already exists with {current_size}; finish that test first.")
        print(f"GPU MachineSet already exists: {existing[0]['metadata']['name']}", file=sys.stderr)
        return
    cpu = [m for m in machines if m["spec"]["template"]["spec"]["providerSpec"]["value"].get("kind") == "AzureMachineProviderSpec"
           and not m["spec"]["template"]["spec"]["providerSpec"]["value"].get("vmSize", "").startswith("Standard_N")]
    if not cpu:
        raise ValueError("No classic ARO CPU MachineSet found. This helper does not create an ARO cluster.")
    providers = [m["spec"]["template"]["spec"]["providerSpec"]["value"] for m in cpu]
    locations = {p["location"] for p in providers}
    groups = {p["resourceGroup"] for p in providers}
    if len(locations) != 1 or len(groups) != 1:
        raise ValueError("CPU MachineSets must belong to one Azure region and resource group.")
    location, group = next(iter(locations)), next(iter(groups))
    group_id = read("az", "group", "show", "-n", group, "-o", "json")["id"]
    skus = read("az", "vm", "list-skus", "-l", location, "--resource-type", "virtualMachines", "--all", "-o", "json")
    sku = next((s for s in skus if s["name"] == args.vm_size), None)
    if not sku:
        raise ValueError(f"{args.vm_size} is not listed in {location}.")
    usage = read("az", "vm", "list-usage", "-l", location, "-o", "json")
    machine = plan(cpu, sku, usage, group_id, subscription)
    manifest = json.dumps(machine, indent=2) + "\n"
    if args.apply:
        subprocess.run(["oc", "create", "-f", "-"], input=manifest, text=True, check=True)
        print("Wait for the Machine and Node to become Ready, then install NFD and the NVIDIA GPU Operator.")
    else:
        print(manifest, end="")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, subprocess.CalledProcessError) as error:
        sys.exit(str(error))
