---
name: finance-idea-generation
description: Archived-news company research adapted from Anthropic idea-generation.
---

# Company idea generation

Use the supplied fund memo, archived facts and dated public web reporting to build a small thematic shortlist. Search the web for the concrete catalyst and read one returned source ID before synthesizing, when eligible sources exist. The backend excludes results dated after the saved cutoff or lacking usable publication dates.

1. Set scope from the fund memo: long-only company shares, global exchanges, the saved information cutoff. Do not ask the user for parameters already present.
2. Identify the concrete catalyst in the news: demand, supply, capacity, pricing, margins, regulation or capital spending. Explain why it matters to this fund.
3. Map the value chain. Compare the named business with a possible supplier, customer or competitor. Look for a supported second-order effect, not just a famous company in the sector. State which business link the archive actually establishes; label assumptions.
4. Challenge the idea. Explain what could invalidate the transmission and whether public news might already be priced in. A favorable headline is not evidence that a stock is cheap or that the market missed it.
5. Return at most three distinct verified company shares with a one-sentence thesis hook, concrete risk and archived citations. Start with the direct business if its case is supported. An empty shortlist is valid. Keep the existing JSON schema and tool budget.

Data limits: the available tools provide archived headlines/excerpts, pre-cutoff public web-news results, bounded publisher excerpts with eligible date metadata and current company identities. Current page fetches and search-index dates are not independent historical snapshots; state the coverage limitation. Page text updated after the cutoff is excluded. Do not run or claim quantitative value/growth screens, valuation comparisons, insider/ownership checks, consensus-disconnect analysis or historical fundamentals without returned evidence. Missing information stays unknown. Identity metadata does not establish listing type, trading liquidity or a primary listing. Do not describe them as verified. Do not invent multiples, fair values, target prices, position recommendations or expected profits. No prices before portfolio commitment, no later news or market outcomes and no orders. Related shares need not move together. Screens and thematic leads are research candidates, not conclusions.
