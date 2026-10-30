package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestWebSearchFiltersDatesAndPrivateURLs(t *testing.T) {
	f := newFetcher()
	f.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if !strings.Contains(r.URL.Query().Get("q"), "before:") {
			t.Error("Search did not include the saved date boundary")
		}
		body := `<rss><channel>
  <item><title>Known catalyst</title><link>https://source.example/past</link><pubDate>Wed, 21 Oct 2026 12:00:00 GMT</pubDate><description>Known public source excerpt.</description></item>
  <item><title>Later winner</title><link>https://source.example/future</link><pubDate>Fri, 30 Oct 2026 12:00:00 GMT</pubDate></item>
  <item><title>No date</title><link>https://source.example/unknown</link></item>
  <item><title>Private target</title><link>http://127.0.0.1/private</link><pubDate>Wed, 21 Oct 2026 12:00:00 GMT</pubDate></item>
  </channel></rss>`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
	})
	result, _, err := f.searchWeb(context.Background(), "chip demand", lastNewsWeek(fixtureTime()))
	if err != nil || len(result.Sources) != 1 || len(result.Rejected) != 2 {
		t.Fatal(result, err)
	}
	if result.Sources[0].Published != "2026-10-21T12:00:00Z" || result.Sources[0].ID == "" {
		t.Fatal("Missing dated stable source", result)
	}
	clean := modelToolResult("search_web", result).(webSearchResult)
	if len(clean.Rejected) != 0 || len(result.Rejected) != 2 {
		t.Fatal("Excluded URLs must remain in audit only, not in model input")
	}
}
func TestWebReaderNeverUsesLaterOrUndatedPageText(t *testing.T) {
	week := lastNewsWeek(fixtureTime())
	ref := webSource{ID: "W-test", URL: "https://source.example/story", Published: "2026-10-21T12:00:00Z", Excerpt: "Only the dated search excerpt is eligible."}
	for _, tc := range []struct {
		metadata string
		full     bool
	}{
		{`<meta property="article:published_time" content="2026-10-21T12:00:00Z">`, true},
		{`<meta property="article:published_time" content="2026-10-21T12:00:00Z"><meta property="article:modified_time" content="2026-10-30T12:00:00Z">`, false},
		{``, false},
	} {
		f := newFetcher()
		f.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/html"}}, Request: r, Body: io.NopCloser(strings.NewReader(`<html><head>` + tc.metadata + `</head><body>` + strings.Repeat("Publisher source facts. ", 30) + `</body></html>`))}, nil
		})
		result, _, err := f.readWeb(context.Background(), ref, week)
		if err != nil {
			t.Fatal(err)
		}
		if tc.full {
			if result["coverage"] != "publisher_excerpt_with_pre_cutoff_date_metadata" {
				t.Fatal(result)
			}
		} else if result["text"] != ref.Excerpt || result["publisher_error"] == nil {
			t.Fatal("Current/undated page text leaked into historical research", result)
		}
	}
}
