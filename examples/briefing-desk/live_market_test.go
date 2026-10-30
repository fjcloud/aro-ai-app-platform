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
	quotes := []historicalReturn{}
	for _, symbol := range []string{"NVDA", "005930.KS"} {
		result, err := newFetcher().weekReturn(ctx, symbol, corpus.Week, time.Now())
		if err != nil {
			t.Fatal(symbol, err)
		}
		if result.QuoteTime == "" || result.ExitPrice <= 0 || result.EntryDate < corpus.Week.MarketStart {
			t.Fatal(result)
		}
		if symbol == "005930.KS" && (result.Currency != "KRW" || result.FXSourceURL == "" || result.FXQuoteTime == "") {
			t.Fatal("Missing actual FX", result)
		}
		quotes = append(quotes, result)
	}
	b, _ := json.MarshalIndent(map[string]any{"passed": true, "live_market_data": true, "model_calls": 0, "week": corpus.Week, "quotes": quotes}, "", "  ")
	path := "../../e2e/results/company-opportunities/live-market.json"
	if err = os.MkdirAll("../../e2e/results/company-opportunities", 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, append(b, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	t.Log("Actual US and Korean company prices, exchange entry times and USD/KRW conversion verified")
}
