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
)

func TestAzureDeepSeekToolContinuation(t *testing.T) {
	key := filepath.Join(t.TempDir(), "key")
	os.WriteFile(key, []byte("azure-fixture-private"), 0600)
	turn := 0
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		turn++
		if r.URL.Path != "/chat/completions" || r.Header.Get("api-key") != "azure-fixture-private" || r.Header.Get("X-OGX-Provider-Data") != "" || r.Header.Get("Authorization") != "" {
			t.Error("Wrong Azure endpoint or credential header")
		}
		b, _ := io.ReadAll(r.Body)
		var in map[string]any
		json.Unmarshal(b, &in)
		if _, ok := in["reasoning"]; ok || in["reasoning_effort"] != "none" || in["store"] != false || in["model"] != "deepseekv4pro" || strings.Contains(string(b), "azure-fixture-private") {
			t.Error("Unsupported Azure parameter, model fallback or credential exposure")
		}
		var output any
		if turn == 1 {
			output = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "call_feed", "type": "function", "function": map[string]string{"name": "get_headlines", "arguments": "{}"}}}}
		} else {
			if !strings.Contains(string(b), "tool_call_id") || !strings.Contains(string(b), "json_schema") {
				t.Error("Actual tool evidence or final schema missing")
			}
			output = map[string]any{"role": "assistant", "content": `{"headline_ids":["H1"]}`}
		}
		reply(w, 200, map[string]any{"model": "deepseekv4pro", "choices": []any{map[string]any{"finish_reason": "stop", "message": output}}})
	}))
	defer up.Close()
	a, _ := newAzureAgent("https://fixture.cognitiveservices.azure.com/openai/v1", "deepseekv4pro", key)
	a.base = up.URL
	a.fetch.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("api-key") != "" {
			t.Error("Model key sent to a public source")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/xml"}}, Body: io.NopCloser(strings.NewReader(`<rss><channel><item><title>Fixture technology story</title><link>https://example.com/story</link><description>Fixture source evidence</description></item></channel></rss>`)), Request: r}, nil
	})
	items, e := a.collect(context.Background(), "techmeme", nil)
	if e != nil || len(items) != 1 || turn != 2 {
		t.Fatal("Azure continuation failed", e)
	}
}

func TestAzureEndpointAndModelCannotSilentlyChange(t *testing.T) {
	for _, endpoint := range []string{"http://fixture.cognitiveservices.azure.com/openai/v1", "https://example.com/openai/v1", "https://fixture.cognitiveservices.azure.com.evil.example/openai/v1", "https://fixture.cognitiveservices.azure.com/openai/v1?key=secret", "https://user:password@fixture.cognitiveservices.azure.com/openai/v1"} {
		if _, e := newAzureAgent(endpoint, "deepseekv4pro", ""); e == nil {
			t.Fatal("Invalid Azure endpoint accepted")
		}
	}
	if _, e := newAzureAgent("https://fixture.cognitiveservices.azure.com/openai/v1", "other-model", ""); e == nil {
		t.Fatal("Unconfigured Azure deployment accepted")
	}
}

func TestAzureRejectsTruncatedAndUnsupportedResponses(t *testing.T) {
	for _, raw := range []string{
		`{"model":"deepseekv4pro","choices":[{"finish_reason":"length","message":{"content":"{unfinished"}}]}`,
		`{"model":"deepseekv4pro","choices":[{"finish_reason":"content_filter","message":{"content":"blocked"}}]}`,
		`{"model":"deepseekv4pro","choices":[]}`,
		`{"model":"deepseekv4pro","choices":[{"finish_reason":"tool_calls","message":{"tool_calls":[{"id":"x","type":"shell"}]}}]}`,
	} {
		if _, e := azureChatResponse([]byte(raw)); e == nil {
			t.Fatal("Incomplete or unsupported Azure response accepted")
		}
	}
}
