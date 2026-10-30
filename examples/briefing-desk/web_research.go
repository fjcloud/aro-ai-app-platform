package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type webSource struct {
	ID        string `json:"source_id"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Published string `json:"published"`
	Excerpt   string `json:"excerpt"`
}
type webSearchResult struct {
	Query     string              `json:"query"`
	Provider  string              `json:"provider"`
	SearchURL string              `json:"search_url"`
	Cutoff    string              `json:"information_cutoff"`
	Retrieved string              `json:"retrieved"`
	Sources   []webSource         `json:"sources"`
	Rejected  []map[string]string `json:"rejected,omitempty"`
	Coverage  string              `json:"coverage"`
}

func clipText(text string, n int) string {
	r := []rune(strings.Join(strings.Fields(text), " "))
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return string(r)
}
func (f *fetcher) searchWeb(ctx context.Context, query string, week replayWeek) (webSearchResult, string, error) {
	out := webSearchResult{Query: query, Provider: "Bing News RSS", Cutoff: week.Cutoff, Retrieved: stamp(), Sources: []webSource{}, Coverage: "Public web-news search. Dates are publisher dates reported by the search index, not independently archived snapshots. Only results dated at or before the saved cutoff are eligible."}
	query = strings.TrimSpace(query)
	cutoff, err := time.Parse(time.RFC3339, week.Cutoff)
	if err != nil || len(query) < 3 || len(query) > 160 {
		return out, "", errors.New("Use a short research query and a valid saved cutoff")
	}
	q := url.Values{"q": {query + " before:" + cutoff.AddDate(0, 0, 1).Format("2006-01-02")}, "format": {"rss"}, "mkt": {"en-US"}}
	raw := "https://www.bing.com/news/search?" + q.Encode()
	out.SearchURL = raw
	b, _, _, err := f.get(ctx, raw)
	if err != nil {
		return out, raw, err
	}
	var rss struct {
		Channel struct {
			Items []struct {
				Title       string `xml:"title"`
				Link        string `xml:"link"`
				Date        string `xml:"pubDate"`
				Description string `xml:"description"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	if xml.Unmarshal(b, &rss) != nil {
		return out, raw, errors.New("Web search did not return readable RSS")
	}
	seen := map[string]bool{}
	for _, item := range rss.Channel.Items {
		link := item.Link
		u, e := url.Parse(link)
		if e != nil {
			continue
		}
		if strings.HasSuffix(u.Hostname(), "bing.com") && u.Path == "/news/apiclick.aspx" {
			link = u.Query().Get("url")
		}
		u, e = publicURL(link)
		if e != nil {
			continue
		}
		u.Fragment = ""
		link = u.String()
		date, e := httpDate(item.Date)
		reason := ""
		if e != nil {
			reason = "No usable publication date"
		} else if date.After(cutoff) {
			reason = "Published after the saved information cutoff"
		}
		if reason != "" {
			out.Rejected = append(out.Rejected, map[string]string{"url": link, "reported_date": item.Date, "reason": reason})
			continue
		}
		if seen[link] {
			continue
		}
		seen[link] = true
		_, excerpt := plainHTML([]byte(item.Description))
		out.Sources = append(out.Sources, webSource{ID: webSourceID(link, week.Cutoff), Title: clipText(item.Title, 250), URL: link, Published: date.UTC().Format(time.RFC3339), Excerpt: clipText(excerpt, 600)})
		if len(out.Sources) == 6 {
			break
		}
	}
	return out, raw, nil
}
func httpDate(raw string) (time.Time, error) {
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC3339} {
		if t, e := time.Parse(layout, raw); e == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("Unknown source date")
}

// A current fetch is not a historical snapshot. Only use publisher text when its
// publication and modification metadata establish eligibility; otherwise retain
// the explicitly dated search excerpt with its narrower coverage.
func (f *fetcher) readWeb(ctx context.Context, ref webSource, week replayWeek) (map[string]any, string, error) {
	cutoff, e := time.Parse(time.RFC3339, week.Cutoff)
	published, pe := time.Parse(time.RFC3339, ref.Published)
	if e != nil || pe != nil || published.After(cutoff) {
		return nil, "", errors.New("Web source is not dated before the saved cutoff")
	}
	result := map[string]any{"source_id": ref.ID, "title": ref.Title, "source_url": ref.URL, "published": ref.Published, "retrieved": stamp(), "coverage": "dated_search_excerpt_only", "text": ref.Excerpt, "date_basis": "Publisher date reported by Bing News RSS; not an independently archived page"}
	b, final, typ, err := f.get(ctx, ref.URL)
	if err != nil {
		result["publisher_error"] = err.Error()
		return result, ref.URL, nil
	}
	if !strings.Contains(typ, "html") {
		result["publisher_error"] = "Publisher response is not readable HTML"
		return result, ref.URL, nil
	}
	pub, modified := pageDates(b)
	if pub.IsZero() {
		result["publisher_error"] = "Publisher date metadata is missing; current page text excluded"
		return result, ref.URL, nil
	}
	if pub.After(cutoff) || !modified.IsZero() && modified.After(cutoff) {
		result["publisher_error"] = "Current page was published or modified after the cutoff; page text excluded"
		return result, ref.URL, nil
	}
	title, text := plainHTML(b)
	if len([]rune(text)) < 160 {
		result["publisher_error"] = "Publisher page has too little readable text"
		return result, ref.URL, nil
	}
	result["title"] = clipText(title, 250)
	result["text"] = clipText(text, 2400)
	result["source_url"] = final
	result["published"] = pub.UTC().Format(time.RFC3339)
	result["coverage"] = "publisher_excerpt_with_pre_cutoff_date_metadata"
	result["date_basis"] = "Current fetch with publisher publication/modification metadata; not a historical snapshot"
	if !modified.IsZero() {
		result["modified"] = modified.UTC().Format(time.RFC3339)
	}
	return result, final, nil
}
func pageDates(b []byte) (time.Time, time.Time) {
	root, e := html.Parse(strings.NewReader(string(b)))
	if e != nil {
		return time.Time{}, time.Time{}
	}
	var published, modified time.Time
	set := func(raw string, dest *time.Time) {
		for _, layout := range []string{time.RFC3339, time.RFC3339Nano, "2006-01-02"} {
			if t, e := time.Parse(layout, raw); e == nil {
				if dest.IsZero() || t.After(*dest) {
					*dest = t
				}
				break
			}
		}
	}
	var jsonDates func(any)
	jsonDates = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			for k, x := range v {
				if s, ok := x.(string); ok {
					if k == "datePublished" {
						set(s, &published)
					}
					if k == "dateModified" {
						set(s, &modified)
					}
				}
				jsonDates(x)
			}
		case []any:
			for _, x := range v {
				jsonDates(x)
			}
		}
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "meta" {
			key, value := "", ""
			for _, a := range n.Attr {
				if a.Key == "name" || a.Key == "property" {
					key = strings.ToLower(a.Val)
				}
				if a.Key == "content" {
					value = a.Val
				}
			}
			switch key {
			case "article:published_time", "datepublished", "date":
				set(value, &published)
			case "article:modified_time", "datemodified", "last-modified":
				set(value, &modified)
			}
		}
		if n.Type == html.ElementNode && n.Data == "script" {
			for _, a := range n.Attr {
				if a.Key == "type" && a.Val == "application/ld+json" && n.FirstChild != nil {
					var v any
					if json.Unmarshal([]byte(n.FirstChild.Data), &v) == nil {
						jsonDates(v)
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return published, modified
}

func webSourceID(link, cutoff string) string {
	return fmt.Sprintf("W-%x", sha256.Sum256([]byte(link+"|"+cutoff)))[:18]
}

type cachedWebSource struct {
	Source webSource
	Cutoff string
	Until  time.Time
}

func (s *server) cacheWebSources(result webSearchResult) {
	s.webMu.Lock()
	defer s.webMu.Unlock()
	if s.webSources == nil {
		s.webSources = map[string]cachedWebSource{}
	}
	now := time.Now()
	for id, ref := range s.webSources {
		if now.After(ref.Until) {
			delete(s.webSources, id)
		}
	}
	for _, ref := range result.Sources {
		s.webSources[ref.ID] = cachedWebSource{Source: ref, Cutoff: result.Cutoff, Until: now.Add(15 * time.Minute)}
	}
	for len(s.webSources) > 128 {
		oldest := ""
		var until time.Time
		for id, ref := range s.webSources {
			if oldest == "" || ref.Until.Before(until) {
				oldest, until = id, ref.Until
			}
		}
		delete(s.webSources, oldest)
	}
}
func (s *server) cachedWebSource(id, cutoff string) (webSource, bool) {
	s.webMu.Lock()
	defer s.webMu.Unlock()
	ref, ok := s.webSources[id]
	return ref.Source, ok && ref.Cutoff == cutoff && time.Now().Before(ref.Until)
}
func modelToolResult(name string, data any) any {
	if name == "search_web" {
		if result, ok := data.(webSearchResult); ok {
			count := len(result.Rejected)
			result.Rejected = nil
			result.Coverage += fmt.Sprintf(" %d results excluded by date checks.", count)
			return result
		}
	}
	return data
}
