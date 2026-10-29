# Sky Control

A live radar game that puts local d3-mini in charge of a fictional airport. Watch
arrivals leave their holds, follow approach routes, land, taxi and depart. Increase
traffic and accelerate time until inference cannot keep up.

## Play

1. Start with **Easy**, then **Start AI**. Click an aircraft or flight strip to see
   its status, altitude, fuel and cleared route. The surface inset magnifies runway traffic.
2. Increase the three sliders and choose **Apply difficulty**, or use **Busy** and
   **Push limit**. Increasing the airborne target admits more aircraft; lowering it
   lets existing traffic finish. Reset for an immediately smaller scenario.
3. Watch total cycle time against the freshness window. Late answers are discarded;
   their red points and counters remain visible. Pause freezes physics; reset
   starts the same seeded scenario at the current difficulty.

Runways can close to new clearances while committed traffic finishes. A medical
priority button adds a natural-language emergency request to a holding aircraft.

## What the model controls

The model reads a compact global traffic snapshot: every active aircraft's phase,
position, altitude, fuel, wait, type and request; runway closure/count/next three
reservations; and every proposed landing/departure clearance with checked constraints.
It chooses one clearance or wait, with typed probabilities. It does not receive the
reference controller's choice or execute generated code.

The Go engine follows deterministic trajectories. It checks runway booking windows
and forecast separation again when a reply arrives. A stale or unsafe instruction
is refused; failure never invokes the reference controller. The simulation keeps
running during inference, and current controls remain responsive.

The model is a dispatcher, not the per-frame flight controller. An explicit safety
shield pauses before a separation violation or the game's two-minute fuel reserve.
Its interventions and unsafe proposals have separate counters. **Zero crashes by
itself is not evidence that the model managed traffic well.**

## The measurable limit

Each decision has a game freshness target of **45 simulation seconds**. At 8×,
the model has 5.625 real seconds; at 60×, only 0.75 seconds. This is a configurable-
pace game SLO, not a real aviation response-time requirement. Queueing and traffic
preparation count toward the window; inference endpoint latency is also recorded
separately. Every reply is validated against current traffic before application.

The UI separates missed deadlines, changed conditions, unsafe proposals, service
errors, arrival backlog and safety stops. Airport capacity does not hide inference
overload. The chart retains each cycle's own deadline when difficulty changes.

Initial real T4/browser tests: a 6-aircraft, 12× sector completed a landing and two
departures with 66 accepted decisions, no deadline miss, no crash and no shield stop.
At 24 aircraft / 60×, three approximately 3.4-second decisions all missed the
0.75-second window and were dropped. These are observations of this prototype,
not throughput guarantees or a claim to outperform a conventional controller.

## Game rules

Northfield is fictional. Positions use NM, altitude uses feet and speeds use knots.
The simplified engine uses 3 NM / 1,000 ft airborne separation, conservative runway
reservations and a larger wake buffer for heavy aircraft. It detects crossing
trajectories between ticks. Ground/low-final margins, flight kinematics, fixed
holding corridors and wake windows are game abstractions.

Primary references for the concepts, not a claim to implement the full procedures:
[FAA radar minima](https://www.faa.gov/air_traffic/publications/atpubs/atc_html/chap5_section_5.html),
[vertical separation](https://www.faa.gov/air_traffic/publications/atpubs/atc_html/chap4_section_5.html),
[same-runway separation](https://www.faa.gov/air_traffic/publications/atpubs/atc_html/chap3_section_10.html).

## Develop and test

```sh
go test -race ./...
go vet ./...
DECISION_URL=http://127.0.0.1:18082/v1/systemone go run .
```

The browser animates the authoritative Go state; background inference never holds
the physics/control lock. Bounded sessions, one GPU slot, a short queue wait,
request cancellation and generation/job checks protect pause/reset. Hosted model
URLs are rejected. The native runtime is the same pinned FP16 d3-mini custom
ServingRuntime on one T4. No second GPU, hosted inference, runtime key, Whisper,
monitoring privileges or SCC changes are needed.

From the matching facilitator workshop checkout:

```sh
python3 scripts/prepare-decision-tetris.py --reuse-runtime --game sky-control
```

Copy all Go/module files, Dockerfile and `web/` into the personal app, replacing the
previous game's Go files. Use the paired Sky Control template's AGENTS.md. Keep
the technical live-transcript app/image name and existing decision-tetris runtime
identity/config. Commit/push, run Tekton and verify the matching Argo revision and
running image. The stable published guide and previous prototypes stay available.

```sh
APP_URL=https://your-app-route BROWSER_STATE=/private/browser-state.json \
  node e2e/sky-control-browser.cjs
```

The real-browser harness runs the actual clock, landing/departure routes and model,
then drives difficulty into measured deadline overload. Chromium uses mock-keychain.
The offline `go run . evaluate` command is a separate explicit model exercise; its
extra simulation advances are labelled and do not replace wall-clock acceptance.

Native MLflow's `sky-control` experiment stores `atc.cycle` and `decision.choose`
spans: outcome, choice, token count, latency, queue time and budget. No raw traffic
prompt or credentials are exported. The community model/runtime uses approximate
FP16 numerics with unverified BF16 golden parity; this remains an experiment on
the Red Hat AI CUDA base, not a Red Hat validated model/runtime combination.
