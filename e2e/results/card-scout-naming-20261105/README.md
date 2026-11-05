# Card Scout naming — 5 November 2026

The current ARO workshop now uses Card Scout for the application and `card-scout`
for its delivery resources. The active template branch is `aro-card-scout`, commit
`1ba31ecb3b6773346f79b000215d814d59724ad9`. Its browser/page title is Card Scout;
the contract requires the subtitle “Scan Pokémon cards. Discover rarity. Check
prices.” The template still contains no completed application to copy.

New deployments use `card-scout-build`, `card-scout-dev`, matching service
accounts, configuration, secrets, image paths, runtime subscription, NetworkPolicy
selectors and MLflow workspace. Card Scout owns its access and agent manifests
under `deploy/card-scout`; its setup no longer imports the archived news helper.
Older prototypes and their historical evidence retain their original names.

Validation passed: 47 workshop helper/manifest tests, 14 prepared-platform tests,
18 template security/image-contract tests, seven Kustomize renderings and Bash
syntax checks for 49 guide shell blocks. The runtime setup test compares generated
configuration, credentials and API requests with the real identity, subscription
and Deployment mounts. Antora builds and Chromium checks of eight guide pages,
images and the old lab URL redirect passed. Chromium used `--use-mock-keychain`.

These are local/fixture and documentation checks, not fresh live inference or
cluster deployment tests. No existing cluster resources or stored card results
were migrated. Existing labs should finish with their original checkout; this
naming applies to a new platform/developer setup. The renamed Playground
screenshots remain unmodified real captures from an older project, as stated in
the guide. The ROSA template on `main` is unchanged.
