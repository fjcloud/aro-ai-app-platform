"""Resource-group selection regression checks; no Azure account required."""
import copy
import importlib.util
from pathlib import Path
import unittest


spec = importlib.util.spec_from_file_location(
    "workshop_env", Path(__file__).resolve().parents[1] / "scripts/workshop-env.py")
workshop_env = importlib.util.module_from_spec(spec)
spec.loader.exec_module(workshop_env)

SUBSCRIPTION = "00000000-0000-0000-0000-000000000001"
GROUP_ID = f"/subscriptions/{SUBSCRIPTION}/resourceGroups/openenv-test"
CLUSTER_ID = GROUP_ID + "/providers/Microsoft.RedHatOpenShift/openShiftClusters/lab"
CLUSTER = {
    "id": CLUSTER_ID,
    "apiserverProfile": {"url": "https://api.example.test:6443/"},
    "clusterProfile": {"resourceGroupId": f"/subscriptions/{SUBSCRIPTION}/resourceGroups/aro-managed"},
}
INFRA = {"status": {
    "apiServerURL": "https://api.example.test:6443",
    "platformStatus": {"type": "Azure", "azure": {"resourceGroupName": "aro-managed"}},
}}


class ResourceGroupDiscovery(unittest.TestCase):
    def setUp(self):
        self.clusters = [copy.deepcopy(CLUSTER)]
        self.infra = copy.deepcopy(INFRA)
        self.group = {"id": GROUP_ID, "name": "openenv-test"}
        self.calls = []

    def read(self, *command):
        self.calls.append(command)
        if command[:3] == ("az", "account", "show"):
            return {"id": SUBSCRIPTION}
        if command[0] == "oc":
            return self.infra
        if command[:3] == ("az", "aro", "list"):
            return self.clusters
        if command[:3] == ("az", "group", "show"):
            return self.group
        self.fail(f"Unexpected CLI call: {command}")

    def test_detects_owner_group_instead_of_managed_machine_group(self):
        values = workshop_env.discover(self.read)
        self.assertEqual(values["RESOURCEGROUP"], "openenv-test")
        self.assertEqual(values["RESOURCEGROUP_ID"], GROUP_ID)
        self.assertEqual(values["ARO_CLUSTER_ID"], CLUSTER_ID)
        self.assertIn(("az", "group", "show", "--name", "openenv-test", "--subscription", SUBSCRIPTION, "-o", "json"), self.calls)

    def test_does_not_select_first_unrelated_cluster(self):
        other = copy.deepcopy(CLUSTER)
        other["apiserverProfile"]["url"] = "https://api.other.test:6443"
        other["id"] = CLUSTER_ID.replace("openenv-test", "unrelated")
        self.clusters.insert(0, other)
        self.assertEqual(workshop_env.discover(self.read)["RESOURCEGROUP_ID"], GROUP_ID)

    def test_ambiguous_cluster_match_stops(self):
        other = copy.deepcopy(CLUSTER)
        other["id"] += "-duplicate"
        self.clusters.append(other)
        with self.assertRaisesRegex(ValueError, "found 2"):
            workshop_env.discover(self.read)

    def test_no_matching_cluster_stops(self):
        self.clusters = []
        with self.assertRaisesRegex(ValueError, "found 0"):
            workshop_env.discover(self.read)

    def test_wrong_subscription_in_resource_id_stops(self):
        self.clusters[0]["id"] = CLUSTER_ID.replace(SUBSCRIPTION, "other-subscription")
        with self.assertRaisesRegex(ValueError, "selected Azure subscription"):
            workshop_env.discover(self.read)

    def test_non_azure_cluster_stops(self):
        self.infra["status"]["platformStatus"]["type"] = "AWS"
        with self.assertRaisesRegex(ValueError, "not on Azure"):
            workshop_env.discover(self.read)

    def test_unexpected_group_response_stops(self):
        self.group["id"] = GROUP_ID.replace("openenv-test", "aro-managed")
        with self.assertRaisesRegex(ValueError, "different resource group"):
            workshop_env.discover(self.read)

    def test_url_case_and_trailing_slash_are_normalized(self):
        self.clusters[0]["apiserverProfile"]["url"] = "https://API.EXAMPLE.TEST:6443/"
        self.assertEqual(workshop_env.discover(self.read)["RESOURCEGROUP_ID"], GROUP_ID)

    def test_missing_or_different_api_port_cannot_match(self):
        self.clusters[0]["apiserverProfile"]["url"] = "https://api.example.test"
        with self.assertRaisesRegex(ValueError, "found 0"):
            workshop_env.discover(self.read)


if __name__ == "__main__":
    unittest.main()
