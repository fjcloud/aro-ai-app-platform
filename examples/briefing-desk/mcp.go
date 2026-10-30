package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// The Playground exposes the same archived evidence as the game agent, never prices.
func (s *server) archiveTool(ctx context.Context, name string, raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	args := struct {
		SourceID string `json:"source_id,omitempty"`
		Query    string `json:"query,omitempty"`
	}{}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&args); err != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, errors.New("Invalid archive tool arguments")
	}
	if name == "list_news" {
		if args.SourceID != "" || args.Query != "" {
			return nil, errors.New("This tool takes no arguments")
		}
	} else if name == "read_archive" || name == "read_web" {
		if args.SourceID == "" || args.Query != "" {
			return nil, errors.New("Use a source_id returned by list_news or search_archive")
		}
	} else if name == "search_archive" || name == "search_companies" || name == "search_web" {
		if args.SourceID != "" || args.Query == "" {
			return nil, errors.New("Use a short query")
		}
	} else {
		return nil, errors.New("Unknown archive tool")
	}
	s.gameMu.Lock()
	week := s.game.Week
	s.gameMu.Unlock()
	docs := s.gameDocuments()
	if name == "list_news" {
		items := []map[string]string{}
		for _, d := range docs {
			if d.Source.ArchivedAt != "" && d.Source.ArchivedAt <= week.Cutoff {
				items = append(items, map[string]string{"source_id": d.ID, "title": d.Summary.Title, "day": d.Source.Published})
			}
		}
		return map[string]any{"week": week, "news": items, "coverage": "Archived primary headlines only; no current prices or realized returns."}, nil
	}
	j := s.agent.job(ctx, "game_ideas", "", "", nil)
	j.archiveWeek = week
	j.archiveDocs = map[string]document{}
	for _, d := range docs {
		if d.GameWeek == week.ID && d.Source.ArchivedAt != "" && d.Source.ArchivedAt <= week.Cutoff {
			j.archiveDocs[d.ID] = d
		}
	}
	if name == "read_web" {
		ref, ok := s.cachedWebSource(args.SourceID, week.Cutoff)
		if !ok {
			return nil, errors.New("Unknown or expired web source_id; search_web again")
		}
		j.webRefs = map[string]webSource{args.SourceID: ref}
	}
	result, _, err := s.agent.runGameTool(ctx, j, name, args.SourceID, args.Query)
	if err == nil && name == "search_web" {
		s.cacheWebSources(result.(webSearchResult))
	}
	return result, err
}
func (s *server) mcpHandler() http.Handler {
	srv := mcp.NewServer(&mcp.Implementation{Name: "signal-desk", Version: "1.0.0"}, nil)
	destructive := false
	defs := []mcp.Tool{{Name: "list_news", Description: "List source IDs and headlines from the loaded historical news week.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}}}
	for _, spec := range toolSpecs() {
		if spec.name == "read_archive" || spec.name == "search_archive" || spec.name == "search_companies" || spec.name == "search_web" || spec.name == "read_web" {
			description := spec.description
			if spec.name == "read_archive" {
				description = "Read an archived source_id returned by list_news or search_archive. No current web access."
			}
			defs = append(defs, mcp.Tool{Name: spec.name, Description: description, InputSchema: spec.input})
		}
	}
	for _, def := range defs {
		openWorld := def.Name == "search_companies" || def.Name == "search_web" || def.Name == "read_web"
		def.Annotations = &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: &destructive, IdempotentHint: true, OpenWorldHint: &openWorld}
		name := def.Name
		srv.AddTool(&def, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, span := otel.Tracer("briefing-desk").Start(ctx, "mcp."+name)
			defer span.End()
			span.SetAttributes(attribute.String("mlflow.spanType", "TOOL"))
			setTraceJSON(span, "mlflow.spanInputs", req.Params.Arguments)
			result, err := s.archiveTool(ctx, name, req.Params.Arguments)
			if err != nil {
				markTraceError(span, err)
				setTraceJSON(span, "mlflow.spanOutputs", map[string]string{"error": err.Error()})
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
			}
			setTraceJSON(span, "mlflow.spanOutputs", result)
			b, _ := json.Marshal(modelToolResult(name, result))
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil
		})
	}
	host := env("MCP_PUBLIC_HOST", "")
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, DisableLocalhostProtection: host != ""})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if host != "" {
			origin := r.Header.Get("Origin")
			if (!strings.EqualFold(r.Host, host) && !strings.EqualFold(r.Host, host+":443")) || (origin != "" && origin != "https://"+host && origin != "https://"+host+":443") {
				http.Error(w, "Unrecognized MCP host or origin", http.StatusForbidden)
				return
			}
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16384)
		h.ServeHTTP(w, r)
	})
}
