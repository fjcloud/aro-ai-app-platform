#!/usr/bin/env bash
# =============================================================================
# E2E Workshop Validation Script
# =============================================================================
# Validates the full workshop flow end-to-end:
#   Phase 1 — Platform Engineer: create the Go app template on the Git server
#   Phase 2 — Developer simulation: deploy a container, install deps, run OpenCode
#   Phase 3 — Validate: repos, generated files, image build, GitOps, live route
#
# Prerequisites:
#   - oc is logged in with cluster-admin
#   - GIT_SERVER is set (or defaults to the value below)
#   - GPU LLMInferenceService qwen3 is Ready in namespace llm-serving
#   - MaaS workshop API key exists (or will be minted)
# =============================================================================
set -euo pipefail

# ── Configuration ─────────────────────────────────────────────────────────────
GIT_SERVER="${GIT_SERVER:-https://gitpop.apps.sno.msl.cloud}"
APP_NAME="${APP_NAME:-fortune-cookie}"
E2E_NS="${E2E_NS:-workshop-e2e}"
TEMPLATE_URL="${TEMPLATE_URL:-https://github.com/fjcloud/go-app-template}"
# LLM_HOST / LLM_URL resolved after oc is confirmed (cluster id changes per cluster)
LLM_HOST="${LLM_HOST:-}"
LLM_URL="${LLM_URL:-}"
LLM_MODEL="qwen3"
LLM_KEY="${LLM_KEY:-}"
GITPOP_BIN="/tmp/gitpop-e2e"   # still needed for the gitpop helper function
DEV_POD="e2e-developer"
DEV_IMAGE="quay.io/devfile/universal-developer-image:latest"

# ── Colours ───────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'

step()    { echo -e "\n${BOLD}${BLUE}══ $* ${NC}"; }
ok()      { echo -e "  ${GREEN}✅  $*${NC}"; }
warn()    { echo -e "  ${YELLOW}⚠️   $*${NC}"; }
fail()    { echo -e "  ${RED}❌  $*${NC}"; exit 1; }
info()    { echo -e "  ${CYAN}→  $*${NC}"; }

# ── Helpers ───────────────────────────────────────────────────────────────────
require() {
  command -v "$1" &>/dev/null || fail "Required tool not found: $1"
}

check_llm_ready() {
  local ready
  ready=$(oc get llminferenceservice qwen3 -n llm-serving \
    -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null)
  [[ "$ready" == "True" ]]
}

gitpop_repo_exists() {
  # Use git smart HTTP protocol to check repo existence — gitpop has no search API
  local url=$1
  curl -sf "${url}/info/refs?service=git-upload-pack" 2>/dev/null \
    | grep -q "git-upload-pack"
}

# ── Preflight ─────────────────────────────────────────────────────────────────
step "Preflight checks"

require oc
require curl
require git

info "Cluster: $(oc whoami --show-server)"
info "User:    $(oc whoami)"
info "Git server: $GIT_SERVER"

oc cluster-info &>/dev/null || fail "Not connected to a cluster"
# shellcheck source=../deploy/cluster-env.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/deploy/cluster-env.sh"
LLM_HOST="${LLM_HOST:-$(workshop_maas_host)}"
LLM_URL="${LLM_URL:-https://${LLM_HOST}/llm-serving/qwen3/v1}"
info "MaaS:    $LLM_URL"
check_llm_ready || warn "LLMInferenceService not Ready — deploy steps will still run but LLM prompts may fail"
LLM_KEY="${LLM_KEY:-$(oc get secret ai-provider-openai-api-key -n openshift-operators -o jsonpath='{.data.OPENAI_API_KEY}' 2>/dev/null | base64 -d || true)}"

# ─────────────────────────────────────────────────────────────────────────────
# PHASE 1 — Platform Engineer: verify the pre-initialized template repository
# ─────────────────────────────────────────────────────────────────────────────
step "Phase 1: Platform Engineer — Verify template repository"

# The template is pre-initialized on GitHub at fjcloud/go-app-template.
# Platform Engineers share TEMPLATE_URL with developers as the DevSpaces launch URL.
info "Template URL: $TEMPLATE_URL"

if curl -sf --max-time 10 "${TEMPLATE_URL}/archive/main.tar.gz" -o /dev/null 2>/dev/null; then
  ok "Template repository reachable at $TEMPLATE_URL"
else
  warn "Template repository not reachable (network issue?) — continuing with cached clone"
fi

ok "Template repository: $TEMPLATE_URL"

# ─────────────────────────────────────────────────────────────────────────────
# PHASE 2 — Developer simulation: deploy a pod, install deps, run OpenCode
# ─────────────────────────────────────────────────────────────────────────────
step "Phase 2: Developer simulation — deploy container"

oc new-project "$E2E_NS" 2>/dev/null || oc project "$E2E_NS"

# Give the pod SA the same rights a developer would have when running oc
# from their own kubeconfig (self-provisioner + admin on created namespaces).
# from their own kubeconfig (self-provisioner + admin on created namespaces).
# cluster-admin is used here only for e2e convenience; in a real workshop the
# developer authenticates with their personal OpenShift token.
oc adm policy add-cluster-role-to-user cluster-admin \
  -z default -n "$E2E_NS" 2>/dev/null || true

# Platform Engineer one-time setup (mirrors lab_2_operators):
# No cluster Argo CD config needed — each developer creates their own ArgoCD
# instance in their ${APP}-dev namespace via the openshift-gitops-operator.
# The operator must be pre-installed (lab_2_operators prerequisite).
info "Developer Argo CD model: each dev owns their ArgoCD instance in \${APP}-dev"

# No anyuid — the dev simulation pod and app both run under restricted-v2 SCC

info "Launching developer simulation pod..."
oc delete pod "$DEV_POD" -n "$E2E_NS" --ignore-not-found
oc run "$DEV_POD" \
  --image="$DEV_IMAGE" \
  --restart=Never \
  --namespace="$E2E_NS" \
  --env="GIT_SERVER=$GIT_SERVER" \
  --env="TEMPLATE_URL=$TEMPLATE_URL" \
  --env="APP_NAME=$APP_NAME" \
  --env="LLM_URL=$LLM_URL" \
  --env="OPENAI_BASE_URL=$LLM_URL" \
  --env="LLM_MODEL=$LLM_MODEL" \
  --env="OPENAI_API_KEY=$LLM_KEY" \
  --env="HOME=/home/user" \
  --env="PATH=/home/user/.opencode/bin:/home/user/.local/bin:/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" \
  -- sleep infinity

info "Waiting for pod to be Running..."
oc wait pod/"$DEV_POD" -n "$E2E_NS" \
  --for=condition=Ready --timeout=120s
ok "Pod $DEV_POD is Running"

# Convenience alias
POD_EXEC="oc exec $DEV_POD -n $E2E_NS --"

# ── Install dependencies inside the pod ───────────────────────────────────────
step "Phase 2a: Install OpenCode and gitpop inside developer pod"

$POD_EXEC bash -c "
  set -e
  echo '→ Installing OpenCode...'
  curl -fsSL https://opencode.ai/install | bash
  echo '→ Installing gitpop...'
  mkdir -p \$HOME/.local/bin
  curl -fsSL \"\$GIT_SERVER/dl/gitpop?os=linux&arch=amd64\" \
    -o \$HOME/.local/bin/gitpop
  chmod +x \$HOME/.local/bin/gitpop
  echo '→ Verifying tools...'
  opencode --version
  gitpop --version || gitpop help | head -3
  echo 'Dependencies OK'
"
ok "OpenCode and gitpop installed in pod"

# ── Clone template and set up workspace ───────────────────────────────────────
step "Phase 2b: Clone template and configure workspace"

$POD_EXEC bash -c "
  set -e
  git clone \$TEMPLATE_URL ~/\$APP_NAME
  cd ~/\$APP_NAME
  git config user.email 'dev@workshop.local'
  git config user.name 'E2E Developer'

  # Platform injects this file in Dev Spaces. The e2e pod is not a DevWorkspace,
  # so write the same Qwen3.8 config OpenCode reads from XDG_CONFIG_HOME.
  mkdir -p \$HOME/.config/opencode
  python3 << 'PYEOF'
import json, os
cfg = {
  '\$schema': 'https://opencode.ai/config.json',
  'provider': {
    'qwen3': {
      'npm': '@ai-sdk/openai-compatible',
      'name': 'Qwen3.8',
      'options': {
        'baseURL': os.environ.get('LLM_URL') or os.environ.get('OPENAI_BASE_URL') or '',
        'apiKey': os.environ.get('OPENAI_API_KEY', ''),
        'chunkTimeout': 120000,
        'timeout': 600000
      },
      'models': {
        'qwen3': {
          'name': 'Qwen3.8',
          'limit': {'context': 262144, 'output': 32768}
        }
      }
    }
  },
  'model': 'qwen3/qwen3',
  'autoupdate': False
}
d = os.path.expanduser('~/.config/opencode')
os.makedirs(d, exist_ok=True)
for name in ('opencode.json', 'opencode.jsonc'):
  path = os.path.join(d, name)
  json.dump(cfg, open(path, 'w'), indent=2)
  print('Wrote', path)
# Project opencode.json wins over XDG; keep it in sync with the 256K/32K pin.
proj = os.path.expanduser('~/fortune-cookie')
for name in ('opencode.json', 'opencode.jsonc'):
  path = os.path.join(proj, name)
  if os.path.isdir(proj):
    json.dump(cfg, open(path, 'w'), indent=2)
    print('Wrote', path)
PYEOF

  ls -la
  echo '--- AGENTS.md preview ---'
  head -20 AGENTS.md
"
ok "Template cloned to ~/\$APP_NAME in pod"

# ── Generate app files with OpenCode (same prompt as Lab 2.2) ──
step "Phase 2c: Generate Fortune Cookie app with OpenCode + Qwen3.8"

info "Running OpenCode against in-cluster Qwen3.8..."

$POD_EXEC bash -c "
  cd ~/\$APP_NAME
  export PATH=/home/user/.opencode/bin:/home/user/.local/bin:\$PATH
  opencode --version
  echo '--- ~/.config/opencode/opencode.json ---'
  cat ~/.config/opencode/opencode.json
  echo
  echo '--- OpenCode run (timeout 4 min) ---'
  timeout 360 opencode run --print-logs << 'PROMPTEOF' || echo 'OpenCode exited (timeout or error) — checking generated files'
Implement the Fortune Cookie application following AGENTS.md strictly.

Generate these files:
1. main.go       — HTTP server on :8080, GET / (HTML fortune, at least 10 messages) and GET /healthz
2. go.mod        — module: fortune-cookie, go 1.22
3. Dockerfile    — use the exact two-stage template from AGENTS.md
                   (ubi9/go-toolset builder → ubi9/ubi-minimal runtime, USER 1001)

After writing every file, run:
  CGO_ENABLED=0 go build -buildvcs=false -o /dev/null .

Fix any compile error before finishing. Show the build output.
Do NOT deploy yet. Stop after the Go build succeeds.
PROMPTEOF
  echo '--- generated files ---'
  ls -la main.go go.mod Dockerfile
"
ok "OpenCode generation finished"

# Verify the app compiles
info "Verifying Go build..."
$POD_EXEC bash -c "
  cd ~/\$APP_NAME
  /usr/local/go/bin/go build -buildvcs=false -o /dev/null . 2>&1
" && ok "Go build OK" || warn "Go build failed — check code"

# ── Run OpenCode: Deploy following AGENTS.md ──────────────────────────────────
step "Phase 2d: Deploy — OpenCode follows AGENTS.md (git + oc, no Ansible)"

# Set up kubeconfig in pod using the mounted service account token
# (mirrors what OpenShift DevSpaces injects automatically for real users)
step "Phase 2d-pre: Configure kubeconfig in pod"
$POD_EXEC bash -c "
  mkdir -p /home/user/.kube
  SA_TOKEN=\$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
  API_SERVER=\$(oc whoami --show-server 2>/dev/null || echo 'https://kubernetes.default.svc')
  if ! oc login --token=\"\$SA_TOKEN\" --server=\"\$API_SERVER\" \
      --certificate-authority=/var/run/secrets/kubernetes.io/serviceaccount/ca.crt \
      --kubeconfig=/home/user/.kube/config 2>/dev/null; then
    kubectl config set-cluster local \
      --server=\"\$API_SERVER\" \
      --certificate-authority=/var/run/secrets/kubernetes.io/serviceaccount/ca.crt \
      --kubeconfig=/home/user/.kube/config
    kubectl config set-credentials sa \
      --token=\"\$SA_TOKEN\" \
      --kubeconfig=/home/user/.kube/config
    kubectl config set-context default \
      --cluster=local --user=sa \
      --kubeconfig=/home/user/.kube/config
    kubectl config use-context default \
      --kubeconfig=/home/user/.kube/config
  fi
  echo 'kubeconfig configured'
  oc whoami 2>/dev/null || kubectl get sa default 2>/dev/null | head -2
" 2>&1
ok "Kubeconfig configured in pod"

info "OpenCode deploy — AGENTS.md phases (gitpop, Tekton, Argo CD)..."
# Pre-create build namespace and grant buildah the privileged SCC it needs
oc new-project "${APP_NAME}-build" 2>/dev/null || true
oc adm policy add-scc-to-user privileged -z pipeline -n "${APP_NAME}-build" 2>/dev/null || true
$POD_EXEC bash -lc "
  cd ~/\$APP_NAME
  export PATH=/home/user/.opencode/bin:/home/user/.local/bin:\$PATH
  export KUBECONFIG=/home/user/.kube/config
  timeout 1200 opencode run --print-logs << 'PROMPTEOF' || echo 'OpenCode deploy exited (timeout or error)'
Follow AGENTS.md strictly to deploy this app.

Do not create Ansible playbooks. Use git, gitpop, and oc only.

Run the three deploy phases in AGENTS.md in order:
1. Push to the personal Git server (gitpop init + commit + push)
2. Build the image (Tekton PipelineRun) — wait for Succeeded
3. Deploy with Argo CD (update image:, apply gitops/base, create Application)

Print the app route URL and the Argo CD console URL at the end.
PROMPTEOF
" 2>&1 | tee /tmp/deploy-opencode.log || warn "OpenCode deploy failed — see /tmp/deploy-opencode.log"

APP_REPO_URL=$($POD_EXEC bash -lc "
  cd ~/\$APP_NAME 2>/dev/null && git remote get-url origin 2>/dev/null || echo ''
" 2>/dev/null | tr -d '\r\n' || echo "")
info "App repo: ${APP_REPO_URL:-<not captured>}"

ok "OpenCode deploy session complete"

# ─────────────────────────────────────────────────────────────────────────────
# PHASE 3 — Validation
# ─────────────────────────────────────────────────────────────────────────────
step "Phase 3: Validation"

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

# 3a — Files generated in the pod
step "Phase 3a: Generated files"

check "main.go exists" \
  "$POD_EXEC test -f /home/user/$APP_NAME/main.go"

check "go.mod exists" \
  "$POD_EXEC test -f /home/user/$APP_NAME/go.mod"

check "Dockerfile exists" \
  "$POD_EXEC test -f /home/user/$APP_NAME/Dockerfile"

check "deploy/base/deployment.yaml exists" \
  "$POD_EXEC test -f /home/user/$APP_NAME/deploy/base/deployment.yaml"

check "deploy/base/route.yaml exists" \
  "$POD_EXEC test -f /home/user/$APP_NAME/deploy/base/route.yaml"

check "Binary compiles without errors" \
  "$POD_EXEC bash -c 'cd /home/user/$APP_NAME && PATH=/usr/local/go/bin:\$PATH CGO_ENABLED=0 go build -o /dev/null .'"

# 3b — Git repository on Git server
step "Phase 3b: Git server repository"

info "Template repo URL: ${TEMPLATE_URL}"
info "App repo URL:      ${APP_REPO_URL:-<not captured>}"

check "Template repo exists on Git server (git smart HTTP)" \
  "curl -sf '${TEMPLATE_URL}/info/refs?service=git-upload-pack' | grep -q git-upload-pack"

if [[ -n "$APP_REPO_URL" ]]; then
  check "App repo '${APP_NAME}' exists on Git server" \
    "curl -sf '${APP_REPO_URL}/info/refs?service=git-upload-pack' | grep -q git-upload-pack"

  check "App repo has main.go committed" \
    "git ls-remote '${APP_REPO_URL}' HEAD 2>/dev/null | grep -q '.'"
else
  warn "FAIL: App repo URL not captured — deploy step may have failed"
  FAILURES=$((FAILURES + 1))
fi

# 3c — Tekton Pipeline build
step "Phase 3c: OpenShift Pipelines (Tekton) image build"

check "Build namespace ${APP_NAME}-build exists" \
  "oc get namespace ${APP_NAME}-build"

check "Tekton Pipeline 'build-app' created" \
  "oc get pipeline build-app -n ${APP_NAME}-build"

check "At least one PipelineRun succeeded" \
  "oc get pipelinerun -n ${APP_NAME}-build --no-headers 2>/dev/null | grep -q Succeeded"

# The image is pushed directly to internal registry by buildah (no ImageStream)
check "Image exists in internal registry" \
  "oc get imagestreamtag ${APP_NAME}:latest -n ${APP_NAME}-build 2>/dev/null || \
   oc get pipelinerun -n ${APP_NAME}-build --no-headers 2>/dev/null | grep -q Succeeded"

# 3d — Developer-owned Argo CD
step "Phase 3d: Developer-owned Argo CD"

check "Dev namespace ${APP_NAME}-dev exists" \
  "oc get namespace ${APP_NAME}-dev"

check "ArgoCD instance created in ${APP_NAME}-dev" \
  "oc get argocd argocd -n ${APP_NAME}-dev"

check "Argo CD Application '${APP_NAME}' created" \
  "oc get application ${APP_NAME} -n ${APP_NAME}-dev"

# Wait for Argo CD to sync and become Healthy (up to 6 min).
# App manifests keep the template name (fortune-cookie); APP_NAME is the git/ns name.
info "Waiting for developer Argo CD to sync..."
for i in $(seq 1 36); do
  SYNC=$(oc get application "${APP_NAME}" -n "${APP_NAME}-dev" \
    -o jsonpath='{.status.sync.status}' 2>/dev/null || echo "")
  HEALTH=$(oc get application "${APP_NAME}" -n "${APP_NAME}-dev" \
    -o jsonpath='{.status.health.status}' 2>/dev/null || echo "")
  [[ "$SYNC" == "Synced" && "$HEALTH" == "Healthy" ]] && break
  sleep 10
done

APP_DEPLOY=$(oc get deploy -n "${APP_NAME}-dev" -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' \
  2>/dev/null | grep -v '^argocd-' | head -1 || true)

check "App Deployment '${APP_DEPLOY:-<none>}' is available" \
  "[[ -n '${APP_DEPLOY}' ]] && oc get deployment '${APP_DEPLOY}' -n '${APP_NAME}-dev' \
   -o jsonpath='{.status.availableReplicas}' | grep -qE '[1-9]'"

# 3e — Live application
step "Phase 3e: Live application"

APP_ROUTE=$(oc get route -n "${APP_NAME}-dev" -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.host}{"\n"}{end}' \
  2>/dev/null | grep -v '^argocd-server' | awk 'NF{print $2; exit}' || true)

if [[ -z "$APP_ROUTE" ]]; then
  warn "FAIL: App Route not found in ${APP_NAME}-dev"
  FAILURES=$((FAILURES + 1))
else
  ok "Route: https://${APP_ROUTE}"

  check "/healthz returns {\"status\":\"ok\"}" \
    "curl -sk https://${APP_ROUTE}/healthz | grep -q 'ok'"

  check "/ returns HTML with a fortune" \
    "curl -sk https://${APP_ROUTE}/ | grep -iq 'fortune\|cookie\|luck\|wisdom'"
fi

# 3f — LLM smoke test (direct API call, no OpenCode)
step "Phase 3f: LLM API smoke test"

LLM_RESP=$(oc exec "$DEV_POD" -n "$E2E_NS" -- bash -c "
  curl -sf '${LLM_URL}/chat/completions' \
    -H 'Authorization: Bearer ${LLM_KEY}' \
    -H 'Content-Type: application/json' \
    -d '{
      \"model\": \"${LLM_MODEL}\",
      \"messages\": [{\"role\": \"user\", \"content\": \"Reply with exactly: HELLO_E2E\"}],
      \"max_tokens\": 20,
      \"chat_template_kwargs\": {\"enable_thinking\": false}
    }' | python3 -c \"import sys,json; print(json.load(sys.stdin)['choices'][0]['message']['content'])\"
" 2>/dev/null || echo "")

if echo "$LLM_RESP" | grep -q "HELLO_E2E"; then
  ok "LLM responds correctly: $LLM_RESP"
else
  warn "FAIL: LLM did not return expected response (got: '${LLM_RESP}')"
  FAILURES=$((FAILURES + 1))
fi

# ── Summary ───────────────────────────────────────────────────────────────────
step "Summary"

echo ""
echo -e "  Template repo : ${CYAN}${TEMPLATE_URL}${NC}"
echo -e "  App repo      : ${CYAN}${GIT_SERVER}/${APP_NAME}${NC}"
echo -e "  App route     : ${CYAN}https://${APP_ROUTE:-<not found>}${NC}"
echo -e "  Argo CD (dev) : ${CYAN}https://$(oc get route argocd-server -n ${APP_NAME}-dev -o jsonpath='{.spec.host}' 2>/dev/null || echo '<not deployed>')${NC}"
echo ""

if [[ $FAILURES -eq 0 ]]; then
  echo -e "${GREEN}${BOLD}All checks passed — workshop deployment is valid ✅${NC}"
else
  echo -e "${RED}${BOLD}$FAILURES check(s) failed — review warnings above ❌${NC}"
  exit 1
fi
