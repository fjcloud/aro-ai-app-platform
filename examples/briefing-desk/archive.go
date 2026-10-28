package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"

	"golang.org/x/net/html"
)

type replayWeek struct {
	ID          string `json:"id"`
	NewsStart   string `json:"news_start"`
	NewsEnd     string `json:"news_end"`
	Cutoff      string `json:"cutoff"`
	MarketStart string `json:"market_start"`
	MarketEnd   string `json:"market_end"`
}

func lastNewsWeek(now time.Time) replayWeek {
	loc, _ := time.LoadLocation("America/New_York")
	n := now.In(loc)
	day := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc)
	monday := day.AddDate(0, 0, -(int(day.Weekday())+6)%7)
	return replayWeek{monday.AddDate(0, 0, -7).Format("2006-01-02"), monday.AddDate(0, 0, -7).Format("2006-01-02"), monday.AddDate(0, 0, -1).Format("2006-01-02"), monday.UTC().Format(time.RFC3339), monday.Format("2006-01-02"), monday.AddDate(0, 0, 4).Format("2006-01-02")}
}

type archivedStory struct {
	ArchiveID   string `json:"archive_id"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	Excerpt     string `json:"excerpt"`
	Day         string `json:"day"`
	SnapshotURL string `json:"snapshot_url"`
	AsOf        string `json:"as_of"`
}
type archiveManifest struct {
	Week         replayWeek      `json:"week"`
	Stories      []archivedStory `json:"stories"`
	SnapshotURLs []string        `json:"snapshot_urls"`
	Missing      []string        `json:"unavailable_ids"`
	Coverage     string          `json:"coverage"`
}

var archiveIDPattern = regexp.MustCompile(`^(\d{6})p(\d+)$`)
var archiveDatePattern = regexp.MustCompile(`var pgrdad='([^']+)'`)

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
func hasClass(n *html.Node, name string) bool {
	for _, v := range strings.Fields(attr(n, "class")) {
		if v == name {
			return true
		}
	}
	return false
}
func findNode(n *html.Node, predicate func(*html.Node) bool) *html.Node {
	if predicate(n) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if v := findNode(c, predicate); v != nil {
			return v
		}
	}
	return nil
}
func archiveText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(v *html.Node) {
		if v.Type == html.ElementNode && (v.Data == "script" || v.Data == "style") {
			return
		}
		if v.Type == html.TextNode {
			b.WriteString(v.Data)
			b.WriteByte(' ')
		}
		for c := v.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}
func parseArchive(b []byte, raw string, week replayWeek) ([]archivedStory, error) {
	date := archiveDatePattern.FindSubmatch(b)
	if len(date) != 2 {
		return nil, errors.New("Archive timestamp is missing")
	}
	loc, _ := time.LoadLocation("America/New_York")
	asof, e := time.ParseInLocation("January 2, 2006, 3:04 PM", string(date[1]), loc)
	if e != nil {
		return nil, errors.New("Invalid archive snapshot date")
	}
	cutoff, _ := time.Parse(time.RFC3339, week.Cutoff)
	if asof.After(cutoff) {
		return nil, errors.New("Archive snapshot is after the replay information cutoff")
	}
	root, e := html.Parse(bytes.NewReader(b))
	if e != nil {
		return nil, e
	}
	out := []archivedStory{}
	seen := map[string]bool{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && hasClass(n, "item") {
			meta := findNode(n, func(v *html.Node) bool { return archiveIDPattern.MatchString(attr(v, "pml")) })
			headline := findNode(n, func(v *html.Node) bool { return v.Type == html.ElementNode && v.Data == "a" && hasClass(v, "ourh") })
			excerpt := findNode(n, func(v *html.Node) bool { return v.Type == html.ElementNode && hasClass(v, "ii") })
			if meta != nil && headline != nil && excerpt != nil {
				id := attr(meta, "pml")
				m := archiveIDPattern.FindStringSubmatch(id)
				day, e := time.Parse("060102", m[1])
				dayText := day.Format("2006-01-02")
				if e == nil && dayText >= week.NewsStart && dayText <= week.NewsEnd && !seen[id] {
					link := attr(headline, "href")
					if _, e := publicURL(link); e == nil {
						title := archiveText(headline)
						text := strings.TrimSpace(strings.TrimLeft(strings.TrimPrefix(archiveText(excerpt), title), " —–-"))
						if len(text) < 20 {
							text = title
						}
						out = append(out, archivedStory{id, link, title, text, dayText, raw, asof.UTC().Format(time.RFC3339)})
						seen[id] = true
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return out, nil
}
func (f *fetcher) archivePage(ctx context.Context, cache, raw string, week replayWeek) ([]archivedStory, error) {
	hash := sha256.Sum256([]byte(raw))
	file := filepath.Join(cache, hex.EncodeToString(hash[:])+".html")
	b, e := os.ReadFile(file)
	if e != nil {
		b, _, _, e = f.getLimit(ctx, raw, 8*1024*1024)
		if e != nil {
			return nil, e
		}
		if e = os.MkdirAll(cache, 0700); e != nil {
			return nil, e
		}
		if e = os.WriteFile(file, b, 0600); e != nil {
			return nil, e
		}
	}
	return parseArchive(b, raw, week)
}
func (f *fetcher) archiveWeek(ctx context.Context, cache string, week replayWeek, note func(string)) (archiveManifest, error) {
	out := archiveManifest{Week: week, Stories: []archivedStory{}, SnapshotURLs: []string{}, Missing: []string{}, Coverage: "Techmeme primary headline IDs from historical snapshots, deduplicated by primary publisher URL. Related discussion links are not separate primary headlines. Unavailable IDs are listed explicitly."}
	loc, _ := time.LoadLocation("America/New_York")
	start, _ := time.ParseInLocation("2006-01-02", week.NewsStart, loc)
	pages := []string{}
	for d := 0; d < 7; d++ {
		for _, hour := range []string{"1200", "2355"} {
			pages = append(pages, "https://www.techmeme.com/"+start.AddDate(0, 0, d).Format("060102")+"/h"+hour)
		}
	}
	pages = append(pages, "https://www.techmeme.com/"+start.AddDate(0, 0, 7).Format("060102")+"/h0000")
	found := map[string]archivedStory{}
	var mu sync.Mutex
	fetchPages := func(urls []string, strict bool) error {
		jobs := make(chan string)
		var wg sync.WaitGroup
		errs := []error{}
		for worker := 0; worker < 2; worker++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for raw := range jobs {
					stories, e := f.archivePage(ctx, cache, raw, week)
					mu.Lock()
					if e != nil {
						if strict {
							errs = append(errs, fmt.Errorf("Archive unavailable: %s: %w", raw, e))
						}
					} else {
						out.SnapshotURLs = append(out.SnapshotURLs, raw)
						for _, s := range stories {
							if old, ok := found[s.ArchiveID]; !ok || s.AsOf < old.AsOf {
								found[s.ArchiveID] = s
							}
						}
					}
					mu.Unlock()
				}
			}()
		}
		for _, raw := range urls {
			select {
			case jobs <- raw:
			case <-ctx.Done():
				close(jobs)
				wg.Wait()
				return ctx.Err()
			}
		}
		close(jobs)
		wg.Wait()
		if len(errs) > 0 {
			return errs[0]
		}
		return nil
	}
	if note != nil {
		note("Reading historical Techmeme snapshots for all seven days")
	}
	if e := fetchPages(pages, true); e != nil {
		return out, e
	}
	maximum := map[string]int{}
	for id := range found {
		m := archiveIDPattern.FindStringSubmatch(id)
		n, _ := strconv.Atoi(m[2])
		if n > maximum[m[1]] {
			maximum[m[1]] = n
		}
	}
	missing := []string{}
	for d := 0; d < 7; d++ {
		prefix := start.AddDate(0, 0, d).Format("060102")
		max := maximum[prefix]
		if max == 0 || max > 200 {
			return out, errors.New("Archive day is missing or has an invalid headline range")
		}
		for n := 1; n <= max; n++ {
			id := fmt.Sprintf("%sp%d", prefix, n)
			if _, ok := found[id]; !ok {
				missing = append(missing, "https://www.techmeme.com/"+prefix+"/p"+strconv.Itoa(n))
			}
		}
	}
	if note != nil {
		note(fmt.Sprintf("Found %d headline IDs; checking %d gaps", len(found), len(missing)))
	}
	if e := fetchPages(missing, false); e != nil {
		return out, e
	}
	for day, max := range maximum {
		for n := 1; n <= max; n++ {
			id := fmt.Sprintf("%sp%d", day, n)
			if _, ok := found[id]; !ok {
				out.Missing = append(out.Missing, id)
			}
		}
	}
	byURL := map[string]archivedStory{}
	for _, s := range found {
		if old, ok := byURL[s.URL]; !ok || s.AsOf < old.AsOf {
			byURL[s.URL] = s
		}
	}
	for _, s := range byURL {
		out.Stories = append(out.Stories, s)
	}
	sort.Slice(out.Stories, func(i, j int) bool {
		if out.Stories[i].Day == out.Stories[j].Day {
			return out.Stories[i].ArchiveID < out.Stories[j].ArchiveID
		}
		return out.Stories[i].Day < out.Stories[j].Day
	})
	sort.Strings(out.Missing)
	sort.Strings(out.SnapshotURLs)
	return out, nil
}
