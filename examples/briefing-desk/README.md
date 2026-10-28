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
