package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

type company struct {
	Symbol   string `json:"symbol"`
	Name     string `json:"name"`
	Exchange string `json:"exchange"`
}

var equitySymbol = regexp.MustCompile(`^[A-Z0-9][A-Z0-9.-]{0,24}$`)
var fundName = regexp.MustCompile(`(?i)\b(ETFs?|ETNs?|funds?|index|UCITS)\b`)

func (f *fetcher) searchCompanies(ctx context.Context, query string) ([]company, string, error) {
	query = strings.TrimSpace(query)
	if len(query) < 2 || len(query) > 120 {
		return nil, "", errors.New("Use a short company name or stock ticker")
	}
	q := url.Values{"q": {query}, "quotesCount": {"10"}, "newsCount": {"0"}}
	raw := "https://query1.finance.yahoo.com/v1/finance/search?" + q.Encode()
	b, _, _, err := f.get(ctx, raw)
	if err != nil {
		return nil, raw, err
	}
	var result struct {
		Quotes []struct {
			Symbol       string `json:"symbol"`
			ShortName    string `json:"shortname"`
			LongName     string `json:"longname"`
			Exchange     string `json:"exchDisp"`
			ExchangeCode string `json:"exchange"`
			Type         string `json:"quoteType"`
		} `json:"quotes"`
	}
	if json.Unmarshal(b, &result) != nil {
		return nil, raw, errors.New("Company reference data is unavailable")
	}
	out := []company{}
	seen := map[string]bool{}
	issuers := map[string]bool{}
	for _, item := range result.Quotes {
		name := item.LongName
		if name == "" {
			name = item.ShortName
		}
		if item.Type != "EQUITY" || fundName.MatchString(name+" "+item.ShortName) || !equitySymbol.MatchString(item.Symbol) || seen[item.Symbol] || issuers[companyKey(name)] || name == "" {
			continue
		}
		exchange := item.Exchange
		if exchange == "" {
			exchange = item.ExchangeCode
		}
		out = append(out, company{Symbol: item.Symbol, Name: name, Exchange: exchange})
		seen[item.Symbol] = true
		issuers[companyKey(name)] = true
		if len(out) == 10 {
			break
		}
	}
	return out, raw, nil
}

func (j *agentJob) stockEvidenceReady() bool {
	types := map[string]bool{}
	for _, e := range j.evidence {
		types[e.Tool] = true
	}
	return types["read_archive"] && types["search_archive"] && j.webAttempts > 0 && (len(j.webRefs) == 0 || j.webReads > 0) && len(j.companyQueries) >= 2
}

// A second exchange listing of one issuer is not another business opportunity.
func companyKey(name string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, name)
}

// Search already returns the saved excerpts. Preserve budget for company lookup
// instead of repeatedly reading the same facts before collecting the identities.
func (j *agentJob) nextStockTool() string {
	types := map[string]bool{}
	for _, e := range j.evidence {
		types[e.Tool] = true
	}
	if !types["read_archive"] {
		return "read_archive"
	}
	if !types["search_archive"] {
		return "search_archive"
	}
	if j.webAttempts == 0 {
		return "search_web"
	}
	if len(j.webRefs) > 0 && j.webReads == 0 {
		return "read_web"
	}
	if len(j.companyQueries) < 2 {
		return "search_companies"
	}
	return ""
}
