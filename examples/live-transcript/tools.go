package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type toolDefinition struct {
	Type     string             `json:"type"`
	Function functionDefinition `json:"function"`
}
type functionDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

func toolDefinitions() []toolDefinition {
	ids := make([]string, 0, len(metricCatalog))
	for _, m := range metricCatalog {
		ids = append(ids, m.ID)
	}
	object := func(properties map[string]any, required []string) map[string]any {
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}
	return []toolDefinition{
		{"function", functionDefinition{"list_metrics", "List supported read-only cluster metrics, units and exact PromQL expressions.", object(map[string]any{}, []string{})}},
		{"function", functionDefinition{"cluster_health", "Read node readiness, operator availability/degradation, CPU, memory and firing alerts at a single timestamp. Empty data is unknown, never healthy.", object(map[string]any{}, []string{})}},
		{"function", functionDefinition{"query_metric", "Query a supported metric now, or its recent trend. Never executes arbitrary PromQL. Rates keep the lookback stated in the catalog.", object(map[string]any{"metric": map[string]any{"type": "string", "enum": ids}, "window": map[string]any{"type": "string", "enum": []string{"", "15m", "1h", "6h"}, "description": "Empty for an instant query; otherwise at most 31 trend samples per series"}}, []string{"metric"})}},
	}
}

type toolArgs struct {
	Metric string `json:"metric,omitempty"`
	Window string `json:"window,omitempty"`
}
type toolOutput struct {
	Metrics  []metric   `json:"metrics,omitempty"`
	Evidence []evidence `json:"evidence,omitempty"`
	Error    string     `json:"error,omitempty"`
}

func (s *server) runTool(ctx context.Context, name string, args json.RawMessage) toolOutput {
	traceName := name
	if name != "list_metrics" && name != "cluster_health" && name != "query_metric" {
		traceName = "unsupported"
	}
	ctx, span := otel.Tracer("cluster-voice").Start(ctx, "tool."+traceName)
	defer span.End()
	span.SetAttributes(attribute.String("mlflow.spanType", "TOOL"))
	var a toolArgs
	dec := json.NewDecoder(strings.NewReader(string(args)))
	dec.DisallowUnknownFields()
	if len(args) == 0 || len(args) > 2048 || dec.Decode(&a) != nil || dec.Decode(new(any)) != io.EOF || string(args) == "null" {
		span.SetStatus(codes.Error, "invalid arguments")
		return toolOutput{Error: "Invalid tool arguments"}
	}
	now := time.Now()
	switch name {
	case "list_metrics":
		if a.Metric != "" || a.Window != "" {
			break
		}
		return toolOutput{Metrics: metricCatalog}
	case "cluster_health":
		if a.Metric != "" || a.Window != "" {
			break
		}
		out := toolOutput{}
		for _, id := range []string{"nodes_ready", "operators_available", "operators_degraded", "cpu_percent", "memory_percent", "alerts_firing"} {
			out.Evidence = append(out.Evidence, s.metrics.query(ctx, id, now, ""))
			if ctx.Err() != nil {
				break
			}
		}
		for _, e := range out.Evidence {
			if e.Status == "error" {
				span.SetStatus(codes.Error, "monitoring query failed")
			}
		}
		return out
	case "query_metric":
		if !safeMetricID.MatchString(a.Metric) {
			break
		}
		e := s.metrics.query(ctx, a.Metric, now, a.Window)
		span.SetAttributes(attribute.String("metric.id", a.Metric), attribute.String("metric.status", e.Status))
		if e.Status == "error" {
			span.SetStatus(codes.Error, "monitoring query failed")
		}
		return toolOutput{Evidence: []evidence{e}}
	}
	span.SetStatus(codes.Error, "unsupported tool or arguments")
	return toolOutput{Error: "Unsupported tool or arguments. Use list_metrics; no mutation or shell tools are available."}
}
func (s *server) mcpHandler() http.Handler {
	srv := mcp.NewServer(&mcp.Implementation{Name: "openshift-health", Version: "1.0.0"}, nil)
	for _, def := range toolDefinitions() {
		name := def.Function.Name
		srv.AddTool(&mcp.Tool{Name: name, Description: def.Function.Description, InputSchema: def.Function.Parameters,
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: ptr(false), IdempotentHint: true, OpenWorldHint: ptr(false)}},
			func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				select {
				case s.toolSlots <- struct{}{}:
					defer func() { <-s.toolSlots }()
				default:
					return nil, errors.New("metrics tools busy")
				}
				out := s.runTool(ctx, name, req.Params.Arguments)
				b, _ := json.Marshal(out)
				failed := out.Error != ""
				for _, e := range out.Evidence {
					if e.Status == "error" {
						failed = true
					}
				}
				return &mcp.CallToolResult{IsError: failed, Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil
			})
	}
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 16384)
		handler.ServeHTTP(w, r)
	})
}
func ptr[T any](v T) *T { return &v }
