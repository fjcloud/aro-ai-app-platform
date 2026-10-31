# Card Scout

Prepared platform for the ARO workshop: analyze a mixed box of cards, sort likely
rare cards first, then ask a native OpenShift AI agent for catalogue verification
and dated market data. The Go application is generated separately from
[its contract](https://github.com/fjcloud/go-app-template/blob/aro-signal-desk/APP_CONTRACT.md);
no completed UI application is supplied here to copy.

## Prepared dataset

The frozen batch contains **48 English Pokémon TCG cards from eight eras**, plus
three negative controls: a Yu-Gi-Oh! card, a receipt and a standalone Pikachu image.

| Set | Released | Cards |
|---|---|---:|
| Base Set | 1999 | 6 |
| Ruby & Sapphire | 2003 | 6 |
| Diamond & Pearl | 2007 | 6 |
| Black & White | 2011 | 6 |
| XY | 2014 | 6 |
| Sun & Moon | 2017 | 6 |
| Sword & Shield | 2020 | 6 |
| Scarlet & Violet | 2023 | 6 |

There are 16 common, 16 uncommon and 16 rare-or-higher cards. This is an
intentionally balanced demo sample, not the distribution in a booster pack.
Inputs are shuffled with a fixed seed. Image hashes freeze the exact inputs.

- `data/inputs.json`: anonymous item IDs, image URLs and SHA-256 checksums.
- `data/references.json`: held-out catalogue IDs, names, dates and rarity labels.
- `.cache/`: downloaded images; excluded from Git.

Inference receives **image bytes only**, with the same questions for every item.
Names, URLs, set IDs and reference labels are not included in the model input.
References are loaded after inference to measure results. The price agent uses
the dataset's known catalogue ID; exact edition recognition from a new uploaded
photograph is outside this experiment.

## Analyze the batch

Use the existing D3 InferenceService. In a second terminal:

```bash
oc port-forward -n decision-serving service/d3-mini 18094:8080
```

Then run:

```bash
python3 examples/card-scout/analyze_batch.py \
  --output examples/card-scout/.cache/analysis.json
jq '.summary, .ranking[:5]' examples/card-scout/.cache/analysis.json
```

The runner downloads missing images and checks their frozen hashes. A prepared
image bundle can populate `.cache/` before the session to avoid external image
downloads. `prepare_dataset.py` is a maintainer tool for creating a new snapshot;
it refuses to overwrite the frozen dataset.

Ranking uses the model's predicted rarity tier, then its rare-option probability.
It is a review priority, not a price ranking. Unknown results remain explicit.
Model probabilities are not calibrated guarantees of correctness.

## Check the price with the native agent

`platform_service.py` serves the fixed dataset, background analysis jobs and
persisted results. Its app-facing API is:

| Route | Purpose |
| --- | --- |
| `GET /api/batch` | Ranked items, progress, errors and cached research |
| `POST /api/batch/analyze` | Start/resume incomplete model decisions |
| `GET /assets/item-NNN` | A fixed, checksummed image |
| `POST /api/cards/item-NNN/research` | Native agent lookup with verified tool evidence |
| `/mcp` | Read-only Streamable HTTP tool |

`market_tools.py` exposes the read-only MCP tool `get_card_market_data(item_id)`.
It verifies the fixed catalogue identity and fetches current TCGdex market data.
It only accepts known batch item IDs and has no trading or purchasing capability.
`agent-instructions.txt` defines the research behavior. Native OGX Responses
performs the tool call and final answer server-side. The MCP service must be
reachable **from OGX**. `probe_agent.py` forces the named tool, checks its evidence
matches the selected item, and builds numeric display fields from the tool data.
A model answer without matching tool evidence is rejected.

For an already running MCP service and an OGX port forward:

```bash
python3 examples/card-scout/probe_agent.py item-039 \
  --mcp-url http://card-scout-platform.signal-desk-dev.svc.cluster.local:8080/mcp \
  --key-file /path/to/your/maas-key \
  --prediction rare_or_higher
```

The MCP URL is reachable from the native OGX backend. The developer calls the
prepared service; its MaaS key and projected MLflow credential stay on the
platform service. The generated app needs neither.

Keep provider currency, metric, update timestamp and source link. Missing prices
remain unavailable. Do not combine normal, reverse-holo, first-edition, graded or
different-language prices into one valuation. Catalogue rarity is distinct from
market value, condition and authenticity.

The minimal application flow is:

```text
Mixed batch → Analyze → Ranked cards → Check price → Agent summary + source
```

The generated Go UI displays cards and proxies this API. The prebuilt Red Hat
UBI Python image includes the source dataset images. `deploy/card-scout` pins its
digest and defines the service, HTTPS MCP Route and persistent analysis volume.
The setup helper handles dynamic MaaS and MLflow values; ordinary resources are
applied with `oc apply -k deploy/card-scout`.

MLflow records batch and per-image decisions, the Responses request and visible
answer, and the actual MCP catalogue/price input and output. Trace context passes
through the native agent to the MCP tool. Keys and image bytes are excluded.
The app receives a direct trace URL for the selected research.

## Sources and validation

- [TCGdex catalogue](https://github.com/tcgdex/cards-database)
- [TCGdex pricing documentation](https://tcgdex.dev/markets-prices)
- Each reference records its exact public API URL and snapshot date.
- [Live T4 and native agent validation](../../e2e/results/card-scout/README.md)
- [Attribution](NOTICE.md)
