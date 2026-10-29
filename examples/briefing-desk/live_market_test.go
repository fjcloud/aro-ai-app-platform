package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestLiveLatestMarketQuote(t *testing.T) {
	if os.Getenv("LIVE_MARKET") != "1" {
		t.Skip("opt-in real Yahoo market data; no model calls")
	}
	corpus, err := bundledCorpus()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	result, err := newFetcher().weekReturn(ctx, "SMH", corpus.Week, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if result.QuoteTime == "" || result.ExitPrice <= 0 || result.EntryDate < corpus.Week.MarketStart {
		t.Fatal(result)
	}
	b, _ := json.MarshalIndent(map[string]any{"passed": true, "live_market_data": true, "model_calls": 0, "week": corpus.Week, "quote": result}, "", "  ")
	path := "../../e2e/results/prepared-inputs/live-market.json"
	if err = os.MkdirAll("../../e2e/results/prepared-inputs", 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, append(b, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("Actual SMH open %.2f → latest %.2f, quote time %s", result.EntryPrice, result.ExitPrice, result.QuoteTime)
}
