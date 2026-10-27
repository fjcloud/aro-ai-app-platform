# Decision Tetris validation — 2026-10-27

Experimental alternative on the live ARO lab `tfclk`. The stable published guide
remains Cluster Voice; this prototype is isolated on `feature/decision-tetris`.

## Local T4 and OpenShift AI

- `vllm-sr/d3-mini`, 4,539,918,336 parameters, revision
  `a475367de0d8133a3c1a2a0dc12303d89cfa2ab4`, runs on the existing single Tesla T4
  in FP16. Observed GPU memory during the final deployment: 10,001 MiB of 16,384 MiB.
- OpenShift AI custom `ServingRuntime/d3-native` and `InferenceService/d3-mini`
  are Ready. The internal Service selects the KServe predictor; no model Route
  exists. Whisper is stopped, with its model configuration/cache retained.
- The native community runtime is pinned to
  `ff293191da0232b4e252bee2edd6ed825cacd45d` and installed in a custom Red Hat AI
  CUDA base image. Built image digest:
  `sha256:259decda193592a021d5c7b7d95c0ca4f5772167cac9a1fe2393774953b4b0db`.
- No hosted game inference, additional GPU, CPU offload, remote model Python,
  privileged SCC or admission bypass. The app rejects hosted inference URLs.
- FP16 changes the published BF16 numerics: the runtime reports
  `numerics=approximate` and `golden.status=unverified`. This is not a Red Hat
  validated model/runtime combination, or proof of published benchmark parity.

## Application delivery and access

- Actual Dev Space source, personal Git and Argo CD match revision
  `4e83d48e1e98a6dc5abd5016873024a08b3d1617`, with a clean workspace.
- PipelineRun `build-tetris-app-wk59p`: Succeeded; Argo CD: Synced / Healthy.
  App image digest:
  `sha256:39ccccf70400b801501f628723ea2698b5dd8664a47e7705b0c8348c756b4600`.
- Both app containers Ready; app and model use `restricted-v2`. Runtime image
  build `build-decision-runtime-bpgfb` uses `pipelines-scc` and `hostUsers: false`.
  Its temporary registry-authentication Secret was deleted after the build.
- Anonymous `/api/game` access returns HTTP 403; `/healthz` returns HTTP 200.
  Authenticated OpenShift browser access and scoped ServiceAccount API access work.
  The new app ServiceAccount cannot read the monitoring API.
- The facilitator helper's `--reuse-runtime` path completed on the live lab.
  Its custom-image build/push path was also exercised through the actual Tekton
  tasks; the resulting image is pinned by digest in the live ServingRuntime.

## Game and observability

The server enumerates legal placements and computes exact outcome features. The
model selects one option; it does not generate executable game code. Model errors
leave the board unchanged, with no heuristic fallback. This is straight-drop,
placement-based Tetris, without gravity, wall kicks or hold.

An initial live local-model run reached 200 placements, 68 lines and score 28,800,
without topping out at the cap. Median inference latency: 1,032 ms; maximum:
1,874 ms. This initial run used the same native runtime in a standalone Pod;
the final delivery uses the OpenShift AI custom InferenceService above.

Real Chromium checks against the final OpenShift AI InferenceService pass (first
model decision: 541 ms in this browser run). They cover actual model moves, probability bars, autoplay,
pause/cancellation, manual placement, reset, mobile layout and browser errors.
An explicitly injected HTTP 502 checks failure handling separately from real
inference. Every Chromium launch uses `--use-mock-keychain`.

Native MLflow's `decision-tetris` experiment is visible in the actual UI, with
trace/latency/input-token metrics. Persisted model turns contain `tetris.turn`
and `decision.choose` spans, with no raw board/prompt text or credentials.
The model generates zero text tokens. MLflow's zero-cost display means no cost
data is configured, not that the GPU is free.

## Paired comparison on the final InferenceService

Identical seven-bag seeds, same legal moves and scoring, 200-piece cap. Both players
receive exact outcome features; the heuristic is the fixed weighted policy in
`engine.go`. No prompt or weights were tuned between games, no fallback was used,
and the common piece sequence was checked for every pair.

| Seed | Heuristic: pieces / lines / score | d3-mini: pieces / lines / score | d3-mini top-out |
| --- | --- | --- | --- |
| 42 | 200 / 71 / 30,600 | 200 / 68 / 28,800 | No, capped |
| 7 | 200 / 78 / 36,000 | 113 / 29 / 6,200 | Yes |
| 2026 | 200 / 75 / 35,600 | 137 / 38 / 10,500 | Yes |

The heuristic survives all three caps and clears more lines in each pair. With this
prompt/runtime/policy, d3-mini has no observed gameplay advantage. Median HTTP
latency is 321–323 ms for the heuristic and 1,340–1,372 ms for the model. Model-only
median inference latency is 907–939 ms. The heuristic's internal computation time
is **not instrumented**: its API `0` is a default, not a sub-millisecond measurement;
the comparison artifact records it as `null`.

These three sequences are a smoke comparison, not a general decision benchmark.
Survivors were stopped at 200 pieces; their full-game survival is unknown. Model
probabilities are not calibrated Tetris success rates. Semantic classification and
instructions that change in natural language are more relevant candidate uses,
but require their own labelled evaluation against simple rules.

## Regression checks and limits

33 Python workshop tests, six Tetris Go test functions with race detection,
`go vet`, the actual Dev Space's Go test/vet/build, and 16 paired template checks
pass. Both repositories' CI checks pass. The existing Cluster Voice Go tests and
Antora build also passed on this feature branch.

The exact-revision Tekton clone correction is regression-tested against a real
Git repository: branch, commit SHA and missing revision. A missing revision fails
instead of silently building the default branch.

See the [deployed browser screenshot](results/decision-tetris/deployed-app.png),
[persisted model trace](results/decision-tetris/persisted-trace.json),
[paired comparison](results/decision-tetris/paired-comparison.json),
[recorded deployment](results/decision-tetris/deployment.json),
[initial 200-turn run](results/decision-tetris/game-200-turns.json), and the paired
comparison in the same results directory. This validates a disposable workshop
prototype, not production capacity or a general claim of model superiority.
