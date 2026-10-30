package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func companyFixture() *fetcher {
	f := newFetcher()
	f.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/finance/search" {
			return nil, fmt.Errorf("Unexpected request: %s", r.URL.Path)
		}
		body := `{"quotes":[{"symbol":"NVDA","longname":"NVIDIA Corporation","exchDisp":"NASDAQ","quoteType":"EQUITY","regularMarketPrice":999999},{"symbol":"AMD","longname":"Advanced Micro Devices","exchDisp":"NASDAQ","quoteType":"EQUITY"},{"symbol":"005930.KS","longname":"Samsung Electronics","exchDisp":"KRX","quoteType":"EQUITY"},{"symbol":"QQQ","longname":"Invesco QQQ Trust","quoteType":"ETF"},{"symbol":"SMNG","longname":"Samsung Group ETF","quoteType":"EQUITY"},{"symbol":"^GSPC","longname":"S&P 500","quoteType":"INDEX"}]}`
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	return f
}

func TestCompanyLookupExcludesFundsAndNeverReturnsPrices(t *testing.T) {
	items, _, err := companyFixture().searchCompanies(context.Background(), "Samsung")
	if err != nil || len(items) != 3 {
		t.Fatal(items, err)
	}
	b, _ := json.Marshal(items)
	for _, bad := range []string{"QQQ", "SMNG", "GSPC", "regularMarketPrice", "999999"} {
		if strings.Contains(string(b), bad) {
			t.Fatal("Fund, index or price leaked into issuer research", bad)
		}
	}
}

func TestStockIdeasAreBoundToLookedUpCompaniesAndArchivedEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, symbol, role string
		refs               []string
		good               bool
	}{
		{"company", "NVDA", "direct", []string{"E1", "E3"}, true},
		{"foreign company", "005930.KS", "supplier", []string{"E1", "E3"}, true},
		{"fund", "QQQ", "direct", []string{"E1"}, false},
		{"unverified ticker", "FORGED", "direct", []string{"E1"}, false},
		{"unsupported relationship", "NVDA", "index", []string{"E1"}, false},
		{"no archived facts", "NVDA", "direct", []string{"E3"}, false},
		{"invented source", "NVDA", "direct", []string{"E9"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			turn := 0
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				turn++
				if turn == 1 {
					calls := []any{}
					for i, c := range []struct{ name, args string }{{"read_archive", `{"source_id":"original"}`}, {"search_archive", `{"query":"chip"}`}, {"search_companies", `{"query":"Nvidia"}`}, {"search_companies", `{"query":"Samsung"}`}} {
						calls = append(calls, map[string]string{"type": "function_call", "name": c.name, "call_id": fmt.Sprint(i), "arguments": c.args})
					}
					reply(w, 200, map[string]any{"status": "completed", "output": calls})
					return
				}
				answer, _ := json.Marshal(map[string]any{"comment": "Archived semiconductor evidence supports this conditional company catalyst.", "ideas": []gameIdea{{Symbol: tc.symbol, Company: "Invented company name", Relationship: tc.role, Thesis: "Archived chip demand could support this issuer, subject to the stated assumptions.", Risk: "The catalyst may already be priced in or the business relationship may not transmit.", Sources: tc.refs}}})
				reply(w, 200, map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{map[string]string{"type": "output_text", "text": string(answer)}}}}})
			}))
			defer up.Close()
			key := filepath.Join(t.TempDir(), "key")
			os.WriteFile(key, []byte("fixture-key"), 0600)
			a, _ := newAgent(up.URL+"/v1", "fixture", key)
			a.fetch = companyFixture()
			week := lastNewsWeek(fixtureTime())
			d := document{ID: "original", URL: "https://example.com/chips", GameWeek: week.ID, Summary: summary{Title: "Chip demand", TLDR: "Archived company catalyst."}, Source: source{ArchivedAt: "2026-10-22T12:00:00Z", ArchiveURL: "https://www.techmeme.com/261022/h1200"}}
			out, err := a.gameIdeas(context.Background(), d, defaults()[0], []document{d}, week)
			if (err == nil) != tc.good {
				t.Fatal("Unexpected stock validation", err)
			}
			if tc.good && (out.Ideas[0].Company == "Invented company name" || out.Ideas[0].Exchange == "") {
				t.Fatal("Company identity was not resolved by the server")
			}
		})
	}
}
