# PR 32 follow-up review — 10 November 2026

Reviewed commit `5c0ed65`: parallel CPU/GPU provisioning, native Dev Spaces
user authentication, retirement of the old CLI credential injection, standard
image caching, namespace permissions and the workspace factory URL.

## Corrections

- Lab 2.1 now uses `registry.redhat.io/devspaces/udi-rhel9:latest` in the `image=`
  parameter. The SHA-256 digest and image-catalogue discovery are removed from
  the launch command. The template's test-browser component is retained.
- The short tag is mutable. It may resolve to an image different from the
  operator's cached reference; the guide explains that such an image pulls on
  demand. It no longer promises an exact cache match.
- `e2e/check.py model-apis` previously impersonated the retired
  `workshop-developer` ServiceAccount. That could pass its negative permission
  checks without testing `dev`. It now checks `oc whoami` in the selected
  workspace and runs both permission checks through that workspace's CLI.
  Administrator and legacy ServiceAccount sessions fail before inference.
- Validation documentation now names the native `dev` identity.

## Review outcome

The parallel MachineSet flow submits both workers before waiting and documents
how to reuse existing pools. Developer grants remain namespaced; the preparation
helper retains pipeline/image-pull permissions and removes only the named legacy
CLI resources. Standard image caching remains operator-managed and does not
block workspace launch. No further blocking issue was identified by static
review and local regression checks.

## Validation and limits

- 51 Python regression tests passed, including rejection of administrator/legacy
  workspace identities and an over-permitted native user.
- All executable shell examples passed `bash -n`.
- Executing the actual URL block with a mocked cluster URL produced the expected
  template, OpenCode provider and Red Hat UDI tag, without `sha256`.
- Antora documentation build passed.

The previous lab 87xpp API no longer resolves. Native CLI authentication, migration
of an existing workspace, the changed image and parallel VM provisioning still
require a live run on an active ARO lab. Earlier 87xpp results used the previous
CLI ServiceAccount and do not establish acceptance of these changes.

## Primary references

- [Automatic OpenShift user token injection](https://docs.redhat.com/en/documentation/red_hat_openshift_dev_spaces/3.28/html/user_guide/assembly_integrating-with-openshift_user_guide)
- [Workspace URL image parameter](https://docs.redhat.com/en/documentation/red_hat_openshift_dev_spaces/3.28/html/user_guide/optional-parameters-for-urls_user_guide)
- [Standard image caching](https://docs.redhat.com/en/documentation/red_hat_openshift_dev_spaces/3.28/html/administration_guide/assembly_caching-images-for-faster-workspace-start_administration_guide)
- [Red Hat UDI catalogue and latest tag](https://catalog.redhat.com/en/software/containers/devspaces/udi-rhel9/673f8460bbf0c33aca0fe316)
