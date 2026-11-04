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

DECISION_STATE = "Inspect the attached image and classify the card and its official printed rarity. Judge only from visible evidence."

QUESTIONS = {'card_kind': {'type': 'choice',
               'instructions': 'Is this a Pokemon Trading Card Game card: yes or no? Pokemon, Trainer and '
                               'Energy cards all count as yes. Select pokemon_tcg for yes, otherwise '
                               'other_card or not_a_card. Judge the printed trading-card layout; a Pokemon '
                               'illustration alone is not a card.',
               'criteria': {'pokemon_tcg': 'A Pokemon Trading Card Game card',
                            'other_card': 'A card from another game, a sports card or a standard playing '
                                          'card',
                            'not_a_card': 'A receipt, photograph or object that is not a trading or playing '
                                          'card',
                            'uncertain': 'Not enough information to identify'}},
 'rarity_symbol': {'type': 'choice',
                   'instructions': 'Inspect the small printed rarity mark in the bottom-right corner, beside '
                                   'the collector number. Report the SHAPE of this mark only. Ignore the '
                                   'character, its fame, the artwork, the large Energy symbols, and the 1st '
                                   'Edition stamp. A blank but fully visible corner is none, as on Basic '
                                   'Energy cards. A cropped, blurry or unreadable corner is uncertain.',
                   'criteria': {'circle': 'A small filled circle or round dot',
                                'diamond': 'A small diamond, a square rotated 45 degrees',
                                'star': 'A small five-pointed star',
                                'none': 'The corner is fully visible and has no printed rarity mark',
                                'uncertain': 'The corner or small rarity mark cannot be read reliably'}}}

# This mapping belongs to the fixed English Base Set, not arbitrary promo cards.
RARITY_BY_SYMBOL = {"circle": "common", "diamond": "uncommon", "star": "rare_or_higher",
                    "none": "common", "uncertain": "uncertain"}


def validate_answer(reply):
    """Validate native shape evidence, then apply Base Set rarity/eligibility rules."""
    answers = reply.get("answers", {})
    expected = {"card_kind": {"pokemon_tcg", "other_card", "not_a_card", "uncertain"},
                "rarity_symbol": set(RARITY_BY_SYMBOL)}
    for name, options in expected.items():
        answer = answers.get(name, {})
        probabilities = answer.get("probabilities", {})
        if answer.get("choice") not in options or set(probabilities) != options:
            raise ValueError("Decision response has an invalid choice or missing probabilities")
        if any(not isinstance(p, (int, float)) or not 0 <= p <= 1 for p in probabilities.values()) or abs(sum(probabilities.values()) - 1) > 0.02:
            raise ValueError("Invalid decision probabilities")
    kind, symbol = answers["card_kind"], answers["rarity_symbol"]
    p = symbol["probabilities"]
    rarity = {"choice": RARITY_BY_SYMBOL[symbol["choice"]],
              "probabilities": {"common": p["circle"] + p["none"], "uncommon": p["diamond"],
                                "rare_or_higher": p["star"], "uncertain": p["uncertain"]}}
    if kind["choice"] != "pokemon_tcg":
        # Applicability is a gate, not a fabricated model confidence of 100%.
        rarity = {"choice": "not_applicable", "probabilities": {}}
    return {"card_kind": kind, "rarity_symbol": symbol, "rarity": rarity}



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
        # No per-item ID, URL, name, print identity or reference answer enters the prompt.
        payload = {
            "model": "vllm-sr/d3-mini",
            "state": DECISION_STATE,
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
        answers = validate_answer(reply)
        results.append({"id": item["id"], "elapsed_seconds": round(time.monotonic() - started, 3), "answers": answers, "raw_answers": reply["answers"], "usage": reply.get("usage")})
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
    scorable = [r for r in pokemon if references[r["id"]]["rarity_bucket"] in valid_rarities]
    binary_ok = lambda r: r["answers"]["rarity"]["choice"] in valid_rarities and ((r["answers"]["rarity"]["choice"] == "rare_or_higher") == (references[r["id"]]["rarity_bucket"] == "rare_or_higher"))
    summary = {
        "images": len(results), "pokemon_cards": len(pokemon),
        "card_kind_correct": sum(r["answers"]["card_kind"]["choice"] == references[r["id"]]["card_kind"] for r in results),
        "binary_rarity_correct": sum(binary_ok(r) for r in scorable),
        "rarity_evaluable_cards": len(scorable),
        "nonstandard_rarity_cards": len(pokemon) - len(scorable),
        "three_tier_rarity_correct": sum(r["answers"]["rarity"]["choice"] == references[r["id"]]["rarity_bucket"] for r in scorable),
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
