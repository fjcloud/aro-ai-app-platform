# Three non-Pokémon controls — 4 November 2026

Card Scout contains 105 images: the unchanged 102 English Base Set cards and:

| Item | Image | Actual D3 kind | Pokémon rarity |
|---|---|---|---|
| item-203 | Standalone Pikachu artwork | not_a_card | not_applicable |
| item-204 | Yu-Gi-Oh! Blue-Eyes White Dragon | other_card | not_applicable |
| item-205 | Magic Black Lotus, Alpha | other_card | not_applicable |

The native T4 probes and fresh deployed 105-image analysis agree. No reference
labels entered inference. The three controls are bundled with source URLs and
checksums, and are excluded from Pokémon rarity and price research. All three
live research POSTs returned 409 before invoking the agent. Model kind and
rarity are preserved; no catalogue-based corrections were applied.

The Other filter contains four images: these three controls plus Grass Energy,
which remains a known model mistake from the existing Base Set benchmark.
All 105 images completed without a request failure. Full browser acceptance,
price/source/trace checks and isolated error fixtures passed. Fourteen platform
tests, 46 helper tests and Antora build passed; image container tests passed.
A repeated controls append added zero items and changed no existing records.

`live-summary.json` records predictions, image digest and scope; `native-probe.json`
contains actual model output; `controls.png` shows the live Other filter.
Browser runs use `--use-mock-keychain`. Sources/attribution remain in the dataset
manifests and `examples/card-scout/NOTICE.md`. No Magic or Yu-Gi-Oh! prices are used.
