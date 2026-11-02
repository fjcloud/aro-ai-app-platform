# Dataset attribution

Card metadata and catalogue images are obtained from [TCGdex](https://www.tcgdex.net/).
The [TCGdex cards-database project](https://github.com/tcgdex/cards-database) is
MIT-licensed. That software licence does not relicense Pokémon card artwork,
trademarks or marketplace data. Pokémon card artwork belongs to its respective
rights holders. The manifest retains source URLs; image bytes are cached locally
for reproducible workshop evaluation, not committed as a new licensed art dataset.

Pikachu Illustrator uses a [Bulbagarden Archives card scan](https://archives.bulbagarden.net/media/upload/5/5f/Pok%C3%A9monIllustratorCoroCoropromo.jpg),
with [Bulbapedia identity](https://bulbapedia.bulbagarden.net/wiki/Pokemon_Illustrator)
and [PSA historical context](https://www.psacard.com/articles/articleview/9248/eye-prize-collecting-pokemon-trophy-cards).
This Japanese contest promo is not represented as an English TCGdex listing.
Its artwork retains the rights of its owners. No auction price is embedded.

The three negative-control image sources are:

- [YGOPRODeck: Blue-Eyes White Dragon](https://images.ygoprodeck.com/images/cards/89631139.jpg).
- [PokeAPI sprites: Pikachu official artwork](https://github.com/PokeAPI/sprites/blob/master/sprites/pokemon/other/official-artwork/25.png).
- [vLLM Semantic Router D3 example receipt](https://huggingface.co/vllm-sr/d3-mini/blob/df99bf4ca4418bc6908ee68e9b7479af1f3b8800/assets/example-receipt.png).

Market values are fetched on demand. Cardmarket and TCGplayer retain their own
terms and data rights; the tool preserves provider names and update timestamps.
