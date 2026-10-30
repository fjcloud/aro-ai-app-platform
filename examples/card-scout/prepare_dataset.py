#!/usr/bin/env python3
"""Freeze a small, mixed-era TCGdex image dataset; no model or API key needed."""
import concurrent.futures
import datetime
import hashlib
import json
import pathlib
import random
import time
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parent
CACHE = ROOT / ".cache"
API = "https://api.tcgdex.net/v2/en"
# Two common, two uncommon and two rare-or-higher cards from each era.
SELECTION = {
    "base1": ["46", "63", "32", "27", "73", "15"],
    "ex1": ["55", "73", "47", "37", "16", "99"],
    "dp1": ["87", "100", "64", "59", "24", "6"],
    "bw1": ["36", "50", "90", "17", "62", "57"],
    "xy1": ["58", "98", "74", "8", "21", "93"],
    "sm1": ["23", "57", "77", "95", "41", "15"],
    "swsh1": ["60", "38", "93", "52", "78", "115"],
    "sv01": ["063", "052", "026", "129", "190", "201"],
}
CONTROLS = [
    ("other_card", "Yu-Gi-Oh! — Blue-Eyes White Dragon",
     "https://images.ygoprodeck.com/images/cards/89631139.jpg"),
    ("not_a_card", "Pikachu illustration without a card",
     "https://raw.githubusercontent.com/PokeAPI/sprites/master/sprites/pokemon/other/official-artwork/25.png"),
    ("not_a_card", "Store receipt",
     "https://huggingface.co/vllm-sr/d3-mini/resolve/df99bf4ca4418bc6908ee68e9b7479af1f3b8800/assets/example-receipt.png"),
]


def fetch(url):
    for attempt in range(3):
        try:
            request = urllib.request.Request(url, headers={"User-Agent": "CardScoutWorkshop/1.0"})
            with urllib.request.urlopen(request, timeout=25) as response:
                return response.read()
        except Exception:
            if attempt == 2:
                raise
            time.sleep(attempt + 1)


def rarity_bucket(rarity):
    value = rarity.lower()
    if value in ("common", "uncommon"):
        return value
    if "rare" in value:
        return "rare_or_higher"
    raise ValueError(f"Unmapped rarity: {rarity!r}")


def image_record(url):
    image = fetch(url)
    digest = hashlib.sha256(image).hexdigest()
    (CACHE / digest).write_bytes(image)
    return {"image_url": url, "image_sha256": digest}


def prepare_card(selection):
    set_id, number, set_info = selection
    card_id = f"{set_id}-{number}"
    url = f"{API}/cards/{card_id}"
    card = json.loads(fetch(url))
    if card["id"] != card_id or card["set"]["id"] != set_id:
        raise ValueError(f"Unexpected catalogue identity for {card_id}")
    item = image_record(card["image"] + "/high.png")
    reference = {
        "card_kind": "pokemon_tcg", "card_id": card_id,
        "name": card["name"], "number": card["localId"],
        "set_id": set_id, "set_name": set_info["name"],
        "release_date": set_info["releaseDate"],
        "rarity": card["rarity"], "rarity_bucket": rarity_bucket(card["rarity"]),
        "source_url": url,
    }
    return item, reference


def main():
    CACHE.mkdir(exist_ok=True)
    data_dir = ROOT / "data"
    data_dir.mkdir(exist_ok=True)
    if (data_dir / "inputs.json").exists():
        raise SystemExit("Dataset already frozen. Remove data/*.json explicitly to create a new snapshot.")
    created = datetime.datetime.now(datetime.timezone.utc).isoformat()
    set_info = {sid: json.loads(fetch(f"{API}/sets/{sid}")) for sid in SELECTION}
    selections = [(sid, number, set_info[sid]) for sid, numbers in SELECTION.items() for number in numbers]
    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
        records = list(pool.map(prepare_card, selections))
    for kind, name, url in CONTROLS:
        records.append((image_record(url), {
            "card_kind": kind, "card_id": None, "name": name,
            "rarity": None, "rarity_bucket": "not_applicable", "source_url": url,
        }))
    random.Random(20261030).shuffle(records)
    inputs, references = [], []
    for i, (item, reference) in enumerate(records, start=1):
        item_id = f"item-{i:03d}"
        inputs.append({"id": item_id, **item})
        references.append({"id": item_id, **reference})
    envelope = {"schema_version": 1, "snapshot_at": created, "shuffle_seed": 20261030}
    (data_dir / "inputs.json").write_text(json.dumps({**envelope, "items": inputs}, indent=2) + "\n")
    (data_dir / "references.json").write_text(json.dumps({**envelope, "items": references}, indent=2) + "\n")
    print(json.dumps({"images": len(inputs), "pokemon_cards": len(selections), "eras": len(SELECTION)}))


if __name__ == "__main__":
    main()
