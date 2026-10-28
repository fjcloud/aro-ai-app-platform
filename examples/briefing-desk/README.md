# Signal Desk / Techmeme terminal

An English market terminal: **fetch → summarize → BUY / SHORT / NO SIGNAL → investigate**.

- Techmeme RSS supplies headlines and original publisher links. Inaccessible pages
  use an explicitly labeled RSS excerpt; they are never presented as full articles.
- OpenShift AI summarizes and investigates. The existing d3-mini on a T4 ranks
  the directional thesis. NO SIGNAL is a valid outcome and blocks investigation.
- Research maps a thesis to QQQ, SPY, SMH or XLK: ETF proxies for their stated
  indices. Timestamped Yahoo Finance chart metadata supplies reference prices.
- Targets, stops and horizon are hypothetical model assumptions. Edit notional,
  target and stop to calculate conditional gross P&L. Markdown uses the same
  edited inputs. This is not a calibrated expected-return estimate. The signal score does not
  estimate the chance of hitting a target.

## Local preview

From this directory:

```sh
PREVIEW_MODE=1 DEV_ASSETS=1 go run .
```

Open http://localhost:8080. Preview uses a separate `preview-data/` directory and
clearly labeled fictional headlines, model signals and prices. It cannot execute
live model jobs. `DEV_ASSETS=1` reloads the local `web/` assets while editing.
The production image embeds those assets.

## Live local development

Use **Azure DeepSeek-V4-Pro**, matching the workshop model and version
(`2026-04-23`), for research and **d3-mini on Apple Metal** for ranking.
The research model is hosted; only the classifier runs on this Mac. Azure credentials can be retrieved through the existing `az login` session;
there is no API key to create manually. The Foundry deployment is Global Standard,
paid per token. The application never switches providers automatically.

On an Apple Silicon Mac (tested with 32 GB unified memory and Python 3.14):

```sh
python3 -m venv /private/tmp/signal-desk-d3-venv
/private/tmp/signal-desk-d3-venv/bin/pip install -r requirements-metal.txt
/private/tmp/signal-desk-d3-venv/bin/python serve-d3-metal.py
```

The pinned model download is about 9 GB. The launcher binds to localhost:18093,
uses FP16 and disables CPU fallback. `/v1/models` reports `device: mps` and
`dtype: float16`. Upstream marks MPS as unvalidated; the local three-case
functional check does not establish numerical parity or investment performance.

In another terminal, set `RESOURCEGROUP` and `FOUNDRY_NAME` to your existing
Foundry account with deployment `deepseekv4pro`, then retrieve its endpoint/key:

```sh
: "${RESOURCEGROUP:?Select the Foundry resource group}"
: "${FOUNDRY_NAME:?Select the Foundry account}"
AZURE_FOUNDRY_URL="$(az cognitiveservices account show -g "$RESOURCEGROUP" \
  -n "$FOUNDRY_NAME" --query properties.endpoint -o tsv)"
AZURE_FOUNDRY_URL="${AZURE_FOUNDRY_URL%/}/openai/v1"
(umask 077; az cognitiveservices account keys list -g "$RESOURCEGROUP" \
  -n "$FOUNDRY_NAME" --query key1 -o tsv > /private/tmp/signal-desk-azure-key)

AGENT_BACKEND=azure AZURE_FOUNDRY_URL="$AZURE_FOUNDRY_URL" \
AZURE_DEPLOYMENT=deepseekv4pro AZURE_KEY_FILE=/private/tmp/signal-desk-azure-key \
DECISION_URL=http://127.0.0.1:18093/v1/systemone DECISION_DEVICE=mps \
DECISION_TIMEOUT=120s AGENT_JOB_TIMEOUT=10m \
LISTEN_ADDR=127.0.0.1:18092 DATA_DIR=/private/tmp/signal-desk-local-data \
DEV_ASSETS=1 go run .
```

Open http://127.0.0.1:18092 and select **FETCH TECHMEME**. These are live sources
and model calls. Only BUY / SHORT stories can launch an investigation. Keys stay
in private files outside Git and are never sent to article sites or the browser.

### Optional Groq Free backend

For a separate [Groq Free account](https://console.groq.com/keys), configure
`AGENT_BACKEND=groq GROQ_MODEL=qwen/qwen3.8-27b` and `GROQ_KEY_FILE` instead of
the Azure settings above. The account plan determines billing and cannot be
verified from an API key. [Quotas](https://console.groq.com/docs/rate-limits) apply.
Short provider cooldowns are visible in activity and retried at most three times
on the same model. Daily exhaustion fails visibly; no paid upgrade is performed.

## Checks

```sh
go test -race ./...
go vet ./...
LIVE_DATA_TESTS=1 go test -run TestLivePublicData -v -count=1
```

The opt-in public-data check reads actual Techmeme RSS and four timestamped ETF
quotes. For a full local browser workflow, run `make test-terminal` from the workshop root. That harness
mocks inference, news and price responses and explicitly labels the resulting
screen as validation fixtures. It exercises real Go HTTP handlers, agent tool
continuation, persistence, classification validation, scenario math and the UI.
Every Chromium launch uses `--use-mock-keychain`.

## OpenShift

Use the existing live-transcript app, OAuth proxy, restricted-v2, single T4,
native OpenShift AI Responses service and Git → Tekton → Argo CD delivery.
The facilitator prepares the native service, MaaS key and persistent data using
`scripts/prepare-briefing-desk.py` from the workshop root. The MaaS key lasts 24h.

Disable PREVIEW_MODE and configure AGENT_URL, AGENT_MODEL, AGENT_KEY_FILE,
DECISION_URL, DECISION_MODEL and DATA_DIR. The existing MLflow settings trace
backend orchestration, actual tool execution and local classification.

Quotes are best-effort reference data with no guaranteed delay, not executable
bid/ask prices. Missing, invalid or >72h-old quotes cannot price a scenario. Gross
P&L omits transaction, borrowing, financing and slippage costs. No orders or client
messages are sent. No model-generated entry price is accepted.
