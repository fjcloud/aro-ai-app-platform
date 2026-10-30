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
	Symbol           string  `json:"symbol"`
	EntryDate        string  `json:"entry_date"`
	ExitDate         string  `json:"exit_date"`
	EntryPrice       float64 `json:"entry_price"`
	ExitPrice        float64 `json:"exit_price"`
	ReturnPct        float64 `json:"return_pct"`
	SourceURL        string  `json:"source_url"`
	Retrieved        string  `json:"retrieved"`
	WeekComplete     bool    `json:"week_complete"`
	Basis            string  `json:"basis"`
	QuoteTime        string  `json:"quote_time,omitempty"`
	PriceType        string  `json:"price_type,omitempty"`
	Currency         string  `json:"currency,omitempty"`
	ExchangeTimezone string  `json:"exchange_timezone,omitempty"`
	EntryTime        string  `json:"entry_time,omitempty"`
	FXEntryRate      float64 `json:"fx_entry_usd_per_unit,omitempty"`
	FXExitRate       float64 `json:"fx_exit_usd_per_unit,omitempty"`
	FXQuoteTime      string  `json:"fx_quote_time,omitempty"`
	FXSourceURL      string  `json:"fx_source_url,omitempty"`
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
	return historicalReturn{Symbol: symbol, EntryDate: first.At.In(loc).Format("2006-01-02"), ExitDate: last.At.In(loc).Format("2006-01-02"), EntryPrice: *first.Open, ExitPrice: *last.Close, ReturnPct: (*last.Close / *first.Open - 1) * 100, SourceURL: raw, Retrieved: stamp(), WeekComplete: !now.Before(end.Add(16*time.Hour + 5*time.Minute)), Basis: "First available session open after the news cutoff → last completed US session close. Gross price return; dividends, fees and slippage excluded."}, nil
}

func latestMarketResult(symbol string, bars []dailyBar, splits []int64, week replayWeek, now time.Time, price *float64, quoteAt int64, raw string) (historicalReturn, error) {
	return latestMarketResultOnExchange(symbol, bars, splits, week, now, price, quoteAt, raw, "America/New_York")
}

func latestMarketResultOnExchange(symbol string, bars []dailyBar, splits []int64, week replayWeek, now time.Time, price *float64, quoteAt int64, raw, timezone string) (historicalReturn, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil || timezone == "" {
		return historicalReturn{}, errors.New("The stock exchange timezone is unavailable")
	}
	cutoff, err := time.Parse(time.RFC3339, week.Cutoff)
	if err != nil {
		return historicalReturn{}, errors.New("The entry session must follow the news cutoff")
	}
	quoteTime := time.Unix(quoteAt, 0)
	if price == nil || !finitePositive(*price) || quoteTime.Before(cutoff) || quoteTime.After(now) {
		return historicalReturn{}, errors.New("No valid market quote is available after the archive cutoff yet")
	}
	var first *dailyBar
	var previous int64
	for i := range bars {
		b := &bars[i]
		if b.At.Unix() <= previous {
			return historicalReturn{}, errors.New("Historical sessions are duplicated or unordered")
		}
		previous = b.At.Unix()
		if first == nil && !b.At.Before(cutoff) && !b.At.After(quoteTime) {
			first = b
		}
	}
	if first == nil || first.Open == nil || !finitePositive(*first.Open) {
		return historicalReturn{}, errors.New("The entry session has no valid opening price")
	}
	if first.At.Before(cutoff) {
		return historicalReturn{}, errors.New("The entry session precedes the news cutoff")
	}
	for _, at := range splits {
		if at >= first.At.Unix() && at <= quoteAt {
			return historicalReturn{}, errors.New("A stock split occurs in the holding period; this simple price-only game cannot settle it")
		}
	}
	return historicalReturn{Symbol: symbol, EntryDate: first.At.In(loc).Format("2006-01-02"), EntryTime: first.At.UTC().Format(time.RFC3339), ExchangeTimezone: timezone, Currency: "USD", ExitDate: quoteTime.In(loc).Format("2006-01-02"), EntryPrice: *first.Open, ExitPrice: *price, ReturnPct: (*price / *first.Open - 1) * 100, SourceURL: raw, Retrieved: stamp(), QuoteTime: quoteTime.UTC().Format(time.RFC3339), PriceType: "latest available quote (may be delayed)", Basis: "First exchange session open after the fixed news cutoff → latest available regular-market quote. Intraday and delayed quotes are timestamped. Gross USD return; dividends, fees and slippage excluded."}, nil
}

type marketChart struct {
	Meta struct {
		Symbol   string   `json:"symbol"`
		Currency string   `json:"currency"`
		Type     string   `json:"instrumentType"`
		Name     string   `json:"longName"`
		Timezone string   `json:"exchangeTimezoneName"`
		Price    *float64 `json:"regularMarketPrice"`
		Time     int64    `json:"regularMarketTime"`
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
}

func (f *fetcher) chart(ctx context.Context, symbol string, start, now time.Time) (marketChart, string, error) {
	q := url.Values{"interval": {"1d"}, "period1": {strconv.FormatInt(start.Unix(), 10)}, "period2": {strconv.FormatInt(now.Unix(), 10)}, "events": {"div,splits"}}
	raw := "https://query1.finance.yahoo.com/v8/finance/chart/" + url.PathEscape(symbol) + "?" + q.Encode()
	b, _, _, e := f.get(ctx, raw)
	if e != nil {
		return marketChart{}, raw, e
	}
	var result struct {
		Chart struct {
			Result []marketChart `json:"result"`
		} `json:"chart"`
	}
	if json.Unmarshal(b, &result) != nil || len(result.Chart.Result) != 1 {
		return marketChart{}, raw, errors.New("Historical market data is unavailable")
	}
	v := result.Chart.Result[0]
	if v.Meta.Symbol != symbol || len(v.Indicators.Quote) != 1 {
		return marketChart{}, raw, errors.New("Historical quote identity is invalid")
	}
	data := v.Indicators.Quote[0]
	if len(data.Open) != len(v.Timestamp) || len(data.Close) != len(v.Timestamp) {
		return marketChart{}, raw, errors.New("Historical quote arrays are incomplete")
	}
	return v, raw, nil
}

func (v marketChart) bars() []dailyBar {
	bars := []dailyBar{}
	for i, t := range v.Timestamp {
		bars = append(bars, dailyBar{time.Unix(t, 0), v.Indicators.Quote[0].Open[i], v.Indicators.Quote[0].Close[i]})
	}
	return bars
}

func (f *fetcher) weekReturn(ctx context.Context, symbol string, week replayWeek, now time.Time) (historicalReturn, error) {
	if !equitySymbol.MatchString(symbol) {
		return historicalReturn{}, errors.New("Invalid company stock symbol")
	}
	cutoff, err := time.Parse(time.RFC3339, week.Cutoff)
	if err != nil {
		return historicalReturn{}, err
	}
	started := time.Now()
	v, raw, err := f.chart(ctx, symbol, cutoff.AddDate(0, 0, -8), now)
	if err != nil {
		return historicalReturn{}, err
	}
	if v.Meta.Type != "EQUITY" || fundName.MatchString(v.Meta.Name) {
		return historicalReturn{}, errors.New("Only individual company shares can be valued; ETFs, funds and indices are excluded")
	}
	if !regexpCurrency.MatchString(v.Meta.Currency) && v.Meta.Currency != "GBp" && v.Meta.Currency != "ZAc" {
		return historicalReturn{}, errors.New("Stock currency is unavailable")
	}
	bars := v.bars()
	splits := []int64{}
	for _, v := range v.Events.Splits {
		splits = append(splits, v.Date)
	}
	if week.ValuationMode == "latest" {
		r, err := latestMarketResultOnExchange(symbol, bars, splits, week, now.Add(time.Since(started)), v.Meta.Price, v.Meta.Time, raw, v.Meta.Timezone)
		if err != nil {
			return r, err
		}
		r.Currency = v.Meta.Currency
		if r.Currency == "GBp" {
			r.Currency = "GBP"
			r.EntryPrice /= 100
			r.ExitPrice /= 100
		}
		if r.Currency == "ZAc" {
			r.Currency = "ZAR"
			r.EntryPrice /= 100
			r.ExitPrice /= 100
		}
		r.FXEntryRate, r.FXExitRate = 1, 1
		if r.Currency != "USD" {
			return f.valueInUSD(ctx, r, now.Add(time.Since(started)))
		}
		return r, nil
	}
	if v.Meta.Currency != "USD" {
		return historicalReturn{}, errors.New("Foreign shares require the latest-quote replay mode")
	}
	return historicalResult(symbol, bars, splits, week, now, raw)
}
