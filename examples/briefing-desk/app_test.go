package main

import (
	"context"
	"encoding/json"
	"fmt"
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
		d.Classes["macro"] = classification{Status: "done", ProfileVersion: 1, SummaryHash: sum.hash(), Priority: "material"}
		d.Research["macro"] = research{Status: "running"}
	})
	p, _ := s.profile("macro")
	p.Criteria += " Additional requirements."
	s.saveProfile(p)
	s2, e := openStore(dir)
	if e != nil {
		t.Fatal(e)
	}
	docs, _ := s2.list()
	if len(docs) != 1 || !docs[0].Classes["macro"].Stale || docs[0].Research["macro"].Status != "failed" {
		t.Fatalf("invalid persisted state %+v", docs)
	}
	copy, _ := s2.get(d.ID)
	copy.Classes["macro"] = classification{}
	again, _ := s2.get(d.ID)
	if again.Classes["macro"].Status != "done" {
		t.Fatal("store returned mutable aliases")
	}
}
func TestMCPJobAuthAndScope(t *testing.T) {
	a, _ := newAgent("http://localhost:1234/v1", "native", "missing", "http://tools.example.com:8090/mcp")
	h := a.mcpHandler()
	request := func(token, host, origin string) int {
		r := httptest.NewRequest("POST", "http://"+host+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		r.Header.Set("X-Signal-Job", "job")
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	j := a.job(context.Background(), "summarize", "", "https://example.com/article", nil)
	a.mu.Lock()
	a.jobs["job"] = j
	a.mu.Unlock()
	if request("bad", "tools.example.com:8090", "") != 401 {
		t.Fatal("unauthorized job accepted")
	}
	if request(j.token, "evil.example.com:8090", "") != 403 || request(j.token, "tools.example.com:8090", "https://evil.example.com") != 403 {
		t.Fatal("host/origin bypass")
	}
	if c := request(j.token, "tools.example.com:8090", ""); c != 200 {
		t.Fatalf("valid MCP initialization failed %d", c)
	}
	if _, _, e := a.runTool(context.Background(), j, "sec_filings", json.RawMessage(`{"ticker":"NVDA"}`)); e == nil {
		t.Fatal("summarizer escaped job scope")
	}
	if _, _, e := a.runTool(context.Background(), j, "read_article", json.RawMessage(`{"url":"https://other.example.com/"}`)); e == nil {
		t.Fatal("model invented allowed URL")
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
		if body["store"] != false || strings.Contains(fmt.Sprint(body["input"]), "private-test-key") {
			t.Error("credential retention risk")
		}
		reply(w, 200, map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{map[string]string{"type": "output_text", "text": `{"title":"Fake summary","tldr":"This summary was never supported by a tool.","points":["a","b"],"overview":"An invented source description."}`}}}}})
	}))
	defer native.Close()
	a, _ := newAgent(native.URL+"/v1", "native", keyFile, "http://tools.example.com:8090/mcp")
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
				reply(w, 200, map[string]any{"answers": map[string]any{"relevance": map[string]any{"choice": "material", "confidence": .7, "input_coverage": coverage, "probabilities": map[string]float64{"material": .7, "monitor": .2, "not_financial": .05, "insufficient_context": .05}}}})
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
	a, _ := newAgent("http://localhost:1234/v1", "native", "missing", "http://tools.example.com:8090/mcp")
	c, _ := newDecisionClient("http://localhost:1235", "local", "")
	return &server{store: st, agent: a, model: c, ctx: context.Background(), imports: make(chan struct{}, 2), classes: make(chan struct{}, 1), researches: make(chan struct{}, 1)}
}
func TestHTTPFinancialGateAndCSRF(t *testing.T) {
	s := testServer(t)
	d, _, _ := s.store.add("https://example.com/article")
	s.store.update(d.ID, func(d *document) {
		d.Status = "ready"
		d.Summary = summary{Title: "Article"}
		d.Classes["macro"] = classification{Status: "done", Priority: "not_financial", ProfileVersion: 1, SummaryHash: d.Summary.hash()}
	})
	h := s.routes()
	for _, tc := range []struct {
		origin string
		status int
	}{{"https://evil.example.com", 403}, {"", 409}} {
		r := httptest.NewRequest("POST", "http://desk.example.com/api/articles/"+d.ID+"/investigate", strings.NewReader(`{"profile_id":"macro"}`))
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("got %d want %d", w.Code, tc.status)
		}
	}
	r := httptest.NewRequest("GET", "http://desk.example.com/api/export/"+d.ID+"?type=research&profile=macro", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 409 {
		t.Fatal("exported missing research")
	}
}
