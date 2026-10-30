package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPArchiveCatalogAndToolRestrictions(t *testing.T) {
	s := testServer(t)
	s.agent.fetch = companyFixture()
	s.clock = fixtureTime
	s.initGame()
	d, _, _ := s.store.add("https://example.com/archive")
	s.store.update(d.ID, func(d *document) {
		d.GameWeek = s.game.Week.ID
		d.Source = source{ArchivedAt: "2026-10-20T12:00:00Z", Published: "2026-10-20", ArchiveURL: "https://www.techmeme.com/261020/h1200"}
		d.Summary = summary{Title: "Semiconductor demand grows", TLDR: "Archived source facts"}
	})
	up := httptest.NewServer(s.routes())
	defer up.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: up.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	list, err := session.ListTools(ctx, nil)
	if err != nil || len(list.Tools) != 6 {
		t.Fatal(list, err)
	}
	for _, tc := range []struct {
		name string
		args map[string]any
		bad  bool
	}{
		{"list_news", map[string]any{}, false}, {"read_archive", map[string]any{"source_id": d.ID}, false},
		{"search_archive", map[string]any{"query": "semiconductor"}, false}, {"search_companies", map[string]any{"query": "Samsung"}, false},
		{"search_web", map[string]any{"query": "chip demand"}, false},
		{"read_web", map[string]any{"source_id": webSourceID("https://fixture.example/web", s.game.Week.Cutoff)}, false},
		{"read_web", map[string]any{"source_id": "https://attacker.example"}, true},
		{"index_universe", map[string]any{}, true},
		{"read_archive", map[string]any{"source_id": "https://attacker.example"}, true},
		{"search_archive", map[string]any{"query": "chips", "url": "http://attacker"}, true},
		{"market_snapshot", map[string]any{}, true}, {"list_news", map[string]any{"query": "ignored"}, true},
	} {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
		if tc.bad {
			if err == nil && !result.IsError {
				t.Fatal("Accepted unsupported tool/arguments", tc.name)
			}
		} else if err != nil || result.IsError {
			t.Fatal(tc.name, result, err)
		}
		if !tc.bad && tc.name == "read_archive" && !strings.Contains(fmt.Sprint(result.Content[0]), "Archived source facts") {
			t.Fatal(result)
		}
	}
	for _, bad := range []string{`null`, `{"source_id":"x","url":"http://attacker"}`} {
		if _, err := s.archiveTool(ctx, "read_archive", json.RawMessage(bad)); err == nil {
			t.Fatal("Unvalidated input")
		}
	}
}
func TestMCPProxyHostAndOrigin(t *testing.T) {
	t.Setenv("MCP_PUBLIC_HOST", "desk.apps.example.com")
	s := testServer(t)
	s.initGame()
	up := httptest.NewServer(s.routes())
	defer up.Close()
	for _, tc := range []struct {
		host, origin string
		code         int
	}{{"desk.apps.example.com", "", 200}, {"attacker.example", "", 403}, {"desk.apps.example.com", "https://attacker.example", 403}} {
		req, _ := http.NewRequest("POST", up.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`))
		req.Host = tc.host
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		res, err := up.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != tc.code {
			t.Fatal(tc, res.StatusCode)
		}
	}
}
