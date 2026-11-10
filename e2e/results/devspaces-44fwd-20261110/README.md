# Dev Spaces startup — 44fwd, 10 November 2026

The workspace was Pending with `FailedScheduling: Insufficient cpu`. All four
CPU workers were Ready, including the additional D8 worker. The workspace
requested 680 millicores, while each eligible worker had less free requested CPU.

Image Puller had six cache containers requesting 50m each: 300m per CPU worker.
Its live CheCluster had `images: ""` but no `cachingCPURequest`. The repository's
CheCluster manifest already sets 10m, but the guide's update command omitted it.
The lab checkout was on `main` at `c4bae1b`, not the PR branch.

Applied the guide's corrected CheCluster patch, explicitly setting
`cachingCPURequest: "10m"` while retaining the operator-managed standard catalogue.
The operator recreated the cache DaemonSet with six 10m containers, freeing 240m
per CPU worker. The existing workspace immediately scheduled; no VM was added,
workspace CPU reservation reduced, or pod security changed.

The first attempt subsequently failed with a postStart command timeout. Restarted
the same DevWorkspace, retaining its PVC. Bootstrap then installed gitpop, and
the workspace reached `Running` with `Ready=True`; its pod was Ready. Red Hat
UDI `latest`, OpenCode and the test-browser images all pulled successfully.
OpenCode reported 1.18.14. CLI checks allowed PipelineRuns in `card-scout-build`
and Deployments in `card-scout-dev`.

This lab still uses the older `workshop-developer` CLI ServiceAccount from main.
This run does not validate the PR's native `dev` token migration or constitute
a full application-generation/delivery E2E. No existing user data was removed.

The guide now applies the CPU reservation in its cache-update command as well
as in the initial manifest. The five platform-services shell blocks passed
syntax checks and the Antora build passed.
