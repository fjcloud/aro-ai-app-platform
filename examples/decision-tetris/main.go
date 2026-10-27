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
)

//go:embed web/*
var assets embed.FS

type session struct {
	mu      sync.Mutex
	game    *game
	id      string
	last    *decision
	history []decision
	updated time.Time
}
type server struct {
	mu                     sync.Mutex
	sessions               map[string]*session
	model                  *decisionClient
	slots                  chan struct{}
	calls, failures, lines atomic.Int64
}
type view struct {
	Game      *game      `json:"game"`
	ID        string     `json:"id"`
	Moves     []move     `json:"moves"`
	Last      *decision  `json:"last"`
	History   []decision `json:"history"`
	Model     string     `json:"model"`
	Inference string     `json:"inference"`
}

func id() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
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
	if c, err := r.Cookie("tetris_session"); err == nil {
		if g := s.sessions[c.Value]; g != nil {
			g.updated = now
			return g, nil
		}
	}
	if len(s.sessions) >= 64 {
		return nil, errors.New("workshop session capacity reached")
	}
	token := id()
	g := &session{game: newGame(42), id: id(), updated: now}
	s.sessions[token] = g
	http.SetCookie(w, &http.Cookie{Name: "tetris_session", Value: token, Path: "/", MaxAge: 7200, HttpOnly: true, Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https", SameSite: http.SameSiteStrictMode})
	return g, nil
}
func reply(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
func fail(w http.ResponseWriter, status int, message string) {
	reply(w, status, map[string]string{"error": message})
}
func (s *server) snapshot(g *session) view {
	return view{g.game, g.id, g.game.moves(), g.last, g.history, s.model.model, "local GPU service"}
}
func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	files, _ := fs.Sub(assets, "web")
	mux.Handle("GET /", http.FileServer(http.FS(files)))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /api/game", func(w http.ResponseWriter, r *http.Request) {
		g, err := s.get(w, r)
		if err != nil {
			fail(w, 503, err.Error())
			return
		}
		g.mu.Lock()
		defer g.mu.Unlock()
		reply(w, 200, s.snapshot(g))
	})
	mux.HandleFunc("POST /api/game/new", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Seed int64 `json:"seed"`
		}
		if !decode(w, r, &in) {
			return
		}
		g, err := s.get(w, r)
		if err != nil {
			fail(w, 503, err.Error())
			return
		}
		if !g.mu.TryLock() {
			fail(w, 409, "Pause the current decision before starting a new game")
			return
		}
		defer g.mu.Unlock()
		g.game = newGame(in.Seed)
		g.id = id()
		g.last = nil
		g.history = nil
		reply(w, 200, s.snapshot(g))
	})
	mux.HandleFunc("POST /api/game/step", s.step)
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.Write([]byte("# TYPE tetris_model_requests_total counter\ntetris_model_requests_total " + strconv.FormatInt(s.calls.Load(), 10) + "\n# TYPE tetris_model_errors_total counter\ntetris_model_errors_total " + strconv.FormatInt(s.failures.Load(), 10) + "\n# TYPE tetris_lines_total counter\ntetris_lines_total " + strconv.FormatInt(s.lines.Load(), 10) + "\n"))
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'self'; base-uri 'none'; object-src 'none'")
		if r.Method == "POST" {
			if o := r.Header.Get("Origin"); o != "" {
				u, err := url.Parse(o)
				if err != nil || u.Host != r.Host || (u.Scheme != "https" && u.Scheme != "http") {
					fail(w, 403, "Cross-origin request refused")
					return
				}
			}
		}
		mux.ServeHTTP(w, r)
	})
}
func decode(w http.ResponseWriter, r *http.Request, in any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(in) != nil {
		fail(w, 400, "Invalid game request")
		return false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		fail(w, 400, "Expected one JSON object")
		return false
	}
	return true
}
func (s *server) step(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID   string `json:"id"`
		Turn int    `json:"turn"`
		Mode string `json:"mode"`
		Move string `json:"move"`
	}
	if !decode(w, r, &in) {
		return
	}
	g, err := s.get(w, r)
	if err != nil {
		fail(w, 503, err.Error())
		return
	}
	if !g.mu.TryLock() {
		fail(w, 409, "A decision is already running")
		return
	}
	defer g.mu.Unlock()
	if in.ID != g.id || in.Turn != g.game.Turn {
		fail(w, 409, "Game state changed; reload before playing")
		return
	}
	if g.game.Over {
		fail(w, 409, "Game over; start a new game")
		return
	}
	ctx, span := otel.Tracer("decision-tetris").Start(r.Context(), "tetris.turn")
	defer span.End()
	span.SetAttributes(attribute.String("mlflow.spanType", "AGENT"), attribute.String("decision.source", in.Mode), attribute.String("game.piece", g.game.Piece))
	r = r.WithContext(ctx)
	moves := g.game.moves()
	var out decision
	switch in.Mode {
	case "model":
		if len(moves) == 1 {
			out = decision{Source: "forced", Choice: moves[0].ID, Probabilities: map[string]float64{moves[0].ID: 1}}
		} else {
			select {
			case s.slots <- struct{}{}:
				defer func() { <-s.slots }()
			default:
				fail(w, 429, "The local GPU is busy; try again shortly")
				return
			}
			s.calls.Add(1)
			ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
			defer cancel()
			out, err = s.model.choose(ctx, g.game, moves)
			if err != nil {
				s.failures.Add(1)
				fail(w, 502, err.Error())
				return
			}
		}
	case "heuristic":
		out = decision{Source: "heuristic", Choice: heuristic(moves).ID}
	case "manual":
		out = decision{Source: "manual", Choice: in.Move}
	default:
		fail(w, 400, "Choose model, heuristic or manual mode")
		return
	}
	if r.Context().Err() != nil {
		return
	}
	var chosen *move
	for i := range moves {
		if moves[i].ID == out.Choice {
			chosen = &moves[i]
			break
		}
	}
	if chosen == nil {
		fail(w, 400, "Illegal placement")
		return
	}
	out.Features = chosen.Features
	out.TraceID = span.SpanContext().TraceID().String()
	span.SetAttributes(attribute.String("decision.choice", out.Choice), attribute.Int("game.lines_cleared", chosen.Features.Lines), attribute.Int("game.holes", chosen.Features.Holes), attribute.Int64("model.latency_ms", out.LatencyMS))
	g.game.place(*chosen)
	s.lines.Add(int64(chosen.Features.Lines))
	g.last = &out
	g.history = append([]decision{out}, g.history...)
	if len(g.history) > 20 {
		g.history = g.history[:20]
	}
	reply(w, 200, s.snapshot(g))
}
func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
func main() {
	model, err := newDecisionClient(env("DECISION_URL", "http://d3-mini.decision-serving.svc.cluster.local:8080/v1/systemone"), env("DECISION_MODEL", "vllm-sr/d3-mini"), os.Getenv("DECISION_KEY_FILE"))
	if err != nil {
		log.Fatal(err)
	}
	shutdownTracing, err := initTracing(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		shutdownTracing(ctx)
	}()
	s := &server{sessions: map[string]*session{}, model: model, slots: make(chan struct{}, 2)}
	httpServer := &http.Server{Addr: env("LISTEN_ADDR", ":8080"), Handler: s.routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 45 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		httpServer.Shutdown(shutdown)
	}()
	log.Print("Decision Tetris listening")
	if err = httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
