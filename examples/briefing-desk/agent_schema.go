package main

// Match the application's existing validators, without relying on prose prompts
// to keep the provider's final response in the expected shape.
func researchFinalFormat(j *agentJob) map[string]any {
	text := func(min, max int) map[string]any {
		return map[string]any{"type": "string", "minLength": min, "maxLength": max}
	}
	array := func(item any, min, max int) map[string]any {
		return map[string]any{"type": "array", "items": item, "minItems": min, "maxItems": max}
	}
	enum := func(values []string) map[string]any {
		return map[string]any{"type": "string", "enum": values}
	}
	var shape map[string]any
	switch j.stage {
	case "memo_reason":
		shape = schema(map[string]any{"reason": text(20, 700), "point_ids": array(map[string]any{"type": "string", "pattern": "^P[1-5]$"}, 1, 5)}, "reason", "point_ids")
	case "collect":
		ids := []string{}
		for _, h := range j.headlines {
			ids = append(ids, h.ID)
		}
		shape = schema(map[string]any{"headline_ids": array(enum(ids), len(ids), len(ids))}, "headline_ids")
	case "summarize":
		shape = schema(map[string]any{"title": text(3, 120), "tldr": text(15, 500), "points": array(text(1, 1000), 2, 5), "overview": text(20, 1200), "market_context": text(20, 500)}, "title", "tldr", "points", "overview", "market_context")
	default:
		ids := []string{}
		for _, ev := range j.evidence {
			ids = append(ids, ev.ID)
		}
		refs := array(enum(ids), 1, 6)
		finding := schema(map[string]any{"text": text(3, 2500), "sources": refs}, "text", "sources")
		plan := schema(map[string]any{
			"symbol": enum([]string{"QQQ", "SPY", "SMH", "XLK"}), "direction": enum([]string{"buy", "short", "no_signal"}),
			"thesis": text(10, 1200), "target_move_pct": map[string]any{"type": "number", "minimum": 0, "maximum": 25},
			"stop_move_pct": map[string]any{"type": "number", "minimum": 0, "maximum": 15},
			"horizon_days":  map[string]any{"type": "integer", "minimum": 1, "maximum": 90}, "sources": refs,
		}, "symbol", "direction", "thesis", "target_move_pct", "stop_move_pct", "horizon_days", "sources")
		shape = schema(map[string]any{
			"headline": text(3, 160), "facts": array(finding, 1, 4), "hypotheses": array(finding, 1, 4),
			"unknowns": array(text(3, 1000), 1, 5), "next_checks": array(text(3, 1000), 1, 5), "plan": plan,
		}, "headline", "facts", "hypotheses", "unknowns", "next_checks", "plan")
	}
	return map[string]any{"type": "json_schema", "name": "signal_desk_" + j.stage, "strict": true, "schema": shape}
}
