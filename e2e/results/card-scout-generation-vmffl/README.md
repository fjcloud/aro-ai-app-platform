# Card Scout generation on vmffl

Live generation and delivery validation, November 5, 2026. The starting checkout
was the ARO template at `700e324`, with no Go application source. OpenCode
1.18.14 in a real Dev Space used `deepseek/deepseekv4pro` through the prepared
OpenShift AI MaaS gateway. The agent read the contract/scaffold and wrote the Go
server, embedded HTML/CSS/JavaScript, tests and Dockerfile from scratch. No
completed application or workshop reference implementation was supplied.

## Model availability

The Azure management API reports the active deployment as **DeepSeek-V4-Pro
2026-04-23**, GlobalStandard in East US 2. It is the newest Pro version offered
in this account's model list. **DeepSeek-V4.1-Flash 2026-09-10** is also offered,
with Preview lifecycle status. This does not establish that Pro is more capable;
this generation test used the documented Pro deployment. Flash was not compared
on this task. The complete non-secret query result is in `model-catalog.json`.

## What the test found

This was **not a successful one-shot generation**. The initial implementation
needed review feedback:

- Tests duplicated routing instead of calling the production router; the
  deployed root handler also needed strict path matching. Redirects needed to
  be returned unchanged rather than followed. A router factory with an injected
  upstream URL fixed these issues; tests exercise that actual production code.
- Dev Spaces Go cache errors interrupted compilation. Sequential commands with
  a shared isolated cache, bounded parallelism and a workspace restart completed
  successfully. The IDE also stopped once for browser inactivity during CLI-only
  automation; project sources persisted and the test resumed with the IDE open.
- Initial browser checks found horizontal overflow at 390px after a price result,
  and an unhandled initial batch-fetch failure with no visible Retry action.
  These findings were sent back to DeepSeek for correction.
- The documented filter-preference exercise was requested with `localStorage`,
  retaining full-batch filtering before pagination.
- The scaffold Route had a 120s timeout despite the contract's 210s research
  deadline. A temporary HTTPS Route with 240s timeout returned status 200 after
  **125.3s** (`route-timeout.txt`). The temporary probe resources were removed.

Template PR [12](https://github.com/fjcloud/go-app-template/pull/12) clarifies the
production-router test boundary and raises the Route timeout to 240s. It adds no
completed app implementation. Its 18 regression tests and Route server dry-run
passed before merge.

## Delivery and evidence

The first real clone/validate/build/push run was `build-app-27mnb`; its source
revision was `101423755b494d1177b3a6270ecc818b22950433`. The initial deployment
passed the independent delivery check, including exact Git/image alignment,
restricted-v2 admission and absence of app credentials or Kubernetes tokens.
The UI corrections were delivered as a second real Tekton/Argo CD iteration.

Final delivery: revision `be55856bf6e24a022b8be9a87ed1fbc9a7eaee01`,
Tekton `build-app-sjkq2`, Argo CD **Synced / Healthy**, image
`sha256:cb59fa9114784855a5c3b16dab963be41213345510dd134bef9b2f6eb41556a5`.
The running pod digest matches the build. App route:
https://card-scout-card-scout-dev.apps.h02mzao2.eastus.aroapp.io

`delivery-check.txt` and `delivery.json` record revision, image and security
checks. `browser.json` verifies the public API, 12-card pagination, full-batch
filters, filter restoration, all control rejections, fresh native price research,
actual source/trace links, complete image proportions, no overflow at 390px and
no JavaScript errors. The Analyze batch action returned 202 and recovered the
cached collection. `browser-fixtures.json` verifies initial 503 error/working
Retry, apostrophes, zero versus null, HTML escaping, unsafe-link rejection and
price-error selection preservation. These fixtures do not replace live model
calls. `trace.json` records the new research trace; a real authenticated browser
opened its exact MLflow graph and verified the agent and actual MCP tool spans.

Final results are recorded alongside this document. Live API/browser checks are
kept separate from explicitly synthetic outage, zero/null price and unsafe-link
fixtures. The prepared 105-image T4 results predated this generation exercise;
resuming cached batch analysis tests application recovery, not a new visual
accuracy benchmark.

No API keys, kubeconfigs, browser state, private Git URLs or raw model/tool logs
are published here. Browser runs used Chromium with `--use-mock-keychain`.

Desktop and mobile captures: [desktop](desktop.png), [390px mobile](mobile.png).
