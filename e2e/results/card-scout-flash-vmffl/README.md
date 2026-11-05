# Card Scout regenerated with DeepSeek V4.1 Flash

Live replacement test on vmffl, November 5, 2026. The previous Pro-generated
frontend was removed: Argo CD Application, app Deployment/Service/Route and
`card-scout:latest` tag. Its source was privately archived outside the cluster
and replaced with a fresh template checkout at `b1ae1b7`, with no Go app source.
The facilitator selected Flash in `opencode.json`; OpenCode generated the new
application from the contract without receiving the previous implementation.

## Model and scope

- Coding model: **DeepSeek-V4.1-Flash, version 2026-09-10**, GlobalStandard in
  East US 2, accessed through OpenShift AI MaaS.
- Tool: OpenCode 1.18.14, explicitly selecting `deepseek/deepseekv41flash` in a
  new session. Direct completion and function-call probes passed before generation.
- The prepared dataset, D3/T4 service and price-research backend were reused.
  The existing native price agent continues to use its Pro deployment; this test
  changes the coding model, not that runtime agent.
- Current lab developer credentials are scoped to the experimental Flash MaaS
  subscription. Canonical workshop model defaults were not switched by this test.

## Review findings

This also required review feedback; it was not a one-shot result.

1. The initial proxy silently capped every response at 256 KiB, truncating actual
   card images (Charizard: 360481 bytes; Black Lotus: 2186336 bytes). It also
   followed redirects and dropped Location. The first old-revision build,
   `build-app-6x2fb`, was cancelled. Flash corrected the proxy and added real-router
   tests for a >2 MiB image, >256 KiB JSON, and an untouched redirect/target that
   must never be contacted. The corrected first delivery used `build-app-5fj8m`.
2. Browser fixtures found a missing initial-load Retry action, a refresh failure
   hidden behind old research evidence, and clickable non-HTTPS URLs. Flash fixed
   these, aligned result labels/values, and implemented the documented localStorage
   filter exercise. Retry is labelled **Check price again** for price research.
   Prior evidence is explicitly marked when a new attempt fails. The final
   delivery used `build-app-6jnrc`.

The Go source, UI and their tests were written and corrected by Flash. No completed
application source or prior implementation was supplied. Template
[PR 13](https://github.com/fjcloud/go-app-template/pull/13) makes the validated
complete-body/redirect checks explicit for future generations. The isolated Go cache
and existing user-namespace Tekton scaffold were reused; no privilege changes
were needed.

## Final delivery

App: https://card-scout-card-scout-dev.apps.h02mzao2.eastus.aroapp.io

- Git revision: `b9ad750e00458365f444c7bb9debc672b4f9199b`.
- Image: `sha256:f0ce65a883317c0cf4516fe3313e15e4ccd0efe94422a63fe060fa6ec9b02fe7`.
- Argo CD: **Synced / Healthy**; running pod digest matches the build.
- App admitted under **restricted-v2**, no model key or Kubernetes token mounted.
- Go race tests, vet and production build passed in the actual Dev Space.

`delivery.json` and `delivery-check.txt` record the independent revision, digest,
public access and security checks. `image-integrity.json` compares actual
Charizard, Pikachu artwork and Black Lotus responses byte-for-byte with the
platform, including the complete 2 MiB image and original content types.

`browser.json` records live tests: 12-card pagination, full-batch filters,
preference restoration, cached Analyze batch recovery, control rejection, fresh
native price research with tool-derived numbers, exact source/trace links,
whole images on desktop/mobile and no horizontal overflow or JavaScript errors.

`browser-fixtures.json` separately verifies initial 503 recovery, apostrophes,
zero versus null prices, escaping/HTTPS-only links, visible refresh failure with
previous evidence, working retry and a recoverable empty filter. These are
explicitly synthetic cases, not replacements for live model calls.

`trace.json` identifies a fresh research trace. An authenticated browser opened
its exact native MLflow graph and checked `card.research`, `agent.responses` and
`tool.get_card_market_data`. Browser runs used `--use-mock-keychain`.

Screenshots: [desktop](desktop.png), [390px mobile](mobile.png).

## Comparison limits

See the [Pro generation run](../card-scout-generation-vmffl/README.md). Both models
completed this application with review feedback. This is not a controlled model
benchmark: Flash started with the clarified contract/240s Route and a prepared
platform, while the Pro run also encountered cache and inactivity interruptions.
No superiority or visual-classification accuracy claim follows from this single
example. The prepared 105-image analysis was reused, not rerun as an accuracy test.

No API keys, kubeconfigs, browser state, private Git URLs or raw tool logs are
included in this record.
