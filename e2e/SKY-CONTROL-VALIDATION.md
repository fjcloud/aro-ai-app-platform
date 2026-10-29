# Sky Control validation — 2026-10-27

**Passed on the live ARO lab `tfclk`.** The real local T4 model controls a moving
airport simulation. Difficulty exposes measured inference saturation while the
engine's explicit safety shield prevents crashes. This is an experimental branch;
the published guide and earlier game prototypes remain available.

## Wall-clock browser acceptance

The authenticated Chromium harness uses the actual Go timer and local inference.
Physics continues during requests; controls do not wait for the model to finish.
Every Chromium launch uses `--use-mock-keychain`.

| Observation | Nominal | Stressed |
| --- | --- | --- |
| Airborne target | 6 | 24 |
| Simulation rate | 12× | 60× |
| New arrivals | 0/min during this controlled run | 4/min |
| Real decision window | 3,750 ms | 750 ms |
| Median total cycle | 957.5 ms | 3,376 ms |
| Median inference endpoint latency | 955.5 ms | 3,283 ms |
| Maximum recorded input tokens | 1,581 | 4,617 |
| Decisions accepted | 46, including maintain/wait decisions | 0 |
| Late decisions discarded | 0 | 3/3 |
| Completed movements | 1 landing, 2 departures | 0 |
| Crashes / shield stops / inference errors | 0 / 0 / 0 | 0 / 0 / 0 |

The stressed sector displayed **AI LIMIT** even when an arrival backlog also
existed. Every late reply exceeded its own recorded budget and was dropped.
No artificial inference delay, hosted model or substituted controller was used.

The browser also verified flight/route selection, pause freezing the physics clock,
runway closure to new clearances, a medical-priority request, reset, preset/slider
updates, mobile layout and no browser exceptions. The nominal screenshot captures
the first clearances; the separate stress screenshot shows the measured limit.

## What is, and is not, attributed to the model

d3-mini chooses a typed landing/departure clearance or wait. It receives all active
aircraft's compact status, requests, runway summaries and checked action options.
The engine's reference-controller choice is never supplied. The Go engine owns
route following, runway windows, fuel, physics and separation. It rechecks current
conditions after inference, independently rejecting stale or unsafe commands.

The 45-simulation-second freshness target is a **game SLO**, not an aviation rule
or a computed time to collision. Acceleration divides the available real time.
Queue wait and traffic preparation count toward it; endpoint latency is separate.
An overdue response never takes effect, even if it proposes an otherwise valid move.

Zero crashes alone is not evidence of good AI control: the shield can stop before
a separation/fuel-reserve violation. Its stops, unsafe proposals, stale conditions,
deadline misses, service errors and outside-sector backlog have separate counters.
No shield intervention was needed in the nominal acceptance run. The stressed
aircraft continued their existing holds; the late decisions created no new routes.

## Deployment and access

- Personal Git, actual Dev Space and Argo revision:
  `be0fba63b7068cd32ba3e5343fd3c73d80ee9618`; clean workspace, test/race/vet/build pass.
- PipelineRun `build-sky-app-rnvtm`: Succeeded. Argo: Synced / Healthy; both app
  containers Ready. Running image:
  `sha256:0270b64bcae2ad1202f7428c5afb74ebbef1494d5c7b0242093181545226ab78`.
- App and model: `restricted-v2`; build: `pipelines-scc`, `hostUsers: false`.
  Existing OAuth/CA/image guard and technical app/runtime identity are retained.
  No privileged SCC, extra GPU, hosted game inference or monitoring grants added.
- Anonymous `/api/state`: HTTP 403. `/healthz`: HTTP 200. Actual OpenShift login works.
- Custom OpenShift AI `InferenceService/d3-mini`: Ready, internal Service only,
  no model Route. Same pinned FP16 community runtime on the existing single T4.
  Observed GPU allocation after final stress: **12,309 MiB / 16,384 MiB**.
- Model `vllm-sr/d3-mini`, revision `a475367de0d8133a3c1a2a0dc12303d89cfa2ab4`,
  native CUDA FP16, no CPU offload. The runtime was restarted before acceptance
  to clear previous vision-test allocations. It reports approximate numerics and
  unverified BF16 golden parity; this is not a Red Hat validated combination.
- The facilitator's `--reuse-runtime --game sky-control` setup completed and
  prepared the separate native MLflow experiment. No temporary registry Secret remains.

## Observability and regression

Native MLflow experiment **sky-control**, ID 4, workspace `live-transcript-dev`,
is visible in the real UI with trace/latency/token metrics. Persisted decisions
contain parent `atc.cycle` and child `decision.choose` spans, with aircraft count,
speed, outcome, queue/total time, budget and tokens. No raw traffic prompt or
credentials are exported. Zero output tokens are generated. A deadline miss is
an application outcome, distinct from an HTTP/inference error.

33 Python checks, all four Go modules with race detection, Go vet, 16 paired
template checks and CI/Antora pass. Sky Control's four Go test functions cover
between-tick conflicts, safety stops, seeded movement/reservations, actual
landing/departure progress, responsive controls during blocked inference,
pause/cancellation, origin checks, late-response rejection and saturation labels.

An additional one-hour deterministic reference run with **new arrivals enabled**
(seed 43, target 4, 0.2 arrivals/min) completed **9 landings and 2 departures**,
with zero crashes, shield stops or remaining backlog. This checks physics/admission;
it is explicitly a reference-controller test, not a model performance result.

## Limits and artifacts

The airport, corridors and kinematics are fictional. Separation concepts are
inspired by the official FAA sources linked in the reference README; this does
not implement the full aviation procedures. The measured settings/sample do not
establish optimal scheduling, general reliability or concurrent-user capacity.
The UI retains the last 60 cycles; cumulative counters can therefore exceed the
recorded recent history. No claim of superiority over a conventional controller.

[Browser results](results/sky-control/browser.json),
[nominal view](results/sky-control/deployed-app.png),
[stressed view](results/sky-control/deployed-stress.png),
[deployment](results/sky-control/deployment.json),
[persisted trace](results/sky-control/persisted-trace.json),
[native MLflow UI](results/sky-control/mlflow-ui.json),
[dynamic-arrival reference](results/sky-control/dynamic-arrivals-reference.json).
