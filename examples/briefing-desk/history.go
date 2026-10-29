package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"time"
)

type historicalReturn struct {
	Symbol       string  `json:"symbol"`
	EntryDate    string  `json:"entry_date"`
	ExitDate     string  `json:"exit_date"`
	EntryPrice   float64 `json:"entry_price"`
	ExitPrice    float64 `json:"exit_price"`
	ReturnPct    float64 `json:"return_pct"`
	SourceURL    string  `json:"source_url"`
	Retrieved    string  `json:"retrieved"`
	WeekComplete bool    `json:"week_complete"`
	Basis        string  `json:"basis"`
}
type dailyBar struct {
	At          time.Time
	Open, Close *float64
}

func historicalResult(symbol string, bars []dailyBar, splitDates []int64, week replayWeek, now time.Time, raw string) (historicalReturn, error) {
	loc, _ := time.LoadLocation("America/New_York")
	start, e := time.ParseInLocation("2006-01-02", week.MarketStart, loc)
	if e != nil {
		return historicalReturn{}, e
	}
	end, _ := time.ParseInLocation("2006-01-02", week.MarketEnd, loc)
	var first, last *dailyBar
	var lastClose time.Time
	previous := int64(0)
	for i := range bars {
		b := &bars[i]
		if b.At.Unix() <= previous {
			return historicalReturn{}, errors.New("Historical sessions are duplicated or unordered")
		}
		previous = b.At.Unix()
		d := b.At.In(loc)
		date := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc)
		if date.Before(start) || date.After(end) {
			continue
		}
		if first == nil {
			first = b
		}
		closeAt := date.Add(16 * time.Hour)
		if closeAt.Add(5 * time.Minute).After(now) {
			continue
		}
		if b.Close == nil || !finitePositive(*b.Close) {
			return historicalReturn{}, errors.New("A completed session has no valid closing price")
		}
		last = b
		lastClose = closeAt
	}
	if first == nil || last == nil {
		return historicalReturn{}, errors.New("No completed trading session is available for this week yet")
	}
	if first.Open == nil || !finitePositive(*first.Open) {
		return historicalReturn{}, errors.New("The entry session has no valid opening price")
	}
	if first.At.After(last.At) {
		return historicalReturn{}, errors.New("No closing valuation exists after entry")
	}
	for _, at := range splitDates {
		if at >= first.At.Unix() && at <= lastClose.Unix() {
			return historicalReturn{}, errors.New("A stock split occurs in the holding period; this simple price-only game cannot settle it")
		}
	}
	return historicalReturn{symbol, first.At.In(loc).Format("2006-01-02"), last.At.In(loc).Format("2006-01-02"), *first.Open, *last.Close, (*last.Close / *first.Open - 1) * 100, raw, stamp(), !now.Before(end.Add(16*time.Hour + 5*time.Minute)), "First available session open after the news cutoff → last completed US session close. Gross price return; dividends, fees and slippage excluded."}, nil
}
func (f *fetcher) weekReturn(ctx context.Context, symbol string, week replayWeek, now time.Time) (historicalReturn, error) {
	known := false
	for _, v := range instruments {
		known = known || v.Symbol == symbol
	}
	if !known {
		return historicalReturn{}, errors.New("Unsupported index proxy")
	}
	loc, _ := time.LoadLocation("America/New_York")
	start, e := time.ParseInLocation("2006-01-02", week.MarketStart, loc)
	if e != nil {
		return historicalReturn{}, e
	}
	q := url.Values{"interval": {"1d"}, "period1": {strconv.FormatInt(start.Unix(), 10)}, "period2": {strconv.FormatInt(now.Unix(), 10)}, "events": {"div,splits"}}
	raw := "https://query1.finance.yahoo.com/v8/finance/chart/" + symbol + "?" + q.Encode()
	b, _, _, e := f.get(ctx, raw)
	if e != nil {
		return historicalReturn{}, e
	}
	var result struct {
		Chart struct {
			Result []struct {
				Meta struct {
					Symbol   string `json:"symbol"`
					Currency string `json:"currency"`
				} `json:"meta"`
				Timestamp  []int64 `json:"timestamp"`
				Indicators struct {
					Quote []struct {
						Open  []*float64 `json:"open"`
						Close []*float64 `json:"close"`
					} `json:"quote"`
				} `json:"indicators"`
				Events struct {
					Splits map[string]struct {
						Date int64 `json:"date"`
					} `json:"splits"`
				} `json:"events"`
			} `json:"result"`
		} `json:"chart"`
	}
	if json.Unmarshal(b, &result) != nil || len(result.Chart.Result) != 1 {
		return historicalReturn{}, errors.New("Historical market data is unavailable")
	}
	v := result.Chart.Result[0]
	if v.Meta.Symbol != symbol || v.Meta.Currency != "USD" || len(v.Indicators.Quote) != 1 {
		return historicalReturn{}, errors.New("Historical quote identity or currency is invalid")
	}
	data := v.Indicators.Quote[0]
	if len(data.Open) != len(v.Timestamp) || len(data.Close) != len(v.Timestamp) {
		return historicalReturn{}, errors.New("Historical quote arrays are incomplete")
	}
	bars := []dailyBar{}
	for i, t := range v.Timestamp {
		bars = append(bars, dailyBar{time.Unix(t, 0), data.Open[i], data.Close[i]})
	}
	splits := []int64{}
	for _, v := range v.Events.Splits {
		splits = append(splits, v.Date)
	}
	return historicalResult(symbol, bars, splits, week, now, raw)
}
