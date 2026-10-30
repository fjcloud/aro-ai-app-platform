package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRepositorySnapshotDoesNotRollWithTheCalendar(t *testing.T) {
	corpus, err := bundledCorpus()
	if err != nil {
		t.Fatal(err)
	}
	if len(corpus.Stories) == 0 || corpus.Week.ValuationMode != "latest" {
		t.Fatal("Repository snapshot missing", corpus.Week)
	}
	s := testServer(t)
	t.Setenv("NEWS_CORPUS", "bundled")
	s.clock = func() time.Time { return fixtureTime().AddDate(0, 2, 0) }
	s.initGame()
	if s.game.Week != corpus.Week {
		t.Fatal("Pinned repository news rolled to a different week")
	}
	cutoff, err := time.Parse(time.RFC3339, corpus.Week.Cutoff)
	if err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("America/New_York")
	entry, err := time.ParseInLocation("2006-01-02", corpus.Week.MarketStart, loc)
	if err != nil || entry.Add(9*time.Hour+30*time.Minute).Before(cutoff) {
		t.Fatal("Market entry precedes the available news")
	}
}

func TestLoadingRepositoryNewsNeedsNoTechmemeRequest(t *testing.T) {
	s := testServer(t)
	t.Setenv("NEWS_CORPUS", "bundled")
	s.initGame()
	newsCalls, modelCalls := 0, 0
	s.agent.fetch = &fetcher{client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		newsCalls++
		return nil, fmt.Errorf("Unexpected public news request")
	})}}
	s.model.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		modelCalls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"answers":{"opportunity":{"type":"score","score":3,"probabilities":{"0":0,"1":0,"2":0,"3":1,"4":0},"input_coverage":"complete"}}}`)), Header: http.Header{}, Request: r}, nil
	})}
	s.loadGameWeek()
	if s.game.Status != "ready" || newsCalls != 0 || modelCalls != len(s.gameDocuments()) {
		t.Fatalf("status=%s error=%s news=%d model=%d", s.game.Status, s.game.Error, newsCalls, modelCalls)
	}
	// Existing scored news can migrate without reusing cached ETF ideas or
	// changing/deleting the previous paper game's stored portfolio.
	s.game.Proposals["old"] = gameProposal{Status: "done", Ideas: []gameIdea{{Symbol: "QQQ"}}}
	s.game.Round.Positions = []paperPosition{{Symbol: "QQQ", Amount: 1000}}
	s.saveGame()
	old := filepath.Join(s.store.dir, "game-"+s.game.Week.ID+".json")
	if err := os.Rename(s.gameFile(), old); err != nil {
		t.Fatal(err)
	}
	s.initGame()
	if s.game.Status != "ready" || len(s.game.Proposals) != 0 || len(s.game.Round.Positions) != 0 || modelCalls != len(s.gameDocuments()) {
		t.Fatal("Equities migration reused ETF ideas or lost cached scores", s.game.Status)
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatal("Earlier portfolio file was removed", err)
	}
}

func TestLatestQuoteValuesTheSnapshotWithoutUsingAPartialDailyClose(t *testing.T) {
	corpus, err := bundledCorpus()
	if err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("America/New_York")
	entry, _ := time.ParseInLocation("2006-01-02", corpus.Week.MarketStart, loc)
	at := entry.Add(12 * time.Hour)
	bars := []dailyBar{{At: entry.Add(9*time.Hour + 30*time.Minute), Open: ptr(100), Close: ptr(999)}}
	quoteAt := at.Add(-time.Minute).Unix()
	result, err := latestMarketResult("NVDA", bars, nil, corpus.Week, at, ptr(105), quoteAt, "https://example.com/quote")
	if err != nil || result.ExitPrice != 105 || result.ReturnPct < 4.999 || result.ReturnPct > 5.001 || result.QuoteTime != time.Unix(quoteAt, 0).UTC().Format(time.RFC3339) {
		t.Fatal(result, err)
	}
	for _, q := range []int64{at.Add(time.Minute).Unix(), entry.Add(-time.Hour).Unix()} {
		if _, err := latestMarketResult("NVDA", bars, nil, corpus.Week, at, ptr(105), q, ""); err == nil {
			t.Fatal("Accepted future or pre-entry quote")
		}
	}
	if _, err := latestMarketResult("NVDA", bars, nil, corpus.Week, at, nil, quoteAt, ""); err == nil {
		t.Fatal("Invented a missing quote")
	}
	// The archived news stays fixed, but valuation can advance into later weeks.
	later := at.AddDate(0, 0, 10)
	if result, err := latestMarketResult("NVDA", bars, nil, corpus.Week, later, ptr(110), later.Add(-time.Minute).Unix(), ""); err != nil || result.ExitDate == result.EntryDate {
		t.Fatal(result, err)
	}
}

func TestRefreshingAQuoteAdvancesItsDisplayedTimestamp(t *testing.T) {
	s := testServer(t)
	t.Setenv("NEWS_CORPUS", "bundled")
	s.initGame()
	loc, _ := time.LoadLocation("America/New_York")
	entry, _ := time.ParseInLocation("2006-01-02", s.game.Week.MarketStart, loc)
	now := entry.Add(12 * time.Hour)
	s.clock = func() time.Time { return now }
	s.game.Round.Positions = []paperPosition{{Symbol: "NVDA", Amount: 1000}}
	s.game.Round.Invested = 1000
	calls := 0
	s.agent.fetch = &fetcher{client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		q := now.Add(-time.Duration(3-calls) * time.Minute).Unix()
		body := fmt.Sprintf(`{"chart":{"result":[{"meta":{"symbol":"NVDA","currency":"USD","instrumentType":"EQUITY","exchangeTimezoneName":"America/New_York","regularMarketPrice":%d,"regularMarketTime":%d},"timestamp":[%d],"indicators":{"quote":[{"open":[100],"close":[999]}]}}]}}`, 100+calls*5, q, entry.Add(9*time.Hour+30*time.Minute).Unix())
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
	})}}
	s.ctx = context.Background()
	s.settleRound(s.game.Round.ID)
	first := s.game.Round.AsOf
	s.settleRound(s.game.Round.ID)
	if s.game.Round.Status != "settled" || s.game.Round.AsOf <= first || s.game.Round.Positions[0].Amount != 1000 {
		t.Fatal(s.game.Round)
	}
}
