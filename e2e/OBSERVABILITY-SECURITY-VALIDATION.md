# Observability and scoped delivery validation

Validated on **26 October 2026**, on the assigned **mnzq5 ARO 4.21.22** cluster. The platform, developer workspace, application, MaaS endpoint and single-T4 Whisper service remain available. The participant cleanup sections have been removed from both ARO and ROSA; RHDP expiration determines the environment lifetime.

## Working paths

- Reapplied `scripts/install-platform.sh` successfully, including all readiness gates. Cluster Observability Operator 1.5.3 and Red Hat OpenTelemetry 0.158.0 run the native OpenShift AI metrics stack with one Prometheus replica, 5 GiB storage and seven-day retention. All 33 cluster operators were Available without degradation after the check.
- Enabled Kuadrant observability and MaaS tenant telemetry, with model/subscription dimensions and no user/group attribution. Corrected Whisper collection with a ServiceMonitor selecting the actual predictor Service and HTTP port; the generated KServe monitor did not select that Service.
- Opened **Observe → Dashboards → Workshop AI — ARO** in the OpenShift console. The browser rendered the coding-token curve, including a real request's increase from 22 to 70 tokens. [Screenshot](results/observability-security/console-dashboard.png).
- Queried all nine dashboard expressions through OpenShift Thanos: coding tokens, quota-accepted requests, quota rejections, Whisper completions, model-side p95, queued requests, T4 utilization, T4 memory and scrape health. [Samples](results/observability-security/metrics.json).
- Made a real DeepSeek request from the workspace through MaaS: HTTP 200, 14 input / 34 output / 48 total tokens, 1.56 seconds total request duration. This is not TTFT.
- Repeated the isolated quota test and revoked/deleted its own subscription and short-lived key. The main workshop subscription stayed available. [Result](results/observability-security/quota.json).

The native OpenShift AI dashboard opens with Cluster, Models and LLM-oriented views. Some native panels show no data for this combination of external Foundry inference and a classic speech InferenceService. The participant guide therefore uses the verified console dashboard for workshop measurements, rather than promising data in every native panel. The external-model integration remains Technology Preview.

## Permissions and full delivery

The facilitator prepares application namespaces and injects a **24-hour, scoped service-account kubeconfig** into Dev Spaces. No administrator credential is injected. The live workspace identifies as `system:serviceaccount:live-transcript-dev:workshop-developer`. It can create PipelineRuns and Deployments in the two application namespaces; it cannot create namespaces, cluster bindings, namespaced role bindings, or use the `privileged` or `anyuid` SCCs. [Authorization checks](results/observability-security/permissions.json).

From that real workspace and identity, followed the template runbook using the tested speech reference as application source:

1. Ran Go tests, created a personal GitPop repository, committed and pushed the application.
2. Applied the golden pipeline and started **build-app-84mbn**. Clone, deployment-image validation and build/push all succeeded, with registry TLS verification enabled.
3. Applied the personal Argo CD scaffold and Application for revision **67463e22b04eb3df79445d0c3d348feabf440adf**.
4. Verified the matching pushed revision was **Synced / Healthy**, the public HTTPS health endpoint answered, and the application pod was admitted under **restricted-v2** with non-root execution, dropped capabilities and no privilege escalation.

[Delivery evidence](results/observability-security/delivery.json) and [independent developer assertions](results/observability-security/developer.txt) record the exact revision, pods and security settings. This validates deterministic delivery from the runbook; it is not a new unattended LLM-generation run.

Build containers use the standard **pipelines-scc**, `hostUsers: false` and Red Hat's Buildah image, without `privileged` or privilege escalation. UID 0 is inside the container's Linux user namespace and maps to a non-root host UID. The application uses a separate restricted security context. No privileged, anyuid, cluster-admin or custom-SCC grant was added.

Live testing exposed and fixed two user-namespace mounting issues: enter the mounted workspace explicitly after container startup, and copy the build context off the idmapped PVC before Buildah's chroot copier runs. Workspaces mount at `/source`, outside Tekton's internal `/workspace` mount. Namespace UID/MCS allocation must finish before customizing the build supplemental-group range; pre-setting it on creation prevented other security annotations from being allocated.

## Browser speech check

The public Go application was tested with Chromium's microphone fixture against the real T4 Whisper service, using `--use-mock-keychain`. Text appeared while recording, two independent clips completed in **294 ms** and **147 ms**, and the first text appeared after **5.8 seconds**, including the capture window. Transcript export, microphone release, permission denial and backend-error handling passed. [Browser result](results/observability-security/speech-browser.json).

## Scope and automated checks

- ARO: 33 helper tests; both template branches: 14 tests each; ROSA: 13 helper tests.
- Both Antora builds, source shell syntax, local links/fragments and Kustomize rendering passed.
- The same scoped-access preparation, non-privileged shared build tasks, template restrictions and final-cleanup removal were applied to ROSA. A new full live ROSA run was not performed in this change.
- Temporary build experiments in `unprivileged-build-check` are removed after evidence collection; the actual workshop application and model services are retained.
