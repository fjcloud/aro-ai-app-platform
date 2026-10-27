package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image/png"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRulesPriorityAndExceptions(t *testing.T) {
	base := dossier{Name: "NOVA VALE", PermitName: "NOVA VALE", Destination: "EUROPA", Expires: "2037-06-30", Cargo: "MACHINE PARTS", Licence: "PRESENT", Seal: "INTACT", Status: "COMMERCIAL"}
	tests := []struct {
		name   string
		rule   int
		change func(*dossier)
		want   string
	}{
		{"valid", 0, func(*dossier) {}, "clear"},
		{"expiry inclusive", 0, func(d *dossier) { d.Expires = "2037-04-17" }, "clear"},
		{"expired", 0, func(d *dossier) { d.Expires = "2037-04-16" }, "reject"},
		{"names differ", 0, func(d *dossier) { d.PermitName = "ARLO SOL" }, "reject"},
		{"destination", 0, func(d *dossier) { d.Destination = "IO" }, "reject"},
		{"missing licence", 0, func(d *dossier) { d.Licence = "MISSING" }, "inspect"},
		{"medical exemption", 0, func(d *dossier) { d.Cargo = "MEDICAL SUPPLIES"; d.Licence = "MISSING"; d.Seal = "SEALED" }, "clear"},
		{"medical exemption withdrawn", 1, func(d *dossier) { d.Cargo = "MEDICAL SUPPLIES"; d.Licence = "MISSING"; d.Seal = "SEALED" }, "inspect"},
		{"relic quarantine", 1, func(d *dossier) { d.Cargo = "ANCIENT RELICS" }, "inspect"},
		{"diplomatic exemption", 2, func(d *dossier) { d.Status = "DIPLOMATIC"; d.Licence = "MISSING" }, "clear"},
		{"ban beats diplomacy", 2, func(d *dossier) { d.Status = "DIPLOMATIC"; d.Cargo = "VOID CRYSTALS" }, "reject"},
		{"broken seal beats diplomacy", 2, func(d *dossier) { d.Status = "DIPLOMATIC"; d.Cargo = "MEDICAL SUPPLIES"; d.Seal = "BROKEN" }, "inspect"},
		{"identity beats exemption", 2, func(d *dossier) { d.Status = "DIPLOMATIC"; d.PermitName = "ARLO SOL" }, "reject"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := base
			tt.change(&d)
			got, why := verdict(d, directives[tt.rule])
			if got != tt.want || why == "" {
				t.Fatalf("got %s (%s), want %s", got, why, tt.want)
			}
		})
	}
}

func TestGPUWaitCancellationAndRelease(t *testing.T) {
	slots := make(chan struct{}, 1)
	slots <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := acquireGPU(ctx, slots); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled queued request: %v", err)
	}
	if len(slots) != 1 {
		t.Fatal("cancelled request changed the active slot")
	}
	started, done := make(chan struct{}), make(chan error, 1)
	go func() { close(started); done <- acquireGPU(context.Background(), slots) }()
	<-started
	select {
	case err := <-done:
		t.Fatalf("occupied GPU did not wait: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	<-slots
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("queued request did not acquire the freed GPU")
	}
	if len(slots) != 1 {
		t.Fatal("acquired request lost its slot")
	}
	<-slots
}

func TestDeckImagesAndScores(t *testing.T) {
	a, b := newShift(42), newShift(42)
	seen := map[string]bool{}
	for i, d := range a.Cases {
		if d != b.Cases[i] {
			t.Fatal("seed changed documents")
		}
		a.Round = i
		expected, _ := verdict(d, a.directive())
		seen[expected] = true
		image, err := png.Decode(bytes.NewReader(paper(d)))
		if err != nil || image.Bounds().Dx() != 1000 || image.Bounds().Dy() != 650 {
			t.Fatal("document image is not the shared 1000x650 evidence")
		}
	}
	if len(seen) != 3 {
		t.Fatal("test deck must exercise all dispositions")
	}
	a.Round = 0
	v, _ := verdict(a.Cases[0], a.directive())
	a.resolve(v, decision{Choice: v})
	if a.Score != 100 || a.HumanCorrect != 1 || a.ModelCorrect != 1 {
		t.Fatal("correct result not scored")
	}
	a.Round++
	a.resolve("", decision{Choice: "clear"})
	if a.HumanPlayed != 1 || a.Score != 100 {
		t.Fatal("observer round scored as human play")
	}
}

func TestHTTPImageProtocolSessionsAndAtomicFailure(t *testing.T) {
	var broken atomic.Bool
	var calls atomic.Int32
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if broken.Load() {
			w.WriteHeader(503)
			w.Write([]byte("private-upstream-fixture"))
			return
		}
		var payload struct {
			State     string         `json:"state"`
			Images    []string       `json:"images"`
			Questions map[string]any `json:"questions"`
		}
		if json.NewDecoder(r.Body).Decode(&payload) != nil || len(payload.Images) != 1 || payload.Questions["verdict"] == nil {
			t.Error("not an image decision request")
		}
		if strings.Contains(payload.State, "ZURI VOSS") || strings.Contains(payload.State, "EP-5653") {
			t.Error("extracted private document fields leaked into text")
		}
		image, e := base64.StdEncoding.DecodeString(strings.TrimPrefix(payload.Images[0], "data:image/png;base64,"))
		if e != nil || !bytes.Equal(image, paper(newShift(42).Cases[min(int(calls.Load()-1), 1)])) {
			// Subsequent atomicity checks can inspect another round; all images must
			// still be valid PNGs. The first call must be byte-identical evidence.
			if calls.Load() == 1 {
				t.Error("model did not receive the player's exact image")
			}
		}
		if _, e = png.Decode(bytes.NewReader(image)); e != nil {
			t.Error("invalid PNG payload")
		}
		reply(w, 200, map[string]any{"answers": map[string]any{"verdict": map[string]any{"choice": "clear", "confidence": .8, "probabilities": map[string]float64{"clear": .8, "inspect": .1, "reject": .1}, "input_coverage": "complete"}}, "usage": map[string]int{"input_tokens": 1000}})
	}))
	defer model.Close()
	c, e := newDecisionClient(model.URL, "vllm-sr/d3-mini", "")
	if e != nil {
		t.Fatal(e)
	}
	s := &server{sessions: map[string]*session{}, model: c, renderDocument: paper, slots: make(chan struct{}, 1)}
	app := httptest.NewServer(s.routes())
	defer app.Close()
	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar}
	request := func(path, body string) (int, []byte) {
		t.Helper()
		var r *http.Response
		var err error
		if body == "" {
			r, err = browser.Get(app.URL + path)
		} else {
			r, err = browser.Post(app.URL+path, "application/json", strings.NewReader(body))
		}
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		var out bytes.Buffer
		out.ReadFrom(r.Body)
		return r.StatusCode, out.Bytes()
	}
	status, raw := request("/api/shift", "")
	var v view
	if json.Unmarshal(raw, &v) != nil || status != 200 {
		t.Fatal("initial view failed")
	}
	for _, secret := range []string{`"cases"`, `"expected"`, `"permit_name"`, "ZURI VOSS"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("answer key/private fields exposed before play: %s", secret)
		}
	}
	if st, _ := request(v.Document, ""); st != 200 {
		t.Fatal("document unavailable")
	}
	body := func(action string) string {
		b, _ := json.Marshal(map[string]any{"id": v.ID, "round": v.Round, "action": action})
		return string(b)
	}
	status, raw = request("/api/shift/decide", body("clear"))
	if status != 200 {
		t.Fatal(string(raw))
	}
	json.Unmarshal(raw, &v)
	if v.Result == nil || !v.Result.ModelCorrect || v.HumanCorrect != 1 {
		t.Fatal("first verdict not resolved")
	}
	before := calls.Load()
	if st, _ := request("/api/shift/decide", body("clear")); st != 409 || calls.Load() != before {
		t.Fatal("duplicate stamp called model")
	}
	next, _ := json.Marshal(map[string]any{"id": v.ID, "round": v.Round})
	status, raw = request("/api/shift/next", string(next))
	if status != 200 {
		t.Fatal("next failed")
	}
	json.Unmarshal(raw, &v)
	broken.Store(true)
	status, raw = request("/api/shift/decide", body("reject"))
	if status != 502 || strings.Contains(string(raw), "private-upstream-fixture") {
		t.Fatal("upstream failure not safely handled")
	}
	_, raw = request("/api/shift", "")
	json.Unmarshal(raw, &v)
	if v.Result != nil || v.Round != 1 || v.HumanPlayed != 1 || len(v.History) != 1 {
		t.Fatal("failed inference changed game or applied fallback")
	}
	broken.Store(false)
	status, raw = request("/api/shift/decide", body("reject"))
	if status != 200 {
		t.Fatal(string(raw))
	}
	json.Unmarshal(raw, &v)
	if v.Result.ModelCorrect || !v.Result.HumanCorrect {
		t.Fatal("wrong model answer was hidden or automatically corrected")
	}
	req, _ := http.NewRequest("POST", app.URL+"/api/shift/new", strings.NewReader(`{"seed":7}`))
	req.Header.Set("Origin", "https://unrelated.invalid")
	r, err := browser.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != 403 {
		t.Fatal("cross-origin mutation accepted")
	}
	if _, e := newDecisionClient("https://hosted.invalid/v1/systemone", "d3-mini", ""); e == nil {
		t.Fatal("hosted inference accepted")
	}
}

func TestDocumentRenderingDoesNotBlockShiftMutation(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	c, err := newDecisionClient("http://127.0.0.1:1/v1/systemone", "d3-mini", "")
	if err != nil {
		t.Fatal(err)
	}
	s := &server{sessions: map[string]*session{}, model: c, slots: make(chan struct{}, 1),
		renderDocument: func(d dossier) []byte { close(entered); <-release; return paper(d) }}
	app := httptest.NewServer(s.routes())
	defer app.Close()
	defer once.Do(func() { close(release) })
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	r, err := client.Get(app.URL + "/api/shift")
	if err != nil {
		t.Fatal(err)
	}
	var v view
	json.NewDecoder(r.Body).Decode(&v)
	r.Body.Close()
	done := make(chan error, 1)
	go func() {
		r, err := client.Get(app.URL + v.Document)
		if err == nil {
			r.Body.Close()
		}
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("document renderer never started")
	}
	// A deliberately slow image render must not make the server report that an
	// inspection is running or prevent the user starting another shift.
	r, err = client.Post(app.URL+"/api/shift/new", "application/json", strings.NewReader(`{"seed":7}`))
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != 200 {
		t.Fatalf("image generation blocked a valid shift mutation: HTTP %d", r.StatusCode)
	}
	once.Do(func() { close(release) })
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
