package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestContinuousSeparationAndProtection(t *testing.T) {
	a := flight{ID: "A", Phase: "approach", Position: point{-4, 0, 5000}}
	b := flight{ID: "B", Phase: "approach", Position: point{0, -4, 5000}}
	an, bn := a, b
	an.Position = point{4, 0, 5000}
	bn.Position = point{0, 4, 5000}
	if !trajectoriesConflict(a, b, an, bn, 0) {
		t.Fatal("between-frame crossing was missed")
	}
	b.Position.Z = 6200
	bn.Position.Z = 6200
	if trajectoriesConflict(a, b, an, bn, 0) {
		t.Fatal("vertically separated flights conflict")
	}
	w := newWorld(42, settings{6, 0, 8})
	w.Flights = []flight{a, {ID: "B", Phase: "approach", Position: point{-3.5, 0, 5000}}}
	w.advance(.1)
	if w.Stopped == "" || w.Stats.SafetyStops != 1 || w.Stats.Crashes != 0 || w.Seconds != 0 {
		t.Fatal("unsafe movement did not stop before a collision")
	}
}

func TestSaturationReasonsAndLateClearanceRejection(t *testing.T) {
	c, _ := newDecisionClient("http://127.0.0.1:1", "d3-mini", "")
	s := &server{model: c, slots: make(chan struct{}, 1)}
	g := &session{world: newWorld(42, settings{24, 4, 60}), playing: true, job: 1, generation: 1}
	g.world.Backlog = 100
	if got := s.snapshot(g).State; got != "capacity" {
		t.Fatalf("capacity saturation: %s", got)
	}
	g.history = []event{{Status: "late", TotalMS: 3000, BudgetMS: 750}}
	if got := s.snapshot(g).State; got != "critical" {
		t.Fatalf("airport backlog hid inference overload: %s", got)
	}
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			Questions map[string]struct {
				Criteria map[string]string `json:"criteria"`
			} `json:"questions"`
		}
		json.NewDecoder(r.Body).Decode(&p)
		probs := map[string]float64{}
		for k := range p.Questions["clearance"].Criteria {
			probs[k] = 0
		}
		probs["wait"] = 1
		time.Sleep(900 * time.Millisecond)
		reply(w, 200, map[string]any{"answers": map[string]any{"clearance": map[string]any{"choice": "wait", "confidence": 1, "probabilities": probs, "input_coverage": "complete"}}})
	}))
	defer model.Close()
	s.model, _ = newDecisionClient(model.URL, "d3-mini", "")
	g.pending = true
	s.infer(context.Background(), g, g.world.clone(), 1, 1)
	if g.world.Stats.Late != 1 || g.world.Stats.Accepted != 0 || g.world.Stats.Errors != 0 || g.history[len(g.history)-1].Status != "late" {
		t.Fatal("a valid but overdue response was applied or hidden")
	}
}
func TestClearancesReservationsAndSeededProgress(t *testing.T) {
	w := newWorld(42, settings{6, 0, 8})
	copy := newWorld(42, settings{6, 0, 8})
	for i, p := range w.Flights {
		if p.ID != copy.Flights[i].ID || p.Position != copy.Flights[i].Position {
			t.Fatal("seed changed initial traffic")
		}
	}
	options := w.candidates()
	a := w.reference(options)
	if a.Kind == "wait" {
		t.Fatal("initial sector has no useful safe clearance")
	}
	if e := w.apply(a); e != nil {
		t.Fatal(e)
	}
	if e := w.apply(a); e == nil {
		t.Fatal("duplicate clearance accepted")
	}
	closed := w.clone()
	closed.Runways[0].Closed = true
	for _, c := range closed.candidates() {
		if c.Runway == "09L" && c.Safe {
			t.Fatal("closed runway offered as safe")
		}
	}
	// Many deterministic physics steps with independently selected reference
	// clearances exercise landing, takeoff, taxi and occupied-runway protection.
	for n := 0; n < 3600 && w.Stopped == ""; n++ {
		if n%15 == 0 {
			c := w.reference(w.candidates())
			if err := w.apply(c); err != nil {
				t.Fatal(err)
			}
		}
		w.advance(1)
	}
	if w.Stats.Crashes != 0 {
		t.Fatal("reference run crashed")
	}
	if w.Stats.Landed < 2 || w.Stats.Departed < 1 {
		t.Fatalf("no useful traffic progress: %+v (%s)", w.Stats, w.Stopped)
	}
	if w.Stopped != "" {
		t.Fatalf("reference run needed a safety intervention: %s", w.Stopped)
	}
}
func TestLiveLoopDoesNotBlockOnInferenceAndRejectsStaleResponses(t *testing.T) {
	entered, release := make(chan struct{}, 1), make(chan struct{})
	var once sync.Once
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		reply(w, 200, map[string]any{"answers": map[string]any{"clearance": map[string]any{"choice": "wait", "confidence": 1, "probabilities": map[string]float64{"wait": 1}, "input_coverage": "complete"}}})
	}))
	defer model.Close()
	defer once.Do(func() { close(release) })
	c, _ := newDecisionClient(model.URL, "d3-mini", "")
	s := &server{sessions: map[string]*session{}, model: c, slots: make(chan struct{}, 1)}
	app := httptest.NewServer(s.routes())
	defer app.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.run(ctx)
	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar, Timeout: time.Second}
	r, err := browser.Get(app.URL + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	var v view
	json.NewDecoder(r.Body).Decode(&v)
	r.Body.Close()
	post := func(action string) {
		t.Helper()
		r, err := browser.Post(app.URL+"/api/control", "application/json", strings.NewReader(`{"id":"`+v.ID+`","action":"`+action+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		if r.StatusCode != 200 {
			t.Fatal("control failed")
		}
		json.NewDecoder(r.Body).Decode(&v)
	}
	post("play")
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("model never started")
	}
	before := v.World.Seconds
	time.Sleep(150 * time.Millisecond)
	r, err = browser.Get(app.URL + "/api/state")
	if err != nil {
		t.Fatal("radar blocked on inference")
	}
	json.NewDecoder(r.Body).Decode(&v)
	r.Body.Close()
	if v.World.Seconds <= before || !v.Pending {
		t.Fatal("physics did not continue while model was pending")
	}
	post("pause")
	if v.Playing {
		t.Fatal("pause blocked on inference")
	}
	once.Do(func() { close(release) })
	// The partial fake distribution is deliberately rejected by the client;
	// pausing must still discard it without creating a model-error verdict.
	time.Sleep(50 * time.Millisecond)
	r, err = browser.Get(app.URL + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	json.NewDecoder(r.Body).Decode(&v)
	r.Body.Close()
	if v.World.Stats.Accepted != 0 || v.World.Stats.Errors != 0 {
		t.Fatal("a cancelled inference changed the sector")
	}
	req, _ := http.NewRequest("POST", app.URL+"/api/control", strings.NewReader(`{}`))
	req.Header.Set("Origin", "https://unrelated.invalid")
	res, err := browser.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatal("foreign-origin control accepted")
	}
}
