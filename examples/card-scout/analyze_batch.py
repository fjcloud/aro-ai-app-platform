#!/usr/bin/env python3
"""Send image bytes only to D3, rank decisions, then evaluate against held-out labels."""
import argparse
import base64
import datetime
import hashlib
import json
import pathlib
import time
import urllib.error
import urllib.request

from prepare_dataset import CACHE, ROOT, fetch

QUESTIONS = {
    "card_kind": {
        "type": "choice",
        "instructions": "What does this image show? Pokemon TCG includes Trainer and Energy cards. Identify the actual card game, not just a generic fantasy card.",
        "criteria": {"pokemon_tcg": "A Pokemon Trading Card Game card", "other_card": "A card from another game, a sports card or a standard playing card", "not_a_card": "A receipt, photograph or object that is not a trading or playing card", "uncertain": "Not enough information to identify"},
    },
    "rarity": {
        "type": "choice",
        "instructions": "For a Pokemon TCG card, identify its printed rarity using the rarity mark near the collector number. On classic cards, a circle is Common, a diamond is Uncommon, a star is Rare. Do not confuse character popularity or evolution stage with rarity. For modern cards use their printed rarity symbols. If it is not a Pokemon TCG card select not_applicable. If the rarity is unreadable or cannot be established select uncertain.",
        "criteria": {"common": "Common rarity, classic circle symbol", "uncommon": "Uncommon rarity, classic diamond symbol", "rare_or_higher": "Rare or higher rarity, classic star or a modern rare tier", "not_applicable": "This is not a Pokemon TCG card", "uncertain": "Printed rarity cannot be determined"},
    },
}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", default="http://127.0.0.1:18094")
    parser.add_argument("--output", type=pathlib.Path, default=ROOT / ".cache" / "analysis.json")
    args = parser.parse_args()
    inputs = json.loads((ROOT / "data" / "inputs.json").read_text())
    CACHE.mkdir(exist_ok=True)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    results = []
    for item in inputs["items"]:
        path = CACHE / item["image_sha256"]
        image = path.read_bytes() if path.exists() else fetch(item["image_url"])
        if hashlib.sha256(image).hexdigest() != item["image_sha256"]:
            raise SystemExit(f"Image changed for {item['id']}; refusing a different input.")
        path.write_bytes(image)
        mime = "image/jpeg" if image[:2] == b"\xff\xd8" else "image/png"
        # No ID, URL, filename, catalogue name, edition or reference label enters the prompt.
        payload = {
            "model": "vllm-sr/d3-mini",
            "state": "Inspect the attached image and classify the card and its official printed rarity. Judge only from visible evidence.",
            "images": [f"data:{mime};base64," + base64.b64encode(image).decode()],
            "questions": QUESTIONS,
        }
        started = time.monotonic()
        request = urllib.request.Request(args.url.rstrip("/") + "/v1/systemone", data=json.dumps(payload).encode(), headers={"Content-Type": "application/json"})
        try:
            with urllib.request.urlopen(request, timeout=120) as response:
                reply = json.load(response)
        except urllib.error.HTTPError as exc:
            raise SystemExit(f"{item['id']}: HTTP {exc.code}: {exc.read()[:500]!r}") from exc
        answers = reply["answers"]
        results.append({"id": item["id"], "elapsed_seconds": round(time.monotonic() - started, 3), "answers": answers, "usage": reply.get("usage")})
        # Checkpoint completed model outputs before evaluation or catalogue lookup.
        args.output.write_text(json.dumps({"dataset_snapshot_at": inputs["snapshot_at"], "results": results}, indent=2) + "\n")
        print(json.dumps({"id": item["id"], "kind": answers["card_kind"]["choice"], "rarity": answers["rarity"]["choice"], "seconds": results[-1]["elapsed_seconds"]}), flush=True)

    # Reference data is loaded only AFTER all inference requests have completed.
    references = {r["id"]: r for r in json.loads((ROOT / "data" / "references.json").read_text())["items"]}
    tier = {"rare_or_higher": 3, "uncommon": 2, "common": 1}
    candidates = [r for r in results if r["answers"]["card_kind"]["choice"] == "pokemon_tcg"]
    candidates.sort(key=lambda r: (tier.get(r["answers"]["rarity"]["choice"], 0), r["answers"]["rarity"]["probabilities"].get("rare_or_higher", 0)), reverse=True)
    ranking = [{"rank": i, "id": r["id"], "predicted_rarity": r["answers"]["rarity"]["choice"], "rare_probability": r["answers"]["rarity"]["probabilities"].get("rare_or_higher", 0)} for i, r in enumerate(candidates, start=1)]
    pokemon = [r for r in results if references[r["id"]]["card_kind"] == "pokemon_tcg"]
    valid_rarities = set(tier)
    binary_ok = lambda r: r["answers"]["rarity"]["choice"] in valid_rarities and ((r["answers"]["rarity"]["choice"] == "rare_or_higher") == (references[r["id"]]["rarity_bucket"] == "rare_or_higher"))
    summary = {
        "images": len(results), "pokemon_cards": len(pokemon),
        "card_kind_correct": sum(r["answers"]["card_kind"]["choice"] == references[r["id"]]["card_kind"] for r in results),
        "binary_rarity_correct": sum(binary_ok(r) for r in pokemon),
        "three_tier_rarity_correct": sum(r["answers"]["rarity"]["choice"] == references[r["id"]]["rarity_bucket"] for r in pokemon),
        "mean_seconds": round(sum(r["elapsed_seconds"] for r in results) / len(results), 3),
        "by_era": {},
    }
    for set_id in sorted({references[r["id"]]["set_id"] for r in pokemon}):
        subset = [r for r in pokemon if references[r["id"]]["set_id"] == set_id]
        summary["by_era"][set_id] = {"cards": len(subset), "binary_rarity_correct": sum(binary_ok(r) for r in subset)}
    errors = [{"id": r["id"], "reference": references[r["id"]], "predicted": r["answers"]["rarity"]["choice"]} for r in pokemon if r["answers"]["rarity"]["choice"] != references[r["id"]]["rarity_bucket"]]
    output = {"dataset_snapshot_at": inputs["snapshot_at"], "analyzed_at": datetime.datetime.now(datetime.timezone.utc).isoformat(), "summary": summary, "ranking": ranking, "errors": errors, "results": results}
    args.output.write_text(json.dumps(output, indent=2) + "\n")
    print("SUMMARY", json.dumps(summary), flush=True)


if __name__ == "__main__":
    main()
