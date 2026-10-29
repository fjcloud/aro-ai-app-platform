#!/usr/bin/env python3
"""Discover the connected ARO resource; optionally log in using its Azure credentials."""
import argparse
import json
from pathlib import Path
import re
import shlex
import subprocess
import sys
from urllib.parse import urlsplit


def read_json(*command):
    return json.loads(subprocess.check_output(command, text=True, timeout=120))


def endpoint(value):
    try:
        url = urlsplit(value)
        if url.scheme != "https" or not url.hostname or url.username or url.password:
            return None
        return url.hostname.lower(), url.port or 443, url.path.rstrip("/")
    except (ValueError, TypeError, AttributeError):
        return None


def discover(read=read_json):
    subscription = read("az", "account", "show", "-o", "json")["id"]
    infra = read("oc", "--request-timeout=30s", "get", "infrastructure", "cluster", "-o", "json")
    status = infra.get("status", {})
    if status.get("platformStatus", {}).get("type") != "Azure":
        raise ValueError("The connected OpenShift cluster is not on Azure.")
    api = endpoint(status.get("apiServerURL", ""))
    if api is None:
        raise ValueError("The cluster does not report a valid HTTPS API URL.")
    clusters = read("az", "aro", "list", "--subscription", subscription, "-o", "json")
    matches = [c for c in clusters if endpoint(c.get("apiserverProfile", {}).get("url", "")) == api]
    if len(matches) != 1:
        raise ValueError(f"Expected one ARO resource matching the connected cluster, found {len(matches)}. Check az account show, oc whoami --show-server and Azure read permissions.")
    cluster_id = matches[0]["id"]
    match = re.fullmatch(r"/subscriptions/([^/]+)/resourceGroups/([^/]+)/providers/Microsoft.RedHatOpenShift/openShiftClusters/[^/]+", cluster_id, re.IGNORECASE)
    if not match or match[1].lower() != subscription.lower():
        raise ValueError("The ARO resource ID does not belong to the selected Azure subscription.")
    group_name = match[2]
    group = read("az", "group", "show", "--name", group_name, "--subscription", subscription, "-o", "json")
    expected_id = f"/subscriptions/{subscription}/resourceGroups/{group_name}"
    if group["id"].lower() != expected_id.lower() or group["name"].lower() != group_name.lower():
        raise ValueError("Azure returned a different resource group than the ARO resource owner.")
    return {"SUBSCRIPTION": subscription, "RESOURCEGROUP": group["name"],
            "RESOURCEGROUP_ID": group["id"], "ARO_CLUSTER_ID": cluster_id}


def login(values, read=read_json, run=subprocess.run):
    credentials = read("az", "aro", "list-credentials",
                       "--name", values["ARO_CLUSTER_ID"].rsplit("/", 1)[-1],
                       "--resource-group", values["RESOURCEGROUP"],
                       "--subscription", values["SUBSCRIPTION"], "-o", "json")
    username = credentials.get("kubeadminUsername")
    password = credentials.get("kubeadminPassword")
    if not isinstance(username, str) or not username or not isinstance(password, str) or not password:
        raise ValueError("Azure returned no ARO administrator credentials.")
    server = run(["oc", "whoami", "--show-server"], capture_output=True, text=True, timeout=30)
    if server.returncode or endpoint(server.stdout.strip()) is None:
        raise ValueError("Could not identify the existing ARO API server.")
    # Capture diagnostics: subprocess errors must never print a command containing the password.
    try:
        result = run(["oc", "login", "--server", server.stdout.strip(),
                      "--username", username, "--password", password],
                     capture_output=True, text=True, timeout=120)
    except subprocess.SubprocessError:
        raise ValueError("ARO login failed. Check Azure access and the assigned cluster credentials.") from None
    if result.returncode:
        raise ValueError("ARO login failed. Check Azure access and the assigned cluster credentials.")
    print("Signed in to the existing ARO cluster.")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path,
                        default=Path(__file__).resolve().parents[1] / "state/workshop.env")
    parser.add_argument("--login", action="store_true",
                        help="Sign in with the ARO administrator credentials retrieved from Azure")
    args = parser.parse_args()
    try:
        values = discover()
        if args.login:
            login(values)
        args.output.parent.mkdir(parents=True, exist_ok=True)
        temporary = args.output.with_suffix(".env.tmp")
        temporary.write_text("# Discovered from the connected ARO cluster; no credentials.\n" +
                             "".join(f"export {key}={shlex.quote(value)}\n" for key, value in values.items()))
        temporary.replace(args.output)
    except (ValueError, KeyError, OSError, subprocess.SubprocessError) as error:
        print(f"Workshop environment detection failed: {error}", file=sys.stderr)
        return 1
    print(f"Workshop resource group: {values['RESOURCEGROUP']}")
    print(f"Resource group ID: {values['RESOURCEGROUP_ID']}")
    print(f"Saved {args.output}. Load it with: source {shlex.quote(str(args.output))}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
