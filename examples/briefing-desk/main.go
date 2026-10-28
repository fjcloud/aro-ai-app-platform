package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

//go:embed web/*
var assets embed.FS

type scanState struct {
	Status string  `json:"status"`
	Error  string  `json:"error,omitempty"`
	Events []event `json:"events"`
}
type server struct {
	store                        *store
	agent                        *agent
	model                        *decisionClient
	ctx                          context.Context
	imports, classes, researches chan struct{}
	mu                           sync.Mutex
	ops                          sync.Mutex
	scan                         scanState
}

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, msg string) {
	reply(w, status, map[string]string{"error": msg})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(new(any)) != io.EOF {
		fail(w, 400, "Expected one valid JSON request")
		return false
	}
	return true
}
func (s *server) record(docID string, e event) {
	if err := s.store.update(docID, func(d *document) {
		d.Events = append(d.Events, e)
		if len(d.Events) > 40 {
			d.Events = d.Events[len(d.Events)-40:]
		}
	}); err != nil {
		log.Print("Could not persist article activity")
	}
}
func (s *server) importArticle(docID, profileID string) {
	defer func() { <-s.imports }()
	ctx, cancel := context.WithTimeout(s.ctx, 4*time.Minute)
	defer cancel()
	ctx, span := otel.Tracer("briefing-desk").Start(ctx, "news.import")
	defer span.End()
	span.SetAttributes(attribute.String("mlflow.spanType", "CHAIN"))
	d, ok := s.store.get(docID)
	if !ok {
		return
	}
	s.store.update(docID, func(d *document) {
		d.Status = "summarizing"
		d.Error = ""
		d.TraceID = span.SpanContext().TraceID().String()
	})
	src, sum, e := s.agent.summarize(ctx, d.URL, func(e event) { s.record(docID, e) })
	if e != nil {
		s.store.update(docID, func(d *document) { d.Status = "failed"; d.Error = e.Error() })
		return
	}
	if e = s.store.update(docID, func(d *document) {
		d.Status = "ready"
		src.Published = d.Source.Published
		d.Source = src
		d.Summary = sum
		d.Markdown = documentMarkdown(*d)
		d.Events = append(d.Events, event{"summary", "English summary saved", stamp(), 0})
	}); e != nil {
		return
	}
	if profileID != "" {
		select {
		case s.classes <- struct{}{}:
			s.classify(docID, profileID)
		case <-ctx.Done():
			s.record(docID, event{"classification", "Classifier busy; select Classify to retry", stamp(), 0})
		}
	}
}
func (s *server) classify(docID, profileID string) {
	defer func() { <-s.classes }()
	d, ok := s.store.get(docID)
	if !ok || d.Status != "ready" {
		return
	}
	p, ok := s.store.profile(profileID)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()
	ctx, span := otel.Tracer("briefing-desk").Start(ctx, "news.triage")
	defer span.End()
	c := classification{ProfileID: p.ID, ProfileVersion: p.Version, SummaryHash: d.Summary.hash(), Status: "running", At: stamp(), TraceID: span.SpanContext().TraceID().String()}
	s.store.update(docID, func(d *document) { d.Classes[p.ID] = c })
	out, e := s.model.classify(ctx, d.Summary, p)
	if e != nil {
		c.Status = "failed"
		c.Error = e.Error()
	} else {
		c.Status = "done"
		c.Priority = out.Choice
		c.Confidence = out.Confidence
		c.Probabilities = out.Probabilities
		c.LatencyMS = out.LatencyMS
		c.InputTokens = out.InputTokens
	}
	s.store.update(docID, func(d *document) { d.Classes[p.ID] = c })
}
func (s *server) investigate(docID, profileID string) {
	defer func() { <-s.researches }()
	d, _ := s.store.get(docID)
	p, _ := s.store.profile(profileID)
	ctx, cancel := context.WithTimeout(s.ctx, 4*time.Minute)
	defer cancel()
	ctx, span := otel.Tracer("briefing-desk").Start(ctx, "news.investigate")
	defer span.End()
	trace := span.SpanContext().TraceID().String()
	s.store.update(docID, func(d *document) { r := d.Research[p.ID]; r.TraceID = trace; d.Research[p.ID] = r })
	out, e := s.agent.investigate(ctx, d, p, func(e event) {
		s.store.update(docID, func(d *document) { r := d.Research[p.ID]; r.Events = append(r.Events, e); d.Research[p.ID] = r })
	})
	out.TraceID = trace
	out.Status = "done"
	if e != nil {
		out.Status = "failed"
		out.Error = e.Error()
	}
	if err := s.store.update(docID, func(d *document) { out.Events = d.Research[p.ID].Events; d.Research[p.ID] = out }); err != nil {
		log.Print("Could not persist research result")
	}
}
func (s *server) scanFeed(feedID, profileID string) {
	ctx, cancel := context.WithTimeout(s.ctx, 4*time.Minute)
	defer cancel()
	ctx, span := otel.Tracer("briefing-desk").Start(ctx, "news.scan")
	defer span.End()
	emit := func(e event) { s.mu.Lock(); s.scan.Events = append(s.scan.Events, e); s.mu.Unlock() }
	items, e := s.agent.collect(ctx, feedID, emit)
	if e == nil {
		for _, h := range items {
			d, created, err := s.store.add(h.URL)
			if err != nil {
				e = err
				break
			}
			if !created {
				continue
			}
			s.store.update(d.ID, func(d *document) { d.Source.Published = h.Published })
			select {
			case s.imports <- struct{}{}:
				go s.importArticle(d.ID, profileID)
			case <-ctx.Done():
				e = errors.New("Import queue timed out")
				s.store.update(d.ID, func(d *document) { d.Status = "failed"; d.Error = "Import queue timed out" })
			}
			if e != nil {
				break
			}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scan.Status = "done"
	if e != nil {
		s.scan.Status = "failed"
		s.scan.Error = e.Error()
	} else {
		s.scan.Events = append(s.scan.Events, event{"feed", "Selected articles added to the feed", stamp(), 0})
	}
}
func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	files, _ := fs.Sub(assets, "web")
	mux.Handle("GET /", http.FileServer(http.FS(files)))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		docs, profiles := s.store.list()
		for i := range docs {
			for k, v := range docs[i].Research {
				v.Evidence = nil
				v.Report = report{}
				docs[i].Research[k] = v
			}
		}
		s.mu.Lock()
		scan := s.scan
		scan.Events = append([]event{}, scan.Events...)
		s.mu.Unlock()
		reply(w, 200, map[string]any{"documents": docs, "profiles": profiles, "feeds": feeds, "scan": scan, "classifier": s.model.model, "agent": s.agent.model})
	})
	mux.HandleFunc("GET /api/articles/{id}", func(w http.ResponseWriter, r *http.Request) {
		d, ok := s.store.get(r.PathValue("id"))
		if !ok {
			fail(w, 404, "Article not found")
			return
		}
		d.Source.Text = ""
		reply(w, 200, d)
	})
	mux.HandleFunc("POST /api/profiles", func(w http.ResponseWriter, r *http.Request) {
		var p profile
		if !decode(w, r, &p) {
			return
		}
		p, e := s.store.saveProfile(p)
		if e != nil {
			fail(w, 400, e.Error())
			return
		}
		reply(w, 200, p)
	})
	mux.HandleFunc("POST /api/scan", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			FeedID    string `json:"feed_id"`
			ProfileID string `json:"profile_id"`
		}
		if !decode(w, r, &in) {
			return
		}
		valid := false
		for _, f := range feeds {
			valid = valid || f.ID == in.FeedID
		}
		_, ok := s.store.profile(in.ProfileID)
		if !valid || !ok {
			fail(w, 400, "Select a feed and a fund mandate")
			return
		}
		s.mu.Lock()
		if s.scan.Status == "running" {
			s.mu.Unlock()
			fail(w, 409, "A news scan is already running")
			return
		}
		s.scan = scanState{Status: "running", Events: []event{}}
		s.mu.Unlock()
		go s.scanFeed(in.FeedID, in.ProfileID)
		reply(w, 202, map[string]string{"status": "running"})
	})
	mux.HandleFunc("POST /api/import", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			URL       string `json:"url"`
			ProfileID string `json:"profile_id"`
		}
		if !decode(w, r, &in) {
			return
		}
		if _, e := publicURL(in.URL); e != nil {
			fail(w, 400, e.Error())
			return
		}
		if _, ok := s.store.profile(in.ProfileID); !ok {
			fail(w, 400, "Select a fund mandate")
			return
		}
		select {
		case s.imports <- struct{}{}:
		default:
			fail(w, 429, "Two imports are already running. Try again shortly.")
			return
		}
		s.ops.Lock()
		d, created, e := s.store.add(in.URL)
		if e == nil && !created && d.Status == "failed" {
			e = s.store.update(d.ID, func(d *document) { d.Status = "queued"; d.Error = "" })
			created = e == nil
		}
		s.ops.Unlock()
		if e != nil {
			<-s.imports
			fail(w, 400, e.Error())
			return
		}
		if created {
			go s.importArticle(d.ID, in.ProfileID)
		} else {
			<-s.imports
		}
		reply(w, 202, map[string]string{"id": d.ID})
	})
	mux.HandleFunc("POST /api/articles/{id}/{action}", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ProfileID string `json:"profile_id"`
		}
		if !decode(w, r, &in) {
			return
		}
		s.ops.Lock()
		defer s.ops.Unlock()
		docID := r.PathValue("id")
		d, ok := s.store.get(docID)
		p, pok := s.store.profile(in.ProfileID)
		if !ok || !pok {
			fail(w, 404, "Article or fund mandate not found")
			return
		}
		if d.Status != "ready" {
			fail(w, 409, "Wait for the summary before continuing")
			return
		}
		switch r.PathValue("action") {
		case "classify":
			select {
			case s.classes <- struct{}{}:
			default:
				fail(w, 429, "The T4 classifier is busy. Try again shortly.")
				return
			}
			s.store.update(d.ID, func(d *document) {
				d.Classes[p.ID] = classification{ProfileID: p.ID, Status: "queued", ProfileVersion: p.Version}
			})
			go s.classify(d.ID, p.ID)
		case "investigate":
			c := d.Classes[p.ID]
			if c.Status != "done" || c.ProfileVersion != p.Version || c.SummaryHash != d.Summary.hash() || (c.Priority != "material" && c.Priority != "monitor") {
				fail(w, 409, "Investigate requires a fresh classification with financial relevance")
				return
			}
			if d.Research[p.ID].Status == "running" {
				fail(w, 409, "Research is already running")
				return
			}
			select {
			case s.researches <- struct{}{}:
			default:
				fail(w, 429, "The research agent is busy. Try again shortly.")
				return
			}
			s.store.update(d.ID, func(d *document) {
				d.Research[p.ID] = research{Status: "running", ProfileVersion: p.Version, SummaryHash: d.Summary.hash(), At: stamp(), Events: []event{}}
			})
			go s.investigate(d.ID, p.ID)
		default:
			fail(w, 404, "Unknown article action")
			return
		}
		reply(w, 202, map[string]string{"status": "queued"})
	})
	mux.HandleFunc("GET /api/export/{id}", func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.store.profile(r.URL.Query().Get("profile"))
		if !ok {
			fail(w, 404, "Fund mandate not found")
			return
		}
		text := ""
		if r.PathValue("id") == "briefing" {
			docs, _ := s.store.list()
			text = briefing(p, docs)
		} else {
			d, ok := s.store.get(r.PathValue("id"))
			if !ok || d.Status != "ready" {
				fail(w, 404, "Ready article not found")
				return
			}
			if r.URL.Query().Get("type") == "research" {
				v := d.Research[p.ID]
				if v.Status != "done" || v.ProfileVersion != p.Version || v.SummaryHash != d.Summary.hash() {
					fail(w, 409, "No current completed research draft for this mandate")
					return
				}
				text = researchMarkdown(d, p, v)
			} else {
				text = d.Markdown
			}
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="signal-desk.md"`)
		w.Write([]byte(text))
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; frame-ancestors 'self'; base-uri 'none'; object-src 'none'")
		if r.Method == "POST" {
			if o := r.Header.Get("Origin"); o != "" {
				u, e := url.Parse(o)
				if e != nil || u.Host != r.Host || (u.Scheme != "http" && u.Scheme != "https") {
					fail(w, 403, "Cross-origin request refused")
					return
				}
			}
			if v := r.Header.Get("Sec-Fetch-Site"); v != "" && v != "same-origin" && v != "none" {
				fail(w, 403, "Cross-origin request refused")
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	st, e := openStore(env("DATA_DIR", "./data"))
	if e != nil {
		log.Fatal(e)
	}
	a, e := newAgent(env("AGENT_URL", "http://lsd-genai-playground-service.live-transcript-dev.svc.cluster.local:8321/v1"), env("AGENT_MODEL", "maas-vllm-inference-1/deepseekv4pro"), env("AGENT_KEY_FILE", "/etc/model/api-key"), env("MCP_URL", "http://live-transcript.live-transcript-dev.svc.cluster.local:8090/mcp"))
	if e != nil {
		log.Fatal(e)
	}
	model, e := newDecisionClient(env("DECISION_URL", "http://d3-mini.decision-serving.svc.cluster.local:8080/v1/systemone"), env("DECISION_MODEL", "vllm-sr/d3-mini"), "")
	if e != nil {
		log.Fatal(e)
	}
	shutdown, e := initTracing(ctx)
	if e != nil {
		log.Fatal(e)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		shutdown(ctx)
	}()
	s := &server{store: st, agent: a, model: model, ctx: ctx, imports: make(chan struct{}, 2), classes: make(chan struct{}, 1), researches: make(chan struct{}, 1)}
	public := &http.Server{Addr: env("LISTEN_ADDR", ":8080"), Handler: s.routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	internal := &http.Server{Addr: env("MCP_LISTEN_ADDR", ":8090"), Handler: a.mcpHandler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 90 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16384}
	go func() {
		if e := internal.ListenAndServe(); e != nil && !errors.Is(e, http.ErrServerClosed) {
			log.Print("Internal research tools listener failed")
			stop()
		}
	}()
	go func() {
		<-ctx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		public.Shutdown(ctx)
		internal.Shutdown(ctx)
	}()
	log.Print("Signal Desk listening on ", public.Addr)
	if e := public.ListenAndServe(); e != nil && !errors.Is(e, http.ErrServerClosed) {
		log.Fatal(e)
	}
}
