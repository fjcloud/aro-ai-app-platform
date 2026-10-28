package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

func id() string {
	b := make([]byte, 12)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func env(k, d string) string {
	if v := getenv(k); v != "" {
		return v
	}
	return d
}
func stamp() string { return time.Now().UTC().Format(time.RFC3339) }

type profile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Criteria string `json:"criteria"`
	Version  int    `json:"version"`
}

func defaults() []profile {
	return []profile{
		{"macro", "Global macro", "Assess potential transmission to interest rates, inflation, currencies, sovereign risk and global growth. Separate concrete economic developments from unrelated human-interest stories. Flag uncertainty; a geopolitical headline alone is not proof of a market effect.", 1},
		{"equities", "Technology equities", "Assess potential impact on listed technology companies, AI infrastructure, semiconductors, software revenues, costs, regulation and supply chains. Exclude unrelated sports, culture and general politics without a documented company or sector connection.", 1},
		{"commodities", "Energy & commodities", "Assess potential effects on oil, gas, electricity, metals, agricultural commodities, shipping and industrial supply. Identify actual supply/demand or policy developments; do not infer price moves solely from a dramatic headline.", 1},
	}
}

type source struct {
	URL       string `json:"url"`
	FinalURL  string `json:"final_url"`
	Host      string `json:"host"`
	Title     string `json:"title"`
	Text      string `json:"text,omitempty"`
	Published string `json:"published,omitempty"`
	Fetched   string `json:"fetched"`
	Truncated bool   `json:"truncated"`
}
type summary struct {
	Title    string   `json:"title"`
	TLDR     string   `json:"tldr"`
	Points   []string `json:"points"`
	Overview string   `json:"overview"`
}

func (s summary) hash() string {
	b, _ := json.Marshal(s)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

type event struct {
	Stage      string `json:"stage"`
	Message    string `json:"message"`
	At         string `json:"at"`
	DurationMS int64  `json:"duration_ms,omitempty"`
}
type classification struct {
	ProfileID      string             `json:"profile_id"`
	ProfileVersion int                `json:"profile_version"`
	SummaryHash    string             `json:"summary_hash"`
	Status         string             `json:"status"`
	Priority       string             `json:"priority,omitempty"`
	Confidence     float64            `json:"confidence"`
	Probabilities  map[string]float64 `json:"probabilities,omitempty"`
	LatencyMS      int64              `json:"latency_ms"`
	InputTokens    int                `json:"input_tokens"`
	TraceID        string             `json:"trace_id,omitempty"`
	Error          string             `json:"error,omitempty"`
	At             string             `json:"at"`
	Stale          bool               `json:"stale"`
}
type document struct {
	ID       string                    `json:"id"`
	URL      string                    `json:"url"`
	Status   string                    `json:"status"`
	Error    string                    `json:"error,omitempty"`
	Created  string                    `json:"created"`
	Source   source                    `json:"source"`
	Summary  summary                   `json:"summary"`
	Markdown string                    `json:"markdown,omitempty"`
	Events   []event                   `json:"events"`
	Classes  map[string]classification `json:"classifications"`
	Research map[string]research       `json:"research"`
	TraceID  string                    `json:"trace_id,omitempty"`
}
type evidence struct {
	ID        string          `json:"id"`
	Tool      string          `json:"tool"`
	URL       string          `json:"url"`
	Retrieved string          `json:"retrieved"`
	Data      json.RawMessage `json:"data"`
}
type finding struct {
	Text    string   `json:"text"`
	Sources []string `json:"sources"`
}
type report struct {
	Headline   string    `json:"headline"`
	Facts      []finding `json:"facts"`
	Hypotheses []finding `json:"hypotheses"`
	Unknowns   []string  `json:"unknowns"`
	NextChecks []string  `json:"next_checks"`
}
type research struct {
	Status         string     `json:"status"`
	Error          string     `json:"error,omitempty"`
	ProfileVersion int        `json:"profile_version"`
	SummaryHash    string     `json:"summary_hash"`
	TraceID        string     `json:"trace_id"`
	Events         []event    `json:"events"`
	Evidence       []evidence `json:"evidence"`
	Report         report     `json:"report"`
	At             string     `json:"at"`
}

func researchMarkdown(d document, p profile, r research) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\nResearch draft · %s · %s\n\nMandate: %s\n\n", markdownText(r.Report.Headline), stamp(), markdownText(p.Name), markdownText(p.Criteria))
	for _, section := range []struct {
		name  string
		items []finding
	}{{"Verified source facts", r.Report.Facts}, {"Possible financial transmission — hypotheses", r.Report.Hypotheses}} {
		fmt.Fprintf(&b, "## %s\n\n", section.name)
		for _, f := range section.items {
			fmt.Fprintf(&b, "- %s [%s]\n", markdownText(f.Text), strings.Join(f.Sources, ", "))
		}
		b.WriteString("\n")
	}
	for _, section := range []struct {
		name  string
		items []string
	}{{"Unknowns", r.Report.Unknowns}, {"Next checks", r.Report.NextChecks}} {
		fmt.Fprintf(&b, "## %s\n\n", section.name)
		for _, v := range section.items {
			fmt.Fprintf(&b, "- %s\n", markdownText(v))
		}
		b.WriteString("\n")
	}
	b.WriteString("## Evidence\n\n")
	for _, e := range r.Evidence {
		fmt.Fprintf(&b, "- %s · %s · [Source](%s) · retrieved %s\n", e.ID, e.Tool, linkURL(e.URL), e.Retrieved)
	}
	return b.String()
}
func markdownText(s string) string {
	return strings.NewReplacer("\\", "\\\\", "[", "\\[", "]", "\\]", "<", "&lt;", ">", "&gt;", "*", "\\*", "_", "\\_", "`", "\\`").Replace(s)
}
func linkURL(s string) string {
	return strings.NewReplacer("(", "%28", ")", "%29", "<", "%3C", ">", "%3E", " ", "%20").Replace(s)
}
func documentMarkdown(d document) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n> %s\n\n", markdownText(d.Summary.Title), markdownText(d.Summary.TLDR))
	b.WriteString("## Key points\n\n")
	for _, p := range d.Summary.Points {
		fmt.Fprintf(&b, "- %s\n", markdownText(p))
	}
	fmt.Fprintf(&b, "\n## Summary\n\n%s\n\n## Source\n\n[Original article](%s)\n\nRetrieved: %s\n", markdownText(d.Summary.Overview), linkURL(d.Source.FinalURL), d.Source.Fetched)
	if d.Source.Published != "" {
		fmt.Fprintf(&b, "\nPublication metadata: %s\n", markdownText(d.Source.Published))
	}
	if d.Source.Truncated {
		b.WriteString("\nNote: the readable source text was limited to the configured extraction budget.\n")
	}
	return b.String()
}

var priorities = []string{"material", "monitor", "insufficient_context", "not_financial"}

func rank(p string) int {
	for i, v := range priorities {
		if p == v {
			return i
		}
	}
	return 9
}
func briefing(p profile, docs []document) string {
	sort.Slice(docs, func(i, j int) bool {
		return rank(docs[i].Classes[p.ID].Priority) < rank(docs[j].Classes[p.ID].Priority)
	})
	var b strings.Builder
	fmt.Fprintf(&b, "# Briefing for %s\n\nCompiled: %s\n\nFocus: %s\n\n", markdownText(p.Name), stamp(), markdownText(p.Criteria))
	for _, d := range docs {
		c, ok := d.Classes[p.ID]
		if !ok || c.Status != "done" || c.ProfileVersion != p.Version || c.SummaryHash != d.Summary.hash() || (c.Priority != "material" && c.Priority != "monitor") {
			continue
		}
		fmt.Fprintf(&b, "## %s\n\n**%s**\n\n%s\n\n", markdownText(d.Summary.Title), strings.ToUpper(c.Priority), markdownText(d.Summary.TLDR))
		for _, point := range d.Summary.Points {
			fmt.Fprintf(&b, "- %s\n", markdownText(point))
		}
		fmt.Fprintf(&b, "\n[Source](%s)\n\n", linkURL(d.Source.FinalURL))
	}
	return b.String()
}
