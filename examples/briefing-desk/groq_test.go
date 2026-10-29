package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGroqNativeToolContinuation(t *testing.T) {
	key := filepath.Join(t.TempDir(), "key")
	os.WriteFile(key, []byte("fixture-private-key"), 0600)
	turn := 0
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		turn++
		if r.URL.Path != "/responses" || r.Header.Get("Authorization") != "Bearer fixture-private-key" || r.Header.Get("X-OGX-Provider-Data") != "" {
			t.Error("Incorrect provider authentication or endpoint")
		}
		b, _ := io.ReadAll(r.Body)
		var in map[string]any
		json.Unmarshal(b, &in)
		if _, ok := in["store"]; ok || strings.Contains(string(b), "fixture-private-key") || in["model"] != "qwen/qwen3.8-27b" {
			t.Error("Unsupported retention parameter, credential exposure or model fallback")
		}
		var output any
		if turn == 1 {
			if in["tool_choice"] != "required" {
				t.Error("Evidence retrieval was optional")
			}
			output = map[string]string{"type": "function_call", "name": "read_article", "call_id": "call_article", "arguments": `{"source_id":"original"}`}
		} else {
			if len(in["tools"].([]any)) != 0 || !strings.Contains(string(b), "json_schema") {
				t.Error("Final response must use JSON mode without tools")
			}
			if !strings.Contains(string(b), "function_call_output") || !strings.Contains(string(b), "A fixture report") {
				t.Error("Actual tool result missing from continuation")
			}
			output = map[string]any{"type": "message", "content": []any{map[string]string{"type": "output_text", "text": `{"title":"Fixture summary","tldr":"A fixture report confirms synthetic evidence for this test only.","points":["Fixture evidence one","Fixture evidence two"],"overview":"A fixture report provides enough text to validate real tool continuation."}`}}}
		}
		reply(w, 200, map[string]any{"status": "completed", "model": "qwen/qwen3.8-27b", "output": []any{output}})
	}))
	defer up.Close()
	a, _ := newGroqAgent("qwen/qwen3.8-27b", key)
	a.base = up.URL
	a.fetch.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/html"}}, Body: io.NopCloser(strings.NewReader("<main>" + strings.Repeat("A fixture report confirms synthetic evidence. ", 20) + "</main>")), Request: r}, nil
	})
	_, out, e := a.summarize(context.Background(), "https://example.com/news", nil)
	if e != nil || out.Title != "Fixture summary" || turn != 2 {
		t.Fatal("Native Groq continuation failed", e)
	}
}

func TestGroqQuotaAndModelMismatchFailClosed(t *testing.T) {
	key := filepath.Join(t.TempDir(), "key")
	os.WriteFile(key, []byte("fixture-private-key"), 0600)
	for _, status := range []int{429, 200} {
		up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if status == 429 {
				w.WriteHeader(429)
				io.WriteString(w, "secret-provider-error fixture-private-key")
			} else {
				reply(w, 200, map[string]any{"status": "completed", "model": "other-model", "output": []any{}})
			}
		}))
		a, _ := newGroqAgent("qwen/qwen3.8-27b", key)
		a.base = up.URL
		var out any
		e := a.respond(context.Background(), a.job(context.Background(), "collect", "techmeme", "", nil), "fixture", &out)
		if e == nil || strings.Contains(e.Error(), "fixture-private-key") || strings.Contains(e.Error(), "secret-provider-error") {
			t.Fatal("Quota/model mismatch was not safely surfaced", e)
		}
		up.Close()
	}
	if _, e := newGroqAgent("another-model", key); e == nil {
		t.Fatal("Unconfigured model accepted")
	}
}

func TestGroqCooldownKeepsModelAndHonorsCancellation(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		calls := 0
		ctx, cancel := context.WithCancel(context.Background())
		up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if calls == 1 {
				w.Header().Set("Retry-After", "0.01")
				w.WriteHeader(429)
				if canceled {
					cancel()
				}
				return
			}
			reply(w, 200, map[string]any{"status": "completed", "model": "qwen/qwen3.8-27b", "output": []any{map[string]any{"type": "message", "content": []any{map[string]string{"type": "output_text", "text": `{"ready":true}`}}}}})
		}))
		a, _ := newGroqAgent("qwen/qwen3.8-27b", "")
		a.base = up.URL
		_, e := a.nativeResponse(ctx, a.job(ctx, "collect", "techmeme", "", nil), []byte(`{"model":"qwen/qwen3.8-27b"}`), "fixture-key")
		if canceled && (e == nil || calls != 1) || !canceled && (e != nil || calls != 2) {
			t.Fatal("Cooldown or cancellation failed", e, calls)
		}
		cancel()
		up.Close()
	}
	for _, raw := range []string{"", "NaN", "+Inf", "86400", "-1"} {
		if _, ok := inferenceRetryAfter(raw); ok {
			t.Fatal("Invalid or daily quota cooldown accepted", raw)
		}
	}
}

func TestLiveGroqCollection(t *testing.T) {
	if os.Getenv("LIVE_GROQ_TESTS") != "1" {
		t.Skip("opt-in live Groq and public RSS check; requires a Free account key")
	}
	a, e := newGroqAgent("qwen/qwen3.8-27b", env("GROQ_KEY_FILE", ""))
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	items, e := a.collect(ctx, "techmeme", func(ev event) { t.Log(ev.Stage, ev.Message) })
	if e != nil || len(items) < 1 {
		t.Fatal("Live Groq/Techmeme collection failed", e)
	}
	t.Log("Actual verified Techmeme headlines selected:", len(items))
}

func TestLiveHostedResearch(t *testing.T) {
	if os.Getenv("LIVE_RESEARCH") != "1" {
		t.Skip("opt-in direct research probe; does not alter classifications or the app's NO SIGNAL gate")
	}
	b, e := os.ReadFile(os.Getenv("LIVE_SOURCE_FILE"))
	if e != nil {
		t.Fatal("Set LIVE_SOURCE_FILE to a locally saved, real article document")
	}
	var d document
	if json.Unmarshal(b, &d) != nil || d.Status != "ready" {
		t.Fatal("Expected a successfully imported real article")
	}
	var a *agent
	if env("AGENT_BACKEND", "groq") == "azure" {
		a, e = newAzureAgent(env("AZURE_FOUNDRY_URL", ""), "deepseekv4pro", env("AZURE_KEY_FILE", ""))
	} else {
		a, e = newGroqAgent("qwen/qwen3.8-27b", env("GROQ_KEY_FILE", ""))
	}
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	r, e := a.investigate(ctx, d, defaults()[0], func(ev event) { t.Log(ev.Stage, ev.Message) })
	if e != nil {
		t.Fatal(e)
	}
	tools := []string{}
	for _, ev := range r.Evidence {
		tools = append(tools, ev.Tool)
	}
	t.Log("Direct research probe on actual news; app classification unchanged:", "tools", tools, "direction", r.Report.Plan.Direction)
	if path := os.Getenv("LIVE_REPORT_FILE"); path != "" {
		b, _ := json.MarshalIndent(map[string]any{"passed": true, "scope": "Direct research probe on a real imported article; app NO SIGNAL gate unchanged", "model": a.model, "source_url": d.URL, "tools": tools, "direction": r.Report.Plan.Direction, "pricing_error": r.Report.Plan.PricingError, "at": stamp()}, "", "  ")
		if e := os.WriteFile(path, append(b, '\n'), 0600); e != nil {
			t.Fatal(e)
		}
	}
}
