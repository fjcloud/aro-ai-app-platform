# Signal Desk: weekly paper game

Validated locally on 28 October 2026. A replacement ARO deployment remains untested.

- **Real corpus:** 289 distinct primary Techmeme stories from 19–25 October. One unavailable archive ID (`261021p29`) is disclosed in the UI and JSON export. Headlines/excerpts are archived source material, not full publisher articles or generated full-text summaries.
- **Real scoring:** all 289 ranked against the semiconductor memo with native d3-mini on Apple Metal, FP16. MPS remains unvalidated upstream; this does not establish accelerator parity or investment accuracy.
- **Real research:** Azure DeepSeek-V4-Pro `2026-04-23` uses `read_archive`, `search_archive` and `index_universe` only. Current-web and price tools are unavailable to the replay agent.
- **Real settlement:** a $1,000 SMH test position entered at the 26 October open and valued at the 27 October completed close returned +$50.12. This is one replay outcome, not a future return estimate. The whole fund was $10,050.12; the week was explicitly incomplete. The round was reset afterward; prior results are retained.
- **Browser fixtures:** seven archive days, 21 news, memo edits/re-scoring, two index investments, +$100 and −$50 outcomes, $10,050 final fund, immutable committed choices, full JSON export and mobile layout. Fixture clocks exclude the current incomplete session.
- **Go checks:** archive cutoff and tool restrictions, quote identity, invalid/missing prices, future-close exclusion, budget checks, server-owned symbols and persisted choices before market requests. Regression/race tests and vet run with the application.

Prices are observed daily opens/closes, not executable quotes. Returns exclude dividends, fees and slippage. Research is restricted to archived evidence, but historical model knowledge cannot be eliminated; do not describe this as a blind backtest.

Evidence: [live run](results/weekly-game/live.json), [live UI](results/weekly-game/live.png), [fixture run](results/weekly-game/fixture.json), [fixture UI](results/weekly-game/fixture.png).

Run `make test`, `make test-terminal`, then `node e2e/briefing-desk-live-local.cjs` with the live local services. Every Chromium launch includes `--use-mock-keychain`.
