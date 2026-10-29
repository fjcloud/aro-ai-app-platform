package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Explicit real-model evaluation. Expected answers are never sent to inference.
func evaluate(args []string) error {
	f := flag.NewFlagSet("evaluate", flag.ContinueOnError)
	seeds := f.String("seeds", "42,7,2026", "comma-separated deterministic courier-deck seeds")
	output := f.String("output", "", "write sanitized per-case results here")
	images := f.String("images", "", "optionally write synthetic test documents here")
	if err := f.Parse(args); err != nil {
		return err
	}
	client, e := newDecisionClient(env("DECISION_URL", "http://127.0.0.1:18082/v1/systemone"), env("DECISION_MODEL", "vllm-sr/d3-mini"), "")
	if e != nil {
		return e
	}
	type trial struct {
		Seed        int64  `json:"seed"`
		Round       int    `json:"round"`
		Directive   string `json:"directive"`
		Expected    string `json:"expected"`
		Choice      string `json:"choice"`
		Correct     bool   `json:"correct"`
		LatencyMS   int64  `json:"latency_ms"`
		InputTokens int    `json:"input_tokens"`
	}
	results := []trial{}
	correct := 0
	for _, raw := range strings.Split(*seeds, ",") {
		seed, e := strconv.ParseInt(raw, 10, 64)
		if e != nil {
			return e
		}
		s := newShift(seed)
		for !s.over() {
			image := paper(s.Cases[s.Round])
			if *images != "" {
				if e := os.MkdirAll(*images, 0755); e != nil {
					return e
				}
				if e := os.WriteFile(filepath.Join(*images, fmt.Sprintf("seed-%d-round-%02d.png", seed, s.Round+1)), image, 0644); e != nil {
					return e
				}
			}
			d, e := client.choose(context.Background(), image, s.directive())
			if e != nil {
				return fmt.Errorf("seed %d round %d: %w", seed, s.Round+1, e)
			}
			expected, _ := verdict(s.Cases[s.Round], s.directive())
			ok := d.Choice == expected
			if ok {
				correct++
			}
			v := trial{seed, s.Round + 1, s.directive().Name, expected, d.Choice, ok, d.LatencyMS, d.InputTokens}
			results = append(results, v)
			b, _ := json.Marshal(v)
			fmt.Println(string(b))
			s.Round++
		}
	}
	report := map[string]any{"model": client.model, "task": "Read synthetic passport/permit images and apply ordered natural-language directives", "correct": correct, "total": len(results), "accuracy": float64(correct) / float64(len(results)), "cases": results, "limitations": []string{"Synthetic documents only; not a general OCR or reasoning benchmark.", "FP16 on the lab T4; published BF16 benchmark parity is unverified.", "No extracted fields or answer key were sent to the model."}}
	b, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(b))
	if *output != "" {
		return os.WriteFile(*output, append(b, '\n'), 0644)
	}
	return nil
}
