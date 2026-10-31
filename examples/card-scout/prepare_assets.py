#!/usr/bin/env python3
"""Populate the immutable image cache while building the platform image."""
import concurrent.futures
import hashlib
import json

from prepare_dataset import CACHE, ROOT, fetch


def prepare(item):
    path = CACHE / item["image_sha256"]
    data = path.read_bytes() if path.exists() else fetch(item["image_url"])
    if hashlib.sha256(data).hexdigest() != item["image_sha256"]:
        raise ValueError("Image changed for " + item["id"])
    path.write_bytes(data)


if __name__ == "__main__":
    CACHE.mkdir(exist_ok=True)
    items = json.loads((ROOT / "data" / "inputs.json").read_text())["items"]
    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
        list(pool.map(prepare, items))
    print(f"Verified {len(items)} fixed dataset images")
