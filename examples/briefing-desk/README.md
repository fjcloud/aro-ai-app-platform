# Signal Desk

A simple paper-investment game: **last week's news → your fund's ranking → index ideas → invest → reveal this week's result**.

1. Edit the fund memo (semiconductors/components by default).
2. Load the previous Monday–Sunday from Techmeme's archives. Export the complete collected corpus as JSON, including coverage gaps.
   If Techmeme blocks cloud requests, use **Import news** with a same-week facilitator export. Imported dates/URLs are checked, uploaded scores are ignored, and the decision model scores the facts. The UI identifies manual provenance.
3. Open a ranked story. DeepSeek compares archived news and proposes up to three long-only ETF index proxies: QQQ, SPY, SMH or XLK.
4. Allocate the **$10,000 virtual budget**, then reveal the result. The entire portfolio is saved and frozen before market prices are requested.
5. Compare entry at this week's first available US session open with the latest completed session close. While the week is running, results are explicitly provisional. Play again to start another round.

The interface has one column: memo, paper fund and ranked news. Thirty stories are shown initially; Show more reveals the rest. No live orders are sent.

d3-mini supplies the actual memo relevance score: `100 × expected ordinal level / 4`. It is not confidence, market direction or predicted profit. Both favorable and adverse news can score highly. Memo edits re-score saved news and invalidate old ideas.

News uses archived Techmeme headlines/excerpts, without fetching today's publisher pages. Imports use daily snapshots, backfill missing primary post IDs, deduplicate publisher URLs and disclose unavailable IDs. This does not include every related discussion link or guarantee full publisher-text coverage.

Game research can only read/search that archived week and consult a fixed index catalog. It cannot call current web or price tools. This remains a historical educational replay: model training knowledge is not erased, so it is not a blind or calibrated backtest. Historical prices come from Yahoo Finance daily data. Returns are gross price-only, excluding dividends, fees and slippage; unavailable data is reported rather than invented.

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

To prepare that file from a network where Techmeme is available (no model or key needed):

```sh
set -o pipefail
go run . export-news | jq . > techmeme-news.json
```

## Validation

```sh
go test -race ./...
go vet ./...
```

From the workshop root, `make test-terminal` runs the complete HTTP/browser flow
with explicit fixtures: seven-day imports, memo re-scoring, bounded research,
portfolio freezing, positive/negative returns, exports and mobile layout. Chromium uses
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

Keep the existing signal-desk app, OAuth, restricted-v2, single T4, native
OpenShift AI Responses service and Git → Tekton → Argo CD delivery. Set
`AGENT_BACKEND=rhoai` (the default), `AGENT_URL`, `AGENT_MODEL`, `AGENT_KEY_FILE`,
`DECISION_URL`, `DECISION_MODEL` and `DATA_DIR`. The replacement ARO environment
still needs live validation. `scripts/prepare-briefing-desk.py` prepares the native
service and MaaS credential; the MaaS key lasts 24 hours.
