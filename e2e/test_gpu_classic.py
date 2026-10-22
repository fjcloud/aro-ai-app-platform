"""Preflight and isolation checks for adding a GPU to an existing ARO cluster."""
import copy
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("gpu", Path(__file__).parents[1] / "scripts/gpu-classic.py")
gpu = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gpu)


class GPUPlanTest(unittest.TestCase):
    def setUp(self):
        label = gpu.MACHINESET_LABEL
        self.machine = {"apiVersion": "machine.openshift.io/v1beta1", "kind": "MachineSet",
                        "metadata": {"name": "cpu", "uid": "old", "resourceVersion": "123"},
                        "status": {"readyReplicas": 1}, "spec": {
                            "replicas": 3,
                            "selector": {"matchLabels": {label: "cpu", "machine.openshift.io/cluster-api-cluster": "lab"}},
                            "template": {"metadata": {"labels": {label: "cpu"}}, "spec": {
                                "providerSpec": {"value": {"image": {"sku": "421-v2"},
                                                           "zone": "2", "location": "eastus", "vmSize": "Standard_D4s_v3",
                                                           "subnet": "existing-worker-subnet", "credentialsSecret": {"name": "existing"}}}}}}}
        self.sku = {"name": "Standard_NC24ads_A100_v4", "family": "StandardNCADSA100v4Family", "restrictions": [], "locationInfo": [{"zones": ["2"]}]}
        self.usage = [{"name": {"value": "StandardNCADSA100v4Family"}, "limit": 24, "currentValue": 0},
                      {"name": {"value": "cores"}, "limit": 100, "currentValue": 38}]

    def plan(self):
        return gpu.plan([self.machine], self.sku, self.usage, "/subscriptions/lab-sub/resourceGroups/cluster", "lab-sub")

    def test_isolated_one_worker_preserves_network_and_cpu_template(self):
        original = copy.deepcopy(self.machine)
        result = self.plan()
        self.assertEqual(self.machine, original)
        self.assertEqual(result["spec"]["replicas"], 1)
        self.assertNotIn("status", result)
        self.assertNotIn("uid", result["metadata"])
        self.assertEqual(result["spec"]["selector"]["matchLabels"][gpu.MACHINESET_LABEL], result["metadata"]["name"])
        s = result["spec"]["template"]["spec"]
        self.assertEqual(s["providerSpec"]["value"]["subnet"], "existing-worker-subnet")
        self.assertEqual(s["taints"][0]["effect"], "NoSchedule")

    def test_missing_family_quota(self):
        self.usage[0]["limit"] = 0
        with self.assertRaisesRegex(ValueError, "24 unused StandardNCADSA100v4Family"):
            self.plan()

    def test_consumed_regional_quota(self):
        self.usage[1]["currentValue"] = 90
        with self.assertRaisesRegex(ValueError, "regional vCPUs"):
            self.plan()

    def test_different_subscription(self):
        with self.assertRaisesRegex(ValueError, "different subscriptions"):
            gpu.plan([self.machine], self.sku, self.usage, "/subscriptions/other/resourceGroups/cluster", "lab-sub")

    def test_restricted_zone(self):
        self.sku["restrictions"] = [{"type": "Zone", "restrictionInfo": {"zones": ["2"]}}]
        with self.assertRaisesRegex(ValueError, "available GPU zone"):
            self.plan()

    def test_four_t4_require_64_cores(self):
        self.sku["name"] = "Standard_NC64as_T4_v3"
        self.sku["family"] = "Standard NCASv3_T4 Family"
        self.usage[0]["name"]["value"] = self.sku["family"]
        self.usage[0]["limit"] = 512
        self.usage[1]["limit"] = 3340
        result = self.plan()
        self.assertEqual(result["spec"]["template"]["spec"]["providerSpec"]["value"]["vmSize"], self.sku["name"])
        self.usage[0]["limit"] = 63
        with self.assertRaisesRegex(ValueError, "64 unused"):
            self.plan()

    def test_generation_one(self):
        self.machine["spec"]["template"]["spec"]["providerSpec"]["value"]["image"]["sku"] = "aro_421"
        with self.assertRaisesRegex(ValueError, "generation 2"):
            self.plan()


if __name__ == "__main__":
    unittest.main()
