# ARO Signal Desk validation

Follow Acts 0–3 on an assigned ARO lab: actual operator/model setup, curl + jq probes, Dev Spaces, DeepSeek/OpenCode generation, Tekton, Argo CD and the browser game. Fixture success alone is not live acceptance.

From the authenticated bastion:

```sh
bash e2e/platform-run.sh
bash e2e/dev-run.sh
bash e2e/usage-run.sh
python3 e2e/check-observability.py | jq .
python3 e2e/maas-protocol.py | jq .
```

Platform/usage/protocol probes make billable model calls. `APP_NAME` defaults to `live-transcript`. Use `EXPECT_UPDATED_APP=true` for the shipped relevance-filter exercise and `E2E_NS`, `DEV_POD`, `DEV_CONTAINER=dev` to compare actual workspace source, pushed revision and Argo CD revision.

Validate the UI with a full archived-week import, real T4 scores, an actual native Responses tool loop, a virtual allocation and subsequent-week observed prices. Verify unavailable archive IDs, incomplete-week labels and errors. Confirm an MLflow research trace and the same archive tools in the native Playground. Public app access requires OpenShift OAuth; credentials remain private. Every Chromium launch includes `--use-mock-keychain`.

For the real Route browser test, save a private OpenShift browser session and model metadata from the Lab 1.4 port-forward, then run:

```sh
APP_URL=https://your-app-route BROWSER_STATE=/private/path/login-state.json \
D3_METADATA=/private/path/d3-models.json NEWS_IMPORT_FILE=/path/to/current-week-news.json \
EXPECT_UPDATED_APP=true APPLICATION_REVISION=your-deployed-git-revision \
  node e2e/briefing-desk-aro.cjs
```

The test tries direct collection first. A blocked collection fails unless an explicit import file is supplied; the report records that recovery. It never invents news, scores or prices.

Local regression checks:

```sh
make test
make test-terminal
make docs
```

The browser fixture test covers seven archive days, memo changes, native agent research, two investments with positive/negative returns, immutable portfolio choices, exports and mobile layout. Go tests also cover MCP discovery, strict tool arguments, archive boundaries, source safety and completed-session valuation.

[Signal Desk validation and evidence](BRIEFING-DESK-VALIDATION.md) distinguishes real local Metal/Azure runs from fixture checks and ARO results. The optional `e2e/briefing-desk-live-local.cjs` tests the local Metal setup; it does not establish ARO compatibility.

Earlier speech, cluster-health and game experiments remain in their reference directories and historical validation records. They are not prerequisites or acceptance criteria of the Signal Desk workshop.
