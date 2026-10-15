# E2E Workshop Validation

Maps to workshop Act 1 (labs 1.1–1.4) and Act 2 (labs 2.1–2.3). Neither needs the Dev Spaces UI.

| Script | Persona | What it checks |
|--------|---------|----------------|
| `e2e/platform-run.sh` | Platform Engineer | GPU node pool, operators, DSC (KServe + MaaS + dashboard + OGX), Dev Spaces, `LLMInferenceService/qwen3`, MaaS CRs, minted `sk-oai-` key, smoke `POST /chat/completions` |
| `e2e/dev-run.sh` | Developer | Template repo, OpenCode-generated Fortune Cookie, Tekton image, Argo CD sync, live route |

Cleanup: `e2e/platform-cleanup.sh` and `e2e/dev-cleanup.sh`.

## Prerequisites

- `oc` logged in with cluster-admin
- Workshop GitOps already applied (`deploy/operators`, `deploy/instances`, `deploy/inference`)
- `LLMInferenceService/qwen3` Ready in `llm-serving` and published through Models-as-a-Service

## Usage

```bash
export GIT_SERVER=https://gitpop.apps.sno.msl.cloud
bash e2e/platform-run.sh
bash e2e/dev-run.sh
```

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `GIT_SERVER` | `https://gitpop.apps.sno.msl.cloud` | Git server base URL |
| `APP_NAME` | `fortune-cookie` | Name of the generated app (`dev-run.sh`) |
| `E2E_NS` | `workshop-e2e` | Namespace for the developer simulation pod |
| `LLM_NS` | `llm-serving` | Model namespace (`platform-run.sh`) |
| `MAAS_HOST` | `maas.<apps-domain>` | MaaS gateway hostname |

The scripts exit 0 if all checks pass, 1 if any fail. Each check prints ✅ or ⚠️ FAIL.
