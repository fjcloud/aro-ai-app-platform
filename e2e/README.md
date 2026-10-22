# ARO workshop validation

Follow Acts 0–3 on the assigned ARO environment, including the real Dev Spaces browser session, OpenCode plan/build/change/audit prompts, Tekton builds, and personal Argo CD. These checks verify the resulting resources; they do not replace the participant exercises.

Run from the bastion with `oc` authenticated to the assigned cluster:

```sh
bash e2e/platform-run.sh
EXPECT_UPDATED_APP=true bash e2e/dev-run.sh
bash e2e/usage-run.sh
```

| Check | Evidence |
|---|---|
| Platform | Ready workers, cluster-admin, Dev Spaces / GitOps / Pipelines, workshop Tasks, Foundry configuration, real completion |
| Developer | Latest Tekton build Succeeded, personal Application Synced / Healthy, public HTTPS app and health endpoint, transcript copy/export, resource / probe / security / TLS audit |
| Usage | Real Foundry response with input and output token counts, before the Act 3 deployment deletion |

The scripts use Python 3 and `oc`. They print no credentials, create no cluster resources, and do not mint or rotate keys. Platform and usage checks each make one billable model call. `APP_NAME` defaults to `live-transcript`; `EXPECT_UPDATED_APP=true` checks the Copy transcript exercise.

To also verify the generated source, local Go build and pushed Git revision in the real workspace, pass `E2E_NS=<workspace-namespace> DEV_POD=<workspace-pod> DEV_CONTAINER=dev` to `dev-run.sh`.

For browser checks, launch Chromium with `--use-mock-keychain`. Verify catalog login in the published Cloud Terminal, the embedded WeTTY shell, OpenShift login, the Dev Spaces factory and editor, the public app, and the personal Argo CD console.

At the end, follow Lab 3.1 to delete only the Foundry deployment created for the workshop. RHDP owns the assigned cluster and its lifetime. The older cleanup scripts predate the Foundry flow and are not part of these validation commands.

[Full live validation and final environment state](VALIDATION.md) records the 2026-10-22 run on ARO 4.21.22.

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
