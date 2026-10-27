package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
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

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func id() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, msg string) {
	reply(w, status, map[string]string{"error": msg})
}

type probability struct {
	ID          string  `json:"id"`
	Probability float64 `json:"probability"`
}
type event struct {
	At          float64       `json:"at"`
	Choice      string        `json:"choice"`
	Flight      string        `json:"flight,omitempty"`
	Runway      string        `json:"runway,omitempty"`
	Kind        string        `json:"kind"`
	Status      string        `json:"status"`
	Reason      string        `json:"reason"`
	TotalMS     int64         `json:"total_ms"`
	InferenceMS int64         `json:"inference_ms"`
	QueueMS     int64         `json:"queue_ms"`
	PrepareMS   int64         `json:"prepare_ms"`
	BudgetMS    int64         `json:"budget_ms"`
	InputTokens int           `json:"input_tokens"`
	Confidence  float64       `json:"confidence"`
	TraceID     string        `json:"trace_id,omitempty"`
	Top         []probability `json:"top,omitempty"`
}
type session struct {
	mu               sync.Mutex
	id               string
	world            *world
	playing, pending bool
	seen             atomic.Int64
	pendingSince     time.Time
	generation, job  uint64
	cancel           context.CancelFunc
	history          []event
	lastStart        time.Time
}
type server struct {
	mu                    sync.Mutex
	sessions              map[string]*session
	model                 *decisionClient
	slots                 chan struct{}
	calls, failures, late atomic.Int64
}
type view struct {
	ID          string  `json:"id"`
	World       *world  `json:"world"`
	Playing     bool    `json:"playing"`
	Pending     bool    `json:"pending"`
	PendingMS   int64   `json:"pending_ms"`
	BudgetMS    int64   `json:"budget_ms"`
	History     []event `json:"history"`
	Model       string  `json:"model"`
	State       string  `json:"state"`
	Message     string  `json:"message"`
	Utilization float64 `json:"utilization"`
}

func (s *server) get(w http.ResponseWriter, r *http.Request) (*session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, g := range s.sessions {
		if now.UnixNano()-g.seen.Load() > int64(10*time.Minute) {
			delete(s.sessions, k)
		}
	}
	if c, e := r.Cookie("sky_session"); e == nil {
		if g := s.sessions[c.Value]; g != nil {
			g.seen.Store(now.UnixNano())
			return g, nil
		}
	}
	if len(s.sessions) >= 16 {
		return nil, errors.New("Workshop session capacity reached")
	}
	token := id()
	g := &session{id: id(), world: newWorld(42, settings{6, .4, 8}), generation: 1}
	g.seen.Store(now.UnixNano())
	s.sessions[token] = g
	http.SetCookie(w, &http.Cookie{Name: "sky_session", Value: token, Path: "/", MaxAge: 7200, HttpOnly: true, Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https", SameSite: http.SameSiteStrictMode})
	return g, nil
}
func (s *server) snapshot(g *session) view {
	v := view{ID: g.id, World: g.world.clone(), Playing: g.playing, Pending: g.pending, BudgetMS: int64(freshnessSeconds / g.world.Config.Speed * 1000), History: append([]event{}, g.history...), Model: s.model.model, State: "ready", Message: "Sector ready. Start the AI controller."}
	if g.pending {
		v.PendingMS = time.Since(g.pendingSince).Milliseconds()
	}
	if g.playing {
		v.State = "nominal"
		v.Message = "Live control. Cleared routes continue while the model thinks."
	} else if len(g.history) > 0 {
		v.State = "paused"
		v.Message = "Sector paused."
	}
	if len(g.history) > 0 {
		last := g.history[len(g.history)-1]
		if last.Status != "cancelled" {
			v.Utilization = float64(last.TotalMS) / float64(v.BudgetMS)
		}
		if g.playing && v.Utilization >= 1 {
			v.State = "critical"
			v.Message = "Inference exceeds the current decision-freshness window."
		} else if g.playing && v.Utilization >= .7 {
			v.State = "warning"
			v.Message = "Inference is approaching the decision-freshness limit."
		}
		if last.Status == "error" {
			v.State = "critical"
			v.Message = "Inference failed. No automatic clearance was substituted."
		}
	}
	if g.pending && v.PendingMS > v.BudgetMS {
		v.State = "critical"
		v.Message = "Inference is overdue. Aircraft continue their previously assigned routes."
	}
	if g.world.Backlog > g.world.Config.Aircraft && v.State != "critical" {
		v.State = "capacity"
		v.Message = "Arrival demand exceeds admission capacity; extra aircraft wait outside the sector."
	}
	if g.world.Stopped != "" {
		v.State = "protected"
		v.Message = g.world.Stopped
	}
	return v
}
func (s *server) run(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	last := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			dt := math.Min(now.Sub(last).Seconds(), .5)
			last = now
			s.mu.Lock()
			all := make([]*session, 0, len(s.sessions))
			for _, g := range s.sessions {
				all = append(all, g)
			}
			s.mu.Unlock()
			for _, g := range all {
				g.mu.Lock()
				if now.UnixNano()-g.seen.Load() > int64(2*time.Minute) {
					g.playing = false
					if g.cancel != nil {
						g.cancel()
					}
				}
				if g.playing && g.world.Stopped == "" {
					g.world.advance(dt * g.world.Config.Speed)
					if g.world.Stopped != "" {
						g.playing = false
					}
				}
				if g.playing && !g.pending && now.Sub(g.lastStart) >= 300*time.Millisecond {
					waiting := false
					for _, p := range g.world.Flights {
						if p.Phase == "holding" || p.Phase == "ready" {
							waiting = true
							break
						}
					}
					if waiting {
						g.pending = true
						g.pendingSince = now
						g.lastStart = now
						g.job++
						job := g.job
						generation := g.generation
						state := g.world.clone()
						jobCtx, cancel := context.WithCancel(ctx)
						g.cancel = cancel
						go s.infer(jobCtx, g, state, job, generation)
					}
				}
				g.mu.Unlock()
			}
		}
	}
}
func (s *server) infer(ctx context.Context, g *session, state *world, job, generation uint64) {
	started := time.Now()
	ctx, span := otel.Tracer("sky-control").Start(ctx, "atc.cycle")
	defer span.End()
	span.SetAttributes(attribute.String("mlflow.spanType", "AGENT"), attribute.Int("atc.aircraft", len(state.Flights)), attribute.Float64("atc.sim_speed", state.Config.Speed))
	options := state.candidates()
	prepared := time.Now()
	queueTimer := time.NewTimer(2 * time.Second)
	defer queueTimer.Stop()
	var out decision
	var err error
	acquired := false
	select {
	case s.slots <- struct{}{}:
		acquired = true
	case <-ctx.Done():
		err = ctx.Err()
	case <-queueTimer.C:
		err = errors.New("Local inference queue exceeded two seconds")
	}
	queued := time.Now()
	if acquired {
		s.calls.Add(1)
		out, err = s.model.choose(ctx, state, options)
		<-s.slots
	}
	ended := time.Now()
	budget := int64(freshnessSeconds / state.Config.Speed * 1000)
	e := event{At: state.Seconds, Choice: out.Choice, Kind: "wait", Status: "accepted", TotalMS: ended.Sub(started).Milliseconds(), PrepareMS: prepared.Sub(started).Milliseconds(), QueueMS: queued.Sub(prepared).Milliseconds(), InferenceMS: out.LatencyMS, BudgetMS: budget, InputTokens: out.InputTokens, Confidence: out.Confidence, TraceID: span.SpanContext().TraceID().String()}
	for key, p := range out.Probabilities {
		e.Top = append(e.Top, probability{key, p})
	}
	sort.Slice(e.Top, func(i, j int) bool { return e.Top[i].Probability > e.Top[j].Probability })
	if len(e.Top) > 3 {
		e.Top = e.Top[:3]
	}
	chosen := clearance{}
	for _, a := range options {
		if a.ID == out.Choice {
			chosen = a
			e.Flight = a.Flight
			e.Kind = a.Kind
			e.Runway = a.Runway
			break
		}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.job != job {
		return
	}
	g.pending = false
	cancelled := ctx.Err() != nil
	if g.cancel != nil {
		g.cancel()
		g.cancel = nil
	}
	if !g.playing || g.generation != generation || cancelled {
		e.Status = "cancelled"
		e.Reason = "Sector paused or settings changed while inference was running."
	} else {
		g.world.Stats.Decisions++
		switch {
		case err != nil:
			g.world.Stats.Errors++
			s.failures.Add(1)
			e.Status = "error"
			e.Reason = err.Error()
			span.SetStatus(codes.Error, "local inference failed")
		case e.TotalMS > budget:
			g.world.Stats.Late++
			s.late.Add(1)
			e.Status = "late"
			e.Reason = "Decision exceeded the 45-simulation-second freshness window; no clearance applied."
		case !chosen.Safe:
			g.world.Stats.Unsafe++
			e.Status = "unsafe"
			e.Reason = chosen.Reason
		default:
			if applyErr := g.world.apply(chosen); applyErr != nil {
				g.world.Stats.Stale++
				e.Status = "stale"
				e.Reason = "Conditions changed: " + applyErr.Error()
			} else {
				g.world.Stats.Accepted++
				if chosen.Kind == "wait" {
					e.Reason = "Maintain assigned holds and previously cleared routes."
				} else {
					e.Reason = fmtClearance(chosen)
				}
			}
		}
	}
	span.SetAttributes(attribute.String("atc.outcome", e.Status), attribute.String("atc.choice", e.Choice), attribute.Int64("atc.total_ms", e.TotalMS), attribute.Int64("atc.queue_ms", e.QueueMS), attribute.Int64("atc.budget_ms", e.BudgetMS))
	g.history = append(g.history, e)
	if len(g.history) > 60 {
		g.history = g.history[len(g.history)-60:]
	}
}
func fmtClearance(a clearance) string {
	if a.Kind == "land" {
		return a.Flight + " cleared via the approach corridor to runway " + a.Runway + "."
	}
	return a.Flight + " cleared to taxi, take off and depart via runway " + a.Runway + "."
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		fail(w, 400, "Invalid sector request")
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
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		g, e := s.get(w, r)
		if e != nil {
			fail(w, 503, e.Error())
			return
		}
		g.mu.Lock()
		v := s.snapshot(g)
		g.mu.Unlock()
		reply(w, 200, v)
	})
	mux.HandleFunc("POST /api/control", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID     string    `json:"id"`
			Action string    `json:"action"`
			Config *settings `json:"config,omitempty"`
			Seed   int64     `json:"seed,omitempty"`
			Runway string    `json:"runway,omitempty"`
		}
		if !decode(w, r, &in) {
			return
		}
		g, e := s.get(w, r)
		if e != nil {
			fail(w, 503, e.Error())
			return
		}
		g.mu.Lock()
		defer g.mu.Unlock()
		if in.ID != g.id {
			fail(w, 409, "Sector changed; refresh before controlling it")
			return
		}
		switch in.Action {
		case "play":
			if g.world.Stopped != "" {
				fail(w, 409, "Reset this protected sector before continuing")
				return
			}
			g.playing = true
		case "pause":
			g.playing = false
			g.generation++
			if g.cancel != nil {
				g.cancel()
			}
		case "settings":
			if in.Config == nil || !in.Config.valid() {
				fail(w, 400, "Aircraft 4–24, arrivals 0–6/min, speed 1–60× are supported")
				return
			}
			g.world.Config = *in.Config
			g.world.fill()
			g.generation++
			if g.cancel != nil {
				g.cancel()
			}
		case "reset":
			config := g.world.Config
			if in.Config != nil {
				config = *in.Config
			}
			if !config.valid() {
				fail(w, 400, "Invalid difficulty settings")
				return
			}
			if g.cancel != nil {
				g.cancel()
			}
			g.job++
			g.generation++
			g.id = id()
			g.pending = false
			g.playing = false
			g.world = newWorld(in.Seed, config)
			g.history = nil
		case "emergency":
			g.world.emergency()
			g.generation++
			if g.cancel != nil {
				g.cancel()
			}
		case "runway":
			found := false
			for i := range g.world.Runways {
				if g.world.Runways[i].ID == in.Runway {
					g.world.Runways[i].Closed = !g.world.Runways[i].Closed
					found = true
				}
			}
			if !found {
				fail(w, 400, "Unknown runway")
				return
			}
			g.generation++
			if g.cancel != nil {
				g.cancel()
			}
		default:
			fail(w, 400, "Unknown sector control")
			return
		}
		reply(w, 200, s.snapshot(g))
	})
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.Write([]byte("# TYPE atc_model_requests_total counter\natc_model_requests_total " + itoa(s.calls.Load()) + "\n# TYPE atc_model_errors_total counter\natc_model_errors_total " + itoa(s.failures.Load()) + "\n# TYPE atc_model_late_total counter\natc_model_late_total " + itoa(s.late.Load()) + "\n"))
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; frame-ancestors 'self'; base-uri 'none'; object-src 'none'")
		if r.Method == "POST" {
			if o := r.Header.Get("Origin"); o != "" {
				u, e := url.Parse(o)
				if e != nil || u.Host != r.Host || (u.Scheme != "http" && u.Scheme != "https") {
					fail(w, 403, "Cross-origin control refused")
					return
				}
			}
		}
		mux.ServeHTTP(w, r)
	})
}
func itoa(n int64) string { return fmt.Sprintf("%d", n) }
func main() {
	if len(os.Args) > 1 && os.Args[1] == "evaluate" {
		if err := evaluate(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}
	model, err := newDecisionClient(env("DECISION_URL", "http://d3-mini.decision-serving.svc.cluster.local:8080/v1/systemone"), env("DECISION_MODEL", "vllm-sr/d3-mini"), "")
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	s := &server{sessions: map[string]*session{}, model: model, slots: make(chan struct{}, 1)}
	go s.run(ctx)
	h := &http.Server{Addr: env("LISTEN_ADDR", ":8080"), Handler: s.routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	go func() {
		<-ctx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		h.Shutdown(ctx)
	}()
	log.Print("Sky Control listening on ", h.Addr)
	if err := h.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
