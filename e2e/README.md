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

The `aro-card-scout` template provides the minimal app's acceptance tests.
From the generated checkout in Dev Spaces:

```sh
npm ci && npm run setup
npm test
APP_URL=https://your-app-route npm run test:live
```

The first command prepares the test browser. `npm test` builds the real Go app
with the race detector and exercises a local fixture API: complete images, fixed
routes, analyze/reload, price evidence, mobile layout and retry recovery. No
fixture responses are installed in the running platform. The live test compares
the app with the real platform dataset and performs a price lookup with source
and trace links. OpenCode fixes the app until both pass; tests stay unchanged.

After delivery, independently verify source, GitOps, image digest and permissions:

```sh
E2E_NS=your-devspaces-namespace DEV_POD=your-workspace-pod \
  bash e2e/dev-run.sh
```

The older `card-scout-browser.cjs` and dated evidence scripts target the earlier
paginated/filter UI. They are historical checks, not requirements for the minimal
contract. See the latest validation record for the one-prompt generation result.

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

The [Flash regeneration](results/card-scout-flash-vmffl/README.md) records a fresh
frontend replacement using V4.1 Flash, its required corrections and final checks.

The [7vxhk participant run](results/aro-7vxhk/README.md) covers a fresh cluster,
Acts 0–3, DeepSeek Pro generation and three actual deliveries, API/browser/trace
checks, model errors, workspace startup measurements and a real Git push after
restart. It records the fresh-workspace injector fix, image caching and persistent
Git helper, plus portable browser checks for that generated implementation.

The [minimal Card Scout trial](results/card-scout-minimal-7vxhk/README.md) validates
the current template's built-in acceptance tests. One DeepSeek Pro prompt completed
three automatic correction rounds and real delivery, with no follow-up code
feedback and no modifications to the supplied tests or scaffold.
