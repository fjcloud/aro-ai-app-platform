package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

//go:embed web/*
var assets embed.FS

type session struct {
	mu      sync.Mutex
	id      string
	shift   *shift
	updated time.Time
}
type server struct {
	mu                       sync.Mutex
	sessions                 map[string]*session
	model                    *decisionClient
	slots                    chan struct{}
	calls, failures, correct atomic.Int64
}
type view struct {
	ID             string       `json:"id"`
	Seed           int64        `json:"seed"`
	Round          int          `json:"round"`
	Total          int          `json:"total"`
	Over           bool         `json:"over"`
	Document       string       `json:"document"`
	StandingOrders string       `json:"standing_orders"`
	Directive      directive    `json:"directive"`
	Result         *resolution  `json:"result"`
	History        []resolution `json:"history"`
	HumanPlayed    int          `json:"human_played"`
	HumanCorrect   int          `json:"human_correct"`
	ModelCorrect   int          `json:"model_correct"`
	Streak         int          `json:"streak"`
	Score          int          `json:"score"`
	Model          string       `json:"model"`
}

func id() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, msg string) {
	reply(w, status, map[string]string{"error": msg})
}

func (s *server) get(w http.ResponseWriter, r *http.Request) (*session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, g := range s.sessions {
		if now.Sub(g.updated) > 2*time.Hour {
			delete(s.sessions, k)
		}
	}
	if c, e := r.Cookie("checkpoint_session"); e == nil {
		if g := s.sessions[c.Value]; g != nil {
			g.updated = now
			return g, nil
		}
	}
	if len(s.sessions) >= 64 {
		return nil, errors.New("Workshop session capacity reached")
	}
	token := id()
	g := &session{id: id(), shift: newShift(42), updated: now}
	s.sessions[token] = g
	http.SetCookie(w, &http.Cookie{Name: "checkpoint_session", Value: token, Path: "/", MaxAge: 7200, HttpOnly: true, Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https", SameSite: http.SameSiteStrictMode})
	return g, nil
}
func (s *server) snapshot(g *session) view {
	v := view{ID: g.id, Seed: g.shift.Seed, Round: g.shift.Round, Total: shiftLength, Over: g.shift.over(), StandingOrders: standingOrders, Directive: g.shift.directive(), Result: g.shift.Result, History: g.shift.History, HumanPlayed: g.shift.HumanPlayed, HumanCorrect: g.shift.HumanCorrect, ModelCorrect: g.shift.ModelCorrect, Streak: g.shift.Streak, Score: g.shift.Score, Model: s.model.model}
	if !v.Over {
		v.Document = "/api/document?id=" + g.id + "&round=" + strconv.Itoa(g.shift.Round)
	}
	return v
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		fail(w, 400, "Invalid game request")
		return false
	}
	if d.Decode(new(any)) != io.EOF {
		fail(w, 400, "Expected one JSON object")
		return false
	}
	return true
}
func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	files, _ := fs.Sub(assets, "web")
	mux.Handle("GET /", http.FileServer(http.FS(files)))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /api/shift", func(w http.ResponseWriter, r *http.Request) {
		g, e := s.get(w, r)
		if e != nil {
			fail(w, 503, e.Error())
			return
		}
		g.mu.Lock()
		defer g.mu.Unlock()
		reply(w, 200, s.snapshot(g))
	})
	mux.HandleFunc("POST /api/shift/new", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Seed int64 `json:"seed"`
		}
		if !decode(w, r, &in) {
			return
		}
		g, e := s.get(w, r)
		if e != nil {
			fail(w, 503, e.Error())
			return
		}
		if !g.mu.TryLock() {
			fail(w, 409, "Pause the current inspection first")
			return
		}
		defer g.mu.Unlock()
		g.id = id()
		g.shift = newShift(in.Seed)
		reply(w, 200, s.snapshot(g))
	})
	mux.HandleFunc("GET /api/document", func(w http.ResponseWriter, r *http.Request) {
		g, e := s.get(w, r)
		if e != nil {
			fail(w, 503, e.Error())
			return
		}
		g.mu.Lock()
		defer g.mu.Unlock()
		if g.shift.over() || r.URL.Query().Get("id") != g.id || r.URL.Query().Get("round") != strconv.Itoa(g.shift.Round) {
			fail(w, 409, "Document is no longer current")
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(paper(g.shift.Cases[g.shift.Round]))
	})
	mux.HandleFunc("POST /api/shift/decide", s.decide)
	mux.HandleFunc("POST /api/shift/next", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID    string `json:"id"`
			Round int    `json:"round"`
		}
		if !decode(w, r, &in) {
			return
		}
		g, e := s.get(w, r)
		if e != nil {
			fail(w, 503, e.Error())
			return
		}
		if !g.mu.TryLock() {
			fail(w, 409, "An inspection is running")
			return
		}
		defer g.mu.Unlock()
		if in.ID != g.id || in.Round != g.shift.Round || g.shift.Result == nil || g.shift.over() {
			fail(w, 409, "Shift state changed; reload before continuing")
			return
		}
		g.shift.Round++
		g.shift.Result = nil
		reply(w, 200, s.snapshot(g))
	})
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.Write([]byte("# TYPE checkpoint_model_requests_total counter\ncheckpoint_model_requests_total " + strconv.FormatInt(s.calls.Load(), 10) + "\n# TYPE checkpoint_model_errors_total counter\ncheckpoint_model_errors_total " + strconv.FormatInt(s.failures.Load(), 10) + "\n# TYPE checkpoint_model_correct_total counter\ncheckpoint_model_correct_total " + strconv.FormatInt(s.correct.Load(), 10) + "\n"))
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; frame-ancestors 'self'; base-uri 'none'; object-src 'none'")
		if r.Method == "POST" {
			if o := r.Header.Get("Origin"); o != "" {
				u, e := url.Parse(o)
				if e != nil || u.Host != r.Host || (u.Scheme != "https" && u.Scheme != "http") {
					fail(w, 403, "Cross-origin request refused")
					return
				}
			}
		}
		mux.ServeHTTP(w, r)
	})
}
func (s *server) decide(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID     string `json:"id"`
		Round  int    `json:"round"`
		Action string `json:"action"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Action != "clear" && in.Action != "inspect" && in.Action != "reject" && in.Action != "watch" {
		fail(w, 400, "Choose clear, inspect, reject or watch")
		return
	}
	g, e := s.get(w, r)
	if e != nil {
		fail(w, 503, e.Error())
		return
	}
	if !g.mu.TryLock() {
		fail(w, 409, "An inspection is running")
		return
	}
	defer g.mu.Unlock()
	if in.ID != g.id || in.Round != g.shift.Round || g.shift.over() || g.shift.Result != nil {
		fail(w, 409, "Shift state changed; reload before deciding")
		return
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		fail(w, 429, "The local GPU is busy; try again shortly")
		return
	}
	ctx, span := otel.Tracer("orbit-checkpoint").Start(r.Context(), "checkpoint.inspect")
	defer span.End()
	span.SetAttributes(attribute.String("mlflow.spanType", "AGENT"), attribute.Int("checkpoint.round", g.shift.Round+1), attribute.String("checkpoint.directive", g.shift.directive().Name))
	s.calls.Add(1)
	out, err := s.model.choose(ctx, paper(g.shift.Cases[g.shift.Round]), g.shift.directive())
	if err != nil {
		s.failures.Add(1)
		span.SetStatus(codes.Error, "local visual inference failed")
		fail(w, 502, err.Error())
		return
	}
	if r.Context().Err() != nil {
		return
	}
	out.TraceID = span.SpanContext().TraceID().String()
	human := in.Action
	if human == "watch" {
		human = ""
	}
	resolved := g.shift.resolve(human, out)
	if resolved.ModelCorrect {
		s.correct.Add(1)
	}
	span.SetAttributes(attribute.String("checkpoint.model_verdict", out.Choice), attribute.String("checkpoint.expected", resolved.Expected), attribute.Bool("checkpoint.model_correct", resolved.ModelCorrect), attribute.String("checkpoint.human_verdict", human), attribute.Bool("checkpoint.human_correct", resolved.HumanCorrect))
	reply(w, 200, s.snapshot(g))
}
func main() {
	if len(os.Args) > 1 && os.Args[1] == "evaluate" {
		if err := evaluate(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}
	model, err := newDecisionClient(env("DECISION_URL", "http://d3-mini.decision-serving.svc.cluster.local:8080/v1/systemone"), env("DECISION_MODEL", "vllm-sr/d3-mini"), os.Getenv("DECISION_KEY_FILE"))
	if err != nil {
		log.Fatal(err)
	}
	shutdown, err := initTracing(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		shutdown(ctx)
	}()
	s := &server{sessions: map[string]*session{}, model: model, slots: make(chan struct{}, 1)}
	h := &http.Server{Addr: env("LISTEN_ADDR", ":8080"), Handler: s.routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 85 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		h.Shutdown(ctx)
	}()
	log.Print("Orbit Checkpoint listening on ", h.Addr)
	if e := h.ListenAndServe(); e != nil && !errors.Is(e, http.ErrServerClosed) {
		log.Fatal(e)
	}
}
