package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Opt-in browser harness. All model replies and market/news HTTP responses are
// explicit fixtures; production has no fixture-model fallback.
func TestTerminalBrowserFixtures(t *testing.T)       { terminalBrowserFixtures(t, false) }
func TestRepositoryNewsBrowserFixtures(t *testing.T) { terminalBrowserFixtures(t, true) }
func terminalBrowserFixtures(t *testing.T, bundled bool) {
	if bundled {
		t.Setenv("NEWS_CORPUS", "bundled")
	} else {
		t.Setenv("NEWS_CORPUS", "live")
	}

	if os.Getenv("BROWSER_E2E") != "1" {
		t.Skip("opt-in Playwright integration")
	}
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "https://mlflow.fixture.example/mlflow/v1/traces")
	t.Setenv("MLFLOW_EXPERIMENT_ID", "1")
	t.Setenv("MLFLOW_WORKSPACE", "signal-desk-dev")
	const scope = "LOCAL VALIDATION: model and data fixtures, no live inference"
	key := filepath.Join(t.TempDir(), "key")
	os.WriteFile(key, []byte("fixture-key"), 0600)
	native := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input []json.RawMessage `json:"input"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("Invalid native request")
			return
		}
		var first struct {
			Content string `json:"content"`
		}
		json.Unmarshal(body.Input[0], &first)
		count := 0
		var source source
		for _, raw := range body.Input {
			var v struct {
				Type   string `json:"type"`
				Output string `json:"output"`
			}
			json.Unmarshal(raw, &v)
			if v.Type == "function_call_output" {
				count++
				var e struct {
					Tool string          `json:"tool"`
					Data json.RawMessage `json:"data"`
				}
				json.Unmarshal([]byte(v.Output), &e)
				if e.Tool == "read_article" {
					json.Unmarshal(e.Data, &source)
				}
			}
		}
		call := func(name, args string) {
			reply(w, 200, map[string]any{"status": "completed", "output": []any{map[string]string{"type": "function_call", "name": name, "call_id": fmt.Sprint("call-", count), "arguments": args}}})
		}
		finish := func(v any) {
			text, _ := json.Marshal(v)
			reply(w, 200, map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{map[string]string{"type": "output_text", "text": string(text)}}}}})
		}
		switch {
		case strings.HasPrefix(first.Content, "Research company stock opportunities"):
			switch count {
			case 0:
				call("read_archive", `{"source_id":"original"}`)
				return
			case 1:
				call("search_archive", `{"query":"chip"}`)
				return
			case 2:
				call("search_companies", `{"query":"Nvidia"}`)
				return
			case 3:
				call("search_companies", `{"query":"AMD"}`)
				return
			}
			finish(map[string]any{"comment": "Fixture archive-only research for the fund memo.", "ideas": []gameIdea{{Symbol: "NVDA", Relationship: "direct", Thesis: "Fixture semiconductor exposure based on archived chip demand.", Risk: "Fixture company concentration and uncertain market transmission.", Sources: []string{"E1", "E3"}}, {Symbol: "AMD", Relationship: "competitor", Thesis: "Fixture distinct chipmaker alternative based on archived news.", Risk: "Fixture competitive risk and uncertain financial transmission.", Sources: []string{"E1", "E3"}}}})
		case strings.HasPrefix(first.Content, "Explain this memo opportunity score:"):
			finish(memoRationale{Reason: "Fixture explanation links the source evidence to the fund memo.", PointIDs: []string{"P1"}})
		case strings.HasPrefix(first.Content, "Call get_headlines"):
			if count == 0 {
				call("get_headlines", "{}")
				return
			}
			finish(map[string]any{"headline_ids": []string{"H1", "H2", "H3"}})
		case strings.HasPrefix(first.Content, "Call read_article"):
			if count == 0 {
				call("read_article", `{"source_id":"original"}`)
				return
			}
			finish(summary{Title: source.Title, TLDR: "Fixture summary: " + source.Title + ". A synthetic source used to validate the complete application workflow.", Points: []string{"This is a fixture model response.", "Live inference is not asserted."}, Overview: "A fixture source summary with enough detail to validate the app protocol and the local browser workflow.", MarketContext: "Fixture financial context: validate this hypothesis during research."})
		default:
			switch count {
			case 0:
				call("read_article", `{"source_id":"original"}`)
				return
			case 1:
				call("market_snapshot", "{}")
				return
			case 2:
				call("search_news", `{"query":"chip"}`)
				return
			}
			direction := "buy"
			if strings.Contains(source.Title, "bearish") {
				direction = "short"
			}
			if strings.Contains(source.Title, "neutral") {
				direction = "no_signal"
			}
			finish(report{Headline: "Fixture index research", Facts: []finding{{"This statement comes from the retrieved fixture source.", []string{"E1"}}}, Hypotheses: []finding{{"A conditional technology-index exposure could express this fictional catalyst.", []string{"E1", "E3"}}}, Unknowns: []string{"No actual market reaction was established."}, NextChecks: []string{"Reconnect live models for real research."}, Plan: &tradePlan{Symbol: "QQQ", Direction: direction, Thesis: "A fictional Nasdaq-100 exposure used to test the priced scenario and signal path.", TargetMovePct: 3, StopMovePct: 1.5, HorizonDays: 5, Sources: []string{"E1", "E2"}}})
		}
	}))
	defer native.Close()
	decision := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var x struct {
			State string `json:"state"`
		}
		json.NewDecoder(r.Body).Decode(&x)
		var parsed struct {
			Summary summary `json:"source_summary"`
			Memo    string  `json:"investment_memo"`
		}
		json.Unmarshal([]byte(x.State), &parsed)
		score := .4
		if strings.Contains(parsed.Summary.Title, "bullish") {
			score = 3.6
		}
		if strings.Contains(parsed.Summary.Title, "bearish") {
			score = 2
		}
		probs := map[string]float64{"0": 0, "1": 0, "2": 0, "3": 0, "4": 0}
		if strings.Contains(parsed.Memo, "Retail-only fixture memo") {
			score = .4
		}
		switch score {
		case .4:
			probs["0"] = .6
			probs["1"] = .4
		case 2:
			probs["2"] = 1
		default:
			probs["3"] = .4
			probs["4"] = .6
		}
		reply(w, 200, map[string]any{"answers": map[string]any{"opportunity": map[string]any{"type": "score", "score": score, "confidence": .8, "input_coverage": "complete", "probabilities": probs}}, "usage": map[string]int{"input_tokens": 120}})

	}))
	defer decision.Close()
	fixtureFetch := &fetcher{&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := ""
		typ := "text/html"
		switch {
		case strings.Contains(r.URL.Hostname(), "techmeme") && strings.Contains(r.URL.Path, "/h"):
			day := strings.Split(strings.Trim(r.URL.Path, "/"), "/")[0]
			date, _ := time.Parse("060102", day)
			hour := 12
			if strings.HasSuffix(r.URL.Path, "h2355") {
				hour = 23
				date = date.Add(55 * time.Minute)
			}
			if strings.HasSuffix(r.URL.Path, "h0000") {
				hour = 0
			}
			date = date.Add(time.Duration(hour) * time.Hour)
			body = "<script>var pgrdad='" + date.Format("January 2, 2006, 3:04 PM") + "';</script>"
			for i, name := range []string{"bullish", "bearish", "neutral"} {
				body += fmt.Sprintf(`<div class="item"><span pml="%sp%d"></span><div class="ii"><a class="ourh" href="https://fixture.example/%s/%s">Fixture %s chip news</a> — Archived fixture excerpt: a concrete source fact used to test the complete historical game.</div></div>`, day, i+1, day, name, name)
			}
		case strings.Contains(r.URL.Hostname(), "techmeme"):
			typ = "text/xml"
			body = `<rss><channel>`
			for _, name := range []string{"bullish", "bearish", "neutral"} {
				body += fmt.Sprintf(`<item><title>Fixture %s chip story</title><link>https://www.techmeme.com/fixture/%s#story</link><description>&lt;a href="https://fixture.example/%s"&gt;Fixture %s chip story&lt;/a&gt;</description><pubDate>%s</pubDate></item>`, name, name, name, name, time.Now().Format(time.RFC1123Z))
			}
			body += `</channel></rss>`
		case r.URL.Path == "/v1/finance/search":
			typ = "application/json"
			body = `{"quotes":[{"symbol":"NVDA","longname":"NVIDIA Corporation","exchDisp":"NASDAQ","quoteType":"EQUITY"},{"symbol":"AMD","longname":"Advanced Micro Devices","exchDisp":"NASDAQ","quoteType":"EQUITY"}]}`
		case strings.Contains(r.URL.Hostname(), "finance.yahoo"):
			typ = "application/json"
			symbol := strings.TrimPrefix(r.URL.Path, "/v8/finance/chart/")
			loc, _ := time.LoadLocation("America/New_York")
			start := time.Date(2026, 10, 26, 9, 30, 0, 0, loc)
			if bundled {
				corpus, err := bundledCorpus()
				if err != nil {
					t.Fatal(err)
				}
				day, _ := time.ParseInLocation("2006-01-02", corpus.Week.MarketStart, loc)
				start = day.Add(9*time.Hour + 30*time.Minute)
			}
			close := 110.0
			if symbol == "QQQ" || symbol == "AMD" {
				close = 95
			}
			body = fmt.Sprintf(`{"chart":{"result":[{"meta":{"symbol":%q,"currency":"USD","instrumentType":"EQUITY","exchangeTimezoneName":"America/New_York","regularMarketPrice":%f,"regularMarketTime":%d},"timestamp":[%d,%d],"indicators":{"quote":[{"open":[100,100],"close":[%f,%f]}]}}]}}`, symbol, close, start.Add(2*time.Hour).Unix(), start.Unix(), start.AddDate(0, 0, 1).Unix(), close, close)
		default:
			name := strings.Trim(r.URL.Path, "/")
			body = fmt.Sprintf(`<html><title>Fixture %s chip story</title><article>%s</article></html>`, name, strings.Repeat("A synthetic publisher text with enough readable content for the retrieval tool. ", 5))
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Content-Type": []string{typ}}, Request: r}, nil
	})}}
	a, _ := newAgent(native.URL+"/v1", "fixture-model", key)
	a.fetch = fixtureFetch
	a.market = &marketService{fetch: fixtureFetch}
	model, _ := newDecisionClient(decision.URL, "fixture-d3", "")
	st, _ := openStore(t.TempDir())
	s := &server{ctx: context.Background(), store: st, agent: a, model: model, imports: make(chan struct{}, 2), classes: make(chan struct{}, 1), researches: make(chan struct{}, 1)}
	s.clock = fixtureTime
	if bundled {
		corpus, _ := bundledCorpus()
		loc, _ := time.LoadLocation("America/New_York")
		day, _ := time.ParseInLocation("2006-01-02", corpus.Week.MarketStart, loc)
		s.clock = func() time.Time { return day.Add(12 * time.Hour) }
	}
	s.initGame()
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/state" || r.URL.Path == "/api/game" {
			rec := httptest.NewRecorder()
			s.routes().ServeHTTP(rec, r)
			var data map[string]any
			json.Unmarshal(rec.Body.Bytes(), &data)
			data["validation_fixture"] = true
			reply(w, rec.Code, data)
			return
		}
		s.routes().ServeHTTP(w, r)
	}))
	defer app.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	script := "../../e2e/briefing-desk-browser.cjs"
	if bundled {
		script = "../../e2e/saved-news-browser.cjs"
	}
	cmd := exec.CommandContext(ctx, "node", script)
	cmd.Env = append(os.Environ(), "APP_URL="+app.URL, "TEST_SCOPE="+scope)
	out, e := cmd.CombinedOutput()
	t.Log(string(out))
	if e != nil {
		t.Fatal("Local fixture browser workflow failed", e)
	}
}
