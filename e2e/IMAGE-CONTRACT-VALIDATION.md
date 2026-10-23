# Deployment image contract — 23 October 2026

The old application template shipped `image: placeholder`. The coding agent had to replace it after building, so a missed step let Argo CD request a nonexistent image.

## Fix

Both `go-app-template` branches now declare their exact internal-registry image and `imagePullPolicy: Always`. The Tekton image parameter has the same default. The pipeline runs `clone → validate-image → build`; validation compares the Kustomize-rendered workload image to the build target and checks that the target belongs to the application’s build namespace. A mismatch fails before Buildah.

The runbook keeps that image, fixes legacy placeholders in Git before building, checks the published ImageStreamTag before creating an Argo CD Application, and waits for the matching revision and rollout. It also identifies the existing `system:image-puller` ClusterRole correctly; cross-project access is granted with a namespaced RoleBinding.

## Validation

- Ten regression tests passed on each template branch, and the GitHub Actions checks passed.
- Four real TaskRuns used the same validation step on ARO 4.21.22 (`natfzu`). A matching image succeeded. A literal placeholder, a placeholder build target, and a Kustomize image override failed with exit code 1 and explicit error messages.
- Full PipelineRun `image-guard-validation-42kth` succeeded with all three tasks. Buildah pushed the image to the internal registry.
- The existing participant checkout was clean and matched the old template’s runbook/pipeline. The image contract was committed and pushed to that personal repository. Application Go source, assets and Dockerfile were not changed.
- The application rolled out successfully and a Ready pod pulled the exact digest produced by that build. Argo CD finished Synced / Healthy at the pushed personal revision; the public HTTPS `/healthz` returned `{"status":"ok"}`.
- The developer checker passed image, resource, probe, security, HTTPS and workspace Git/build checks.

See [the recorded results](results/image-contract.json). Temporary negative-test resources were removed after verification. The working application and updated build pipeline remain available.

This validates the image-reference regression and delivery path. It does not claim that unrelated registry outages, missing pull permissions or arbitrary changes outside the workshop workflow cannot cause an image pull failure. ROSA received the same template fix and unit checks; this live run was on ARO.
