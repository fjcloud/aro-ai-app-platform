# Card Scout

Prepared ARO platform: classify the complete **English Base Set (1999)**, rank
likely rare cards, then ask the native OpenShift AI agent to verify a catalogue
reference and retrieve dated market data. The developer generates a small Go UI
[from its contract](https://github.com/fjcloud/go-app-template/blob/aro-signal-desk/APP_CONTRACT.md).
No finished application source is supplied here to copy.

## Dataset

All **102 cards**, numbered 1/102 through 102/102, using original **1st Edition
scans** from TCGdex. No modern cards, Japanese promos or negative controls.

| Category | Cards |
|---|---:|
| Pokémon | 69 |
| Trainer | 26 |
| Energy | 7 |

The rarity reference contains **38 Common, 32 Uncommon and 32 Rare** cards.
A circle means Common, a diamond Uncommon and a star Rare. Basic Energy cards
are Common even without a printed rarity symbol. The 1st Edition stamp is a
print run, not a rarity tier. A complete set is not a booster distribution.

- `data/inputs.json`: opaque item IDs, source URLs and SHA-256 checksums.
- `data/references.json`: held-out names, collector numbers, categories and rarity.
- `data/images/`: all exact source images, bundled for offline startup/builds.
- `.cache/`: local working image cache, excluded from Git.

New IDs `item-101` through `item-202` avoid stale browser images from the old
multi-era snapshot. They are internal identifiers, not the printed card numbers.
A snapshot change invalidates persisted classifications and price lookups.

D3 receives image bytes only, with the same questions for every image. Names,
URLs, set IDs and reference labels are not sent as answers. The price agent
uses the known catalogue reference; this does not authenticate arbitrary uploads
or assess physical condition.

## Run and evaluate

```bash
oc port-forward -n decision-serving service/d3-mini 18094:8080
```

In a second terminal:

```bash
python3 examples/card-scout/prepare_assets.py
python3 examples/card-scout/analyze_batch.py --output /tmp/card-scout-analysis.json
jq '.summary, .ranking[:5]' /tmp/card-scout-analysis.json
```

`prepare_dataset.py` is a maintainer tool. It refuses to overwrite a snapshot
without `--replace`. Images are checksummed, and reference labels are read only
after inference for evaluation. The ranking uses predicted rarity, then the
rare-option probability. It is not a price ranking or a calibrated guarantee.

## Prepared API and agent

| Route | Purpose |
|---|---|
| `GET /api/batch` | Dataset metadata, ranked items, progress and saved research |
| `POST /api/batch/analyze` | Start/resume incomplete decisions |
| `GET /assets/item-NNN` | Fixed source image |
| `POST /api/cards/item-NNN/research` | Native agent + verified MCP evidence |
| `/mcp` | Read-only Streamable HTTP tool |

`market_tools.py` exposes `get_card_market_data(item_id)`. It accepts only known
batch IDs and fetches the corresponding TCGdex record. Native OGX Responses
calls the tool and summarizes it; structured prices come from tool evidence,
never model-generated numbers. Failed/missing prices stay unavailable.

The source's aggregate quote is **not a verified 1st Edition valuation**. The
scan's stamp does not establish the edition of that quote. The tool and agent
retain this limitation, provider dates, currency and source. Do not substitute
an Unlimited or Shadowless quote and label it 1st Edition.

The generated Go UI calls only `CARD_SCOUT_URL`; model keys, state, MCP and
MLflow remain on the prepared platform. MLflow records per-image probabilities,
agent requests, tool evidence and the visible answer without keys or image bytes.

`deploy/card-scout` pins the platform image and supplies the service, HTTPS MCP
Route and persistent volume. The setup helper supplies dynamic MaaS/MLflow values.

## Sources

- [TCGdex Base Set catalogue](https://api.tcgdex.net/v2/en/sets/base1)
- [TCGdex database](https://github.com/tcgdex/cards-database)
- [Pricing documentation](https://tcgdex.dev/markets-prices)
- Each item retains its exact API/image URL and checksum.
- [Attribution](NOTICE.md)
