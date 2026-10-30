package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeGameFinalizesAfterRequiredEvidence(t *testing.T) {
	turns := 0
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		turns++
		var in map[string]any
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Error(err)
		}
		tools := in["tools"].([]any)
		if turns == 1 {
			if len(tools) != 3 {
				t.Error("Archive research must start with its three read-only tools")
			}
			reply(w, 200, map[string]any{"status": "completed", "output": []any{
				map[string]string{"type": "function_call", "name": "read_archive", "call_id": "read", "arguments": `{"source_id":"original"}`},
				map[string]string{"type": "function_call", "name": "search_archive", "call_id": "search", "arguments": `{"query":"semiconductor demand"}`},
				map[string]string{"type": "function_call", "name": "search_companies", "call_id": "issuer", "arguments": `{"query":"Nvidia"}`},
				map[string]string{"type": "function_call", "name": "search_companies", "call_id": "ecosystem", "arguments": `{"query":"AMD"}`},
			}})
			return
		}
		if turns != 2 || len(tools) != 0 || !strings.Contains(fmt.Sprint(in["text"]), "json_schema") {
			t.Error("Native agent kept tools available after gathering all required evidence")
		}
		history, _ := json.Marshal(in["input"])
		for _, evidence := range []string{"function_call_output", "Archived chip demand", "Current company identity metadata"} {
			if !strings.Contains(string(history), evidence) {
				t.Errorf("Missing actual evidence in synthesis: %s", evidence)
			}
		}
		reply(w, 200, map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{map[string]string{"type": "output_text", "text": `{"comment":"Archived evidence is insufficient to propose a supported investment.","ideas":[]}`}}}}})
	}))
	defer up.Close()
	key := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(key, []byte("fixture-native-key"), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := newAgent(up.URL+"/v1", "native-test", key)
	if err != nil {
		t.Fatal(err)
	}
	a.fetch = companyFixture()
	week := lastNewsWeek(fixtureTime())
	d := document{ID: "original-id", URL: "https://example.com/chips", GameWeek: week.ID, Summary: summary{Title: "Archived chip demand", TLDR: "A real archived source excerpt for this fixture."}, Source: source{ArchivedAt: "2026-10-22T12:00:00Z", ArchiveURL: "https://www.techmeme.com/261022/h0800", Published: "2026-10-22"}}
	result, err := a.gameIdeas(context.Background(), d, defaults()[0], []document{d}, week)
	if err != nil || turns != 2 || len(result.Evidence) != 4 || len(result.Ideas) != 0 {
		t.Fatalf("Native synthesis did not finish from actual evidence: turns=%d evidence=%d err=%v", turns, len(result.Evidence), err)
	}
}
