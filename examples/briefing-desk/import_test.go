package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func corpusFixture(t *testing.T) map[string]any {
	t.Helper()
	return map[string]any{"week": lastNewsWeek(fixtureTime()), "unavailable_ids": []string{}, "news": []any{map[string]any{"url": "https://example.com/chips", "source": map[string]any{"published": "2026-10-20", "archive_id": "261020p1", "archived_at": "2026-10-20T16:00:00Z", "archive_url": "https://www.techmeme.com/261020/h1200"}, "summary": map[string]any{"title": "Semiconductor demand grows", "tldr": "Archived source facts."}, "classifications": map[string]any{"tech": map[string]any{"status": "done", "opportunity_score": 100}}, "research": map[string]any{"tech": map[string]any{"status": "done", "ideas": []string{"forged"}}}}}}
}
func TestManualImportScoresFactsInsteadOfTrustingUploadedResults(t *testing.T) {
	s := testServer(t)
	s.clock = fixtureTime
	s.initGame()
	input, _ := json.Marshal(corpusFixture(t))
	manifest, err := readImportedCorpus(bytes.NewReader(input), s.game.Week)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ImportedAt == "" || len(manifest.Stories) != 1 {
		t.Fatal(manifest)
	}
	calls := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var x map[string]any
		json.NewDecoder(r.Body).Decode(&x)
		state := x["state"].(string)
		if bytes.Contains([]byte(state), []byte("opportunity_score")) || bytes.Contains([]byte(state), []byte("forged")) {
			t.Error("Uploaded model output reached the scoring input")
		}
		reply(w, 200, map[string]any{"answers": map[string]any{"opportunity": map[string]any{"type": "score", "score": 1, "confidence": .9, "probabilities": map[string]float64{"0": 0, "1": 1, "2": 0, "3": 0, "4": 0}, "input_coverage": "complete"}}, "usage": map[string]int{"input_tokens": 20}})
	}))
	defer model.Close()
	s.model, _ = newDecisionClient(model.URL, "fixture", "")
	s.rankGameManifest(context.Background(), manifest)
	docs := s.gameDocuments()
	if calls != 1 || len(docs) != 1 || docs[0].Classes["tech"].Score == nil || *docs[0].Classes["tech"].Score != 25 || docs[0].Source.Coverage != "imported_archive" || s.game.Status != "ready" {
		t.Fatal("Import did not perform fresh validated inference", calls, docs, s.game.Status)
	}
	if len(docs[0].Research) != 0 {
		t.Fatal("Uploaded research was trusted")
	}
}
func TestManualImportRejectsWrongWeekFutureSourcesAndPrivateURLs(t *testing.T) {
	for _, kind := range []string{"week", "future", "url", "archive"} {
		t.Run(kind, func(t *testing.T) {
			x := corpusFixture(t)
			item := x["news"].([]any)[0].(map[string]any)
			src := item["source"].(map[string]any)
			switch kind {
			case "week":
				x["week"] = replayWeek{ID: "wrong"}
			case "future":
				src["archived_at"] = "2026-10-28T00:00:00Z"
			case "url":
				item["url"] = "http://127.0.0.1/admin"
			case "archive":
				src["archive_url"] = "https://attacker.example/261020/h1200"
			}
			b, _ := json.Marshal(x)
			if _, e := readImportedCorpus(bytes.NewReader(b), lastNewsWeek(fixtureTime())); e == nil {
				t.Fatal("Invalid import accepted", kind)
			}
		})
	}
}
func TestImportCannotReplaceCommittedPortfolio(t *testing.T) {
	s := testServer(t)
	s.clock = fixtureTime
	s.initGame()
	s.game.Round.Status = "settled"
	b, _ := json.Marshal(corpusFixture(t))
	r := httptest.NewRequest("POST", "http://example.com/api/game/import", bytes.NewReader(b))
	w := httptest.NewRecorder()
	s.importGameWeek(w, r)
	if w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestActualNewsExportImport(t *testing.T) {
	path := os.Getenv("LIVE_CORPUS_FILE")
	if path == "" {
		t.Skip("Set LIVE_CORPUS_FILE to validate an actual current-week export")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	manifest, err := readImportedCorpus(f, lastNewsWeek(time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Accepted %d actual archived stories; uploaded model outputs are excluded", len(manifest.Stories))
}

func TestImportAcceptsNewsInTheExactCutoffSnapshot(t *testing.T) {
	x := corpusFixture(t)
	src := x["news"].([]any)[0].(map[string]any)["source"].(map[string]any)
	src["published"] = "2026-10-25"
	src["archive_id"] = "261025p99"
	src["archived_at"] = "2026-10-26T04:00:00Z"
	src["archive_url"] = "https://www.techmeme.com/261026/h0000"
	b, _ := json.Marshal(x)
	if _, e := readImportedCorpus(bytes.NewReader(b), lastNewsWeek(fixtureTime())); e != nil {
		t.Fatal(e)
	}
}
