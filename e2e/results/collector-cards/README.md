# Collector-card extension — 2 November 2026

The dataset now contains **57 images: 54 Pokémon cards and three controls**.
The first 51 input and reference records remain unchanged, including IDs and
image hashes. All 57 image hashes were verified. App contract, guide, backend
metadata and acceptance checks use the new collection size.

Five exact English TCGdex records and one Japanese historical promo were added.
The Illustrator scan is bundled with its original source URL and checksum because
Bulbagarden blocks downloads from GitHub build runners. Attribution is retained
in the [dataset notice](../../../examples/card-scout/NOTICE.md).

## Actual T4 decisions

| Item | Card | D3 prediction | Catalogue reference |
|---|---|---|---|
| item-052 | Charizard, Base Set #4 | rare_or_higher | Rare |
| item-053 | Shining Charizard, Neo Destiny #107 | uncertain | Rare |
| item-054 | Umbreon Gold Star, POP Series 5 #17 | common | Rare |
| item-055 | Shining Magikarp, Neo Revelation #66 | rare_or_higher | Rare |
| item-056 | Birthday Pikachu, Wizards Promo #24 | uncertain | Common |
| item-057 | Pikachu Illustrator, Japanese unnumbered contest promo | uncertain | No standard printed rarity |

These are actual model outputs, not labels inserted into the inference prompt.
A collectible's scarcity and price are distinct from its printed rarity.
The classifier misses Umbreon Gold Star's rarity; uncertainty and incorrect
predictions remain visible. The special Illustrator reference is excluded from
standard binary/three-tier rarity accuracy denominators by the batch runner.

## Live acceptance

Deployed to the existing ARO 54w5q workshop, using the same generated Go app
without changing or rebuilding that app. All **57 images completed, zero failed**.
The live browser and isolated edge-case checks passed, including real research,
filters, pagination, reload, mobile layout, exact tool-derived prices and traces.

Additional actual browser requests verified Charizard's dated EUR reference and
Illustrator's curated identity with **Unavailable** prices, source and trace links.
The Illustrator MCP evidence has no pricing; the validator rejects an invented
price for that item. It does not borrow another Pikachu listing or an auction
record. This is a curated identity reference, not a live pricing feed.

The application remains public and restricted-v2, with no mounted model key or
Kubernetes token. Delivery/image checks passed for the unchanged application
revision `585d0c9511896e912c852298903cc5750df9c2f7`. The previous Dev Space had
stopped automatically for inactivity; the optional workspace-exec recheck was
therefore not completed again. The earlier real generation and Go checks are in
the [baseline review](../aro-54w5q/README.md).

Local checks passed: 45 helpers, eight platform tests, append idempotence,
original-record preservation and all image hashes. CI also passed the workshop
regression suites and documentation build. The initial image build failed on a
source HTTP 403; bundling the same Illustrator scan resolved it, and the public
image build and its tests passed.

- `collector-decisions.json`: actual six-card predictions and probabilities.
- `app-live.json`, `app-fixtures.json`: live browser versus isolated test results.
- `item-052-research.json`, `item-054-research.json`, `item-057-research.json`: actual native agent results and trace URLs.
- `illustrator-result.png`: live UI showing the special card without an invented price.
- The platform image is pinned in [the deployment](../../../deploy/card-scout/platform.yaml).
