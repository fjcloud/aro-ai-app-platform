# English Base Set / complete card images — 4 November 2026

Card Scout now uses all **102 English Base Set cards (1999)** with **1st Edition
source scans**. The former multi-era collection and negative controls are absent.
The 69 Pokémon, 26 Trainers and seven Energy cards cover collector numbers 1–102.
All 102 scans were visually checked for the stamp; source bytes and SHA-256
checksums are bundled unchanged. There are 38 Common, 32 Uncommon and 32 Rare
catalogue references. See `dataset.json` for provenance and checks.

- [Live application](https://signal-desk-signal-desk-dev.apps.ijtxiwk6.eastus.aroapp.io)
- [Current app contract](https://github.com/fjcloud/go-app-template/blob/aro-signal-desk/APP_CONTRACT.md)

## Application

The contract requires the entire card: borders, name, edition stamp, collector
number and rarity mark. The generated application's fixed-height `cover` images
were replaced with natural-height, portrait images on desktop and mobile.
Browser acceptance now checks the actual image/frame proportions and rejects
`object-fit: cover`. The source's price-scope note is displayed next to quotes.
Catalogue aggregate prices are **not verified 1st Edition valuations**.

The Go app remains a public UI and fixed proxy. Models, keys, dataset, state,
MCP and tracing stay on the prepared service. No SCC, privileges or app permissions
were added. The app's Go tests passed with race detection; vet and production
build passed. Git, Tekton and Argo CD delivered the matching committed source;
the deployed image digest is recorded in `delivery.json`.

## Live model benchmark

| Measure | Observed |
|---|---:|
| Images analyzed / request failures | 102 / 0 |
| Card-kind decisions correct | 101 / 102 |
| Common / uncommon / rare decisions correct | 75 / 102 |
| Rare cards detected | 31 / 32 |
| Non-rare cards classified Rare | 0 |
| Uncertain rarity decisions | 23 |

The remaining four errors are distinct from the 23 abstentions. Grass Energy
was incorrectly classified as another card game and appears under Other. A
complete Pokémon-only source set does not guarantee perfect model output.
Predictions remain visible; catalogue labels are not used to overwrite them.
The rank is predicted rarity, then rare-option probability, not price.

D3 received only image bytes and shared classification instructions. Per-item reference
names, numbers and rarity labels were held out of its requests. The shared
instructions provide Base Set Energy rarity rules to every image. First Edition
is a print run, not a rarity tier. Basic Energy has no printed rarity mark and
may be uncertain. These results concern clean source scans, not phone photos.
The original conservative decision criteria were retained after a more verbose
prototype increased false-positive rare predictions.

Dataset, question, state-instruction or model changes invalidate the analysis
cache. New opaque IDs prevent old browser caches displaying multi-era images.
The previous snapshot's state was backed up before replacement.

## Evidence

- `dataset.json`: complete set, source, category/rarity counts and stamp inspection.
- `analysis-summary.json`, `live-batch.json`: actual model choices, probabilities,
  errors and trace link; no simulated classifications.
- `app-live.json`, `app-fixtures.json`: live browser and isolated failure/empty/
  escaping/unsafe-link/zero/null checks, recorded separately.
- `app-desktop.png`, `app-mobile.png`: complete-card rendering.
- `delivery.json`: source revision, successful pipeline, Argo state and registry/
  running image comparison.

The prepared image was published by CI and tested without model calls before
its digest was pinned. Local checks passed: 46 helper tests, 10 platform tests
and Antora build. Browser tests use `--use-mock-keychain`. Lab URLs and model
keys expire; MaaS and scoped developer identities last 24 hours.
