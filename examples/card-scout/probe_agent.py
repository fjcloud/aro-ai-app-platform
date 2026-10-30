#!/usr/bin/env python3
"""Probe native OGX + MCP; extract displayed market data from tool evidence."""
import argparse
import json
import pathlib
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parent


def market_evidence(response, item_id):
    if response.get("status") != "completed":
        raise ValueError("Agent did not complete")
    calls = [r for r in response.get("output", []) if r.get("type") == "mcp_call" and r.get("server_label") == "card_scout_market" and r.get("name") == "get_card_market_data"]
    if len(calls) != 1:
        raise ValueError("Expected one verified market tool call; refusing an unsupported answer")
    call = calls[0]
    if call.get("error") or json.loads(call["arguments"]) != {"item_id": item_id}:
        raise ValueError("Market lookup failed or used a different item")
    evidence = json.loads(call["output"])
    if evidence.get("item_id") != item_id or evidence.get("status") not in ("ok", "not_a_pokemon_card"):
        raise ValueError("Market data is unavailable or does not match the selected item")
    if evidence["status"] == "ok" and evidence.get("source_url") != "https://api.tcgdex.net/v2/en/cards/" + evidence.get("card_id", ""):
        raise ValueError("Unexpected market evidence source")
    return evidence


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("item_id")
    parser.add_argument("--url", default="http://127.0.0.1:18321/v1")
    parser.add_argument("--mcp-url", required=True, help="MCP address reachable from OGX")
    parser.add_argument("--model", default="maas-vllm-inference-1/deepseekv4pro")
    parser.add_argument("--key-file", type=pathlib.Path, required=True)
    parser.add_argument("--prediction", choices=("common", "uncommon", "rare_or_higher", "uncertain"), default="uncertain")
    parser.add_argument("--output", type=pathlib.Path, default=ROOT / ".cache" / "agent-pricing.json")
    args = parser.parse_args()
    key = args.key_file.read_text().strip()
    if not key:
        raise SystemExit("Model key file is empty")
    payload = {
        "model": args.model,
        "input": f"Check {args.item_id} from our batch. D3 predicted {args.prediction}. What is this card and what online market reference prices are available?",
        "instructions": (ROOT / "agent-instructions.txt").read_text(),
        "tools": [{"type": "mcp", "server_label": "card_scout_market", "server_url": args.mcp_url, "require_approval": "never", "allowed_tools": ["get_card_market_data"]}],
        "tool_choice": {"type": "mcp", "server_label": "card_scout_market", "name": "get_card_market_data"},
        "reasoning": {"effort": "none"}, "max_output_tokens": 1000,
        "max_infer_iters": 4, "parallel_tool_calls": False, "store": False, "stream": False,
    }
    request = urllib.request.Request(args.url.rstrip("/") + "/responses", data=json.dumps(payload).encode(), headers={"Content-Type": "application/json", "X-OGX-Provider-Data": json.dumps({"vllm_api_token": key})})
    with urllib.request.urlopen(request, timeout=120) as reply:
        raw = reply.read().decode()
    if key in raw:
        raise SystemExit("Unexpected credential in response; refusing to store it")
    response = json.loads(raw)
    evidence = market_evidence(response, args.item_id)
    # Numeric UI fields must come from evidence, never parsed/generated prose.
    provider = (evidence.get("pricing") or {}).get("cardmarket") or {}
    euro = provider.get("unit") == "EUR"
    display = {
        "item_id": args.item_id, "status": evidence["status"],
        "card": evidence.get("card"), "currency": "EUR" if euro else None,
        "cardmarket_trend": provider.get("trend") if euro else None,
        "cardmarket_average_7d": provider.get("avg7") if euro else None,
        "provider_updated_at": provider.get("updated") if euro else None,
        "source_url": evidence.get("source_url"),
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps({"display": display, "evidence": evidence, "response": response}, indent=2) + "\n")
    print(json.dumps(display, indent=2))


if __name__ == "__main__":
    main()
