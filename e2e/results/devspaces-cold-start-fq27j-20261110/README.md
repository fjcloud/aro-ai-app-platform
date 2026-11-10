# Root cause: first Dev Spaces start fails with a misleading hook timeout

Investigated and reproduced on fq27j, 10 November 2026. The working participant
workspace was left running; reproduction used a separate diagnostic Pod and fresh
emptyDir volumes, never the participant's PVC. The diagnostic Pod was removed.

## Cause and evidence

The platform mounted read-only OpenCode ConfigMap files below
`/home/user/.config/opencode`. When kubelet prepared these subPath mounts on a
fresh home, it created `.config` and `opencode` owned by root with mode **2755**.
The workspace UID belonged to the directory's group, but group write was absent.

The Red Hat UDI entrypoint runs `mkdir -p "$HOME/.config/containers"`. With this
mount layout it exited **1** with:

```text
mkdir: cannot create directory '/home/user/.config/containers': Permission denied
```

On the original failed workspace, CRI-O recorded the main process exiting 1
roughly 50 ms after container start. The concurrent postStart hook exited 137;
DWO presented that as SIGKILL "due to timeout". A slow gitpop download or a short
postStart timeout was not established as the cause.

The restarted participant workspace had group-write permissions on `.config`.
Reusing its initialized PVC therefore hid the fresh-home failure. Its successful
restart alone did not establish a fix.

## Controlled comparison

All cases used the same Red Hat UDI digest, workspace UID/security context and
persistent-home initialization command. Each case started with a new home.
No model secrets or participant files were mounted into the diagnostic Pod.

| Case | Result |
| --- | --- |
| UDI with fresh home, without the OpenCode mount | First start succeeds |
| UDI with the existing ConfigMap mount below `~/.config` | First start exits 1 with the permission error above |
| ConfigMap moved to `/etc/workshop-opencode`, first new home | First start succeeds |
| Same fix, second independent new home | First start succeeds |

Both fixed runs also used the OpenCode injector and the workshop bootstrap hook
(with editor startup omitted in the isolated probe). gitpop installed, the home
was writable, and `opencode debug config` loaded **deepseek/deepseekv4pro** from
`OPENCODE_CONFIG=/etc/workshop-opencode/opencode.json`. The working directory had
no project configuration, so the mounted platform file was actually exercised.
Dummy model environment values were used; no inference was claimed for this test.

## Fix and scope

The read-only ConfigMap now mounts outside the writable home. `OPENCODE_CONFIG`
selects the file; writable XDG data, state and cache remain on the home volume.
Lab 2.1 copies the platform configuration from that explicit path. Two regression
tests guard the mount location and configuration selection; all **53** local
Python tests and the Antora build pass.

The fix is in the PR for future platform preparation. The global ConfigMap on
fq27j was deliberately left unchanged to avoid restarting the active participant
workspace. Applying the updated platform manifests is required before creating
new workspaces with the fix. These were isolated fresh-home startup tests, not
a new full workshop generation/build/delivery E2E.

The separate missing-user-token issue was already resolved using the dashboard's
native **Advanced → Refresh Kubeconfig** action; see the fq27j identity report.

## Reference

[OpenCode v1.18.14 configuration loader](https://github.com/anomalyco/opencode/blob/v1.18.14/packages/opencode/src/config/config.ts)
selects an explicit configuration file through `OPENCODE_CONFIG`.
