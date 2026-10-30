#!/usr/bin/env python3
"""Read-only MCP catalogue and current market data for the prepared card batch."""
import datetime
import json
import os
import pathlib
import urllib.request

from mcp.server.fastmcp import Context, FastMCP
from opentelemetry.propagate import extract
from observability import TRACER, record

ROOT = pathlib.Path(__file__).resolve().parent
REFERENCE_FILE = pathlib.Path(os.environ.get("CARD_REFERENCE_FILE", ROOT / "data" / "references.json"))
REFERENCES = {r["id"]: r for r in json.loads(REFERENCE_FILE.read_text())["items"]}
mcp = FastMCP("card-scout-market", host=os.environ.get("MCP_HOST", "0.0.0.0"), port=int(os.environ.get("MCP_PORT", "8080")), stateless_http=True, json_response=True)


def market_lookup(item_id: str) -> dict:
    """Verify a prepared batch item's catalogue identity and get online market prices.

    Only accepts item IDs from the fixed workshop dataset. This is a catalogue
    lookup, not visual identity recognition. Retain provider dates, units and
    variants. Market averages/trends are reference values, not a sale offer or
    authentication/condition assessment. Missing prices remain unavailable.
    """
    reference = REFERENCES.get(item_id)
    if not reference:
        return {"error": "Unknown batch item. Use an item ID from the prepared dataset."}
    if reference["card_kind"] != "pokemon_tcg":
        return {"item_id": item_id, "status": "not_a_pokemon_card", "pricing": None}
    card_id = reference["card_id"]
    url = f"https://api.tcgdex.net/v2/en/cards/{card_id}"
    request = urllib.request.Request(url, headers={"User-Agent": "CardScoutWorkshop/1.0"})
    try:
        with urllib.request.urlopen(request, timeout=20) as response:
            raw = response.read(2 * 1024 * 1024 + 1)
        if len(raw) > 2 * 1024 * 1024:
            raise ValueError("Catalogue response exceeds limit")
        card = json.loads(raw)
        if card.get("id") != card_id:
            raise ValueError("Catalogue returned a different card")
    except Exception as exc:
        return {"item_id": item_id, "status": "source_unavailable", "source_url": url, "pricing": None, "error": str(exc)}
    return {
        "item_id": item_id, "status": "ok", "card_id": card_id,
        "identity_method": "Known catalogue reference from the curated dataset; not inferred from the photo.",
        "card": {"name": card["name"], "number": card["localId"], "set": card["set"]["name"], "set_id": card["set"]["id"], "rarity": card.get("rarity")},
        "source_url": url,
        "retrieved_at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        "pricing": card.get("pricing"),
        "variants": [{k: v for k, v in variant.items() if k in ("type", "size", "stamp", "foil", "variantId", "pricing", "thirdParty")} for variant in card.get("variants_detailed", [])],
        "price_scope": "Provider aggregate reference values. Language, print edition, finish and condition may differ; do not claim a guaranteed resale price. Do not mix currencies or variants.",
    }


@mcp.tool()
def get_card_market_data(item_id: str, ctx: Context) -> dict:
    """Verify a fixed batch item and fetch its dated Cardmarket/TCGplayer prices.

    Uses the dataset's known catalogue ID, not visual edition recognition.
    Preserve provider dates, currencies and variants. Missing prices are unknown.
    """
    request = getattr(ctx.request_context, "request", None)
    carrier = dict(request.headers) if request is not None else {}
    with TRACER.start_as_current_span("tool.get_card_market_data", context=extract(carrier), attributes={"mlflow.spanType": "TOOL"}) as span:
        record(span, "spanInputs", {"item_id": item_id})
        result = market_lookup(item_id)
        record(span, "spanOutputs", result)
        return result


if __name__ == "__main__":
    mcp.run(transport="streamable-http")
