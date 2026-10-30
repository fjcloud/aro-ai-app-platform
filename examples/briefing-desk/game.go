package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type gameIdea struct {
	ID           string   `json:"id"`
	Symbol       string   `json:"symbol"`
	Company      string   `json:"company,omitempty"`
	Exchange     string   `json:"exchange,omitempty"`
	Relationship string   `json:"relationship"`
	Thesis       string   `json:"thesis"`
	Risk         string   `json:"risk"`
	Sources      []string `json:"sources"`
}
type gameProposal struct {
	ArticleID   string         `json:"article_id"`
	MemoVersion int            `json:"memo_version"`
	SummaryHash string         `json:"summary_hash"`
	Status      string         `json:"status"`
	Error       string         `json:"error,omitempty"`
	Comment     string         `json:"comment"`
	Ideas       []gameIdea     `json:"ideas"`
	Evidence    []evidence     `json:"evidence,omitempty"`
	Cutoff      string         `json:"cutoff"`
	TraceID     string         `json:"trace_id,omitempty"`
	TraceURL    string         `json:"trace_url,omitempty"`
	Skill       *researchSkill `json:"skill,omitempty"`
}
type paperPosition struct {
	ArticleID string            `json:"article_id"`
	IdeaID    string            `json:"idea_id"`
	Amount    float64           `json:"amount"`
	Symbol    string            `json:"symbol"`
	Company   string            `json:"company,omitempty"`
	Title     string            `json:"title"`
	Result    *historicalReturn `json:"result,omitempty"`
}
type paperRound struct {
	ID          string          `json:"id"`
	Status      string          `json:"status"`
	Memo        *profile        `json:"memo,omitempty"`
	CommittedAt string          `json:"committed_at,omitempty"`
	Budget      float64         `json:"budget"`
	Positions   []paperPosition `json:"positions"`
	Invested    float64         `json:"invested"`
	PnL         float64         `json:"pnl"`
	Value       float64         `json:"value"`
	ReturnPct   float64         `json:"return_pct"`
	AsOf        string          `json:"as_of,omitempty"`
	Complete    bool            `json:"week_complete"`
	Error       string          `json:"error,omitempty"`
}
type gameState struct {
	Week      replayWeek              `json:"week"`
	Status    string                  `json:"status"`
	Message   string                  `json:"message"`
	Error     string                  `json:"error,omitempty"`
	Manifest  archiveManifest         `json:"manifest"`
	Proposals map[string]gameProposal `json:"proposals"`
	Round     paperRound              `json:"round"`
	Previous  []paperRound            `json:"previous_rounds,omitempty"`
}

func newPaperRound() paperRound {
	return paperRound{ID: id(), Status: "draft", Budget: 10000, Value: 10000, Positions: []paperPosition{}}
}
func (s *server) gameFile() string {
	return filepath.Join(s.store.dir, "game-"+s.game.Week.ID+"-equities-v1.json")
}
func (s *server) saveGame() {
	if e := atomicJSON(s.gameFile(), s.game); e != nil {
		s.game.Error = "Game progress could not be saved"
	}
}
func (s *server) initGame() {
	s.gameMu.Lock()
	defer s.gameMu.Unlock()
	week := lastNewsWeek(s.currentTime())
	if usesBundledNews() && !s.preview {
		manifest, err := bundledCorpus()
		if err != nil {
			s.game = gameState{Week: week, Status: "failed", Error: err.Error(), Proposals: map[string]gameProposal{}, Round: newPaperRound()}
			return
		}
		week = manifest.Week
	}
	s.game = gameState{Week: week, Status: "empty", Proposals: map[string]gameProposal{}, Round: newPaperRound()}
	if b, e := os.ReadFile(s.gameFile()); e == nil {
		json.Unmarshal(b, &s.game)
	} else if usesBundledNews() && !s.preview {
		// Reuse verified news/scores, but start a separate equities round rather
		// than loading the earlier ETF proposal/portfolio file.
		manifest, err := bundledCorpus()
		if err == nil {
			s.game.Manifest = manifest
			docs := s.gameDocuments()
			byURL := map[string]document{}
			for _, d := range docs {
				byURL[d.URL] = d
			}
			p, ok := s.store.profile("tech")
			complete := ok && len(docs) == len(manifest.Stories)
			for _, story := range manifest.Stories {
				d, exists := byURL[story.URL]
				complete = complete && exists && d.Summary.Title == story.Title && d.Summary.TLDR == story.Excerpt && d.Source.ArchivedAt == story.AsOf && currentScore(d, p)
			}
			if complete {
				s.game.Status = "ready"
				s.game.Message = "Saved news and memo scores reused. Research individual company stocks for this round."
				s.saveGame()
			}
		}
	}
	if s.game.Proposals == nil {
		s.game.Proposals = map[string]gameProposal{}
	}
	if s.game.Status == "loading" || s.game.Status == "ranking" {
		s.game.Status = "paused"
		s.game.Message = "Load the week to resume ranking"
	}
	if s.game.Round.Status == "settling" {
		s.game.Round.Status = "awaiting_prices"
	}
	for k, p := range s.game.Proposals {
		if p.Status == "running" {
			p.Status = "failed"
			p.Error = "Research was interrupted. Retry it."
			s.game.Proposals[k] = p
		}
	}
	if s.preview {
		docs, _ := s.store.list()
		for _, d := range docs {
			s.store.update(d.ID, func(d *document) {
				d.GameWeek = week.ID
				d.Source.ArchiveURL = d.URL
				d.Source.ArchivedAt = week.Cutoff
				d.Source.Published = week.NewsStart
			})
		}
		s.game.Status = "ready"
		s.game.Manifest.Week = week
		s.game.Manifest.Coverage = "Synthetic interface preview; no live news or market prices."
	}
}
func (s *server) gameDocuments() []document {
	docs, _ := s.store.list()
	out := []document{}
	for _, d := range docs {
		if d.GameWeek == s.game.Week.ID {
			out = append(out, d)
		}
	}
	return out
}
func (s *server) loadGameWeek() {
	ctx, cancel := context.WithTimeout(s.ctx, 45*time.Minute)
	defer cancel()
	s.gameMu.Lock()
	week := s.game.Week
	s.gameMu.Unlock()
	note := func(text string) { s.gameMu.Lock(); s.game.Message = text; s.saveGame(); s.gameMu.Unlock() }
	if usesBundledNews() {
		manifest, err := bundledCorpus()
		if err != nil {
			s.gameMu.Lock()
			s.game.Status = "failed"
			s.game.Error = err.Error()
			s.saveGame()
			s.gameMu.Unlock()
			return
		}
		s.rankGameManifest(ctx, manifest)
		return
	}
	cache := filepath.Join(s.store.dir, "archive-cache", week.ID)
	manifest, e := s.agent.fetch.archiveWeek(ctx, cache, week, note)
	if e != nil {
		s.gameMu.Lock()
		s.game.Status = "failed"
		s.game.Error = e.Error()
		s.saveGame()
		s.gameMu.Unlock()
		return
	}
	s.rankGameManifest(ctx, manifest)
}

func (s *server) rankGameManifest(ctx context.Context, manifest archiveManifest) {
	week := manifest.Week
	coverage := "archived_excerpt"
	if manifest.ImportedAt != "" {
		coverage = "imported_archive"
	}
	var e error
	note := func(text string) { s.gameMu.Lock(); s.game.Message = text; s.saveGame(); s.gameMu.Unlock() }
	s.gameMu.Lock()
	s.game.Manifest = manifest
	s.game.Status = "ranking"
	s.saveGame()
	s.gameMu.Unlock()
	for _, story := range manifest.Stories {
		s.store.mu.Lock()
		var d document
		for _, old := range s.store.docs {
			if old.GameWeek == week.ID && old.URL == story.URL {
				d = copyDocument(old)
				break
			}
		}
		if d.ID == "" || d.Summary.Title != story.Title || d.Summary.TLDR != story.Excerpt {
			docID := d.ID
			if docID == "" {
				docID = id()
			}
			d = document{ID: docID, URL: story.URL, GameWeek: week.ID, Status: "ready", Created: stamp(), Classes: map[string]classification{}, Research: map[string]research{}, Source: source{URL: story.URL, FinalURL: story.URL, Title: story.Title, Text: story.Excerpt, Coverage: coverage, Published: story.Day, ArchiveID: story.ArchiveID, ArchivedAt: story.AsOf, ArchiveURL: story.SnapshotURL, Fetched: stamp()}, Summary: summary{Title: story.Title, TLDR: story.Excerpt, Points: []string{story.Title, story.Excerpt}, Overview: story.Excerpt}}
			d.Markdown = documentMarkdown(d)
			e = atomicJSON(filepath.Join(s.store.dir, "documents", d.ID+".json"), d)
			if e == nil {
				s.store.docs[d.ID] = d
			}
		}
		s.store.mu.Unlock()
		if e != nil {
			break
		}
	}
	if e == nil {
		docs := s.gameDocuments()
		for i, d := range docs {
			p, ok := s.store.profile("tech")
			if !ok {
				e = errors.New("Fund memo is unavailable")
				break
			}
			if currentScore(d, p) {
				continue
			}
			note(fmt.Sprintf("Ranking archived news: %d of %d", i+1, len(docs)))
			select {
			case s.classes <- struct{}{}:
				s.classify(d.ID, p.ID)
			case <-ctx.Done():
				e = ctx.Err()
			}
			if e != nil {
				break
			}
		}
	}
	if e == nil {
		p, _ := s.store.profile("tech")
		pending := 0
		for _, d := range s.gameDocuments() {
			if !currentScore(d, p) {
				pending++
			}
		}
		if pending > 0 {
			e = fmt.Errorf("%d stories still need ranking. Reload the week to retry; completed scores are kept", pending)
		}
	}
	s.gameMu.Lock()
	defer s.gameMu.Unlock()
	s.game.Status = "ready"
	s.game.Message = fmt.Sprintf("%d archived stories available", len(manifest.Stories))
	if e != nil {
		s.game.Status = "failed"
		s.game.Error = e.Error()
	}
	s.saveGame()
}
func (s *server) proposeGame(docID string) {
	defer func() { <-s.researches }()
	d, ok := s.store.get(docID)
	p, _ := s.store.profile("tech")
	if !ok {
		return
	}
	s.gameMu.Lock()
	accepted := s.game.Proposals[docID]
	s.gameMu.Unlock()
	if accepted.MemoVersion != p.Version || accepted.SummaryHash != d.Summary.hash() {
		s.gameMu.Lock()
		accepted.Status = "failed"
		accepted.Error = "The memo changed. Research this news again."
		s.game.Proposals[docID] = accepted
		s.saveGame()
		s.gameMu.Unlock()
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, s.jobLimit())
	defer cancel()
	ctx, span := otel.Tracer("briefing-desk").Start(ctx, "game.research")
	defer span.End()
	span.SetAttributes(attribute.String("research.skill.name", "finance-idea-generation"), attribute.String("research.skill.upstream_revision", financeSkillRevision), attribute.String("research.skill.source", financeSkillSource))
	proposal, e := s.agent.gameIdeas(ctx, d, p, s.gameDocuments(), s.game.Week)
	proposal.TraceID = "tr-" + span.SpanContext().TraceID().String()
	skill := activeFinanceSkill()
	proposal.Skill = &skill
	proposal.Status = "done"
	if e != nil {
		proposal.Status = "failed"
		proposal.Error = e.Error()
	}
	s.gameMu.Lock()
	s.game.Proposals[docID] = proposal
	s.saveGame()
	s.gameMu.Unlock()
}
func (s *server) settleRound(roundID string) {
	s.gameMu.Lock()
	if s.game.Round.ID != roundID {
		s.gameMu.Unlock()
		return
	}
	round := s.game.Round
	round.Positions = append([]paperPosition{}, round.Positions...)
	week := s.game.Week
	s.gameMu.Unlock()
	ctx, cancel := context.WithTimeout(s.ctx, 45*time.Second)
	defer cancel()
	ctx, span := otel.Tracer("briefing-desk").Start(ctx, "game.settle")
	defer span.End()
	data := map[string]historicalReturn{}
	var failure error
	for _, position := range round.Positions {
		if _, ok := data[position.Symbol]; ok {
			continue
		}
		value, e := s.agent.fetch.weekReturn(ctx, position.Symbol, week, s.currentTime())
		if e != nil {
			failure = e
			break
		}
		data[position.Symbol] = value
	}
	if failure == nil {
		round.PnL = 0
		round.AsOf = ""
		round.Complete = true
		for i, position := range round.Positions {
			r := data[position.Symbol]
			round.Positions[i].Result = &r
			round.PnL += position.Amount * r.ReturnPct / 100
			asOf := r.ExitDate
			if r.QuoteTime != "" {
				asOf = r.QuoteTime
			}
			if r.FXQuoteTime != "" && r.FXQuoteTime < asOf {
				asOf = r.FXQuoteTime
			}
			if round.AsOf == "" || asOf < round.AsOf {
				round.AsOf = asOf
			}
			round.Complete = round.Complete && r.WeekComplete
		}
		round.Value = round.Budget + round.PnL
		round.ReturnPct = round.PnL / round.Budget * 100
		round.Status = "settled"
		round.Error = ""
	} else {
		round.Status = "awaiting_prices"
		round.Error = failure.Error()
	}
	s.gameMu.Lock()
	defer s.gameMu.Unlock()
	if s.game.Round.ID == roundID {
		s.game.Round = round
		s.saveGame()
	}
}
func (s *server) gameRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/game/import", s.importGameWeek)
	mux.HandleFunc("GET /api/game", func(w http.ResponseWriter, r *http.Request) {
		s.gameMu.Lock()
		var snapshot gameState
		b, _ := json.Marshal(s.game)
		json.Unmarshal(b, &snapshot)
		s.gameMu.Unlock()
		for id, proposal := range snapshot.Proposals {
			proposal.TraceURL = mlflowTraceURL(proposal.TraceID)
			snapshot.Proposals[id] = proposal
		}
		docs := s.gameDocuments()
		p, _ := s.store.profile("tech")
		ranked := 0
		for i, d := range docs {
			if currentScore(d, p) {
				ranked++
			}
			docs[i].Source.Text = ""
		}
		sort.Slice(docs, func(i, j int) bool {
			a, b := docs[i].Classes[p.ID].Score, docs[j].Classes[p.ID].Score
			if a == nil {
				return false
			}
			if b == nil {
				return true
			}
			return *a > *b
		})
		reply(w, 200, map[string]any{"game": snapshot, "documents": docs, "memo": p, "ranked": ranked, "total": len(docs), "preview": s.preview})
	})
	mux.HandleFunc("GET /api/game/export", func(w http.ResponseWriter, r *http.Request) {
		s.gameMu.Lock()
		manifest := s.game.Manifest
		s.gameMu.Unlock()
		docs := s.gameDocuments()
		w.Header().Set("Content-Disposition", `attachment; filename="techmeme-`+manifest.Week.ID+`.json"`)
		reply(w, 200, map[string]any{"coverage": manifest.Coverage, "week": manifest.Week, "unavailable_ids": manifest.Missing, "imported_at": manifest.ImportedAt, "news": docs})
	})
	mux.HandleFunc("POST /api/game/load", func(w http.ResponseWriter, r *http.Request) {
		s.gameMu.Lock()
		if s.game.Status == "loading" || s.game.Status == "ranking" {
			s.gameMu.Unlock()
			fail(w, 409, "The week is already loading")
			return
		}
		s.game.Status = "loading"
		s.game.Error = ""
		s.saveGame()
		s.gameMu.Unlock()
		go s.loadGameWeek()
		reply(w, 202, map[string]string{"status": "loading"})
	})
	mux.HandleFunc("POST /api/game/ideas", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ArticleID string `json:"article_id"`
		}
		if !decode(w, r, &in) {
			return
		}
		d, ok := s.store.get(in.ArticleID)
		p, _ := s.store.profile("tech")
		if !ok || d.GameWeek != s.game.Week.ID || !currentScore(d, p) {
			fail(w, 409, "Choose a scored article from the archived week")
			return
		}
		s.gameMu.Lock()
		old := s.game.Proposals[d.ID]
		if old.Status == "running" {
			s.gameMu.Unlock()
			fail(w, 409, "Research is already running")
			return
		}
		select {
		case s.researches <- struct{}{}:
		default:
			s.gameMu.Unlock()
			fail(w, 429, "The research agent is busy")
			return
		}
		s.game.Proposals[d.ID] = gameProposal{ArticleID: d.ID, MemoVersion: p.Version, SummaryHash: d.Summary.hash(), Status: "running", Cutoff: s.game.Week.Cutoff}
		s.saveGame()
		s.gameMu.Unlock()
		go s.proposeGame(d.ID)
		reply(w, 202, map[string]string{"status": "running"})
	})
	mux.HandleFunc("POST /api/game/play", s.playGame)
	mux.HandleFunc("POST /api/game/settle", func(w http.ResponseWriter, r *http.Request) {
		s.gameMu.Lock()
		if s.game.Round.Status == "draft" || s.game.Round.Status == "settling" {
			s.gameMu.Unlock()
			fail(w, 409, "Commit the portfolio first, or wait for settlement")
			return
		}
		s.game.Round.Status = "settling"
		roundID := s.game.Round.ID
		s.saveGame()
		s.gameMu.Unlock()
		go s.settleRound(roundID)
		reply(w, 202, map[string]string{"status": "settling"})
	})
	mux.HandleFunc("POST /api/game/reset", func(w http.ResponseWriter, r *http.Request) {
		s.gameMu.Lock()
		defer s.gameMu.Unlock()
		if s.game.Round.Status == "settling" {
			fail(w, 409, "Wait for the current result")
			return
		}
		if s.game.Round.Status != "draft" {
			s.game.Previous = append(s.game.Previous, s.game.Round)
		}
		s.game.Round = newPaperRound()
		s.saveGame()
		reply(w, 200, s.game.Round)
	})
}
func (s *server) playGame(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RoundID     string          `json:"round_id"`
		MemoVersion int             `json:"memo_version"`
		Positions   []paperPosition `json:"positions"`
	}
	if !decode(w, r, &in) {
		return
	}
	s.gameMu.Lock()
	defer s.gameMu.Unlock()
	p, _ := s.store.profile("tech")
	if in.RoundID != s.game.Round.ID || s.game.Round.Status != "draft" {
		fail(w, 409, "This round is already committed. Start a new round to change the portfolio.")
		return
	}
	if in.MemoVersion != p.Version || len(in.Positions) == 0 || len(in.Positions) > 20 {
		fail(w, 400, "Choose current research ideas and an investment amount")
		return
	}
	total := 0.0
	seen := map[string]bool{}
	positions := []paperPosition{}
	for _, pos := range in.Positions {
		proposal := s.game.Proposals[pos.ArticleID]
		d, ok := s.store.get(pos.ArticleID)
		if !ok || d.GameWeek != s.game.Week.ID || !currentScore(d, p) || proposal.Status != "done" || proposal.MemoVersion != p.Version || proposal.SummaryHash != d.Summary.hash() || seen[d.ID] || !finitePositive(pos.Amount) || pos.Amount < 100 || pos.Amount > 10000 {
			fail(w, 400, "Invalid or outdated investment choice")
			return
		}
		matched := false
		for _, idea := range proposal.Ideas {
			if idea.ID == pos.IdeaID {
				pos.Symbol = idea.Symbol
				pos.Company = idea.Company
				matched = true
				break
			}
		}
		if !matched {
			fail(w, 400, "The index was not proposed for this news")
			return
		}
		seen[d.ID] = true
		pos.Title = d.Summary.Title
		pos.Result = nil
		total += pos.Amount
		positions = append(positions, pos)
	}
	if total > s.game.Round.Budget {
		fail(w, 400, "The portfolio exceeds your $10,000 paper budget")
		return
	}
	round := s.game.Round
	round.Status = "settling"
	round.Positions = positions
	round.Invested = total
	round.Memo = &p
	round.CommittedAt = stamp()
	s.game.Round = round
	// Persist the immutable choices before making any market-data request.
	if e := atomicJSON(s.gameFile(), s.game); e != nil {
		s.game.Round = newPaperRound()
		fail(w, 500, "The portfolio could not be committed")
		return
	}
	go s.settleRound(round.ID)
	reply(w, 202, map[string]string{"status": "settling"})
}
