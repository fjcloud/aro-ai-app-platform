#!/usr/bin/env python3
"""Freeze all 102 English Base Set cards with source images and held-out labels."""
import argparse
import concurrent.futures
import datetime
import hashlib
import json
import pathlib
import time
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parent
CACHE = ROOT / '.cache'
API = 'https://api.tcgdex.net/v2/en'
SET_ID = 'base1'


def fetch(url):
    for attempt in range(3):
        try:
            request = urllib.request.Request(url, headers={'User-Agent': 'CardScoutWorkshop/1.0'})
            with urllib.request.urlopen(request, timeout=25) as response:
                return response.read()
        except Exception:
            if attempt == 2:
                raise RuntimeError(f'Could not download dataset source: {url}') from None
            time.sleep(attempt + 1)


def rarity_bucket(rarity):
    value = rarity.lower()
    if value in ('common', 'uncommon'):
        return value
    if 'rare' in value:
        return 'rare_or_higher'
    raise ValueError(f'Unmapped rarity: {rarity!r}')


def prepare_card(selection):
    number, set_info = selection
    card_id = f'{SET_ID}-{number}'
    url = f'{API}/cards/{card_id}'
    card = json.loads(fetch(url))
    if card['id'] != card_id or card['set']['id'] != SET_ID or not card.get('variants', {}).get('firstEdition'):
        raise ValueError(f'Unexpected catalogue identity or missing first-edition variant for {card_id}')
    image_url = card['image'] + '/high.png'
    image = fetch(image_url)
    digest = hashlib.sha256(image).hexdigest()
    (CACHE / digest).write_bytes(image)
    reference = {
        'card_kind': 'pokemon_tcg', 'card_id': card_id,
        'name': card['name'], 'number': card['localId'],
        'set_id': SET_ID, 'set_name': set_info['name'],
        'release_date': set_info['releaseDate'], 'language': 'en',
        'print_edition': '1st Edition', 'category': card['category'],
        'rarity': card['rarity'], 'rarity_bucket': rarity_bucket(card['rarity']),
        'source_url': url,
    }
    return {'image_url': image_url, 'image_sha256': digest}, reference


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--replace', action='store_true', help='Explicitly replace the snapshot; results from another snapshot are invalidated')
    args = parser.parse_args()
    data_dir = ROOT / 'data'
    if (data_dir / 'inputs.json').exists() and not args.replace:
        raise SystemExit('Dataset already frozen. Use --replace to create a new Base Set snapshot.')
    CACHE.mkdir(exist_ok=True)
    data_dir.mkdir(exist_ok=True)
    info = json.loads(fetch(f'{API}/sets/{SET_ID}'))
    numbers = sorted(int(c['localId']) for c in info['cards'])
    if numbers != list(range(1, 103)) or info['cardCount']['official'] != 102:
        raise ValueError('Expected the complete English Base Set, numbered 1 through 102')
    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
        records = list(pool.map(prepare_card, [(n, info) for n in numbers]))
    inputs, references = [], []
    for n, (item, reference) in enumerate(records, start=1):
        # New opaque IDs prevent browsers reusing old, cached multi-era images.
        item_id = f'item-{100 + n:03d}'
        inputs.append({'id': item_id, **item})
        references.append({'id': item_id, **reference})
    envelope = {'schema_version': 1, 'snapshot_at': datetime.datetime.now(datetime.timezone.utc).isoformat(),
                'name': 'Base Set — English 1st Edition', 'years': '1999', 'set_id': SET_ID,
                'language': 'en', 'print_edition': '1st Edition'}
    for name, items in (('inputs', inputs), ('references', references)):
        (data_dir / f'{name}.json').write_text(json.dumps({**envelope, 'items': items}, indent=2) + '\n')
    images = data_dir / 'images'
    images.mkdir(exist_ok=True)
    for item in inputs:
        digest = item['image_sha256']
        (images / digest).write_bytes((CACHE / digest).read_bytes())
    print(json.dumps({'images': len(inputs), 'set': SET_ID, 'language': 'en', 'edition': '1st Edition'}))


if __name__ == '__main__':
    main()
