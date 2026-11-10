# Historical POC validation

Validated on 3–4 November 2026 before the experiment was parked:

- 24 production-path tests passed: model response validation, source grounding,
  tool orchestration, cancellation, updates/deletions, stream cursors, retries,
  model connection supervision, HTTPS origin checks and the filtered snapshot.
- The public cluster browser checks passed for live SSE, Pause/Resume, source
  links, research traces, shared viewers, desktop and 390px mobile rendering.
- The Dark side browser checks showed 100 actual retained filtered posts and
  seven positive stories. Native scores matched the UI. There were no generated
  summaries or research controls in dark mode, no POSTs when switching views,
  no mobile overflow and no JavaScript errors.
- The cluster app used Red Hat UBI 9 Node.js 22 (22.23.2), restricted-v2,
  a non-root UID, read-only root filesystem and no mounted Kubernetes token.
- A hash-verified transfer preserved 500 original runtime records and seven
  published stories. A deployment restart preserved the stories and resumed
  the WebSocket. The app called the existing cluster D3 service directly.
- A separate component check in the pod used a real prior Euronews post: D3
  classification took 571ms; research took 18.0 seconds with three fetched
  source domains. That component test was not injected into the live feed.

These are historical behavior checks, not an independently measured classifier
accuracy or a steady-state performance guarantee. Raw live-state files, article
excerpts, credentials and research captures are excluded from this archive.
The currently selected workshop remains Card Scout / Pokémon.
