# ARO workshop validation

Follow Acts 0–3 on the assigned ARO environment, including the real Dev Spaces browser session, OpenCode plan/build/change prompts, Tekton builds, and personal Argo CD. These checks verify the resulting resources; they do not replace the participant exercises.

Run from the bastion with `oc` authenticated to the assigned cluster:

```sh
bash e2e/platform-run.sh
EXPECT_UPDATED_APP=true bash e2e/dev-run.sh
bash e2e/usage-run.sh
```

| Check | Evidence |
|---|---|
| Platform | Ready workers, cluster-admin, Dev Spaces / GitOps / Pipelines, workshop Tasks, MaaS external provider/model/catalog, separate workspace key, real Foundry completion |
| Developer | Latest Tekton build Succeeded, personal Application Synced / Healthy, public HTTPS app and health endpoint, transcript copy/export, independent resource / probe / security / TLS checks |
| Usage | Real Foundry response through MaaS with input and output token counts, with MaaS quota and model-side metrics |

The scripts use Python 3 and `oc`. They print no credentials, create no cluster resources, and do not mint or rotate keys. Platform and usage checks each make one billable model call. `APP_NAME` defaults to `live-transcript`; `EXPECT_UPDATED_APP=true` checks the Copy transcript exercise.

To also verify the generated source, local Go build and pushed Git revision in the real workspace, pass `E2E_NS=<workspace-namespace> DEV_POD=<workspace-pod> DEV_CONTAINER=dev` to `dev-run.sh`.

For browser checks, launch Chromium with `--use-mock-keychain`. Verify catalog login in the published Cloud Terminal, the embedded WeTTY shell, OpenShift login, the Dev Spaces factory and editor, the public app, and the personal Argo CD console.

The participant guide keeps the environment available until RHDP expiration. The older teardown scripts are outside the participant flow. Temporary fixtures created by validation scripts are still removed by those tests.

[Historical full live validation and final environment state](VALIDATION.md) records the 2026-10-22 run on ARO 4.21.22.

## Automatic Azure resource group discovery

`make onboard` matches the connected OpenShift API URL to the Azure ARO resource in the selected subscription. It writes the owning resource group’s name and ID to `state/workshop.env`, distinct from ARO’s managed machine group. No cloud resources are created.

```sh
python3 -m unittest discover -s e2e -p 'test_workshop_env.py'
```

The regression checks cover an unrelated first cluster, ambiguous/missing matches, wrong subscriptions, wrong group responses, non-Azure clusters and API URL normalization.

Live check on 23 October 2026: the discovery helper and full onboarding script passed on RHDP workshop `natfzu`, ARO 4.21.22. They selected `openenv-tdfcw` and its resource group ID, matching the RHDP assignment, instead of managed machine group `aro-e4xkjsje`. A deliberately incorrect pre-existing `RESOURCEGROUP` value did not affect discovery. The test used read-only Azure/OpenShift calls and wrote only local non-secret environment files; it did not provision Foundry or GPU resources.

## Visible worker CLI commands

Lab 1.4 uses `az aro show` and Azure VM quota/SKU queries, then explicit `oc get` / `jq` / `oc create` / `oc wait` commands for classic ARO MachineSets. No provisioning helper or Makefile target is used by that lab. The `az aro hcp cluster nodepool` CLI belongs to HCP, not the classic cluster used here.

Validation on `natfzu`, ARO 4.21.22, on 23 October 2026:

- The commands generated a one-replica `Standard_D8s_v3` CPU MachineSet in zone 3 and a one-replica `Standard_NC8as_T4_v3` GPU MachineSet in zone 1.
- Both manifests were accepted by `oc create --dry-run=server`; selectors, VM sizes, GPU node label and GPU taint were checked on the returned objects.
- Azure quota queries returned the DSv3, NCASv3_T4 and regional vCPU limits. The documented SKU command listed the T4 size without restrictions in zones 1, 2 and 3; allow a few minutes for that query.
- The documented MachineSet readiness check passed against the existing GPU pool. Its Node was Ready; the NVIDIA driver had not yet advertised an allocatable GPU on this assignment, which is the next step in Lab 1.4.
- The existing single-T4 pool was already Ready with one replica. It was reused for inspection; no additional GPU was provisioned during this validation.

This validates CLI output and server-side admission. The dry-run CPU/GPU previews were not persisted or provisioned. Earlier live speech reports remain separate evidence for driver and Whisper execution.

## Speech application

After Lab 1.4 and the developer deployment:

```sh
python3 e2e/check.py speech
(cd examples/live-transcript && go test -race ./...)
npm install --no-save @playwright/test@1.56.1
npx playwright install chromium
APP_URL=https://your-app-route AUDIO_FIXTURE=/absolute/french-speech.wav \
  node e2e/speech-browser.cjs
```

Use a synthetic spoken WAV fixture, with `SPEECH_LANGUAGE=en` and an English fixture for the second pass. The browser test uses real microphone capture with Chromium's fake audio device, the live Go backend and the actual Whisper service. It checks text before Stop, multiple independent WAV clips, transcript download, microphone release, backend failure and permission denial. Every browser launch uses `--use-mock-keychain`.

See [RedHatAI INT8 validation](WHISPER-INT8-VALIDATION.md) for the active model and [speech validation](SPEECH-VALIDATION.md) for the initial FP16 baseline. `VALIDATION.md` records the earlier Fortune Cookie baseline and is historical.

## Historical GPU experiments

See [the live GPU comparison](GPU-VALIDATION.md) and [the isolated experiment](../experiments/gpu/README.md). These are historical LLM comparisons; the current workshop keeps Foundry for coding and uses one T4 for Whisper.

Single-GPU model trials, including Bonsai 2 and cyankiwi models, are recorded in [the one-T4 report](SINGLE-T4-VALIDATION.md).

## Deployment image regression

See [image contract validation](IMAGE-CONTRACT-VALIDATION.md) for the placeholder regression, failing/passing Tekton cases, and the successful build and application rollout on `natfzu`.

## External Foundry model through MaaS

See [the live validation on mnzq5](MAAS-FOUNDRY-VALIDATION.md). All operators are now installed in Lab 1.1. Lab 1.2 creates Foundry; Lab 1.3 registers the external provider/model and injects a separate MaaS key. `deploy/coding` is the active coding path; `deploy/inference` contains the historical local Qwen experiment.

```sh
python3 -m unittest discover -s e2e -p 'test_*.py'
python3 e2e/maas-protocol.py
python3 e2e/maas-quota.py
```

The protocol probe makes billable calls for tool use and streaming with a request larger than 16 KiB. It also checks catalog discovery and missing/invalid-key rejection. The quota probe requires an OpenShift user token, creates a named temporary subscription and short-lived key, and revokes/deletes both in `finally`; it refuses to overwrite an existing validation subscription. Quota updates are asynchronous: this is a rate-limit test, not a guarantee against overshoot or a financial spending cap.

OpenCode’s audit exercise has been removed from the participant journey on ARO and ROSA. Resource assertions in the validation scripts remain independent automated checks; historical reports describe the older flow.

## Observability and scoped delivery

See [the live observability and security validation](OBSERVABILITY-SECURITY-VALIDATION.md). From an authenticated bastion with monitoring access:

```sh
python3 e2e/check-observability.py
```

This read-only probe runs the actual nine dashboard expressions. Generate coding and speech activity first and allow scrape propagation; a quiet five-minute window can produce a NaN latency percentile.
