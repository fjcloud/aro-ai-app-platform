package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func fixtureTime() time.Time {
	loc, _ := time.LoadLocation("America/New_York")
	return time.Date(2026, 10, 28, 12, 0, 0, 0, loc)
}
func ptr(v float64) *float64 { return &v }
func TestReplayCalendarAndNoIntradayValuation(t *testing.T) {
	now := fixtureTime()
	week := lastNewsWeek(now)
	if week.NewsStart != "2026-10-19" || week.NewsEnd != "2026-10-25" || week.MarketStart != "2026-10-26" {
		t.Fatal("Wrong replay week", week)
	}
	loc, _ := time.LoadLocation("America/New_York")
	bars := []dailyBar{{time.Date(2026, 10, 26, 9, 30, 0, 0, loc), ptr(100), ptr(102)}, {time.Date(2026, 10, 27, 9, 30, 0, 0, loc), ptr(102), ptr(95)}, {time.Date(2026, 10, 28, 9, 30, 0, 0, loc), ptr(95), ptr(999)}}
	result, e := historicalResult("QQQ", bars, nil, week, now, "https://example.com/prices")
	if e != nil || result.ExitDate != "2026-10-27" || result.ReturnPct > -4.99 || result.ReturnPct < -5.01 || result.WeekComplete {
		t.Fatal("Future/intraday price used", result, e)
	}
	bars[0].Open = nil
	if _, e := historicalResult("QQQ", bars, nil, week, now, ""); e == nil {
		t.Fatal("Missing opening price was invented")
	}
}
func TestArchiveOnlyUsesRequestedHistoricalItems(t *testing.T) {
	week := lastNewsWeek(fixtureTime())
	body := []byte(`<script>var pgrdad='October 19, 2026, 12:00 PM';</script><div class="item"><span pml="261019p1"></span><div class="ii"><strong><a class="ourh" href="https://example.com/past">Past semiconductor news</a></strong> — Actual archived excerpt with enough source facts.</div></div><div class="item"><span pml="261028p1"></span><div class="ii"><a class="ourh" href="https://example.com/future">Future news</a> Result already known.</div></div>`)
	stories, e := parseArchive(body, "https://www.techmeme.com/261019/h1200", week)
	if e != nil || len(stories) != 1 || stories[0].ArchiveID != "261019p1" {
		t.Fatal("Archive scope was not enforced", stories, e)
	}
	future := []byte(strings.Replace(string(body), "October 19, 2026, 12:00 PM", "October 28, 2026, 12:00 PM", 1))
	if _, e := parseArchive(future, "https://www.techmeme.com/261028/h1200", week); e == nil {
		t.Fatal("Post-cutoff snapshot accepted")
	}
	a, _ := newAgent("http://localhost/v1", "fixture", "")
	j := a.job(context.Background(), "game_ideas", "", "", nil)
	j.archiveWeek = week
	j.archiveDocs = map[string]document{"original": {GameWeek: week.ID, Source: source{ArchivedAt: "2026-10-28T12:00:00Z"}}}
	if _, _, e := a.runGameTool(j, "read_archive", "original", ""); e == nil {
		t.Fatal("Live/future article accepted as historical evidence")
	}
	if _, _, e := a.runGameTool(j, "market_snapshot", "", ""); e == nil {
		t.Fatal("Research agent could access current market prices")
	}
}
func TestPortfolioFreezesBeforeMarketDataAndCannotOverspend(t *testing.T) {
	s := testServer(t)
	s.clock = fixtureTime
	s.initGame()
	week := s.game.Week
	p, _ := s.store.profile("tech")
	d, _, _ := s.store.add("https://example.com/archive")
	score := 80.0
	s.store.update(d.ID, func(d *document) {
		d.GameWeek = week.ID
		d.Status = "ready"
		d.Summary = summary{Title: "Archived chip news"}
		d.Classes[p.ID] = classification{Status: "done", Score: &score, ProfileVersion: p.Version, SummaryHash: d.Summary.hash()}
	})
	d, _ = s.store.get(d.ID)
	s.game.Proposals[d.ID] = gameProposal{ArticleID: d.ID, MemoVersion: p.Version, SummaryHash: d.Summary.hash(), Status: "done", Ideas: []gameIdea{{ID: "idea", Symbol: "SMH"}}}
	called := make(chan bool, 1)
	s.agent.fetch.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		b, e := os.ReadFile(s.gameFile())
		var state gameState
		json.Unmarshal(b, &state)
		if e != nil || state.Round.Status != "settling" || len(state.Round.Positions) != 1 {
			t.Error("Prices requested before choices were persisted")
		}
		select {
		case called <- true:
		default:
		}
		return nil, fmt.Errorf("fixture data unavailable")
	})
	roundID := s.game.Round.ID
	request := func(amount float64) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"round_id": roundID, "memo_version": p.Version, "positions": []paperPosition{{ArticleID: d.ID, IdeaID: "idea", Amount: amount, Symbol: "FORGED"}}})
		r := httptest.NewRequest("POST", "http://example.com/api/game/play", strings.NewReader(string(body)))
		w := httptest.NewRecorder()
		s.playGame(w, r)
		return w
	}
	if w := request(10001); w.Code != 400 {
		t.Fatal("Overspending accepted", w.Code)
	}
	if w := request(1000); w.Code != 202 {
		t.Fatal("Valid portfolio rejected", w.Code, w.Body.String())
	}
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("Settlement did not start")
	}
	s.gameMu.Lock()
	symbol := s.game.Round.Positions[0].Symbol
	s.gameMu.Unlock()
	if symbol != "SMH" {
		t.Fatal("User supplied ticker overrode the researched idea")
	}
	if w := request(2000); w.Code != 409 {
		t.Fatal("Committed portfolio was changed", w.Code)
	}
	// Allow the failing fixture settlement to finish before its temporary directory is removed.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.gameMu.Lock()
		done := s.game.Round.Status == "awaiting_prices"
		s.gameMu.Unlock()
		if done {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("Fixture settlement did not finish")
}
func TestHistoricalProviderIdentityValidation(t *testing.T) {
	f := newFetcher()
	f.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"chart":{"result":[{"meta":{"symbol":"FAKE","currency":"EUR"},"timestamp":[]}]}}`)), Request: r}, nil
	})
	if _, e := f.weekReturn(context.Background(), "SMH", lastNewsWeek(fixtureTime()), fixtureTime()); e == nil {
		t.Fatal("Wrong symbol/currency accepted")
	}
}
