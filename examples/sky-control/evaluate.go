package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"
)

func evaluate(args []string) error {
	f := flag.NewFlagSet("evaluate", flag.ContinueOnError)
	count := f.Int("aircraft", 6, "target airborne aircraft")
	speed := f.Float64("speed", 8, "simulation acceleration")
	cycles := f.Int("cycles", 12, "real model decisions")
	output := f.String("output", "", "write JSON evidence")
	if err := f.Parse(args); err != nil {
		return err
	}
	cfg := settings{*count, .4, *speed}
	if !cfg.valid() || *cycles < 1 || *cycles > 100 {
		return fmt.Errorf("invalid evaluation settings")
	}
	client, err := newDecisionClient(env("DECISION_URL", "http://127.0.0.1:18082/v1/systemone"), env("DECISION_MODEL", "vllm-sr/d3-mini"), "")
	if err != nil {
		return err
	}
	w := newWorld(42, cfg)
	records := []map[string]any{}
	for i := 0; i < *cycles && w.Stopped == ""; i++ {
		started := time.Now()
		snapshot := w.clone()
		options := snapshot.candidates()
		d, err := client.choose(context.Background(), snapshot, options)
		elapsed := time.Since(started)
		w.advance(elapsed.Seconds() * cfg.Speed)
		status, why := "accepted", ""
		chosen := clearance{}
		for _, a := range options {
			if a.ID == d.Choice {
				chosen = a
				break
			}
		}
		if err != nil {
			status = "error"
			why = err.Error()
		} else if elapsed.Seconds() > freshnessSeconds/cfg.Speed {
			status = "late"
			why = "decision freshness budget exceeded"
		} else if !chosen.Safe {
			status = "unsafe"
			why = chosen.Reason
		} else if e := w.apply(chosen); e != nil {
			status = "stale"
			why = e.Error()
		}
		if status == "accepted" {
			w.Stats.Accepted++
			if chosen.Kind != "wait" {
				w.advance(15)
			}
		} else if status == "late" {
			w.Stats.Late++
		} else if status == "unsafe" {
			w.Stats.Unsafe++
		} else if status == "stale" {
			w.Stats.Stale++
		} else {
			w.Stats.Errors++
		}
		v := map[string]any{"cycle": i + 1, "choice": d.Choice, "status": status, "reason": why, "total_ms": elapsed.Milliseconds(), "inference_ms": d.LatencyMS, "budget_ms": freshnessSeconds / cfg.Speed * 1000, "input_tokens": d.InputTokens, "sim_seconds": w.Seconds, "landed": w.Stats.Landed, "safety_stops": w.Stats.SafetyStops}
		records = append(records, v)
		b, _ := json.Marshal(v)
		fmt.Println(string(b))
	}
	report := map[string]any{"model": client.model, "config": cfg, "cycles": records, "stats": w.Stats, "stopped": w.Stopped, "note": "Live local model; simulation keeps moving during each inference. Additional 15 simulation seconds after accepted clearances accelerate this offline exercise; browser validation uses the actual wall-clock loop."}
	b, _ := json.MarshalIndent(report, "", "  ")
	if *output != "" {
		return os.WriteFile(*output, append(b, '\n'), 0644)
	}
	return nil
}
