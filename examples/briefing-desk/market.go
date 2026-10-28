package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
)

type instrument struct {
	Symbol    string `json:"symbol"`
	Index     string `json:"index"`
	IssuerURL string `json:"issuer_url"`
}

var instruments = []instrument{
	{"QQQ", "Nasdaq-100", "https://www.invesco.com/qqq-etf/en/about.html"},
	{"SPY", "S&P 500", "https://www.ssga.com/us/en/individual/etfs/state-street-spdr-sp-500-etf-trust-spy"},
	{"SMH", "MVIS US Listed Semiconductor 25", "https://www.vaneck.com/us/en/investments/semiconductor-etf-smh/"},
	{"XLK", "Technology Select Sector", "https://www.ssga.com/us/en/individual/etfs/state-street-technology-select-sector-spdr-etf-xlk"},
}

type quote struct {
	Instrument instrument `json:"instrument"`
	Price      float64    `json:"price"`
	Currency   string     `json:"currency"`
	AsOf       string     `json:"as_of"`
	Retrieved  string     `json:"retrieved"`
	URL        string     `json:"url"`
	Error      string     `json:"error,omitempty"`
	Stale      bool       `json:"stale"`
	Basis      string     `json:"basis"`
}
type marketSnapshot struct {
	Quotes    []quote `json:"quotes"`
	Retrieved string  `json:"retrieved"`
	Coverage  string  `json:"coverage"`
}
type marketService struct {
	mu      sync.Mutex
	fetch   *fetcher
	cached  marketSnapshot
	expires time.Time
}

func (m *marketService) snapshot(ctx context.Context) marketSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	if time.Now().Before(m.expires) {
		return m.cached
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	out := marketSnapshot{Quotes: make([]quote, len(instruments)), Retrieved: stamp(), Coverage: "Yahoo Finance public chart metadata. Quote timestamp is explicit; delay is not guaranteed. Reference prices, not executable bid/ask quotes. ETFs are proxies for indices."}
	var wg sync.WaitGroup
	for i, v := range instruments {
		wg.Add(1)
		go func(i int, v instrument) { defer wg.Done(); out.Quotes[i] = m.fetch.quote(ctx, v) }(i, v)
	}
	wg.Wait()
	m.cached = out
	m.expires = time.Now().Add(time.Minute)
	return out
}
func (f *fetcher) quote(ctx context.Context, v instrument) quote {
	raw := "https://query1.finance.yahoo.com/v8/finance/chart/" + v.Symbol + "?interval=1d&range=5d"
	q := quote{Instrument: v, Retrieved: stamp(), URL: raw, Basis: "Provider last market price; delay not guaranteed"}
	b, _, _, e := f.get(ctx, raw)
	if e != nil {
		q.Error = e.Error()
		return q
	}
	var result struct {
		Chart struct {
			Result []struct {
				Meta struct {
					Symbol   string  `json:"symbol"`
					Price    float64 `json:"regularMarketPrice"`
					At       int64   `json:"regularMarketTime"`
					Currency string  `json:"currency"`
				} `json:"meta"`
			} `json:"result"`
			Error json.RawMessage `json:"error"`
		} `json:"chart"`
	}
	if json.Unmarshal(b, &result) != nil || len(result.Chart.Result) != 1 {
		q.Error = "Market quote unavailable"
		return q
	}
	meta := result.Chart.Result[0].Meta
	if meta.Symbol != v.Symbol || meta.Currency != "USD" || !finitePositive(meta.Price) || meta.At <= 0 || meta.At > time.Now().Add(5*time.Minute).Unix() {
		q.Error = "Market quote failed identity, currency, price or timestamp validation"
		return q
	}
	q.Price = meta.Price
	q.Currency = meta.Currency
	q.AsOf = time.Unix(meta.At, 0).UTC().Format(time.RFC3339)
	q.Stale = time.Since(time.Unix(meta.At, 0)) > 72*time.Hour
	return q
}
func finitePositive(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v > 0 }

type tradePlan struct {
	Symbol        string   `json:"symbol"`
	Direction     string   `json:"direction"`
	Thesis        string   `json:"thesis"`
	TargetMovePct float64  `json:"target_move_pct"`
	StopMovePct   float64  `json:"stop_move_pct"`
	HorizonDays   int      `json:"horizon_days"`
	Sources       []string `json:"sources"`
	Quote         *quote   `json:"quote,omitempty"`
	PricingError  string   `json:"pricing_error,omitempty"`
	Assumption    string   `json:"assumption"`
}
type priceScenario struct {
	Entry     float64 `json:"entry"`
	Target    float64 `json:"target"`
	Stop      float64 `json:"stop"`
	Notional  float64 `json:"notional"`
	Units     float64 `json:"units"`
	TargetPnL float64 `json:"target_pnl"`
	StopPnL   float64 `json:"stop_pnl"`
}

func usableQuote(q quote, now time.Time) bool {
	at, e := time.Parse(time.RFC3339, q.AsOf)
	return e == nil && q.Error == "" && !q.Stale && finitePositive(q.Price) && q.Currency == "USD" && !at.After(now.Add(5*time.Minute)) && now.Sub(at) <= 72*time.Hour
}
func scenario(p tradePlan, notional float64) (priceScenario, error) {
	if p.Quote == nil || !usableQuote(*p.Quote, time.Now()) {
		return priceScenario{}, errors.New("A fresh, sourced reference quote is required")
	}
	if p.Direction != "buy" && p.Direction != "short" || !finitePositive(notional) || notional > 1000000 || !finitePositive(p.TargetMovePct) || p.TargetMovePct > 25 || !finitePositive(p.StopMovePct) || p.StopMovePct > 15 {
		return priceScenario{}, errors.New("Invalid paper scenario inputs")
	}
	sign := 1.0
	if p.Direction == "short" {
		sign = -1
	}
	entry := p.Quote.Price
	return priceScenario{entry, entry * (1 + sign*p.TargetMovePct/100), entry * (1 - sign*p.StopMovePct/100), notional, notional / entry, notional * p.TargetMovePct / 100, -notional * p.StopMovePct / 100}, nil
}
func bindPlan(p *tradePlan, evidence []evidence) error {
	if p == nil {
		return errors.New("Investigation is missing its index thesis")
	}
	p.Assumption = "Target, stop and horizon are hypothetical; likelihood is not estimated. Gross P&L excludes fees, slippage, borrow and financing. Stop-case loss is not a loss cap. No orders."
	p.Quote = nil
	p.PricingError = ""
	if p.Direction == "no_signal" {
		p.PricingError = "Research did not confirm a directional index thesis"
		return nil
	}
	if p.Direction != "buy" && p.Direction != "short" {
		return errors.New("Unsupported plan direction")
	}
	known := false
	for _, v := range instruments {
		known = known || v.Symbol == p.Symbol
	}
	if !known || len(p.Thesis) < 10 || p.HorizonDays < 1 || p.HorizonDays > 90 || !finitePositive(p.TargetMovePct) || p.TargetMovePct > 25 || !finitePositive(p.StopMovePct) || p.StopMovePct > 15 {
		return errors.New("Invalid index scenario")
	}
	if len(p.Sources) == 0 {
		return errors.New("Index thesis is missing source references")
	}
	ids := map[string]bool{}
	for _, e := range evidence {
		ids[e.ID] = true
		if e.Tool == "market_snapshot" {
			var snap marketSnapshot
			if json.Unmarshal(e.Data, &snap) != nil {
				return errors.New("Invalid market evidence")
			}
			for _, q := range snap.Quotes {
				if q.Instrument.Symbol == p.Symbol {
					copy := q
					p.Quote = &copy
				}
			}
		}
	}
	for _, ref := range p.Sources {
		if !ids[ref] {
			return fmt.Errorf("Index thesis cited uncollected evidence")
		}
	}
	if p.Quote == nil {
		p.PricingError = "No market quote was retrieved for this ETF"
	} else if p.Quote.Error != "" {
		p.PricingError = p.Quote.Error
	} else if p.Quote.Stale {
		p.PricingError = "Reference quote is older than 72 hours; priced scenario unavailable"
	}
	return nil
}
