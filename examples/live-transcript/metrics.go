package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// Only catalog expressions are executable. Model/user text is never PromQL.
// Add metrics here, with units and meaningful missing-data semantics.
type metric struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Unit        string `json:"unit"`
	Expression  string `json:"promql"`
}

var metricCatalog = []metric{
	{"nodes_ready", "Ready nodes (1 ready, 0 not ready), by node", "boolean", `max by (node) (kube_node_status_condition{condition="Ready",status="true"})`},
	{"operators_degraded", "Cluster operators (1 degraded, 0 not degraded)", "boolean", `max by (name) (cluster_operator_conditions{condition="Degraded"})`},
	{"operators_available", "Cluster operators (1 available, 0 unavailable)", "boolean", `max by (name) (cluster_operator_conditions{condition="Available"})`},
	{"cpu_percent", "CPU utilization by instance; five-minute rate", "percent", `100 * (1 - avg by (instance) (rate(node_cpu_seconds_total{mode="idle"}[5m])))`},
	{"memory_percent", "Memory utilization by instance", "percent", `100 * (1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes)`},
	{"pod_restarts", "Top 10 containers by restarts in the last hour", "restarts per hour", `topk(10, sum by (namespace,pod,container) (increase(kube_pod_container_status_restarts_total[1h])))`},
	{"pods_pending", "Pending pods by namespace (0 when none, if phase series exist)", "pods", `sum by (namespace) (kube_pod_status_phase{phase="Pending"})`},
	{"alerts_firing", "Firing alerts grouped by alert name and severity; includes persistent informational alerts", "alerts", `sum by (alertname,severity) (ALERTS{alertstate="firing"})`},
	{"gpu_memory_gib", "Whisper GPU framebuffer memory used; requires NVIDIA metrics", "GiB", `max by (Hostname,modelName) (DCGM_FI_DEV_FB_USED{exported_namespace="speech-serving"}) / 1024`},
	{"gpu_utilization", "Whisper GPU utilization; requires NVIDIA metrics", "percent", `max by (Hostname,modelName) (DCGM_FI_DEV_GPU_UTIL{exported_namespace="speech-serving"})`},
	{"whisper_up", "Whisper metric scrape target (1 up, 0 down)", "boolean", `max(up{namespace="speech-serving",service="whisper-predictor"})`},
	{"whisper_requests", "Completed Whisper requests in the last hour", "requests per hour", `sum(increase(vllm:request_success_total{namespace="speech-serving",finished_reason="stop"}[1h]))`},
	{"whisper_latency_p95", "Whisper p95 request latency over the last five minutes; absent/NaN when idle", "seconds", `histogram_quantile(0.95, sum by (le) (rate(vllm:e2e_request_latency_seconds_bucket{namespace="speech-serving"}[5m])))`},
}

type evidence struct {
	Metric    string          `json:"metric"`
	Unit      string          `json:"unit"`
	Query     string          `json:"query"`
	QueriedAt string          `json:"queried_at"`
	Status    string          `json:"status"` // ok, missing, error; never coerce missing to healthy/zero
	Data      json.RawMessage `json:"data,omitempty"`
	Error     string          `json:"error,omitempty"`
}
type metricsClient struct {
	endpoint  string
	tokenFile string
	client    *http.Client
}

func checkedURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("invalid service URL")
	}
	return strings.TrimRight(raw, "/"), nil
}
func serviceClient(caFile string) (*http.Client, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, errors.New("cannot read service CA")
		}
		roots, err := x509.SystemCertPool()
		if err != nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(pem) {
			return nil, errors.New("invalid service CA")
		}
		tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}
	}
	return &http.Client{Transport: tr, Timeout: 30 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}, nil
}
func readCredential(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil || len(strings.TrimSpace(string(b))) == 0 {
		return "", errors.New("service credential unavailable")
	}
	return strings.TrimSpace(string(b)), nil
}
func (c *metricsClient) query(ctx context.Context, id string, at time.Time, window string) evidence {
	e := evidence{Metric: id, QueriedAt: at.UTC().Format(time.RFC3339), Status: "error"}
	var m *metric
	for i := range metricCatalog {
		if metricCatalog[i].ID == id {
			m = &metricCatalog[i]
			break
		}
	}
	if m == nil {
		e.Error = "Unknown metric. Call list_metrics for supported metric IDs."
		return e
	}
	e.Query = m.Expression
	e.Unit = m.Unit
	if c == nil {
		e.Error = "Monitoring is not configured"
		return e
	}
	path := "/api/v1/query"
	expectedType := "vector"
	params := url.Values{"query": {m.Expression}, "time": {fmt.Sprint(at.Unix())}, "timeout": {"8s"}, "limit": {"40"}}
	if window != "" {
		durations := map[string]time.Duration{"15m": 15 * time.Minute, "1h": time.Hour, "6h": 6 * time.Hour}
		duration, ok := durations[window]
		if !ok {
			e.Error = "Window must be 15m, 1h or 6h"
			return e
		}
		path = "/api/v1/query_range"
		expectedType = "matrix"
		params.Del("time")
		params.Set("start", fmt.Sprint(at.Add(-duration).Unix()))
		params.Set("end", fmt.Sprint(at.Unix()))
		params.Set("step", fmt.Sprint(int(duration.Seconds()/30)))
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", c.endpoint+path+"?"+params.Encode(), nil)
	if err != nil {
		e.Error = "Invalid monitoring endpoint"
		return e
	}
	// Read on every request: Kubernetes projected service account tokens rotate.
	if c.tokenFile != "" {
		token, err := readCredential(c.tokenFile)
		if err != nil {
			e.Error = err.Error()
			return e
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := c.client.Do(req)
	if err != nil {
		e.Error = "Monitoring query failed or timed out"
		return e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		e.Error = fmt.Sprintf("Monitoring returned HTTP %d", res.StatusCode)
		return e
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 256*1024+1))
	if err != nil || len(body) > 256*1024 {
		e.Error = "Monitoring result exceeded the response budget"
		return e
	}
	var result struct {
		Status string `json:"status"`
		Data   struct {
			Type   string            `json:"resultType"`
			Result []json.RawMessage `json:"result"`
		} `json:"data"`
		Warnings []string `json:"warnings"`
	}
	if json.Unmarshal(body, &result) != nil || result.Status != "success" || result.Data.Type != expectedType || len(result.Warnings) > 0 {
		e.Error = "Monitoring returned invalid or partial data"
		return e
	}
	if len(result.Data.Result) > 40 {
		e.Error = "Monitoring returned too many series; narrow the catalog expression"
		return e
	}
	e.Status = "ok"
	if len(result.Data.Result) == 0 {
		e.Status = "missing"
	}
	e.Data, _ = json.Marshal(result.Data)
	return e
}

var safeMetricID = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
