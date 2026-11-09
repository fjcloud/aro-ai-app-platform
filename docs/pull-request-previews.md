# Review the workshop before merging

Open the **Rendered workshop preview** link in the pull request's preview comment.
It is a navigable Antora site, including images, source blocks and the normal
Showroom theme. Each page identifies the PR and source commit. Act 0 clones the
review branch, so its proposed configuration is available without merging it.

Same-repository PRs publish to
`https://fjcloud.github.io/workshop-previews/aro/pr-<number>/`.
Updates to the PR replace only that preview folder. The production site and the
workshop's `main` branch are not deployed or changed by the preview workflow.
Fork PRs receive a downloadable `aro-docs-pr-<number>` Actions artifact instead;
the preview publishing credential is never supplied to fork code.

The `DOCS_PREVIEW_DEPLOY_KEY` secret is a deploy key with write access only to the
separate `fjcloud/workshop-previews` repository. It is not a provider credential or
a token with access to the workshop cluster. Previews contain public guide content,
not lab credentials, kubeconfigs or model responses.

For local review:

```sh
npx --yes --package @antora/cli@3.1 --package @antora/site-generator@3.1 antora generate \
  --attribute pr-number=32 --attribute preview-sha=local \
  --attribute workshop-branch=docs/aro-guided-workshop-pr-preview \
  default-site.yml --clean
python3 -m http.server 8443 --bind 127.0.0.1 --directory www
```

Open `http://localhost:8443/`. `make docs` builds the normal workshop without a
review banner. Rendering and syntax checks are not a fresh-cluster E2E test;
the proposed identity flow must be exercised on a fresh lab before merging.
