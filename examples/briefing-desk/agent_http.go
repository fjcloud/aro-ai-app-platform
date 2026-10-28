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
	"strconv"
	"time"
)

func (a *agent) nativeResponse(ctx context.Context, j *agentJob, body []byte, key string) ([]byte, error) {
	if a.groqRequests != nil {
		select {
		case a.groqRequests <- struct{}{}:
			defer func() { <-a.groqRequests }()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	for attempt := 0; ; attempt++ {
		r, e := http.NewRequestWithContext(ctx, "POST", a.base+"/responses", bytes.NewReader(body))
		if e != nil {
			return nil, e
		}
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("User-Agent", "SignalDesk/1.0")
		if a.backend == "groq" {
			r.Header.Set("Authorization", "Bearer "+key)
		} else {
			provider, _ := json.Marshal(map[string]string{"vllm_api_token": key})
			r.Header.Set("X-OGX-Provider-Data", string(provider))
		}
		res, e := a.client.Do(r)
		if e != nil {
			return nil, fmt.Errorf("%s agent unreachable or request timed out", a.backendName())
		}
		raw, e := io.ReadAll(io.LimitReader(res.Body, 1024*1024+1))
		res.Body.Close()
		if e != nil || len(raw) > 1024*1024 {
			return nil, errors.New("Invalid agent response")
		}
		if res.StatusCode == 200 {
			return raw, nil
		}
		if a.backend != "groq" || res.StatusCode != 429 {
			return nil, fmt.Errorf("%s agent returned HTTP %d", a.backendName(), res.StatusCode)
		}
		// Only retry short, provider-requested cooldowns, on the same account
		// and model. Daily exhaustion or missing headers fail visibly.
		wait, ok := groqRetryAfter(res.Header.Get("Retry-After"))
		if !ok || attempt >= 3 {
			return nil, errors.New("Groq quota reached; wait before retrying. No model fallback or plan upgrade was used")
		}
		wait += time.Second // Retry-After may round down to whole seconds.
		j.note("rate_limit", fmt.Sprintf("Groq rate limit; retrying the same model in %.0f seconds", math.Ceil(wait.Seconds())), 0)
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func groqRetryAfter(raw string) (time.Duration, bool) {
	seconds, e := strconv.ParseFloat(raw, 64)
	if e != nil {
		at, e := http.ParseTime(raw)
		if e != nil {
			return 0, false
		}
		seconds = time.Until(at).Seconds()
	}
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 || seconds > 90 {
		return 0, false
	}
	return max(100*time.Millisecond, time.Duration(seconds*float64(time.Second))), true
}
