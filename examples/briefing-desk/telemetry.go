package main

import (
	"context"
	"errors"
	"log"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Refresh bearer auth on every export, rather than caching a projected SA token.
type rotatingExporter struct{ endpoint, experiment, workspace, tokenFile, caFile string }

func (e *rotatingExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	token, err := readCredential(e.tokenFile)
	if err != nil {
		return err
	}
	client, err := serviceClient(e.caFile)
	if err != nil {
		return err
	}
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(e.endpoint), otlptracehttp.WithHTTPClient(client), otlptracehttp.WithHeaders(map[string]string{"Authorization": "Bearer " + token, "x-mlflow-experiment-id": e.experiment, "X-MLFLOW-WORKSPACE": e.workspace}), otlptracehttp.WithTimeout(8*time.Second), otlptracehttp.WithRetry(otlptracehttp.RetryConfig{Enabled: false}))
	if err != nil {
		return err
	}
	defer exporter.Shutdown(ctx)
	return exporter.ExportSpans(ctx, spans)
}
func (*rotatingExporter) Shutdown(context.Context) error { return nil }
func initTracing(ctx context.Context) (func(context.Context) error, error) {
	var processor sdktrace.TracerProviderOption
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")
	if endpoint != "" {
		raw, err := checkedURL(endpoint)
		if err != nil || !strings.HasSuffix(raw, "/v1/traces") {
			return nil, errors.New("OTLP endpoint must end in /v1/traces")
		}
		exp := os.Getenv("MLFLOW_EXPERIMENT_ID")
		workspace := os.Getenv("MLFLOW_WORKSPACE")
		if exp == "" || workspace == "" {
			return nil, errors.New("MLflow experiment and workspace are required")
		}
		processor = sdktrace.WithBatcher(&rotatingExporter{raw, exp, workspace, env("SERVICE_ACCOUNT_TOKEN_FILE", "/var/run/secrets/kubernetes.io/serviceaccount/token"), os.Getenv("SERVICE_CA_FILE")}, sdktrace.WithMaxQueueSize(128), sdktrace.WithMaxExportBatchSize(32))
	} else {
		processor = sdktrace.WithSampler(sdktrace.AlwaysSample())
	}
	provider := sdktrace.NewTracerProvider(processor, sdktrace.WithResource(resource.NewWithAttributes("", attribute.String("service.name", "signal-desk"))))
	otel.SetTracerProvider(provider)
	// Do not log exporter errors that may include backend bodies or credentials.
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(error) { log.Print("Trace export failed; check MLflow availability and workspace permissions") }))
	return provider.Shutdown, nil
}

var mlflowTraceID = regexp.MustCompile(`^tr-[a-f0-9]{32}$`)

// Match the native MLflow trace-drawer route, including its workspace context.
func mlflowTraceURL(traceID string) string {
	if !mlflowTraceID.MatchString(traceID) {
		return ""
	}
	endpoint, err := url.Parse(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"))
	experiment, workspace := os.Getenv("MLFLOW_EXPERIMENT_ID"), os.Getenv("MLFLOW_WORKSPACE")
	if err != nil || endpoint.Host == "" || endpoint.User != nil || (endpoint.Scheme != "https" && endpoint.Scheme != "http") || !strings.HasSuffix(endpoint.Path, "/v1/traces") || experiment == "" || workspace == "" {
		return ""
	}
	endpoint.Path = strings.TrimSuffix(endpoint.Path, "/v1/traces") + "/"
	endpoint.RawQuery, endpoint.RawPath = "", ""
	query := url.Values{"workspace": {workspace}, "selectedEvaluationId": {traceID}}
	endpoint.Fragment = "/experiments/" + url.PathEscape(experiment) + "/traces?" + query.Encode()
	return endpoint.String()
}
