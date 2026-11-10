# Dev Spaces and native developer credentials — fq27j

Validated on 10 November 2026.

## Startup

The first workspace failed with:

```text
Container dev has state [postStart hook] Commands forcefully killed by SIGKILL due to timeout
```

Unlike 44fwd, the image-cache CPU setting was already correct. Restarting the
same DevWorkspace retained its PVC and reached `Running`; all three containers
became Ready. Bootstrap installed gitpop and OpenCode reported 1.18.14.
The precise cause of the first cold-start postStart timeout is still unresolved;
this run establishes recovery, not a permanent fix for that timeout.

## Native CLI identity

After the restart, `oc whoami` reported the workspace's own ServiceAccount, and
both application permission checks returned `no`. The existing namespaced roles
already granted the required rights to `dev`; no additional role grant was needed.

Logged into the Dev Spaces dashboard as `dev` and used:

```text
Workspaces → card-scout-workspace → Advanced → Refresh Kubeconfig
```

The native dashboard API returned HTTP 204 and displayed `Kubeconfig refreshed`.
A subsequent CLI check returned:

```text
dev
yes
yes
```

The screenshot in Lab 2.1 captures this actual successful action. The lab now
explicitly synchronizes credentials and reloads the editor before checking CLI
identity. It does not install a shared ServiceAccount kubeconfig or grant the
workspace ServiceAccount application permissions.

## Actual model API preflight

Ran the current `e2e/check.py model-apis` against the real workspace. All checks
passed: native `dev` identity, denied model-secret reads and model mutation, the
private T4 endpoint, Charizard/Basic Energy decisions, actual DeepSeek usage and
a completed native Responses call with the real catalogue/price tool evidence.
The four documented Lab 2.2 blocks produced the expected five JSON responses.

Antora rendering passed. Application generation and delivery were not repeated
in this validation. Credentials and raw model responses remain outside Git.
