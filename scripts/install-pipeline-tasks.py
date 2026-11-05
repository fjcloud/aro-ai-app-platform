#!/usr/bin/env python3
"""Reuse installed Red Hat tasks, adapting only the ARO user-namespace workspace."""
import copy
import json
import subprocess


def adapt(task, workspace):
    result = {"apiVersion": task["apiVersion"], "kind": "Task",
              "metadata": {"name": "workshop-" + task["metadata"]["name"],
                           "namespace": "openshift-pipelines",
                           "annotations": {"workshop.fjcloud.io/task-source":
                               "openshift-pipelines/" + task["metadata"]["name"]}},
              "spec": copy.deepcopy(task["spec"])}
    for item in result["spec"]["workspaces"]:
        if item["name"] == workspace:
            item["mountPath"] = "/source"
    # ARO user namespaces need an explicit cd after container startup. The vendor
    # scripts already cd/use the workspace; starting at / avoids a stale PVC cwd.
    for step in result["spec"]["steps"]:
        step["workingDir"] = "/"
    return result


def main():
    for name, workspace in [("git-clone", "output"), ("buildah", "source")]:
        task = json.loads(subprocess.check_output(
            ["oc", "get", "task", name, "-n", "openshift-pipelines", "-o", "json"]))
        subprocess.run(["oc", "apply", "-f", "-"],
                       input=json.dumps(adapt(task, workspace)), text=True, check=True)


if __name__ == "__main__":
    main()
