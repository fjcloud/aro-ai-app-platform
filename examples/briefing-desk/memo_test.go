package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestScoreRejectsMissingOrInconsistentOrdinalAnswers(t *testing.T) {
	for _, answer := range []map[string]any{
		{"type": "score", "input_coverage": "complete", "probabilities": map[string]float64{"0": 1, "1": 0, "2": 0, "3": 0, "4": 0}},
		{"type": "score", "score": 4, "input_coverage": "complete", "probabilities": map[string]float64{"0": 1, "1": 0, "2": 0, "3": 0, "4": 0}},
		{"type": "score", "score": 0, "input_coverage": "complete", "probabilities": map[string]float64{"0": 1}},
		{"type": "choice", "score": 0, "input_coverage": "complete", "probabilities": map[string]float64{"0": 1, "1": 0, "2": 0, "3": 0, "4": 0}},
	} {
		up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reply(w, 200, map[string]any{"answers": map[string]any{"opportunity": answer}})
		}))
		client, _ := newDecisionClient(up.URL, "fixture", "")
		if _, e := client.classify(context.Background(), summary{Title: "Fixture"}, defaults()[0]); e == nil {
			t.Fatal("Invalid ordinal score accepted")
		}
		up.Close()
	}
}

func TestLiveMemoScoreSensitivity(t *testing.T) {
	if os.Getenv("LIVE_MEMO_MODEL") != "1" {
		t.Skip("opt-in real D3 memo-sensitivity check")
	}
	r, e := http.Get("http://127.0.0.1:18092/api/state")
	if e != nil {
		t.Fatal(e)
	}
	defer r.Body.Close()
	var state struct {
		Documents []document `json:"documents"`
	}
	if json.NewDecoder(r.Body).Decode(&state) != nil {
		t.Fatal("Invalid local state")
	}
	client, e := newDecisionClient("http://127.0.0.1:18093/v1/systemone", "vllm-sr/d3-mini", "")
	if e != nil {
		t.Fatal(e)
	}
	chips := defaults()[0]
	retail := profile{ID: "retail", Name: "Retail fund", Criteria: "We specialize in consumer internet retail, online marketplaces, merchant revenues, gross merchandise sales and shopper demand. Prioritize material marketplace revenue, earnings and buyer trends. Chip manufacturers and component suppliers are outside our mandate without a documented link to merchant or consumer marketplace demand.", Version: 1}
	cases := []map[string]any{}
	for _, keyword := range []string{"Moore Threads", "Etsy"} {
		var d document
		for _, candidate := range state.Documents {
			if strings.Contains(candidate.Summary.Title, keyword) {
				d = candidate
				break
			}
		}
		if d.ID == "" {
			t.Fatal("Live comparison article unavailable", keyword)
		}
		a, e := client.classify(context.Background(), d.Summary, chips)
		if e != nil {
			t.Fatal(e)
		}
		b, e := client.classify(context.Background(), d.Summary, retail)
		if e != nil {
			t.Fatal(e)
		}
		if keyword == "Moore Threads" && a.Score < b.Score+25 || keyword == "Etsy" && b.Score < a.Score+25 {
			t.Fatal("Memo did not meaningfully change relevance", keyword, a.Score, b.Score)
		}
		cases = append(cases, map[string]any{"article": d.Summary.Title, "source": d.URL, "semiconductor_memo_score": a.Score, "retail_memo_score": b.Score})
	}
	data, _ := json.MarshalIndent(map[string]any{"passed": true, "scope": "Actual D3 scoring of the same real source summaries against two investment memos; no app memo or scores were changed", "at": stamp(), "model": client.model, "cases": cases}, "", "  ")
	if file := os.Getenv("LIVE_MEMO_REPORT"); file != "" {
		if e := os.WriteFile(file, append(data, '\n'), 0600); e != nil {
			t.Fatal(e)
		}
	}
	t.Log(string(data))
}

func TestZeroScoreRemainsAValidCurrentMemoScore(t *testing.T) {
	p := defaults()[0]
	d := document{Summary: summary{Title: "Unrelated fixture"}, Classes: map[string]classification{}}
	zero := 0.0
	d.Classes[p.ID] = classification{Status: "done", ProfileVersion: p.Version, SummaryHash: d.Summary.hash(), Score: &zero, Priority: "low"}
	if !currentScore(d, p) {
		t.Fatal("Zero relevance was confused with a missing score")
	}
	p.Version++
	if currentScore(d, p) {
		t.Fatal("Score was reused for a different memo version")
	}
}
