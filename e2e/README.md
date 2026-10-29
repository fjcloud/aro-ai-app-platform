# ARO Signal Desk validation

Follow Acts 0–3 on an assigned ARO lab: actual operator/model setup, curl + jq probes, Dev Spaces, DeepSeek/OpenCode generation, Tekton, Argo CD and the browser game. Fixture success alone is not live acceptance.

The guide installs operators with `oc apply -k deploy/operators`, adds one CPU and one GPU worker, then enables instances with explicit CLI commands. The developer preflight is Lab 2.2, executed entirely from Dev Spaces.

From the authenticated bastion:

```sh
bash e2e/platform-run.sh
bash e2e/dev-run.sh
bash e2e/usage-run.sh
python3 e2e/check-observability.py | jq .
python3 e2e/maas-protocol.py | jq .
```

Verify the developer's five API command blocks verbatim, plus model-namespace permissions:

```sh
E2E_NS=your-devspaces-namespace DEV_POD=your-workspace-pod \
  python3 e2e/check.py model-apis
```

This requires direct D3 access from Dev Spaces and the Dev Spaces-to-Responses policy from Lab 1.8. It checks model readiness, two complete memo scores, both DeepSeek protocols and scoped namespace permissions. D3 uses no credential; DeepSeek keeps its MaaS key. Credentials and full model responses are never printed.

Platform/usage/protocol/model-apis probes make billable model calls. `APP_NAME` defaults to `signal-desk`. Use `EXPECT_UPDATED_APP=true` for the shipped relevance-filter exercise and `E2E_NS`, `DEV_POD`, `DEV_CONTAINER=dev` to compare actual workspace source, pushed revision and Argo CD revision.

Validate the UI with a full archived-week import, real T4 scores, an actual native Responses tool loop, a virtual allocation and subsequent-week observed prices. Verify unavailable archive IDs, incomplete-week labels and errors. Confirm an MLflow research trace and the same archive tools in the native Playground. Public app access requires OpenShift OAuth; credentials remain private. Every Chromium launch includes `--use-mock-keychain`.

For the real Route browser test, save a private OpenShift browser session and model metadata from the direct d3-mini probe in Lab 2.2, then run:

```sh
APP_URL=https://your-app-route BROWSER_STATE=/private/path/login-state.json \
D3_METADATA=/private/path/d3-models.json NEWS_IMPORT_FILE=/path/to/current-week-news.json \
EXPECT_UPDATED_APP=true APPLICATION_REVISION=your-deployed-git-revision \
  node e2e/briefing-desk-aro.cjs
```

The test tries direct collection first. A blocked collection fails unless an explicit import file is supplied; the report records that recovery. It never invents news, scores or prices.

To repeat the native Playground test with an existing private OpenShift browser session and the short-lived token from Lab 3.2:

```sh
RHOAI_URL=https://your-rhoai-route APP_URL=https://your-app-route \
BROWSER_STATE=/private/path/login-state.json MCP_TOKEN_FILE=/private/path/mcp-token \
  node e2e/agentops-browser.cjs
```

It checks actual Responses MCP calls, rather than accepting a model's claim to have used tools. Set `CAPTURE_GUIDE=true` to refresh the guide screenshots from the real UI; the authorization screenshot is taken before filling the token.

Local regression checks:

```sh
make test
make test-terminal
make docs
```

The browser fixture test covers seven archive days, memo changes, native agent research, two investments with positive/negative returns, immutable portfolio choices, exports and mobile layout. Go tests also cover MCP discovery, strict tool arguments, archive boundaries, source safety and completed-session valuation.

[Signal Desk validation and evidence](BRIEFING-DESK-VALIDATION.md) distinguishes real local Metal/Azure runs from fixture checks and ARO results. The optional `e2e/briefing-desk-live-local.cjs` tests the local Metal setup; it does not establish ARO compatibility.

Earlier speech, cluster-health and game experiments remain in their reference directories and historical validation records. They are not prerequisites or acceptance criteria of the Signal Desk workshop.
