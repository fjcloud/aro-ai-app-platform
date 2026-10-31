# Card Scout validation — 30 October 2026

The prepared service and dataset were deployed and validated on 30 October 2026.
The lab no longer resolves on 31 October; its recorded URLs are historical.
OpenCode with
DeepSeek generated the Go application from the contract; it did not copy the
existing finance application. The current browser/live checks are in
`app-live.json`, with isolated edge-case fixtures in `app-fixtures.json`.
`generated-app.json` records the real coding model, generated commits, successful
Tekton/Argo delivery and matching running image digest.

The generated source is preserved on the
[validation branch](https://github.com/fjcloud/go-app-template/tree/generated/card-scout-e2e),
separate from the participant starter. After the lab expired, a maintainer fixed
the hidden-control CSS rule, exact item-ID digit counts and a redirect-sensitive
test. Go race/vet/build and `app-fixtures-local.json` verify that final source
locally; its redeployment awaits a new lab.

The original feasibility probes below are retained separately from deployment
acceptance. The guide now uses Card Scout.

## Mixed-era image analysis

Real `decision-serving/d3-mini` InferenceService on the existing single T4.
The same two questions were used for every image. Only image bytes entered the
model request; catalogue names, IDs, labels and image URLs were withheld.

| Check | Observed result |
|---|---:|
| Images | 51 |
| Pokémon cards | 48 |
| Non-Pokémon controls | 3 |
| Exact card-kind decisions | 51/51 |
| Rare/non-rare decisions | 45/48 |
| Common/uncommon/rare-or-higher decisions | 40/48 |
| Sum of serial request times | 73.62 s |
| Mean request time, including port-forward transport | 1.443 s |

Binary rarity had two incorrect decisions and one abstention (`uncertain`).
An uncommon Farfetch'd was called rare; a rare Professor's Research was called
common. Lapras ex was uncertain. This small, balanced set of clean catalogue
images is a smoke test, not evidence of accuracy on phone photographs or
arbitrary collections. Probabilities were used as returned, without calibration.

`analysis.json` records predictions, model probabilities, evaluation, all errors
and a ranking computed from predictions only. The hidden reference labels do not
determine the ranking.

## Native OpenShift AI price agent

DeepSeek-V4-Pro through the existing OGX Responses API called the read-only
`get_card_market_data` MCP tool and returned a completed answer. The test MCP
server ran temporarily on loopback in the OGX pod, using its installed MCP SDK.
No platform configuration, image, Deployment or model was changed.

- `native-agent-pricing.json`: completed forced-tool native Responses call.
- `agent-toedscool.json`: current `probe_agent.py` result, matching tool evidence
  and display values extracted deterministically from that evidence.
- `agent-negative-control.json`: Yu-Gi-Oh! item correctly produced no Pokémon
  identity or price.

For `item-039`, the catalogue identified **Toedscool, Scarlet & Violet, #201,
Illustration rare**. The returned Cardmarket EUR trend was **4.09** and the 7-day
average **4.26**, with the provider's full update timestamp and exact API source
retained. These are dated aggregate market metrics, not an offer or estimated
condition-specific sale price.

An exploratory structured-JSON-only response skipped the tool and invented a
different card. The working probe therefore forces the named MCP tool and rejects
answers without a matching successful call. Checks also rejected a different
item ID and an incomplete response. UI numeric fields come from tool evidence;
model prose is not parsed for prices.

The feasibility server and port forwards were removed after testing. The
permanent `card-scout-platform` service now provides the batch API and MCP tool
under restricted-v2, with separate persistent state. Its actual native research
response is in `platform-research.json`. `persisted-trace.json` and the graph
screenshot verify a single parent/child trace: `card.research` → `agent.responses`
→ `tool.get_card_market_data`, including the actual tool inputs and output.

The first deployed batch exposed a stale NetworkPolicy selector for the earlier
application. It was corrected to match the actual prepared-service Deployment,
and a regression test now checks this relationship. Retrying incomplete items
recovered the full batch without replacing successful results.

`playground-current.json` records an actual native UI test with one active MCP
tool and no extra connector token. The guide screenshots were captured from that
run. Browser tests use Chromium with `--use-mock-keychain`.

## Dataset integrity

All 51 image hashes were checked. The input/reference ID sets match; the Pokémon
sample has 16 common, 16 uncommon and 16 rare-or-higher cards. Eight sets contribute
six cards each, covering 1999, 2003, 2007, 2011, 2014, 2017, 2020 and 2023.

See [the dataset README](../../../examples/card-scout/README.md) and
[TCGdex market documentation](https://tcgdex.dev/markets-prices).
