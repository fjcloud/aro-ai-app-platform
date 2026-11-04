# Bottom-right rarity symbols — 4 November 2026

The prepared model now identifies card kind and the small rarity mark next to
the collector number. It chooses `circle`, `diamond`, `star`, `none` or
`uncertain`; the application maps the chosen mark to the Base Set rarity.
This is one native inference request, with an eligibility gate before exposing
rarity. Non-Pokémon/unknown kinds get `not_applicable` and no invented confidence.

| Model mark | Fixed English Base Set policy |
|---|---|
| Circle | Common |
| Diamond (square rotated 45 degrees) | Uncommon |
| Star | Rare |
| Fully visible corner with no mark | Common (Basic Energy) |
| Cropped, blurry or unreadable corner | Uncertain |

The absent-mark rule is scoped to this complete Base Set. It is not a general
rule for scarce promotional cards. The large Energy icons, set artwork and
1st Edition stamp are not rarity marks. An absent mark is not the same as an
unreadable one. D3 can still miss a small mark in a clean scan.

## Actual T4 comparison

All variants used the same 102 checksummed source scans and the same model on
the existing T4. Only image bytes and shared questions entered the request.
The references were read for scoring, not included as per-item model answers.

| Measure | Before | Selected symbol method |
|---|---:|---:|
| Correct three-tier rarity | 75/102 | 94/102 |
| Rare cards found | 31/32 | 32/32 |
| False rare predictions | 0 | 0 |
| Uncertain rarity | 23 | 0 |
| Wrong non-abstaining outputs | 4 | 8 |
| Card-kind accuracy | 101/102 | 101/102 |

The new method makes more definite decisions; eight are wrong. It misses seven
Uncommon diamonds as absent marks and still misidentifies Grass Energy as
another game. All other Basic Energy cards have a detected absent mark and map
to Common. This does not establish accuracy on new editions or phone photos.
The same batch was used to select the prompt, so it is not an independent
holdout/generalization result. The absence of Uncertain choices is not certainty.

Intermediate prompts scored 85/102 (corner instruction, but inappropriate
`not_applicable` choices on Pokémon) and 88/102 (four direct rarity options).
The selected shape output removes that ambiguity without replacing neural
predictions with catalogue labels. Three historical negative controls were
correctly identified and excluded; they remain outside the production dataset.

## Output and tracing

`raw_answers` retains native card-kind and symbol choices/probabilities.
`answers.rarity` maps the chosen symbol. Common probabilities aggregate Circle
and None; the other categories retain their respective symbol probabilities.
The mapped choice follows the chosen visual mark, so it need not be the argmax
of grouped probabilities. Scores are not calibrated correctness guarantees.

MLflow stores native `answers` and separate `mapped_answers`. Keys and image
bytes are excluded. The public API still exposes the existing kind, rarity and
probabilities fields, plus the optional raw `rarity_symbol`; the Go UI is unchanged.
Question-sensitive cache invalidation forces a fresh production analysis.

## Evidence

- `comparison.json`: measured before/after counts and scope.
- `corner-*`, `conditional-*`: exploratory direct-rarity requests/results.
- `symbol-questions.json`, `symbol-analysis.json`: selected native shape inputs,
  raw model outputs and rule-derived rarity. No reference-based corrections.
- `negative-controls.json`: separate historical control probes, not feed items.
- `live-summary.json`, `live-batch.json`: actual deployed image, mapped decisions and errors.
- `app-live.json`, `app-fixtures.json`, screenshots: production browser acceptance and isolated error cases.
- `api-preflight.json`: all four tutorial API blocks executed in the real scoped Dev Space, with five JSON responses including native MCP evidence.

Local validation passed: 46 helper tests, 13 platform tests and Antora build.
The prebuilt image passed its container tests before publication. Browser tests
use `--use-mock-keychain` and check complete portrait images, filters, prices,
source scope, MLflow links and isolated failure cases.

The tutorial now uses the exact tested state and questions. A shortened sample
misidentified Fighting Energy; the aligned request correctly returns Pokemon TCG
and None. Both tutorial model examples and the DeepSeek/native-agent requests
passed in the actual workspace.
