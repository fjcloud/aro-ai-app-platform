# Signal Desk

News ranked for an investment memo. The default example is a semiconductor fund.

**Memo → Techmeme → summary → opportunity score /100 → investigate**

- Each fetch imports the latest 12 stories; saved stories remain in the feed.
- d3-mini scores research relevance and materiality against the memo. Positive and
  negative developments can both score highly. This is neither BUY/SELL nor a
  confidence percentage or an expected return.
- Azure DeepSeek explains each score using source key points. The score itself
  comes from d3-mini's native five-level ordinal head: `100 × expected level / 4`.
- Edit the memo to re-score existing summaries. Scores and research are tied to
  the memo version and source-summary hash; outdated research is hidden.
- Any current scored article can be investigated. Research checks sources and
  financial implications; exported notes retain sourced ETF references and
  explicitly hypothetical scenarios. No orders are sent.

The UI has a memo, a refresh button and a list sorted by score. Open a story for
key points, investigation and Markdown export. Publisher paywalls use a disclosed
Techmeme excerpt rather than a fabricated full-article summary.

## Local development

The active setup uses Azure **DeepSeek-V4-Pro**, version `2026-04-23`, matching the
workshop, and **d3-mini on Apple Metal**. The research LLM is hosted; the scoring
model runs locally.

Start the pinned model on an Apple Silicon Mac (tested with 32 GB and Python 3.14):

```sh
python3 -m venv /private/tmp/signal-desk-d3-venv
/private/tmp/signal-desk-d3-venv/bin/pip install -r requirements-metal.txt
/private/tmp/signal-desk-d3-venv/bin/python serve-d3-metal.py
```

The model download is about 9 GB. The launcher uses localhost:18093, FP16 and no
CPU fallback. Upstream marks MPS as unvalidated; local checks do not establish
numerical parity or financial-performance accuracy.

With `az login` active, set `RESOURCEGROUP` and `FOUNDRY_NAME` to the account that
hosts deployment `deepseekv4pro`. Retrieve its endpoint and private key file:

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

Open http://127.0.0.1:18092. Azure inference is paid per token. Keys stay outside
Git and are never sent to the browser or article sites. Optional Groq Free uses
`AGENT_BACKEND=groq GROQ_MODEL=qwen/qwen3.8-27b GROQ_KEY_FILE=...`; account-plan
billing and quotas apply, with no automatic model fallback.

For a disconnected preview, use `PREVIEW_MODE=1 DEV_ASSETS=1 go run .` with a
separate data directory. Its scores and news are clearly labeled synthetic.

## Validation

```sh
go test -race ./...
go vet ./...
```

From the workshop root, `make test-terminal` runs the complete HTTP/browser flow
with explicit fixtures: scoring, sorted cards, memo edits and automatic re-scoring,
stale-research hiding, investigation, exports and mobile layout. Chromium uses
`--use-mock-keychain`.

With the live local services running:

```sh
node e2e/briefing-desk-live-local.cjs
```

For actual D3 memo sensitivity without changing app data:

```sh
LIVE_MEMO_MODEL=1 go test -run TestLiveMemoScoreSensitivity -v -count=1
```

## OpenShift

Keep the existing live-transcript app, OAuth, restricted-v2, single T4, native
OpenShift AI Responses service and Git → Tekton → Argo CD delivery. Set
`AGENT_BACKEND=rhoai` (the default), `AGENT_URL`, `AGENT_MODEL`, `AGENT_KEY_FILE`,
`DECISION_URL`, `DECISION_MODEL` and `DATA_DIR`. The replacement ARO environment
still needs live validation. `scripts/prepare-briefing-desk.py` prepares the native
service and MaaS credential; the MaaS key lasts 24 hours.
