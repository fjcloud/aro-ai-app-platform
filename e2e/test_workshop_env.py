"""Resource-group selection regression checks; no Azure account required."""
import copy
import contextlib
import importlib.util
import io
from pathlib import Path
import unittest
import subprocess


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

    def test_login_discovery_works_without_an_authenticated_cluster_request(self):
        values = workshop_env.discover(self.read, api_server='https://api.example.test:6443')
        self.assertEqual(values['ARO_CLUSTER_ID'], CLUSTER_ID)
        self.assertFalse(any(command[0] == 'oc' for command in self.calls))

    def test_login_discovery_still_rejects_an_unmatched_server(self):
        with self.assertRaisesRegex(ValueError, 'found 0'):
            workshop_env.discover(self.read, api_server='https://api.other.test:6443')


class AROLogin(unittest.TestCase):
    def setUp(self):
        self.values = {"SUBSCRIPTION": SUBSCRIPTION, "RESOURCEGROUP": "openenv-test",
                       "ARO_CLUSTER_ID": CLUSTER_ID}
        self.credentials = {"kubeadminUsername": "kubeadmin", "kubeadminPassword": "private-test-password"}
        self.calls = []

    def read(self, *command):
        self.calls.append(list(command))
        return self.credentials

    def run_cli(self, command, **kwargs):
        self.calls.append(command)
        if command == ['oc', 'whoami', '--show-server']:
            return subprocess.CompletedProcess(command, 0, 'https://api.example.test:6443\n', '')
        return subprocess.CompletedProcess(command, 0, 'private-test-password', '')

    def test_login_uses_credentials_from_the_matched_aro_resource(self):
        original = self.values.copy()
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            workshop_env.login(self.values, self.read, self.run_cli)
        self.assertEqual(self.calls[0], ['az', 'aro', 'list-credentials', '--name', 'lab',
                                        '--resource-group', 'openenv-test', '--subscription', SUBSCRIPTION, '-o', 'json'])
        self.assertEqual(self.calls[-1], ['oc', 'login', '--server', 'https://api.example.test:6443',
                                        '--username', 'kubeadmin', '--password', 'private-test-password'])
        self.assertNotIn('private-test-password', output.getvalue())
        self.assertEqual(self.values, original)

    def test_missing_credentials_stop_before_oc_login(self):
        self.credentials.pop('kubeadminPassword')
        with self.assertRaisesRegex(ValueError, 'no ARO administrator credentials'):
            workshop_env.login(self.values, self.read, self.run_cli)
        self.assertEqual(len(self.calls), 1)

    def test_login_failure_does_not_expose_password_or_raw_diagnostics(self):
        def fail(command, **kwargs):
            if command[:2] == ['oc', 'login']:
                return subprocess.CompletedProcess(command, 1, '', 'private-test-password')
            return self.run_cli(command, **kwargs)
        with self.assertRaises(ValueError) as caught:
            workshop_env.login(self.values, self.read, fail)
        self.assertNotIn('private-test-password', str(caught.exception))

    def test_login_timeout_does_not_include_password_arguments(self):
        def timeout(command, **kwargs):
            if command[:2] == ['oc', 'login']:
                raise subprocess.TimeoutExpired(command, 120)
            return self.run_cli(command, **kwargs)
        with self.assertRaises(ValueError) as caught:
            workshop_env.login(self.values, self.read, timeout)
        self.assertNotIn('private-test-password', str(caught.exception))


if __name__ == "__main__":
    unittest.main()
