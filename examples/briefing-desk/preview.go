package main

import (
	"encoding/json"
	"errors"
	"fmt"
)

func previewMarket() marketSnapshot {
	out := marketSnapshot{Retrieved: stamp(), Coverage: "SYNTHETIC PREVIEW QUOTES. These are fictional fixture values, not market data."}
	for i, v := range instruments {
		out.Quotes = append(out.Quotes, quote{Instrument: v, Price: []float64{100, 200, 50, 75}[i], Currency: "USD", AsOf: stamp(), Retrieved: stamp(), URL: "https://example.com/demo-quote/" + v.Symbol, Basis: "SYNTHETIC DEMO PRICE"})
	}
	return out
}
func seedPreview(st *store) error {
	docs, _ := st.list()
	for _, d := range docs {
		if d.Source.Coverage != "synthetic_demo" {
			return errors.New("Preview requires a separate data directory; real documents were found")
		}
	}
	if len(docs) > 0 {
		return nil
	}
	p, _ := st.profile("tech")
	snap := previewMarket()
	for i, signal := range []string{"buy", "short", "no_signal"} {
		title := []string{"Demo: chip demand outlook improves", "Demo: technology spending outlook weakens", "Demo: a product update has no clear index impact"}[i]
		brief := []string{"Fictional example: sustained demand could support semiconductor index exposure. Investigate the assumptions before considering a paper position.", "Fictional example: weaker enterprise spending could pressure broad technology exposure. The research file explores a bearish paper scenario.", "Fictional example: a small product announcement provides too little evidence for a directional index thesis."}[i]
		raw := fmt.Sprintf("https://example.com/synthetic-demo/%d", i)
		d, _, e := st.add(raw)
		if e != nil {
			return e
		}
		if e = st.update(d.ID, func(d *document) {
			d.Status = "ready"
			d.Source = source{URL: raw, FinalURL: raw, Host: "synthetic preview", Title: title, Fetched: stamp(), Coverage: "synthetic_demo"}
			d.Summary = summary{Title: title, TLDR: brief, Points: []string{"Synthetic fixture for interface testing.", "No live agent or classifier has run."}, Overview: brief}
			probs := map[string]float64{"buy": .15, "short": .15, "no_signal": .15}
			probs[signal] = .7
			probs[[]string{"short", "no_signal", "buy"}[i]] = .15
			score := []float64{90, 50, 10}[i]
			d.Classes["tech"] = classification{Score: &score, MemoReason: "Synthetic example of relevance to the fund memo, not a financial forecast.", ReasonSources: []string{"P1"}, ProfileID: "tech", ProfileVersion: p.Version, SummaryHash: d.Summary.hash(), Status: "done", Priority: opportunityBand(score), Confidence: .7, Probabilities: probs}
			d.Markdown = documentMarkdown(*d)
			if signal != "no_signal" {
				symbol := "SMH"
				if signal == "short" {
					symbol = "QQQ"
				}
				data, _ := json.Marshal(snap)
				ev := []evidence{{ID: "D1", Tool: "synthetic_fixture", URL: raw, Retrieved: stamp(), Data: json.RawMessage(`{"coverage":"fictional test article"}`)}, {ID: "D2", Tool: "market_snapshot", URL: "https://example.com/synthetic-market", Retrieved: stamp(), Data: data}}
				plan := &tradePlan{Symbol: symbol, Direction: signal, Thesis: brief, TargetMovePct: 3, StopMovePct: 1.5, HorizonDays: 5, Sources: []string{"D1", "D2"}}
				bindPlan(plan, ev)
				d.Research["tech"] = research{Status: "done", ProfileVersion: p.Version, SummaryHash: d.Summary.hash(), At: stamp(), Evidence: ev, Report: report{Headline: title, Facts: []finding{{"This is a fictional interface fixture.", []string{"D1"}}}, Hypotheses: []finding{{brief, []string{"D1"}}}, Unknowns: []string{"No real news, model output or market prices are represented by these fixtures."}, NextChecks: []string{"Connect the OpenShift AI and T4 endpoints to run the real pipeline."}, Plan: plan}}
			}
		}); e != nil {
			return e
		}
	}
	return nil
}
