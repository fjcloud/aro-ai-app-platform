ROOT := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
CONFIRM ?=

.DEFAULT_GOAL := help

.PHONY: help onboard deploy status kubeconfig gpu gpu-classic delete terminal

help:
	@echo "make onboard         Check access to the existing ARO cluster and Azure lab."
	@echo "make deploy          Create the ARO HCP cluster, pull secret, and console login."
	@echo "make status          Show the cluster, node pools, and console URL."
	@echo "make kubeconfig      Refresh state/kubeconfig for oc."
	@echo "make gpu-classic     Plan a single-T4 worker for an existing classic ARO cluster (quota required)."
	@echo "make gpu             Add one GPU node to a separately provisioned ARO HCP cluster."
	@echo "make delete CONFIRM=yes"
	@echo "                     Delete the cluster, its resource groups, and the Entra apps."
	@echo "make terminal        Open the browser Azure CLI terminal."

deploy:
	@"$(ROOT)scripts/deploy.sh"

onboard:
	@bash "$(ROOT)scripts/onboard.sh"

status:
	@"$(ROOT)scripts/status.sh"

kubeconfig:
	@"$(ROOT)scripts/kubeconfig.sh"

gpu:
	@"$(ROOT)scripts/gpu.sh"

gpu-classic:
	@python3 "$(ROOT)scripts/gpu-classic.py" $(GPU_ARGS)

delete:
	@CONFIRM="$(CONFIRM)" "$(ROOT)scripts/delete.sh"

terminal:
	@echo "Open https://fjcloud.github.io/cloud-terminal-pages/ and select the ARO guide, and enter your workshop ID, email, and workshop password."
