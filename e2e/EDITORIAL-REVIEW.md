# Workshop editorial and coherence review — 22 October 2026

Reviewed all 12 participant pages, navigation, the facilitator README, diagrams and their generator, relevant deployment manifests, and both application-template branches.

## Corrections

- Removed the source-stays-on-cluster claim: OpenCode sends coding context to Foundry, while microphone audio goes to internal Whisper through the Go backend.
- Replaced stale Qwen/Fortune Cookie/A100 diagrams with DeepSeek + Live Transcript + Whisper INT8 on one T4. Updated the diagram generator as well as the rendered assets.
- Made classic ARO, T4 quota, CPU headroom, near-real-time clip semantics and the transcription template branch explicit.
- Expanded Act 3 from token-only billing to coding tokens, GPU/CPU/disks and scoped facilitator cleanup after every participant finishes. Account-key lifetime is distinguished from deployment lifetime.
- Verified the example DeepSeek list rates against Azure Retail Prices on the review date; included cached-input accounting and marked session totals as illustrative.

- Aligned lab titles, navigation, persona transitions, terminal locations, timing and background readiness gates. Renamed “Prompt to Production” to “Prompt to Deployment” to match the demo outcome.
- Replaced automatic-compliance promises with agent-assisted review and direct cluster checks. The audit now reports scaffold differences instead of contradicting the template’s no-rewrite rule. Clarified the difference between the declared Deployment and the SCC-admitted Pod.
- Made the one-cluster-per-participant assumption explicit. Shared clusters need facilitator-managed app isolation and shared-resource cleanup.
- Fixed AsciiDoc shell-variable rendering and a missing generated-site favicon where applicable. Simplified the conclusions and template documentation.

## Verification

- Antora 3.1 build completed with no warnings or errors.
- All 12 pages appear in navigation; 327 generated local link/asset references resolve, including checked target fragments.
- All 43 shell source blocks pass `bash -n` (syntax only; no provisioning or cleanup commands executed).
- The documented OpenCode JSON matches the deployed platform ConfigMap.
- Chromium opened all participant pages with no missing article images; overview diagrams fit their content at desktop (1440 px) and mobile (390 px) widths. Browser launched with `--use-mock-keychain`.
- External reference checks covered 18 distinct linked URLs across both guides. Red Hat documentation rejects some automated HEAD requests; those references were checked through documentation/search retrieval. Azure Retail Prices requires GET. No confirmed dead reference remained.

This is an editorial/static/browser review, not a new full cloud E2E run. Existing runtime validation reports remain separate evidence. No cluster or deployment resources were changed during this review; the ARO app’s SCC was inspected read-only. The deferred OpenCode workshop companion experiment remains outside the participant path.
