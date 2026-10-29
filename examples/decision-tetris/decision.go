package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type decisionClient struct {
	endpoint, model, keyFile string
	client                   *http.Client
}
type decision struct {
	Source        string             `json:"source"`
	Model         string             `json:"model"`
	Choice        string             `json:"choice"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
	LatencyMS     int64              `json:"latency_ms"`
	InputTokens   int                `json:"input_tokens"`
	Features      features           `json:"features"`
	TraceID       string             `json:"trace_id,omitempty"`
}

func newDecisionClient(endpoint, model, keyFile string) (*decisionClient, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("DECISION_URL must be an HTTP(S) inference endpoint")
	}
	// This experiment intentionally uses only a local model service or loopback.
	if u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && !strings.HasSuffix(u.Hostname(), ".svc.cluster.local") && !strings.HasSuffix(u.Hostname(), ".svc") {
		return nil, errors.New("DECISION_URL must point to the local model, not a hosted API")
	}
	return &decisionClient{endpoint, model, keyFile, &http.Client{Timeout: 40 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("inference redirects refused") }}}, nil
}
func (d *decisionClient) choose(ctx context.Context, g *game, moves []move) (decision, error) {
	ctx, span := otel.Tracer("decision-tetris").Start(ctx, "decision.choose")
	defer span.End()
	span.SetAttributes(attribute.String("mlflow.spanType", "LLM"), attribute.String("gen_ai.operation.name", "classification"), attribute.String("gen_ai.request.model", d.model), attribute.Int("decision.options", len(moves)))
	criteria := map[string]string{}
	for _, m := range moves {
		f := m.Features
		criteria[m.ID] = fmt.Sprintf("column=%d,rotation=%d,cleared_lines=%d,holes=%d,added_holes=%d,height_sum=%d,max_height=%d,bumpiness=%d", m.Column, m.Rotation*90, f.Lines, f.Holes, f.AddedHoles, f.HeightSum, f.MaxHeight, f.Bumpiness)
	}
	rows := make([]string, height)
	for y, row := range g.Board {
		var s strings.Builder
		for _, c := range row {
			if c == 0 {
				s.WriteByte('.')
			} else {
				s.WriteByte('#')
			}
		}
		rows[y] = s.String()
	}
	state, _ := json.Marshal(map[string]any{"game": "Tetris", "board_top_to_bottom": rows, "current_piece": g.Piece, "next_pieces": g.Next, "lines_cleared": g.Lines, "column_numbers": "0 to 9, left to right", "rules": "Choose a legal straight-drop placement. Outcome features are exact, calculated AFTER line clears. Holes are empty cells below a filled cell. Lower height_sum, max_height, holes and bumpiness are better. The goal is long survival and many cleared lines."})
	payload := map[string]any{"model": d.model, "state": string(state), "questions": map[string]any{"placement": map[string]any{"type": "choice", "require_full_input": true, "instructions": "Choose the best Tetris placement. Strongly avoid new holes and excessive stack height. Prefer clearing lines while keeping a low, flat board. Preserve access to wells for future pieces. Choose exactly one supplied option.", "criteria": criteria}}}
	body, _ := json.Marshal(payload)
	request, err := http.NewRequestWithContext(ctx, "POST", d.endpoint, bytes.NewReader(body))
	if err != nil {
		return decision{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	if d.keyFile != "" {
		key, err := os.ReadFile(d.keyFile)
		if err != nil {
			return decision{}, errors.New("local model credential unavailable")
		}
		request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(key)))
	}
	start := time.Now()
	res, err := d.client.Do(request)
	if err != nil {
		return decision{}, errors.New("local decision model unreachable or timed out")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		span.SetStatus(codes.Error, "local inference failed")
		return decision{}, fmt.Errorf("local decision model returned HTTP %d", res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 256*1024+1))
	if err != nil || len(raw) > 256*1024 {
		return decision{}, errors.New("invalid inference response")
	}
	var result struct {
		Model   string `json:"model"`
		Answers map[string]struct {
			Choice        string             `json:"choice"`
			Confidence    float64            `json:"confidence"`
			Probabilities map[string]float64 `json:"probabilities"`
			Error         string             `json:"error"`
			Coverage      string             `json:"input_coverage"`
		} `json:"answers"`
		Usage struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(raw, &result) != nil {
		return decision{}, errors.New("invalid inference JSON")
	}
	a, ok := result.Answers["placement"]
	if !ok || a.Error != "" {
		return decision{}, errors.New("model could not evaluate the full placement question")
	}
	if a.Coverage != "complete" {
		return decision{}, errors.New("model did not confirm complete input coverage")
	}
	if _, ok := criteria[a.Choice]; !ok {
		return decision{}, errors.New("model chose an illegal placement")
	}
	if math.IsNaN(a.Confidence) || math.IsInf(a.Confidence, 0) || a.Confidence < 0 || a.Confidence > 1 {
		return decision{}, errors.New("invalid model confidence")
	}
	total := 0.0
	for id, p := range a.Probabilities {
		if _, ok := criteria[id]; !ok || math.IsNaN(p) || math.IsInf(p, 0) || p < 0 || p > 1 {
			return decision{}, errors.New("invalid model probability")
		}
		total += p
	}
	if len(a.Probabilities) != len(moves) || math.Abs(total-1) > .02 {
		return decision{}, errors.New("incomplete model probabilities")
	}
	span.SetAttributes(attribute.Int("gen_ai.usage.input_tokens", result.Usage.InputTokens), attribute.Int("gen_ai.usage.output_tokens", 0), attribute.Float64("decision.confidence", a.Confidence))
	return decision{Source: "model", Model: d.model, Choice: a.Choice, Confidence: a.Confidence, Probabilities: a.Probabilities, LatencyMS: time.Since(start).Milliseconds(), InputTokens: result.Usage.InputTokens}, nil
}
