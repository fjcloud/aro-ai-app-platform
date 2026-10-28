package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

var importedSnapshotPath = regexp.MustCompile(`^/(\d{6})/(?:h\d{4}|p\d+)$`)

type importedCorpus struct {
	Week    replayWeek `json:"week"`
	Missing []string   `json:"unavailable_ids"`
	News    []struct {
		URL    string `json:"url"`
		Source struct {
			Published  string `json:"published"`
			ArchiveID  string `json:"archive_id"`
			ArchivedAt string `json:"archived_at"`
			ArchiveURL string `json:"archive_url"`
		} `json:"source"`
		Summary struct {
			Title string `json:"title"`
			TLDR  string `json:"tldr"`
		} `json:"summary"`
	} `json:"news"`
}

// Accept the app's news export, not its scores, proposals or portfolio state.
func readImportedCorpus(r io.Reader, week replayWeek) (archiveManifest, error) {
	var in importedCorpus
	dec := json.NewDecoder(r)
	if dec.Decode(&in) != nil {
		return archiveManifest{}, errors.New("Choose a valid Signal Desk news export JSON file (maximum 8 MiB)")
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return archiveManifest{}, errors.New("The file must contain one news export")
	}
	if in.Week != week {
		return archiveManifest{}, errors.New("The imported news week must match the week displayed in the app")
	}
	if len(in.News) < 1 || len(in.News) > 1500 || len(in.Missing) > 1400 {
		return archiveManifest{}, errors.New("Import between 1 and 1,500 archived news items")
	}
	out := archiveManifest{Week: week, Missing: in.Missing, Stories: []archivedStory{}, ImportedAt: stamp(), Coverage: "Manually imported archive. Dates and source provenance supplied by the file; publisher pages were not fetched. Scores are computed by the server."}
	cutoff, _ := time.Parse(time.RFC3339, week.Cutoff)
	loc, _ := time.LoadLocation("America/New_York")
	seen, snapshots := map[string]bool{}, map[string]bool{}
	for _, d := range in.News {
		u, e := publicURL(d.URL)
		snapshot, se := publicURL(d.Source.ArchiveURL)
		day, de := time.ParseInLocation("2006-01-02", d.Source.Published, loc)
		at, ae := time.Parse(time.RFC3339, d.Source.ArchivedAt)
		match := archiveIDPattern.FindStringSubmatch(d.Source.ArchiveID)
		if e != nil || se != nil || de != nil || ae != nil || len(match) != 3 || match[1] != day.Format("060102") || d.Source.Published < week.NewsStart || d.Source.Published > week.NewsEnd || at.After(cutoff) || at.Before(day) || len(strings.TrimSpace(d.Summary.Title)) < 3 || len(d.Summary.Title) > 800 || len(d.Summary.TLDR) > 4000 {
			return archiveManifest{}, errors.New("News must contain valid pre-cutoff archive dates, source IDs, public URLs and bounded excerpts")
		}
		snapshotParts := importedSnapshotPath.FindStringSubmatch(snapshot.Path)
		if snapshot.Scheme != "https" || (snapshot.Host != "www.techmeme.com" && snapshot.Host != "techmeme.com") || len(snapshotParts) != 2 || snapshotParts[1] != at.In(loc).Format("060102") || snapshot.RawQuery != "" {
			return archiveManifest{}, errors.New("Each archive link must reference its dated Techmeme snapshot")
		}
		host := strings.ToLower(u.Hostname())
		if strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".svc") {
			return archiveManifest{}, errors.New("Private article destinations are not allowed")
		}
		if seen[u.String()] {
			continue
		}
		seen[u.String()] = true
		out.Stories = append(out.Stories, archivedStory{ArchiveID: d.Source.ArchiveID, URL: u.String(), Title: d.Summary.Title, Excerpt: d.Summary.TLDR, Day: d.Source.Published, SnapshotURL: snapshot.String(), AsOf: at.UTC().Format(time.RFC3339)})
		if !snapshots[snapshot.String()] {
			out.SnapshotURLs = append(out.SnapshotURLs, snapshot.String())
			snapshots[snapshot.String()] = true
		}
	}
	for _, missing := range in.Missing {
		parts := archiveIDPattern.FindStringSubmatch(missing)
		if len(parts) != 3 {
			return archiveManifest{}, errors.New("Invalid unavailable archive ID")
		}
		day, e := time.Parse("060102", parts[1])
		if e != nil || day.Format("2006-01-02") < week.NewsStart || day.Format("2006-01-02") > week.NewsEnd {
			return archiveManifest{}, errors.New("Unavailable archive IDs must belong to the imported week")
		}
	}
	return out, nil
}
func (s *server) importGameWeek(w http.ResponseWriter, r *http.Request) {
	s.gameMu.Lock()
	week := s.game.Week
	s.gameMu.Unlock()
	manifest, e := readImportedCorpus(http.MaxBytesReader(w, r.Body, 8<<20), week)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	s.gameMu.Lock()
	if s.game.Round.Status != "draft" || s.game.Status == "loading" || s.game.Status == "ranking" {
		s.gameMu.Unlock()
		fail(w, 409, "Import before investing and while no import or ranking job is running")
		return
	}
	s.game.Status = "ranking"
	s.game.Error = ""
	s.game.Message = "Import accepted. Ranking archived news on the decision model…"
	s.game.Round = newPaperRound()
	s.game.Proposals = map[string]gameProposal{}
	s.saveGame()
	s.gameMu.Unlock()
	// Retire documents omitted by the imported corpus; never use uploaded classifications.
	keep := map[string]bool{}
	for _, v := range manifest.Stories {
		keep[v.URL] = true
	}
	for _, d := range s.gameDocuments() {
		if !keep[d.URL] {
			s.store.update(d.ID, func(d *document) { d.GameWeek = week.ID + "-retired" })
		}
	}
	go func() {
		ctx, cancel := context.WithTimeout(s.ctx, 45*time.Minute)
		defer cancel()
		s.rankGameManifest(ctx, manifest)
	}()
	reply(w, 202, map[string]any{"status": "ranking", "news": len(manifest.Stories)})
}

// Facilitators can prepare an import on an allowed network without model access.
func exportNews(w io.Writer) error {
	cache, e := os.MkdirTemp("", "signal-desk-archive-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(cache)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	manifest, e := newFetcher().archiveWeek(ctx, cache, lastNewsWeek(time.Now()), func(s string) { fmt.Fprintln(os.Stderr, s) })
	if e != nil {
		return e
	}
	news := []map[string]any{}
	for _, v := range manifest.Stories {
		news = append(news, map[string]any{"url": v.URL, "source": map[string]string{"published": v.Day, "archive_id": v.ArchiveID, "archived_at": v.AsOf, "archive_url": v.SnapshotURL}, "summary": map[string]string{"title": v.Title, "tldr": v.Excerpt}})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{"week": manifest.Week, "coverage": manifest.Coverage, "unavailable_ids": manifest.Missing, "news": news})
}
