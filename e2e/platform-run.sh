#!/usr/bin/env bash
# =============================================================================
# Platform Engineer E2E Validation Script
# =============================================================================
# Validates the full Platform Engineer workshop flow:
#   Phase 1 — GPU Node Pool          (lab_1_gpu_nodepool)
#   Phase 2 — Cluster Operators      (lab_2_operators)
#   Phase 3 — Platform Instances     (lab 1.3 Step 1)
#   Phase 4 — LLMInferenceService + MaaS (lab 1.3 Steps 2–3)
#   Phase 5 — Developer Template     (lab 1.4)
#
# Prerequisites:
#   - oc is logged in with cluster-admin
# =============================================================================
set -euo pipefail

# ── Configuration ─────────────────────────────────────────────────────────────
GIT_SERVER="${GIT_SERVER:-https://gitpop.apps.sno.msl.cloud}"
TEMPLATE_URL="${TEMPLATE_URL:-https://github.com/fjcloud/go-app-template}"
GPU_INSTANCE_TYPE="${GPU_INSTANCE_TYPE:-Standard_NC24ads_A100_v4}"
LLM_NS="${LLM_NS:-llm-serving}"
LLM_IS="${LLM_IS:-qwen3}"
MAAS_HOST="${MAAS_HOST:-}"

# ── Colours ───────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'

step()    { echo -e "\n${BOLD}${BLUE}══ $* ${NC}"; }
ok()      { echo -e "  ${GREEN}✅  $*${NC}"; }
warn()    { echo -e "  ${YELLOW}⚠️   $*${NC}"; }
fail()    { echo -e "  ${RED}❌  $*${NC}"; exit 1; }
info()    { echo -e "  ${CYAN}→  $*${NC}"; }

FAILURES=0

check() {
  local label=$1; shift
  if eval "$@" &>/dev/null; then
    ok "$label"
  else
    warn "FAIL: $label"
    FAILURES=$((FAILURES + 1))
  fi
}

csv_succeeded() {
  # Check specific namespaces — avoids permission issues on ARO HCP with -A
  local pattern=$1
  for ns in redhat-ods-operator nvidia-gpu-operator openshift-nfd \
             openshift-operators openshift-gitops-operator openshift-pipelines \
             openshift-gitops cert-manager-operator openshift-lws-operator \
             kuadrant-system cert-manager; do
    if oc get csv -n "$ns" --no-headers 2>/dev/null \
        | grep -i "$pattern" | grep -q "Succeeded"; then
      return 0
    fi
  done
  return 1
}

# ── Preflight ─────────────────────────────────────────────────────────────────
step "Preflight checks"

command -v oc   &>/dev/null || fail "oc not found"
command -v curl &>/dev/null || fail "curl not found"
command -v git  &>/dev/null || fail "git not found"

info "Cluster: $(oc whoami --show-server)"
info "User:    $(oc whoami)"
info "Git server: $GIT_SERVER"
info "Template: $TEMPLATE_URL"

oc cluster-info &>/dev/null || fail "Not connected to a cluster"
ok "Cluster reachable"
# shellcheck source=../deploy/cluster-env.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/deploy/cluster-env.sh"
MAAS_HOST="${MAAS_HOST:-$(workshop_maas_host)}"
LLM_URL_MAAS="https://${MAAS_HOST}/llm-serving/${LLM_IS}/v1"

# ─────────────────────────────────────────────────────────────────────────────
# PHASE 1 — GPU Node Pool (lab_1_gpu_nodepool)
# ─────────────────────────────────────────────────────────────────────────────
step "Phase 1: GPU Node Pool"

GPU_NODES=$(oc get nodes \
  -l "node.kubernetes.io/instance-type=${GPU_INSTANCE_TYPE}" \
  -o jsonpath='{.items[*].metadata.name}' 2>/dev/null || true)

if [[ -n "$GPU_NODES" ]]; then
  ok "GPU node(s) found with instance type ${GPU_INSTANCE_TYPE}: $(echo "$GPU_NODES" | wc -w) node(s)"
  info "Nodes: $GPU_NODES"
else
  warn "FAIL: No ${GPU_INSTANCE_TYPE} GPU node found"
  FAILURES=$((FAILURES + 1))
fi

for node in $GPU_NODES; do
  STATUS=$(oc get node "$node" \
    -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "")
  if [[ "$STATUS" == "True" ]]; then
    ok "GPU node $node is Ready"
  else
    warn "FAIL: GPU node $node NOT Ready (status: ${STATUS:-unknown})"
    FAILURES=$((FAILURES + 1))
  fi
done

# Taint check
check "GPU node has nvidia.com/gpu NoSchedule taint" \
  "oc get nodes -o json | python3 -c \"
import sys,json
d=json.load(sys.stdin)
for n in d['items']:
    taints = n['spec'].get('taints',[])
    if any(t.get('key','')=='nvidia.com/gpu' for t in taints):
        sys.exit(0)
sys.exit(1)
\""

# NFD label check
check "GPU node labeled nvidia.com/gpu.present=true (NFD)" \
  "oc get nodes -l 'nvidia.com/gpu.present=true' --no-headers | grep -q '.'"

# ─────────────────────────────────────────────────────────────────────────────
# PHASE 2 — Cluster Operators (lab_2_operators)
# ─────────────────────────────────────────────────────────────────────────────
step "Phase 2: Cluster Operators — CSV status"

check "Red Hat OpenShift AI (RHOAI) operator Succeeded" \
  "csv_succeeded rhods-operator"

check "NVIDIA GPU Operator Succeeded" \
  "csv_succeeded gpu-operator-certified"

check "Node Feature Discovery (NFD) Succeeded" \
  "csv_succeeded nfd"

check "OpenShift Dev Spaces operator Succeeded" \
  "csv_succeeded devspacesoperator"

check "OpenShift GitOps operator Succeeded" \
  "csv_succeeded openshift-gitops-operator"

check "OpenShift Pipelines operator Succeeded" \
  "csv_succeeded openshift-pipelines-operator"

check "Red Hat Connectivity Link operator Succeeded" \
  "csv_succeeded rhcl-operator"
check "cert-manager operator Succeeded" \
  "csv_succeeded cert-manager-operator"
check "Leader Worker Set operator Succeeded" \
  "csv_succeeded leader-worker-set"

step "Phase 2b: Operator deployments Available"

check "Dev Spaces operator deployment Available" \
  "oc get deployment devspaces-operator -n openshift-operators \
     -o jsonpath='{.status.availableReplicas}' | grep -qE '[1-9]'"

check "KServe controller deployment Available (RHOAI)" \
  "oc get deployment kserve-controller-manager -n redhat-ods-applications \
     -o jsonpath='{.status.availableReplicas}' | grep -qE '[1-9]'"

check "Tekton pipelines controller deployment Available" \
  "oc get deployment tekton-pipelines-controller -n openshift-pipelines \
     -o jsonpath='{.status.availableReplicas}' | grep -qE '[1-9]'"

check "GitOps ArgoCD server deployment Available (openshift-gitops)" \
  "oc get deployment openshift-gitops-server -n openshift-gitops \
     -o jsonpath='{.status.availableReplicas}' | grep -qE '[1-9]'"

step "Phase 2c: Tekton tasks"

check "Tekton remote resolvers deployment Available" \
  "oc get deployment tekton-pipelines-remote-resolvers -n openshift-pipelines \
   -o jsonpath='{.status.availableReplicas}' | grep -qE '[1-9]'"

check "buildah Task installed in openshift-pipelines" \
  "oc get task buildah -n openshift-pipelines"

check "git-clone Task installed in openshift-pipelines" \
  "oc get task git-clone -n openshift-pipelines"

# ─────────────────────────────────────────────────────────────────────────────
# PHASE 3 — Platform Instances (lab 1.3 Step 1)
# ─────────────────────────────────────────────────────────────────────────────
step "Phase 3: Platform Instances"

# DataScienceCluster — KServe, MaaS, dashboard / OGX
check "DataScienceCluster default-dsc exists" \
  "oc get datasciencecluster default-dsc"

for component in KserveReady ModelsAsAServiceReady AIGatewayReady DashboardReady OGXReady; do
  COND=$(oc get datasciencecluster default-dsc \
    -o jsonpath="{.status.conditions[?(@.type==\"${component}\")].status}" 2>/dev/null || echo "")
  if [[ "$COND" == "True" ]]; then
    ok "RHOAI $component — True"
  else
    warn "FAIL: RHOAI $component not True (status: ${COND:-not found})"
    FAILURES=$((FAILURES + 1))
  fi
done

# Dev Spaces CheCluster
CHE_PHASE=$(oc get checluster devspaces -n openshift-operators \
  -o jsonpath='{.status.chePhase}' 2>/dev/null || echo "")
if [[ "$CHE_PHASE" == "Active" ]]; then
  ok "Dev Spaces CheCluster — Active"
else
  warn "FAIL: Dev Spaces CheCluster not Active (phase: ${CHE_PHASE:-not found})"
  FAILURES=$((FAILURES + 1))
fi

DEVSPACES_URL=$(oc get checluster devspaces -n openshift-operators \
  -o jsonpath='{.status.cheURL}' 2>/dev/null || echo "")
[[ -n "$DEVSPACES_URL" ]] && ok "Dev Spaces URL available: $DEVSPACES_URL" \
  || { warn "Dev Spaces URL not yet available"; FAILURES=$((FAILURES+1)); }

# Dev Spaces AI tool registry + Qwen3.8 OpenCode config (lab_4 / lab_5)
check "AI tool registry ConfigMap exists" \
  "oc get configmap ai-tool-registry -n openshift-operators"
check "AI registry includes OpenCode injector" \
  "oc get configmap ai-tool-registry -n openshift-operators -o jsonpath='{.data.registry\.json}' | grep -q 'opencodeai/opencode'"
check "OpenCode workspace config ConfigMap exists" \
  "oc get configmap opencode-workspace-config -n openshift-operators"
check "OpenCode config uses OPENAI_BASE_URL (cluster-agnostic)" \
  "oc get configmap opencode-workspace-config -n openshift-operators -o jsonpath='{.data.opencode\.json}' | grep -q '{env:OPENAI_BASE_URL}'"
check "OpenCode MaaS API key Secret exists" \
  "oc get secret ai-provider-openai-api-key -n openshift-operators"
check "MaaS secret injects OPENAI_BASE_URL" \
  "oc get secret ai-provider-openai-api-key -n openshift-operators -o jsonpath='{.data.OPENAI_BASE_URL}' | grep -q ."
check "VS Code editor config disables GitHub Copilot" \
  "oc get configmap vscode-editor-configurations -n openshift-operators -o jsonpath='{.data.settings\.json}' | grep -q 'chat.disableAIFeatures'"
check "Continue is not a recommended VS Code extension" \
  "! oc get configmap vscode-editor-configurations -n openshift-operators -o jsonpath='{.data.extensions\.json}' | grep -q 'Continue.continue'"
check "Continue workspace ConfigMap is absent" \
  "! oc get configmap continue-workspace-config -n openshift-operators"

# NFD NodeFeatureDiscovery
check "NodeFeatureDiscovery nfd-instance exists" \
  "oc get nodefeaturediscovery nfd-instance -n openshift-nfd"

# GPU ClusterPolicy
check "NVIDIA GPU ClusterPolicy exists" \
  "oc get clusterpolicy gpu-cluster-policy"

GPU_POL_STATE=$(oc get clusterpolicy gpu-cluster-policy \
  -o jsonpath='{.status.state}' 2>/dev/null || echo "")
if [[ "$GPU_POL_STATE" == "ready" ]]; then
  ok "GPU ClusterPolicy state — ready"
else
  warn "FAIL: GPU ClusterPolicy not ready (state: ${GPU_POL_STATE:-unknown})"
  FAILURES=$((FAILURES + 1))
fi

# ─────────────────────────────────────────────────────────────────────────────
# PHASE 4 — LLMInferenceService + Models-as-a-Service
# ─────────────────────────────────────────────────────────────────────────────
step "Phase 4: LLMInferenceService + MaaS"

check "llm-serving namespace exists" \
  "oc get namespace $LLM_NS"

check "LLMInferenceService $LLM_IS exists" \
  "oc get llminferenceservice $LLM_IS -n $LLM_NS"

check "Model cache PVC qwen-model-cache exists" \
  "oc get pvc qwen-model-cache -n $LLM_NS"

check "MaaSModelRef $LLM_IS Ready" \
  "oc get maasmodelref $LLM_IS -n $LLM_NS -o jsonpath='{.status.phase}' | grep -q Ready"

check "MaaSAuthPolicy qwen3-access Active" \
  "oc get maasauthpolicy qwen3-access -n models-as-a-service -o jsonpath='{.status.phase}' | grep -q Active"

check "MaaSSubscription qwen3-workshop Active" \
  "oc get maassubscription qwen3-workshop -n models-as-a-service -o jsonpath='{.status.phase}' | grep -q Active"

check "Gateway maas-default-gateway Programmed" \
  "oc get gateway maas-default-gateway -n openshift-ingress -o jsonpath='{.status.conditions[?(@.type==\"Programmed\")].status}' | grep -q True"

check "Kuadrant AuthPolicy maas-gateway-auth Accepted" \
  "oc get authpolicy maas-gateway-auth -n openshift-ingress -o jsonpath='{.status.conditions[?(@.type==\"Accepted\")].status}' | grep -q True"

LLM_READY=$(oc get llminferenceservice "$LLM_IS" -n "$LLM_NS" \
  -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "")
if [[ "$LLM_READY" == "True" ]]; then
  ok "LLMInferenceService $LLM_IS — Ready"
else
  warn "FAIL: LLMInferenceService $LLM_IS not Ready (status: ${LLM_READY:-unknown})"
  FAILURES=$((FAILURES + 1))
fi

LLM_PODS=$(oc get pod -n "$LLM_NS" \
  -l app.kubernetes.io/name="${LLM_IS}" \
  --field-selector=status.phase=Running \
  --no-headers 2>/dev/null | wc -l || echo 0)
if [[ "$LLM_PODS" -lt 1 ]]; then
  LLM_PODS=$(oc get pod -n "$LLM_NS" --field-selector=status.phase=Running --no-headers 2>/dev/null | grep -c kserve || echo 0)
fi
if [[ "$LLM_PODS" -ge 1 ]]; then
  ok "LLM serving pod running ($LLM_PODS pod(s))"
else
  warn "FAIL: No running LLM serving pod in $LLM_NS"
  FAILURES=$((FAILURES + 1))
fi

step "Phase 4b: MaaS API smoke test"

MAAS_KEY=$(oc get secret ai-provider-openai-api-key -n openshift-operators \
  -o jsonpath='{.data.OPENAI_API_KEY}' 2>/dev/null | base64 -d || true)
if [[ -z "${MAAS_KEY}" || "${MAAS_KEY}" != sk-oai-* ]]; then
  info "Minting workshop MaaS API key..."
  bash "$(dirname "$0")/../deploy/inference/mint-workshop-key.sh" || true
  MAAS_KEY=$(oc get secret ai-provider-openai-api-key -n openshift-operators \
    -o jsonpath='{.data.OPENAI_API_KEY}' 2>/dev/null | base64 -d || true)
fi

LLM_RESP=$(curl -sS --max-time 120 "${LLM_URL_MAAS}/chat/completions" \
  -H "Authorization: Bearer ${MAAS_KEY}" \
  -H "Content-Type: application/json" \
  -d "{
    \"model\": \"${LLM_IS}\",
    \"messages\": [{\"role\": \"user\", \"content\": \"Reply with exactly: PLATFORM_OK\"}],
    \"max_tokens\": 20,
    \"chat_template_kwargs\": {\"enable_thinking\": false}
  }" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['choices'][0]['message']['content'])" 2>/dev/null || echo "")

if echo "$LLM_RESP" | grep -q "PLATFORM_OK"; then
  ok "MaaS API smoke test passed: $LLM_RESP"
else
  warn "FAIL: MaaS did not return expected response (got: '${LLM_RESP}')"
  FAILURES=$((FAILURES + 1))
fi

# ─────────────────────────────────────────────────────────────────────────────
# PHASE 5 — Developer Template (lab 1.4)
# ─────────────────────────────────────────────────────────────────────────────
step "Phase 5: Developer Template Repository"

info "Template URL: $TEMPLATE_URL"
check "Template repo reachable on GitHub" \
  "curl -sf --max-time 10 '${TEMPLATE_URL}/archive/main.tar.gz' -o /dev/null"

TMPDIR=$(mktemp -d)
trap "rm -rf $TMPDIR" EXIT

info "Cloning template repository..."
CLONE_OK=0
if git clone --quiet "$TEMPLATE_URL" "$TMPDIR/go-app-template" 2>/dev/null; then
  ok "Template cloned successfully"
  CLONE_OK=1
else
  warn "FAIL: Could not clone template repository"
  FAILURES=$((FAILURES + 1))
fi

if [[ "$CLONE_OK" -eq 1 ]]; then
  REPO="$TMPDIR/go-app-template"

  step "Phase 5a: Template file structure"
  for f in \
    "AGENTS.md" \
    "devfile.yaml" \
    "README.md" \
    "deploy/base/deployment.yaml" \
    "deploy/base/service.yaml" \
    "deploy/base/route.yaml" \
    "deploy/base/kustomization.yaml" \
    "pipeline/base/pipeline.yaml" \
    "pipeline/base/kustomization.yaml" \
    "gitops/base/argocd.yaml" \
    "gitops/base/kustomization.yaml"; do
    if [[ -f "$REPO/$f" ]]; then
      ok "Template contains $f"
    else
      warn "FAIL: Template missing $f"
      FAILURES=$((FAILURES + 1))
    fi
  done

  step "Phase 5b: AGENTS.md content"
  for pattern in "main.go" "Dockerfile" "ubi9/go-toolset" "gitpop" "oc apply"; do
    check "AGENTS.md mentions '$pattern'" \
      "grep -q '$pattern' '$REPO/AGENTS.md'"
  done

  step "Phase 5c: OpenCode config is platform-injected (not in the Git template)"
  check "opencode-workspace-config is valid JSON" \
    "oc get configmap opencode-workspace-config -n openshift-operators -o jsonpath='{.data.opencode\.json}' | python3 -c \"import sys,json; json.load(sys.stdin)\""
  check "opencode-workspace-config has autoupdate disabled" \
    "oc get configmap opencode-workspace-config -n openshift-operators -o jsonpath='{.data.opencode\.json}' | python3 -c \"import sys,json; d=json.load(sys.stdin); assert d.get('autoupdate')==False\""
  check "OpenCode config names the model Qwen3.8" \
    "oc get configmap opencode-workspace-config -n openshift-operators -o jsonpath='{.data.opencode\.json}' | python3 -c \"import sys,json; d=json.load(sys.stdin); assert d['provider']['qwen3']['name']=='Qwen3.8'; assert d['provider']['qwen3']['models']['qwen3']['name']=='Qwen3.8'\""
  check "opencode-workspace-config caps output tokens" \
    "oc get configmap opencode-workspace-config -n openshift-operators -o jsonpath='{.data.opencode\.json}' | python3 -c \"import sys,json; d=json.load(sys.stdin); assert d['provider']['qwen3']['models']['qwen3']['limit']['context']==262144; assert d['provider']['qwen3']['models']['qwen3']['limit']['output']==32768\""

  step "Phase 5d: devfile.yaml"
  for pattern in "GIT_SERVER" "gitpop"; do
    check "devfile.yaml contains '$pattern'" \
      "grep -q '$pattern' '$REPO/devfile.yaml'"
  done

  step "Phase 5e: AGENTS.md is the deploy runbook (no Ansible)"
  check "AGENTS.md has gitpop init" \
    "grep -q 'gitpop init' '$REPO/AGENTS.md'"
  check "AGENTS.md has oc apply -k pipeline" \
    "grep -q 'oc apply -k pipeline/base' '$REPO/AGENTS.md'"
  check "AGENTS.md has oc apply -k gitops" \
    "grep -q 'oc apply -k gitops/base' '$REPO/AGENTS.md'"
  check "Template has no Ansible scripts/" \
    "test ! -d '$REPO/scripts'"

  step "Phase 5f: Tekton pipeline manifest"
  check "pipeline.yaml references buildah" \
    "grep -q 'buildah' '$REPO/pipeline/base/pipeline.yaml'"
  check "pipeline.yaml has DOCKERFILE parameter" \
    "grep -q 'DOCKERFILE' '$REPO/pipeline/base/pipeline.yaml'"

  step "Phase 5g: ArgoCD manifest"
  check "argocd.yaml is kind ArgoCD" \
    "grep -q 'kind: ArgoCD' '$REPO/gitops/base/argocd.yaml'"

  step "Phase 5h: deployment.yaml uses placeholder image"
  check "deployment.yaml uses 'placeholder' image (updated at deploy time per AGENTS.md)" \
    "grep -q 'placeholder' '$REPO/deploy/base/deployment.yaml'"
fi

step "Phase 5i: Git server reachability"
check "Git server is reachable" \
  "curl -sf --max-time 10 '$GIT_SERVER' -o /dev/null"
check "gitpop binary download endpoint is reachable" \
  "curl -sf --max-time 10 '$GIT_SERVER/dl/gitpop?os=linux&arch=amd64' -o /dev/null"

step "Phase 5j: DevSpaces developer launch URL"
if [[ -n "${DEVSPACES_URL:-}" ]]; then
  LAUNCH_URL="${DEVSPACES_URL}/#${TEMPLATE_URL}?ai-provider=opencodeai/opencode"
  ok "Developer launch URL constructed:"
  info "$LAUNCH_URL"
else
  warn "DevSpaces URL not available — CheCluster may not be Active"
  FAILURES=$((FAILURES + 1))
fi

# ── Summary ───────────────────────────────────────────────────────────────────
step "Summary"

echo ""
echo -e "  Cluster        : ${CYAN}$(oc whoami --show-server)${NC}"
echo -e "  GPU node(s)    : ${CYAN}${GPU_NODES:-<none found>}${NC}"
echo -e "  LLM endpoint   : ${CYAN}${LLM_URL_MAAS}${NC}"
echo -e "  Dev Spaces URL : ${CYAN}${DEVSPACES_URL:-<not ready>}${NC}"
echo -e "  Template repo  : ${CYAN}${TEMPLATE_URL}${NC}"
echo -e "  Git server     : ${CYAN}${GIT_SERVER}${NC}"
if [[ -n "${DEVSPACES_URL:-}" ]]; then
  echo -e "  Developer URL  : ${CYAN}${DEVSPACES_URL}/#${TEMPLATE_URL}?ai-provider=opencodeai/opencode${NC}"
fi
echo ""

if [[ $FAILURES -eq 0 ]]; then
  echo -e "${GREEN}${BOLD}All Platform Engineer checks passed ✅${NC}"
else
  echo -e "${RED}${BOLD}$FAILURES check(s) failed — review warnings above ❌${NC}"
  exit 1
fi
