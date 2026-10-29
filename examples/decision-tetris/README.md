# Decision Tetris — local T4 experiment

A Go web app in which `vllm-sr/d3-mini` chooses legal Tetris placements. The model
runs on the assigned cluster's single NVIDIA T4; the app rejects hosted inference
URLs. No model key or game state comes from the browser.

The game engine owns the board, seven-bag piece sequence, rotations, straight-drop
placements, line clearing and scoring. Each turn sends exact board/outcome features
and the legal option set to the local System One endpoint. The model selects one
option and returns probabilities. The engine applies that option. It never executes
generated commands and never substitutes another player when inference fails.

This is placement-based Tetris, without real-time gravity, SRS wall kicks or hold.
The UI offers autoplay, single steps, pause, a manual player, and an explicitly
labelled heuristic baseline. Reset with the same seed to compare players fairly.

## Run locally

```sh
go test -race ./...
go vet ./...
DECISION_URL=http://127.0.0.1:18082/v1/systemone go run .
```

The default endpoint is
`http://d3-mini.decision-serving.svc.cluster.local:8080/v1/systemone`.
`DECISION_MODEL` defaults to `vllm-sr/d3-mini`. `LISTEN_ADDR` defaults to `:8080`;
keep `127.0.0.1:8080` behind the prepared OpenShift OAuth sidecar.

## Prepare the lab

This is an experimental alternative to the published Cluster Voice workshop.
Start from its prepared ARO platform, CPU/T4 worker, application namespaces,
Tekton tasks and native MLflow. From the bastion, use this committed/pushed branch:

```sh
python3 scripts/prepare-decision-tetris.py
```

The facilitator helper builds a custom runtime using Tekton and Linux user
namespaces, caches the pinned model on a 20 GiB PVC, stops Whisper to free the
single T4, deploys an OpenShift AI custom ServingRuntime/InferenceService, and
prepares app access and MLflow. It uses the existing cluster registry credentials
only for the runtime build and removes its temporary build Secret afterwards.
`--reuse-runtime` reuses an image already built on this assigned cluster.

Copy the Go files, module files, Dockerfile and `web/` into the personal app.
Keep the existing technical app name, image, pipeline and Argo CD application.
The app Deployment uses ServiceAccount `decision-tetris`, ConfigMap
`decision-tetris-config`, the service CA and the existing OAuth/TLS volumes. Remove
the speech/agent model-key mount and use `--openshift-service-account=decision-tetris`
in the OAuth proxy. No monitoring permissions, privileged SCC or new GPU is needed.
Commit and push a clean source state, then run the normal build-app pipeline,
restart the app after the successful image build and verify Argo's matching revision.

## Runtime and precision

The community vLLM Semantic Router native runtime is pinned to
`ff293191da0232b4e252bee2edd6ed825cacd45d`; it runs on the pinned Red Hat AI CUDA
base image in `../../deploy/decision/runtime/Dockerfile`. It does not execute Python code from the model
repository. This is a custom community runtime, not a Red Hat-validated model/runtime
combination or the stock vLLM OpenAI chat server.

The native model package revision is `a475367de0d8133a3c1a2a0dc12303d89cfa2ab4`.
The T4 runs its backbone in FP16, with no CPU offload, FP8, BF16 autocast or hosted
inference. This changes numerics from the published BF16 reference; the runtime
honestly reports `golden.status=unverified` and `numerics=approximate`. Live Tetris
behavior and legal output checks do not establish parity with published benchmarks.

## AgentOps and verification

`OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`, `MLFLOW_EXPERIMENT_ID`, `MLFLOW_WORKSPACE`
and the projected ServiceAccount token export `tetris.turn` and `decision.choose`
spans to the native `decision-tetris` experiment. Spans record the selected placement,
confidence, line/board features, input tokens and latency, without raw board/prompt
text or credentials. The app also exposes model-call/error and line counters at
`/metrics` behind OpenShift authentication.

```sh
APP_URL=https://your-app-route BROWSER_STATE=/private/login-state.json \
  node e2e/decision-tetris-browser.cjs
```

The browser test uses `--use-mock-keychain` and the real local model. Its injected
HTTP error checks are explicitly separate from the real inference turns.

## Model versus heuristic

Tetris is a visual integration demo, not evidence that a decision model improves
on a numerical policy. Both players receive exact outcome features; the fixed
heuristic can exploit them without inference or a GPU. The model's probabilities
are model outputs, not measured probabilities of surviving the game.

Run a paired comparison against the deployed app with an authenticated `oc` session:

```sh
python3 e2e/decision-tetris-compare.py --url https://your-app-route \
  --output /tmp/tetris-comparison.json
```

The same three seven-bag seeds are replayed for both players, up to 200 pieces each.
The script checks their common piece sequence and records every turn, lines, score,
top-out and decision/HTTP latency. It stops on errors instead of replacing model
decisions. This small test measures this prototype, not general decision ability.
See [live validation](../../e2e/DECISION-TETRIS-VALIDATION.md) for the recorded results.

Semantic routing, support-request classification and choices governed by changing
natural-language instructions are more relevant candidates for this model's
choice/yes-no/score interface. Each still needs a task-specific comparison with
simple rules and labelled examples; superiority is not assumed.
