# Orbit Checkpoint

Run the Europa spaceport gate against a local visual decision model. Read a
passport and cargo permit, follow the current directive, then stamp **Clear**,
**Inspect** or **Reject**. The AI independently reads the same document image.
A shift has 12 couriers and a new directive every four inspections.

- **Play:** stamp a verdict before the AI reveals its own. Correct consecutive
  stamps build your score. Keyboard shortcuts: C, I and R.
- **Observe:** choose “Watch AI shift”; pause to inspect any decision. Observed
  rounds do not count as human answers.
- **Inspect:** zoom the full-size documents, including on a phone. After each
  verdict, see the game engine's rule explanation, model probabilities and trace.

## Why this task

The [d3-mini model card](https://huggingface.co/vllm-sr/d3-mini#evaluation) reports
strong document/infographic results: KIE 96.9 and InfographicVQA 93.4. Those are
internal evaluations with approximate rebuilds, not scores guaranteed on our T4.
This game tests document reading and natural-language rules directly. It does not
feed numerical move features to the model as the earlier Tetris prototype did.

The first live T4 test correctly resolved 36/36 synthetic documents across three
preselected seeds. This is a small, controlled test with two fixed layouts, not a
general OCR benchmark or evidence of superiority over an OCR/rules pipeline.

## Local development

```sh
go test -race ./...
go vet ./...
DECISION_URL=http://127.0.0.1:18082/v1/systemone go run .
```

The model must be the local native d3-mini runtime. Hosted inference URLs are
rejected. No runtime model key, Whisper or second GPU is needed.

```sh
DECISION_URL=http://127.0.0.1:18082/v1/systemone go run . evaluate \
  -seeds 42,7,2026 -output /tmp/checkpoint-evaluation.json
```

The evaluator writes every verdict, including mistakes; it never retries or
substitutes an answer. `-images /tmp/checkpoint-documents` saves the exact PNGs.

## Assigned ARO lab

Start with the prepared Cluster Voice/Decision Tetris platform. From this
workshop checkout, the facilitator runs:

```sh
python3 scripts/prepare-decision-tetris.py --reuse-runtime --game orbit-checkpoint
```

Omit `--reuse-runtime` if the custom image has not been built yet. The helper
prepares the same local OpenShift AI runtime and a separate `orbit-checkpoint`
MLflow experiment. Whisper stays stopped to free the single T4. The existing
`decision-tetris` ServiceAccount/config names are retained as technical resources.

Copy this reference's Go files, module files, Dockerfile and `web/` into the personal
app, replacing the Tetris Go files. Use the paired template's `AGENTS.md`. Commit
and push a clean source state, run the usual Tekton pipeline, restart the app after
the image build and verify the matching Argo CD revision. Keep the technical
`live-transcript` app/image name and OAuth sidecar.

## Contract and observability

The server owns the seeded deck, PNG renderer, rules and answer key. Before a
decision, the browser receives only the image URL and public brief. Inference gets
the same PNG plus public rules, never extracted fields, expected answers or case
objects. Documents are synthetic; no external image fetch or generated asset is
needed. The Go Mono document font is provided by the BSD-licensed Go image module.

The model returns one of three typed choices. Invalid, failed or cancelled
inference leaves the round unresolved, without fallback. Wrong valid choices stay
wrong and affect the model score. Explanations come from the deterministic game
engine and are labelled “Rulebook verdict”; they are not invented AI rationales.

`checkpoint.inspect` and `decision.choose` spans record directive, choice,
correctness, tokens and latency in native MLflow, without document images, raw
prompts or credentials. Standard session bounds, same-origin checks, stale/duplicate
rejection, restricted admission and verified service-CA telemetry are retained.

```sh
APP_URL=https://your-app-route BROWSER_STATE=/private/browser-state.json \
  node e2e/orbit-checkpoint-browser.cjs
```

The real browser test plays a full shift against the local model; its injected
HTTP outage is a separate failure-handling check. Chromium uses mock-keychain.

The runtime/model remains experimental community software on a Red Hat AI CUDA
base. FP16 on T4 reports approximate numerics and unverified BF16 golden parity.
The published guide and Tetris branch remain available during this experiment.
