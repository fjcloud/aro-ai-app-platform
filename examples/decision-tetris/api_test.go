package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRealDecisionProtocolAndFailures(t *testing.T) {
	g := newGame(42)
	moves := g.moves()
	bad := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model     string `json:"model"`
			Questions map[string]struct {
				Type     string            `json:"type"`
				Full     bool              `json:"require_full_input"`
				Criteria map[string]string `json:"criteria"`
			} `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		q := request.Questions["placement"]
		if !q.Full || q.Type != "choice" || request.Model != "vllm-sr/d3-mini" || len(q.Criteria) != len(moves) {
			t.Error("wrong model contract")
		}
		if bad {
			reply(w, 503, map[string]string{"secret": "never expose this"})
			return
		}
		prob := map[string]float64{}
		for _, m := range moves {
			prob[m.ID] = 0
		}
		prob[moves[0].ID] = 1
		reply(w, 200, map[string]any{"answers": map[string]any{"placement": map[string]any{"choice": moves[0].ID, "confidence": .9, "probabilities": prob, "input_coverage": "complete"}}, "usage": map[string]int{"input_tokens": 200}})
	}))
	defer upstream.Close()
	c, err := newDecisionClient(upstream.URL, "vllm-sr/d3-mini", "")
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.choose(context.Background(), g, moves)
	if err != nil || out.Choice != moves[0].ID || out.InputTokens != 200 {
		t.Fatalf("inference failed: %+v %v", out, err)
	}
	bad = true
	if _, err = c.choose(context.Background(), g, moves); err == nil || err.Error() != "local decision model returned HTTP 503" {
		t.Fatal("inference errors must be generic")
	}
	if _, err = newDecisionClient("https://decision.vllm-sr.ai/v1/systemone", "vllm-sr/d3-mini", ""); err == nil {
		t.Fatal("hosted inference accepted")
	}
}
func TestSessionsStaleTurnsAndOrigin(t *testing.T) {
	c, _ := newDecisionClient("http://127.0.0.1:1/v1/systemone", "vllm-sr/d3-mini", "")
	s := &server{sessions: map[string]*session{}, model: c, slots: make(chan struct{}, 1)}
	ts := httptest.NewServer(s.routes())
	defer ts.Close()
	r, err := http.Get(ts.URL + "/api/game")
	if err != nil {
		t.Fatal(err)
	}
	var initial view
	json.NewDecoder(r.Body).Decode(&initial)
	cookie := r.Cookies()[0]
	r.Body.Close()
	send := func(body any, origin string) *http.Response {
		t.Helper()
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", ts.URL+"/api/game/step", bytes.NewReader(b))
		req.AddCookie(cookie)
		req.Header.Set("Origin", origin)
		res, err := ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
		return res
	}
	body := map[string]any{"id": initial.ID, "turn": 0, "mode": "heuristic"}
	if send(body, "https://attacker.invalid").StatusCode != 403 {
		t.Fatal("cross-origin accepted")
	}
	if send(body, ts.URL).StatusCode != 200 {
		t.Fatal("baseline turn failed")
	}
	if send(body, ts.URL).StatusCode != 409 {
		t.Fatal("stale turn accepted")
	}
	body["turn"] = 1
	body["mode"] = "model"
	if send(body, ts.URL).StatusCode != 502 {
		t.Fatal("missing model silently fell back")
	}
	r, err = http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		t.Fatal("health broken")
	}
}
