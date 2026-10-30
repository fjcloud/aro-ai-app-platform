package main

import (
	"net/url"
	"strings"
	"testing"
)

func TestMLflowTraceLinkHasWorkspaceAndExactTrace(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "https://rh-ai.example/mlflow/v1/traces")
	t.Setenv("MLFLOW_EXPERIMENT_ID", "7")
	t.Setenv("MLFLOW_WORKSPACE", "signal-desk-dev")
	id := "tr-68572a2f27ca5496944535407529c188"
	u, err := url.Parse(mlflowTraceURL(id))
	if err != nil || u.Host != "rh-ai.example" || u.Path != "/mlflow/" {
		t.Fatal("Wrong MLflow location", u, err)
	}
	fragment, _ := url.Parse(u.Fragment)
	if fragment.Path != "/experiments/7/traces" || fragment.Query().Get("workspace") != "signal-desk-dev" || fragment.Query().Get("selectedEvaluationId") != id {
		t.Fatal("Link must open the specific trace in its workspace", u)
	}
	for _, bad := range []string{"", "tr-bad", id + "&workspace=other"} {
		if mlflowTraceURL(bad) != "" {
			t.Fatal("Invalid trace accepted")
		}
	}
	t.Setenv("MLFLOW_WORKSPACE", "")
	if mlflowTraceURL(id) != "" {
		t.Fatal("Incomplete tracing configuration must not invent a link")
	}
}
func TestFinanceSkillIsBundledAndIdentifiesItsLimits(t *testing.T) {
	for _, content := range []string{"Map the value chain", "Missing information stays unknown", "at most three", "No prices before portfolio commitment"} {
		if !strings.Contains(financeIdeaSkill, content) {
			t.Fatalf("Missing finance method: %s", content)
		}
	}
	if activeFinanceSkill().UpstreamRevision == "" || activeFinanceSkill().SourceURL == "" {
		t.Fatal("Skill provenance is required")
	}
}
