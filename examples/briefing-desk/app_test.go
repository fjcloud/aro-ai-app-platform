package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPublicDestinations(t *testing.T) {
	for _, v := range []string{"http://127.0.0.1/", "http://169.254.169.254/latest/meta-data/", "http://[::1]/", "http://[::ffff:127.0.0.1]/", "https://user:pass@example.com/", "file:///etc/passwd", "https://example.com:8080/", "http://localhost/"} {
		if _, e := publicURL(v); e == nil {
			t.Errorf("accepted %s", v)
		}
	}
	for _, raw := range []string{"100.100.100.100", "192.0.2.3", "198.18.1.1", "64:ff9b::7f00:1", "2002:7f00:1::", "fc00::1"} {
		if publicIP(net.ParseIP(raw)) {
			t.Errorf("accepted reserved IP %s", raw)
		}
	}
	if !publicIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("rejected public IP")
	}
	ctx, c := context.WithTimeout(context.Background(), time.Second)
	defer c()
	if _, _, _, e := newFetcher().get(ctx, "http://127.0.0.1/"); e == nil {
		t.Fatal("fetch allowed loopback")
	}
}
func TestReadableSourceAndEscaping(t *testing.T) {
	title, text := plainHTML([]byte(`<html><title>A title</title><nav>nav</nav><main>Real article <p>More facts.</p></main><script>malicious()</script></html>`))
	if title != "A title" || strings.Contains(text, "malicious") || strings.Contains(text, "nav") {
		t.Fatalf("unexpected extraction %s %s", title, text)
	}
	s := documentMarkdown(document{Summary: summary{Title: "<script>alert(1)</script>", TLDR: "[click](javascript:evil)", Points: []string{"*text*"}}, Source: source{FinalURL: "https://example.com/(test)"}})
	if strings.Contains(s, "<script>") || strings.Contains(s, "[click]") {
		t.Fatal("Markdown injection was not escaped")
	}
}
func TestPersistentStateAndStaleMandate(t *testing.T) {
	dir := t.TempDir()
	s, e := openStore(dir)
	if e != nil {
		t.Fatal(e)
	}
	d, _, _ := s.add("https://example.com/article")
	sum := summary{Title: "Stored"}
	s.update(d.ID, func(d *document) {
		d.Status = "ready"
		d.Summary = sum
		d.Classes["tech"] = classification{Status: "done", ProfileVersion: 1, SummaryHash: sum.hash(), Priority: "buy"}
		d.Research["tech"] = research{Status: "running"}
	})
	p, _ := s.profile("tech")
	p.Criteria += " Additional requirements."
	s.saveProfile(p)
	s2, e := openStore(dir)
	if e != nil {
		t.Fatal(e)
	}
	docs, _ := s2.list()
	if len(docs) != 1 || !docs[0].Classes["tech"].Stale || docs[0].Research["tech"].Status != "failed" {
		t.Fatalf("invalid persisted state %+v", docs)
	}
	copy, _ := s2.get(d.ID)
	copy.Classes["tech"] = classification{}
	again, _ := s2.get(d.ID)
	if again.Classes["tech"].Status != "done" {
		t.Fatal("store returned mutable aliases")
	}
}
func TestNativeResponsesCredentialsAndNoFabricatedToolSuccess(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "key")
	os.WriteFile(keyFile, []byte("private-test-key"), 0600)
	native := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" || !strings.Contains(r.Header.Get("X-OGX-Provider-Data"), "private-test-key") {
			t.Error("native provider auth missing")
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		tool := body["tools"].([]any)[0].(map[string]any)
		if tool["type"] != "function" || tool["name"] != "read_article" {
			t.Error("Expected native Responses function tool")
		}
		if body["store"] != false || strings.Contains(fmt.Sprint(body["input"]), "private-test-key") {
			t.Error("credential retention risk")
		}
		reply(w, 200, map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{map[string]string{"type": "output_text", "text": `{"title":"Fake summary","tldr":"This summary was never supported by a tool.","points":["a","b"],"overview":"An invented source description."}`}}}}})
	}))
	defer native.Close()
	a, _ := newAgent(native.URL+"/v1", "native", keyFile)
	_, _, e := a.summarize(context.Background(), "https://example.com/", nil)
	if e == nil || !strings.Contains(e.Error(), "successfully read") {
		t.Fatal("accepted summary without actual tool read", e)
	}
}
func TestClassifierRejectsIncompleteCoverage(t *testing.T) {
	for _, coverage := range []string{"truncated", "complete"} {
		t.Run(coverage, func(t *testing.T) {
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload map[string]any
				json.NewDecoder(r.Body).Decode(&payload)
				if !strings.Contains(fmt.Sprint(payload["state"]), "mandate") {
					t.Error("mandate missing")
				}
				reply(w, 200, map[string]any{"answers": map[string]any{"signal": map[string]any{"choice": "buy", "confidence": .7, "input_coverage": coverage, "probabilities": map[string]float64{"buy": .7, "short": .2, "no_signal": .1}}}})
			}))
			defer up.Close()
			c, _ := newDecisionClient(up.URL, "test", "")
			_, e := c.classify(context.Background(), summary{Title: "Article"}, defaults()[0])
			if (e == nil) != (coverage == "complete") {
				t.Fatal("coverage validation failed", e)
			}
		})
	}
}
func testServer(t *testing.T) *server {
	t.Helper()
	st, e := openStore(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	a, _ := newAgent("http://localhost:1234/v1", "native", "missing")
	c, _ := newDecisionClient("http://localhost:1235", "local", "")
	return &server{store: st, agent: a, model: c, ctx: context.Background(), imports: make(chan struct{}, 2), classes: make(chan struct{}, 1), researches: make(chan struct{}, 1)}
}
func TestHTTPFinancialGateAndCSRF(t *testing.T) {
	s := testServer(t)
	d, _, _ := s.store.add("https://example.com/article")
	s.store.update(d.ID, func(d *document) {
		d.Status = "ready"
		d.Summary = summary{Title: "Article"}
		d.Classes["tech"] = classification{Status: "done", Priority: "no_signal", ProfileVersion: 1, SummaryHash: d.Summary.hash()}
	})
	h := s.routes()
	for _, tc := range []struct {
		origin string
		status int
	}{{"https://evil.example.com", 403}, {"", 409}} {
		r := httptest.NewRequest("POST", "http://desk.example.com/api/articles/"+d.ID+"/investigate", strings.NewReader(`{"profile_id":"tech"}`))
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("got %d want %d", w.Code, tc.status)
		}
	}
	r := httptest.NewRequest("GET", "http://desk.example.com/api/export/"+d.ID+"?type=research&profile=tech", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 409 {
		t.Fatal("exported missing research")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestNativeToolLoopRetrievesRealResults(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "key")
	os.WriteFile(keyFile, []byte("private-key"), 0600)
	turns := 0
	native := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		turns++
		var body struct {
			Input []json.RawMessage `json:"input"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if turns == 1 {
			reply(w, 200, map[string]any{"status": "completed", "output": []any{map[string]string{"type": "function_call", "name": "get_headlines", "call_id": "call1", "arguments": "{}"}}})
			return
		}
		if len(body.Input) != 3 || !strings.Contains(string(body.Input[2]), "A real source headline") || !strings.Contains(string(body.Input[2]), "E1") {
			t.Error("Native agent did not receive actual tool evidence")
		}
		reply(w, 200, map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{map[string]string{"type": "output_text", "text": `{"headline_ids":["H1"]}`}}}}})
	}))
	defer native.Close()
	a, _ := newAgent(native.URL+"/v1", "native", keyFile)
	a.fetch = &fetcher{&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`<rss><channel><item><title>A real source headline</title><link>https://example.com/story</link><pubDate>today</pubDate><description>Source excerpt</description></item></channel></rss>`)), Header: http.Header{"Content-Type": []string{"text/xml"}}, Request: r}, nil
	})}}
	events := []event{}
	items, e := a.collect(context.Background(), "techmeme", func(e event) { events = append(events, e) })
	if e != nil || len(items) != 1 || turns != 2 || len(events) < 3 {
		t.Fatalf("Tool loop failed: %v %v", items, e)
	}
	j := a.job(context.Background(), "summarize", "", "https://example.com/article", nil)
	if _, _, e := a.runTool(context.Background(), j, "sec_filings", json.RawMessage(`{"ticker":"NVDA"}`)); e == nil {
		t.Fatal("Tool escaped stage scope")
	}
	if _, _, e := a.runTool(context.Background(), j, "read_article", json.RawMessage(`{"source_id":"invented"}`)); e == nil {
		t.Fatal("Agent invented an allowed URL")
	}
	if _, _, e := a.runTool(context.Background(), j, "read_article", json.RawMessage(`{"source_id":"original","shell":"evil"}`)); e == nil {
		t.Fatal("Unexpected tool argument accepted")
	}
}

func TestStructuredProviderEnvelope(t *testing.T) {
	for _, v := range []string{`{"ok":true}`, "Here is the result:\n```json\n{\"ok\":true}\n```"} {
		var out struct {
			OK bool `json:"ok"`
		}
		if e := decodeStructured(v, &out); e != nil || !out.OK {
			t.Fatal("Valid structured envelope rejected", e)
		}
	}
	for _, v := range []string{"No result", "```json\n{}\n```\n```json\n{}\n```", "{} trailing text"} {
		var out any
		if decodeStructured(v, &out) == nil {
			t.Fatal("Accepted invalid or ambiguous result")
		}
	}
}

func TestSourceReferencesSurviveRepeatedSearches(t *testing.T) {
	a, _ := newAgent("http://localhost:1234/v1", "native", "missing")
	calls := 0
	a.fetch = &fetcher{&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		body := fmt.Sprintf(`<rss><channel><item><title>Relevant economy news</title><link>https://example.com/story-%d</link></item></channel></rss>`, calls)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
	})}}
	j := a.job(context.Background(), "investigate", "", "https://example.com/original", nil)
	a.runTool(context.Background(), j, "search_news", json.RawMessage(`{"query":"economy"}`))
	first := j.refs["N1"]
	if first == "" {
		t.Fatal("Missing first source reference")
	}
	a.runTool(context.Background(), j, "search_news", json.RawMessage(`{"query":"economy"}`))
	if j.refs["N1"] != first || len(j.refs) != 3 {
		t.Fatal("A later search reassigned an existing source ID")
	}
}
