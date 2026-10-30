package main

import (
	"context"
	"errors"
	"math"
	"regexp"
	"time"
)

var regexpCurrency = regexp.MustCompile(`^[A-Z]{3}$`)

func (f *fetcher) valueInUSD(ctx context.Context, r historicalReturn, now time.Time) (historicalReturn, error) {
	entry, err := time.Parse(time.RFC3339, r.EntryTime)
	if err != nil {
		return r, errors.New("Stock entry time is unavailable for currency conversion")
	}
	started := time.Now()
	chart, raw, err := f.chart(ctx, r.Currency+"=X", entry.AddDate(0, 0, -8), now)
	if err != nil {
		return r, errors.New("FX data is unavailable; the foreign-currency position cannot be valued")
	}
	observed := now.Add(time.Since(started))
	quoteAt := time.Unix(chart.Meta.Time, 0)
	if chart.Meta.Type != "CURRENCY" || chart.Meta.Currency != r.Currency || chart.Meta.Price == nil || !finitePositive(*chart.Meta.Price) || quoteAt.Before(entry) || quoteAt.After(observed) {
		return r, errors.New("FX identity, price or timestamp is invalid")
	}
	var rate *float64
	var rateAt time.Time
	var previous int64
	for _, b := range chart.bars() {
		if b.At.Unix() <= previous {
			return r, errors.New("FX sessions are duplicated or unordered")
		}
		previous = b.At.Unix()
		if !b.At.After(entry) && b.Open != nil && finitePositive(*b.Open) {
			rate = b.Open
			rateAt = b.At
		}
	}
	if rate == nil || entry.Sub(rateAt) > 7*24*time.Hour {
		return r, errors.New("No known FX opening observation exists before stock entry")
	}
	// Yahoo CCY=X quotes currency units per USD. Convert both observations to
	// USD per local unit before computing the USD portfolio's gross return.
	r.FXEntryRate, r.FXExitRate = 1 / *rate, 1 / *chart.Meta.Price
	r.FXQuoteTime, r.FXSourceURL = quoteAt.UTC().Format(time.RFC3339), raw
	r.ReturnPct = (r.ExitPrice*r.FXExitRate/(r.EntryPrice*r.FXEntryRate) - 1) * 100
	if !finitePositive(r.FXEntryRate) || !finitePositive(r.FXExitRate) || math.IsNaN(r.ReturnPct) || math.IsInf(r.ReturnPct, 0) {
		return r, errors.New("FX conversion produced an invalid return")
	}
	r.Basis += " FX entry uses the last daily opening reference known by stock entry; FX valuation uses the timestamped latest rate."
	return r, nil
}
