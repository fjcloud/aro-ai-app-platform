package main

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTechmemePublisherAndExcerpt(t *testing.T) {
	got := publisherURL(`<a href="https://www.techmeme.com/1#item">permalink</a><a href="https://publisher.example/story#section">Publisher</a>`, "https://www.techmeme.com/1#item")
	if got != "https://publisher.example/story" {
		t.Fatal("Publisher URL was not resolved safely", got)
	}
	got = publisherURL(`<a href="https://x.com/author">@author</a><span><b><a href="https://x.com/author/status/123">Actual news headline</a></b></span>`, "https://www.techmeme.com/1#item")
	if got != "https://x.com/author/status/123" {
		t.Fatal("RSS attribution profile was mistaken for the news source", got)
	}
	if got := publisherURL(`<a href="http://169.254.169.254/latest">bad</a>`, "https://www.techmeme.com/1#item"); got != "https://www.techmeme.com/1" {
		t.Fatal("Unsafe publisher reference was accepted", got)
	}
	a, _ := newAgent("http://localhost:1234/v1", "native", "unused")
	a.fetch = &fetcher{&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader("denied")), Header: http.Header{}, Request: r}, nil
	})}}
	j := a.job(context.Background(), "summarize", "", "https://publisher.example/story", nil)
	j.fallback = source{Title: "RSS headline", RSSExcerpt: strings.Repeat("Sourced RSS excerpt. ", 8), FeedURL: feeds[0].URL}
	data, _, e := a.runTool(context.Background(), j, "read_article", json.RawMessage(`{"source_id":"original"}`))
	if e != nil {
		t.Fatal(e)
	}
	s := data.(source)
	if s.Coverage != "rss_excerpt" || s.Text != j.fallback.RSSExcerpt || s.FeedURL != feeds[0].URL {
		t.Fatal("Fallback did not disclose excerpt-only coverage", s)
	}
}
func TestScenarioMathAndQuoteBinding(t *testing.T) {
	q := quote{Instrument: instruments[0], Price: 100, Currency: "USD", AsOf: stamp(), URL: "https://example.com/quote"}
	for _, direction := range []string{"buy", "short"} {
		p := tradePlan{Symbol: "QQQ", Direction: direction, TargetMovePct: 3, StopMovePct: 1.5, HorizonDays: 5, Thesis: "Hypothetical exposure", Sources: []string{"E1"}, Quote: &quote{Price: 1}}
		data, _ := json.Marshal(marketSnapshot{Quotes: []quote{q}})
		if e := bindPlan(&p, []evidence{{ID: "E1", Tool: "market_snapshot", Data: data}}); e != nil {
			t.Fatal(e)
		}
		if p.Quote.Price != 100 {
			t.Fatal("Model-supplied quote was trusted")
		}
		s, e := scenario(p, 10000)
		if e != nil || s.TargetPnL != 300 || s.StopPnL != -150 || s.Units != 100 {
			t.Fatal("Incorrect gross P&L", s, e)
		}
		if direction == "buy" && (s.Target != 103 || s.Stop != 98.5) {
			t.Fatal("Incorrect long price levels", s)
		}
		if direction == "short" && (math.Abs(s.Target-97) > 1e-9 || math.Abs(s.Stop-101.5) > 1e-9) {
			t.Fatal("Incorrect short price levels", s)
		}
	}
	p := tradePlan{Direction: "buy", Quote: &q, TargetMovePct: 3, StopMovePct: 1.5}
	for _, at := range []string{time.Now().Add(-73 * time.Hour).UTC().Format(time.RFC3339), time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "bad"} {
		p.Quote.AsOf = at
		if _, e := scenario(p, 10000); e == nil {
			t.Fatal("Outdated/future/invalid quote priced a scenario")
		}
	}
}
func TestQuoteRejectsWrongIdentityAndMissingTimestamp(t *testing.T) {
	for _, body := range []string{`{"chart":{"result":[{"meta":{"symbol":"BAD","regularMarketPrice":100,"currency":"USD","regularMarketTime":1}}]}}`, `{"chart":{"result":[{"meta":{"symbol":"QQQ","regularMarketPrice":100,"currency":"USD"}}]}}`} {
		f := &fetcher{&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
		})}}
		if q := f.quote(context.Background(), instruments[0]); q.Error == "" {
			t.Fatal("Invalid quote accepted")
		}
	}
}
func TestPreviewIsIsolatedAndCannotExecuteModels(t *testing.T) {
	s := testServer(t)
	if e := seedPreview(s.store); e != nil {
		t.Fatal(e)
	}
	s.preview = true
	r := httptest.NewRequest("POST", "http://localhost/api/scan", strings.NewReader(`{"feed_id":"techmeme","profile_id":"tech"}`))
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, r)
	if w.Code != 409 {
		t.Fatal("Preview attempted live model execution")
	}
	docs, _ := s.store.list()
	if len(docs) != 3 {
		t.Fatal("Missing preview stories")
	}
	d, _, _ := s.store.add("https://example.com/real-source")
	s.store.update(d.ID, func(d *document) { d.Source.Coverage = "publisher_readable_text" })
	if seedPreview(s.store) == nil {
		t.Fatal("Mixed live and synthetic data directory accepted")
	}
}
func TestLivePublicData(t *testing.T) {
	if os.Getenv("LIVE_DATA_TESTS") != "1" {
		t.Skip("opt-in public network check")
	}
	f := newFetcher()
	items, _, e := f.headlines(context.Background(), "techmeme")
	if e != nil || len(items) < 3 {
		t.Fatal("Live Techmeme RSS failed", e)
	}
	for _, h := range items {
		if strings.Contains(h.URL, "#") {
			t.Fatal("Unresolved RSS fragment")
		}
	}
	t.Log("Actual Techmeme articles:", len(items))
	m := (&marketService{fetch: f}).snapshot(context.Background())
	for _, q := range m.Quotes {
		if q.Error != "" {
			t.Fatalf("%s quote unavailable: %s", q.Instrument.Symbol, q.Error)
		}
		t.Log(q.Instrument.Symbol, "timestamp", q.AsOf, "currency", q.Currency)
	}
}
