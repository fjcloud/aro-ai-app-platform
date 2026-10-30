package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// Opt-in proposed-source validation in the existing app pod. It reads the public
// game snapshot and mounted model config without changing news, scores or rounds.
func TestLiveResearchWorkflow(t *testing.T) {
	if os.Getenv("LIVE_RESEARCH_VALIDATION") != "1" {
		t.Skip("Opt-in real model and public web calls")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var state struct {
		Game      gameState  `json:"game"`
		Documents []document `json:"documents"`
		Memo      profile    `json:"memo"`
	}
	resp, err := http.Get(env("LIVE_GAME_URL", "http://127.0.0.1:8080/api/game"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if err = json.NewDecoder(resp.Body).Decode(&state); err != nil || len(state.Documents) == 0 {
		t.Fatal("Saved game snapshot unavailable", err)
	}
	a, err := newAgent(os.Getenv("AGENT_URL"), os.Getenv("AGENT_MODEL"), env("AGENT_KEY_FILE", "/etc/model/api-key"))
	if err != nil {
		t.Fatal(err)
	}
	shutdown, err := initTracing(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer shutdown(context.Background())
	ctx, span := otel.Tracer("signal-desk").Start(ctx, "game.research.validation")
	span.SetAttributes(attribute.String("research.skill.name", "finance-idea-generation"), attribute.String("research.skill.upstream_revision", financeSkillRevision))
	d := state.Documents[0]
	setTraceJSON(span, "mlflow.spanInputs", map[string]any{"title": d.Summary.Title, "fund": state.Memo.Name, "information_cutoff": state.Game.Week.Cutoff, "scope": "Proposed-source live validation, not public app delivery"})
	out, err := a.gameIdeas(ctx, d, state.Memo, state.Documents, state.Game.Week)
	if err != nil {
		markTraceError(span, err)
	}
	setTraceJSON(span, "mlflow.spanOutputs", map[string]any{"comment": out.Comment, "ideas": out.Ideas})
	tid := "tr-" + span.SpanContext().TraceID().String()
	span.End()
	if err != nil {
		t.Fatal(err)
	}
	tools := []string{}
	seen := map[string]bool{}
	for _, e := range out.Evidence {
		tools = append(tools, e.Tool)
		seen[e.Tool] = true
	}
	for _, tool := range []string{"read_archive", "search_archive", "search_web", "search_companies"} {
		if !seen[tool] {
			t.Fatal("Missing live tool", tool)
		}
	}
	issuers := map[string]bool{}
	for _, idea := range out.Ideas {
		key := companyKey(idea.Company)
		if issuers[key] {
			t.Fatal("Repeated issuer")
		}
		issuers[key] = true
	}
	report := map[string]any{"passed": true, "scope": "Proposed-source validation in the existing app pod; not public deployment", "at": stamp(), "trace_id": tid, "trace_url": mlflowTraceURL(tid), "tools": tools, "ideas": out.Ideas, "comment": out.Comment}
	b, _ := json.MarshalIndent(report, "", "  ")
	if file := os.Getenv("LIVE_RESEARCH_REPORT"); file != "" {
		if err = os.WriteFile(file, append(b, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Contains(string(b), "api-key") {
		t.Fatal("Credential leaked into report")
	}
	t.Log(string(b))
}
