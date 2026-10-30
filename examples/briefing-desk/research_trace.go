package main

import (
	"context"
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// The research audit contains prompts, public tool arguments/excerpts and visible
// model answers. HTTP headers, provider credentials and reasoning blocks are never
// included. Each public tool result is recorded separately from the LLM history.
func setTraceJSON(span trace.Span, key string, value any) {
	if span == nil {
		return
	}
	b, err := json.Marshal(value)
	if err != nil {
		return
	}
	if len(b) > 65536 {
		b, _ = json.Marshal(map[string]any{"truncated": true, "note": "Large model history; complete public tool results are on their individual spans", "excerpt": clipText(string(b), 60000)})
	}
	span.SetAttributes(attribute.String(key, string(b)))
}
func markTraceError(span trace.Span, err error) {
	if span == nil || err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
func startResearchCall(ctx context.Context, j *agentJob, model, provider string, turn int, payload map[string]any) (context.Context, trace.Span) {
	if j.stage != "game_ideas" {
		return ctx, nil
	}
	ctx, span := otel.Tracer("signal-desk").Start(ctx, fmt.Sprintf("llm.research.%d", turn))
	span.SetAttributes(attribute.String("mlflow.spanType", "LLM"), attribute.String("gen_ai.request.model", model), attribute.String("gen_ai.provider.name", provider))
	setTraceJSON(span, "mlflow.spanInputs", payload)
	return ctx, span
}
func finishResearchCall(span trace.Span, j *agentJob, raw []byte, err error) {
	if span == nil {
		return
	}
	defer span.End()
	if err != nil {
		markTraceError(span, err)
		setTraceJSON(span, "mlflow.spanOutputs", map[string]string{"error": err.Error()})
		return
	}
	var response struct {
		Status string            `json:"status"`
		Model  string            `json:"model"`
		Output []json.RawMessage `json:"output"`
		Usage  *struct {
			Input  int `json:"input_tokens"`
			Output int `json:"output_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(raw, &response) != nil {
		markTraceError(span, fmt.Errorf("Invalid model response"))
		return
	}
	visible := []any{}
	for _, item := range response.Output {
		var v struct {
			Type      string `json:"type"`
			Name      string `json:"name"`
			CallID    string `json:"call_id"`
			Arguments string `json:"arguments"`
			Content   []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if json.Unmarshal(item, &v) != nil {
			continue
		}
		if v.Type == "function_call" {
			visible = append(visible, map[string]string{"type": v.Type, "name": v.Name, "call_id": v.CallID, "arguments": v.Arguments})
		}
		if v.Type == "message" {
			for _, c := range v.Content {
				if c.Type == "output_text" {
					visible = append(visible, map[string]string{"type": "output_text", "text": c.Text})
				}
			}
		}
	}
	setTraceJSON(span, "mlflow.spanOutputs", map[string]any{"status": response.Status, "model": response.Model, "output": visible})
	if response.Usage != nil {
		span.SetAttributes(attribute.Int("gen_ai.usage.input_tokens", response.Usage.Input), attribute.Int("gen_ai.usage.output_tokens", response.Usage.Output))
	}
	if response.Status != "completed" {
		markTraceError(span, fmt.Errorf("Model did not complete its response"))
	}
}
