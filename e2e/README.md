# ARO Card Scout validation

Follow Acts 0–3 on an assigned classic ARO 4.21+ lab. Live acceptance means
installing the platform, provisioning one CPU and one T4 worker, testing the
actual APIs from Dev Spaces, generating the Go app with DeepSeek/OpenCode,
and delivering it through Tekton and Argo CD. Fixtures alone are not acceptance.

Run the shell blocks from the guide in order. Act 0 and Act 1 use the bastion;
Act 2 uses the scoped Dev Spaces identity. The Playground tests the CLI-created
`OGXServer/lsd-genai-playground` in `card-scout-dev`; it does not provision it.

## Platform and developer APIs

From the authenticated bastion, with the workshop checkout:

```sh
bash e2e/platform-run.sh
E2E_NS=your-devspaces-namespace DEV_POD=your-workspace-pod \
  python3 e2e/check.py model-apis
```

The API check executes all four Lab 2.2 command blocks verbatim in the real
workspace. It requires five JSON responses: D3 discovery, Pokémon and control
image decisions, MaaS Chat Completions, and a completed native Responses call
with a successful `get_card_market_data` call for the requested catalogue ID.
It also checks that the developer cannot read model Secrets or edit the model.
D3 needs no credential; DeepSeek uses the injected MaaS key.

## Generated app and browser

After OpenCode commits, pushes, builds and deploys the application:

```sh
E2E_NS=your-devspaces-namespace DEV_POD=your-workspace-pod \
  bash e2e/dev-run.sh
APP_URL=https://your-app-route node e2e/card-scout-browser.cjs
```

The delivery check compares workspace source, pushed main, Argo CD revision,
Tekton target and running image digest. The browser check requires 105 completed
real image analyses, full-batch filters, pagination, no price action for controls,
a native price lookup, exact tool-derived prices, dated source and trace links,
filter persistence and a mobile layout. Its separate fixtures exercise errors,
missing/zero prices and edge cases using explicitly synthetic API responses.

`card-scout-browser.cjs` targets the generated validation app's accessible UI and
selectors. A newly generated implementation can use different DOM selectors;
adapt those selectors while retaining the same acceptance behavior.

## Playground, traces and measurements

Use a private authenticated browser state for the cluster UI:

```sh
RHOAI_URL=https://your-rhoai-route BROWSER_STATE=/private/path/login-state.json \
  node e2e/native-agent-browser.cjs
bash e2e/usage-run.sh
python3 e2e/check-observability.py | jq .
python3 e2e/maas-protocol.py | jq .
python3 e2e/maas-quota.py
```

Follow Lab 3.2 to exercise the same price MCP tool in the Playground. Open the
app's exact MLflow trace link and inspect `card.research` → `agent.responses` →
`tool.get_card_market_data`, with real inputs, output, catalogue identity and
provider dates. Verify a batch trace and its `d3.classify_card` spans too. A model
claim to have called a tool or a visible UI answer does not establish trace export.
Model keys and provider headers must not appear in traces or browser output.

The model and quota probes consume real model tokens. The app and its read-only
MCP connector are public HTTPS endpoints; OpenShift AI and MLflow use normal
cluster authentication. Every Chromium launch includes `--use-mock-keychain`.

## Local checks and dated evidence

```sh
python3 -m venv .venv
. .venv/bin/activate
python -m pip install -r e2e/requirements.txt
make test
make test-terminal
make docs
```

[Collector-card extension](results/collector-cards/README.md) records the
57-image dataset and live tests of the added collector cards.
[Fresh ARO 54w5q review](results/aro-54w5q/README.md) records the complete
2 November participant run and corrections.
[Earlier Card Scout evidence](results/card-scout/README.md) separates real model,
agent, delivery and browser runs from fixtures and local checks. Record the lab,
date, source revision and actual result of a new run; retain earlier evidence as
historical. The finance, speech and game directories are earlier experiments,
not requirements of the current Card Scout workshop.

## Configuration review on a fresh lab

See [vmffl validation](results/aro-vmffl/README.md) for the November 5, 2026
configuration tests, rejected hypotheses and precise validation scope.

The [vmffl generation run](results/card-scout-generation-vmffl/README.md) records
DeepSeek/OpenCode generation, review findings, two real deliveries and independent
live/fixture browser checks. It was not a one-shot generation success.
