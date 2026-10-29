# Workshop news snapshot

`techmeme.json` contains 161 primary Techmeme stories dated 26–28 October 2026, collected on 29 October. Source links, dated snapshots and the collection time are included. It stores headlines and short excerpts, without model scores, research or prices. All detected primary IDs were recovered; publisher URLs are deduplicated.

The news cutoff is 29 October 2026 at 00:05 America/New_York, including the midnight archive snapshot. Paper entry is the first US market session open after that cutoff. Prices are fetched at comparison time and show their actual quote timestamp, including delayed or intraday quotes. The news remains fixed across sessions.

The application embeds this JSON. Loading the workshop never needs to contact Techmeme. Keep this directory when copying the Go reference.

Maintainers can collect a new snapshot of the completed days of the current week on an allowed network:

```sh
cd examples/briefing-desk
set -o pipefail
go run . export-news | jq . > /tmp/techmeme-news.json
cp /tmp/techmeme-news.json news/techmeme.json
go test -race ./...
```

Commit the dated source data with the matching workshop instructions. `NEWS_EXPORT_CACHE` optionally preserves downloaded archive pages between collection attempts. Original publisher links are retained for attribution; full publisher articles are not included.
