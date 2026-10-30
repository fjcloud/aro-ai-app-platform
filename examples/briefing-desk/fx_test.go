package main

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestForeignStockEntryUsesItsExchangeSessionAfterTheCutoff(t *testing.T) {
	corpus, err := bundledCorpus()
	if err != nil {
		t.Fatal(err)
	}
	cutoff, _ := time.Parse(time.RFC3339, corpus.Week.Cutoff)
	loc, _ := time.LoadLocation("Asia/Seoul")
	early := time.Date(2026, 10, 29, 9, 0, 0, 0, loc)
	entry := early.AddDate(0, 0, 1)
	if !early.Before(cutoff) {
		t.Fatal("Fixture must precede the known news")
	}
	bars := []dailyBar{{At: early, Open: ptr(9000), Close: ptr(9500)}, {At: entry, Open: ptr(10000), Close: ptr(11000)}}
	now := entry.Add(8 * time.Hour)
	r, err := latestMarketResultOnExchange("005930.KS", bars, nil, corpus.Week, now, ptr(11000), now.Add(-time.Minute).Unix(), "https://example.com/stock", "Asia/Seoul")
	if err != nil || r.EntryDate != "2026-10-30" || r.EntryTime != entry.UTC().Format(time.RFC3339) {
		t.Fatal("Used a Korean opening before news availability", r, err)
	}
}

func TestUSDPortfolioIncludesFXAndNeverUsesAFutureEntryRate(t *testing.T) {
	entry := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	now := entry.Add(8 * time.Hour)
	for _, tc := range []struct {
		name                   string
		first                  int64
		symbol, currency, kind string
		good                   bool
	}{
		{"known rate", entry.Add(-time.Hour).Unix(), "KRW=X", "KRW", "CURRENCY", true},
		{"future rate", entry.Add(time.Hour).Unix(), "KRW=X", "KRW", "CURRENCY", false},
		{"wrong pair", entry.Add(-time.Hour).Unix(), "EUR=X", "KRW", "CURRENCY", false},
		{"wrong currency", entry.Add(-time.Hour).Unix(), "KRW=X", "EUR", "CURRENCY", false},
		{"not FX", entry.Add(-time.Hour).Unix(), "KRW=X", "KRW", "EQUITY", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFetcher()
			f.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				body := fmt.Sprintf(`{"chart":{"result":[{"meta":{"symbol":%q,"currency":%q,"instrumentType":%q,"regularMarketPrice":1250,"regularMarketTime":%d},"timestamp":[%d],"indicators":{"quote":[{"open":[1000],"close":[9999]}]}}]}}`, tc.symbol, tc.currency, tc.kind, now.Add(-time.Minute).Unix(), tc.first)
				return &http.Response{StatusCode: 200, Header: http.Header{}, Request: req, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			r, err := f.valueInUSD(context.Background(), historicalReturn{Symbol: "005930.KS", Currency: "KRW", EntryPrice: 10000, ExitPrice: 11000, EntryTime: entry.Format(time.RFC3339)}, now)
			if (err == nil) != tc.good {
				t.Fatal(r, err)
			}
			if tc.good && (math.Abs(r.ReturnPct-(-12)) > 0.000001 || r.FXQuoteTime == "" || r.FXSourceURL == "") {
				t.Fatal("Local +10% was incorrectly treated as a USD +10% return", r)
			}
		})
	}
}

func TestValuationRejectsFundsEvenWhenTickerSyntaxIsValid(t *testing.T) {
	f := newFetcher()
	f.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Request: r, Body: io.NopCloser(strings.NewReader(`{"chart":{"result":[{"meta":{"symbol":"QQQ","currency":"USD","instrumentType":"ETF"},"timestamp":[],"indicators":{"quote":[{"open":[],"close":[]}]}}]}}`))}, nil
	})
	corpus, _ := bundledCorpus()
	if _, err := f.weekReturn(context.Background(), "QQQ", corpus.Week, time.Now()); err == nil {
		t.Fatal("ETF valuation accepted")
	}
}

func TestPenceQuotesAreNormalizedBeforeUSDConversion(t *testing.T) {
	corpus, _ := bundledCorpus()
	entry := time.Date(2026, 10, 29, 8, 0, 0, 0, time.UTC)
	now := entry.Add(6 * time.Hour)
	f := newFetcher()
	f.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		meta := fmt.Sprintf(`"symbol":"VOD.L","currency":"GBp","instrumentType":"EQUITY","exchangeTimezoneName":"Europe/London","regularMarketPrice":120,"regularMarketTime":%d`, now.Add(-time.Minute).Unix())
		at, price := entry.Unix(), 100.0
		if strings.Contains(r.URL.Path, "GBP=X") {
			meta = fmt.Sprintf(`"symbol":"GBP=X","currency":"GBP","instrumentType":"CURRENCY","regularMarketPrice":0.8,"regularMarketTime":%d`, now.Add(-time.Minute).Unix())
			at = entry.Add(-time.Hour).Unix()
			price = 0.8
		}
		body := fmt.Sprintf(`{"chart":{"result":[{"meta":{%s},"timestamp":[%d],"indicators":{"quote":[{"open":[%f],"close":[%f]}]}}]}}`, meta, at, price, price)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Request: r, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	r, err := f.weekReturn(context.Background(), "VOD.L", corpus.Week, now)
	if err != nil || r.Currency != "GBP" || r.EntryPrice != 1 || r.ExitPrice != 1.2 || math.Abs(r.ReturnPct-20) > 0.000001 {
		t.Fatal("Pence were displayed or converted as pounds", r, err)
	}
}
