package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type feed struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

var feeds = []feed{{"techmeme", "Techmeme", "https://www.techmeme.com/feed.xml"}}

type headline struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Published string `json:"published"`
	Excerpt   string `json:"excerpt"`
	FeedURL   string `json:"feed_url,omitempty"`
}
type fetcher struct{ client *http.Client }

var blockedPrefixes = []string{"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4", "::/96", "::ffff:0:0/96", "64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "2001::/23", "2001:db8::/32", "2002::/16", "fc00::/7", "fe80::/10", "ff00::/8"}

func publicIP(ip net.IP) bool {
	a, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	a = a.Unmap()
	if !a.IsGlobalUnicast() {
		return false
	}
	for _, p := range blockedPrefixes {
		if netip.MustParsePrefix(p).Contains(a) {
			return false
		}
	}
	return true
}
func publicURL(raw string) (*url.URL, error) {
	u, e := url.Parse(raw)
	if e != nil || len(raw) > 2048 || u.Hostname() == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Fragment != "" || (u.Port() != "" && u.Port() != "443" && u.Port() != "80") {
		return nil, errors.New("Use a public HTTP(S) article URL on port 80 or 443")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && !publicIP(ip) {
		return nil, errors.New("Private and reserved destinations are not allowed")
	}
	if !strings.Contains(u.Hostname(), ".") {
		return nil, errors.New("Use a public hostname")
	}
	return u, nil
}
func newFetcher() *fetcher {
	tr := &http.Transport{Proxy: nil, ResponseHeaderTimeout: 15 * time.Second, TLSHandshakeTimeout: 10 * time.Second, MaxIdleConns: 10, IdleConnTimeout: 30 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, e := net.SplitHostPort(address)
			if e != nil {
				return nil, e
			}
			ips, e := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if e != nil || len(ips) == 0 {
				return nil, errors.New("Public source DNS unavailable")
			}
			for _, ip := range ips {
				if !publicIP(ip) {
					return nil, errors.New("Private and reserved destinations are not allowed")
				}
			}
			d := net.Dialer{Timeout: 8 * time.Second}
			var last error
			for _, ip := range ips {
				c, e := d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if e == nil {
					return c, nil
				}
				last = e
			}
			return nil, last
		}}
	return &fetcher{&http.Client{Transport: tr, Timeout: 25 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 3 {
			return errors.New("Too many source redirects")
		}
		_, e := publicURL(r.URL.String())
		return e
	}}}
}
func (f *fetcher) get(ctx context.Context, raw string) ([]byte, string, string, error) {
	if _, e := publicURL(raw); e != nil {
		return nil, "", "", e
	}
	r, e := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if e != nil {
		return nil, "", "", e
	}
	r.Header.Set("User-Agent", "SignalDesk/1.0 (research workshop; contact: fjacquin@redhat.com)")
	r.Header.Set("Accept", "text/html,application/json,application/rss+xml,text/xml,text/csv;q=0.9,*/*;q=0.5")
	res, e := f.client.Do(r)
	if e != nil {
		return nil, "", "", errors.New("Public source unreachable or request timed out")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, "", "", fmt.Errorf("Public source returned HTTP %d", res.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(res.Body, 2*1024*1024+1))
	if e != nil || len(b) > 2*1024*1024 {
		return nil, "", "", errors.New("Source exceeds the 2 MiB retrieval limit")
	}
	return b, res.Request.URL.String(), res.Header.Get("Content-Type"), nil
}
func plainHTML(b []byte) (title, text string) {
	n, e := html.Parse(strings.NewReader(string(b)))
	if e != nil {
		return "", ""
	}
	var out strings.Builder
	var walk func(*html.Node, bool)
	walk = func(n *html.Node, inTitle bool) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "noscript", "nav", "footer", "header", "form", "svg":
				return
			}
			inTitle = inTitle || n.Data == "title"
		}
		if n.Type == html.TextNode {
			t := strings.Join(strings.Fields(n.Data), " ")
			if inTitle {
				title += t + " "
			} else if t != "" {
				out.WriteString(t + " ")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, inTitle)
		}
	}
	walk(n, false)
	return strings.TrimSpace(title), strings.TrimSpace(out.String())
}
func (f *fetcher) article(ctx context.Context, raw string) (source, error) {
	b, final, typ, e := f.get(ctx, raw)
	if e != nil {
		return source{}, e
	}
	u, _ := url.Parse(final)
	s := source{URL: raw, FinalURL: final, Host: u.Hostname(), Fetched: stamp()}
	switch {
	case strings.Contains(typ, "html"):
		s.Title, s.Text = plainHTML(b)
	case strings.HasPrefix(typ, "text/plain"):
		s.Text = string(b)
	default:
		return s, errors.New("Article must be readable HTML or plain text")
	}
	if len([]rune(s.Text)) < 160 {
		return s, errors.New("Article has too little readable text; it may require a subscription or JavaScript")
	}
	if r := []rune(s.Text); len(r) > 60000 {
		s.Text = string(r[:60000])
		s.Truncated = true
	}
	return s, nil
}
func (f *fetcher) headlines(ctx context.Context, feedID string) ([]headline, string, error) {
	raw := ""
	for _, v := range feeds {
		if v.ID == feedID {
			raw = v.URL
		}
	}
	if raw == "" {
		return nil, "", errors.New("Unknown news feed")
	}
	b, final, _, e := f.get(ctx, raw)
	if e != nil {
		return nil, raw, e
	}
	var rss struct {
		Channel struct {
			Items []struct {
				Title       string `xml:"title"`
				Link        string `xml:"link"`
				Date        string `xml:"pubDate"`
				Description string `xml:"description"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	if xml.Unmarshal(b, &rss) != nil {
		return nil, raw, errors.New("News feed could not be decoded")
	}
	out := []headline{}
	for _, v := range rss.Channel.Items {
		link := v.Link
		if feedID == "techmeme" {
			link = publisherURL(v.Description, v.Link)
		}
		if _, e := publicURL(link); e != nil {
			continue
		}
		_, excerpt := plainHTML([]byte(v.Description))
		out = append(out, headline{Title: v.Title, URL: link, Published: v.Date, Excerpt: excerpt, FeedURL: raw})
		if len(out) == 12 {
			break
		}
	}
	if len(out) == 0 {
		return nil, raw, errors.New("News feed returned no articles")
	}
	return out, final, nil
}

// Techmeme permalinks contain fragments; the RSS description links to the publisher.
func publisherURL(description, fallback string) string {
	clean := func(raw string) string {
		u, e := url.Parse(raw)
		if e != nil {
			return ""
		}
		u.Fragment = ""
		if _, e = publicURL(u.String()); e != nil {
			return ""
		}
		return u.String()
	}
	n, e := html.Parse(strings.NewReader(description))
	if e != nil {
		return clean(fallback)
	}
	result := ""
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if result != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key == "href" {
					u, e := url.Parse(a.Val)
					if e == nil && u.Hostname() != "www.techmeme.com" && u.Hostname() != "techmeme.com" && u.Path != "" && u.Path != "/" {
						result = clean(a.Val)
						if result != "" {
							return
						}
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	if result != "" {
		return result
	}
	return clean(fallback)
}

var companies = map[string]string{"NVDA": "0001045810", "AAPL": "0000320193", "MSFT": "0000789019", "AMZN": "0001018724", "GOOG": "0001652044", "TSLA": "0001318605", "XOM": "0000034088", "JPM": "0000019617"}

func (f *fetcher) filings(ctx context.Context, ticker string) (any, string, error) {
	cik := companies[strings.ToUpper(ticker)]
	if cik == "" {
		return nil, "", errors.New("Supported SEC tickers: NVDA, AAPL, MSFT, AMZN, GOOG, TSLA, XOM, JPM")
	}
	raw := "https://data.sec.gov/submissions/CIK" + cik + ".json"
	b, final, _, e := f.get(ctx, raw)
	if e != nil {
		return nil, raw, e
	}
	var x struct {
		Name    string   `json:"name"`
		Tickers []string `json:"tickers"`
		Filings struct {
			Recent struct {
				Form []string `json:"form"`
				Date []string `json:"filingDate"`
				Acc  []string `json:"accessionNumber"`
				Doc  []string `json:"primaryDocument"`
			} `json:"recent"`
		} `json:"filings"`
	}
	if json.Unmarshal(b, &x) != nil {
		return nil, raw, errors.New("SEC response could not be decoded")
	}
	recent := []map[string]string{}
	r := x.Filings.Recent
	for i := 0; i < len(r.Form) && i < 5 && i < len(r.Date) && i < len(r.Acc) && i < len(r.Doc); i++ {
		recent = append(recent, map[string]string{"form": r.Form[i], "date": r.Date[i], "url": "https://www.sec.gov/Archives/edgar/data/" + strings.TrimLeft(cik, "0") + "/" + strings.ReplaceAll(r.Acc[i], "-", "") + "/" + r.Doc[i]})
	}
	return map[string]any{"entity": x.Name, "tickers": x.Tickers, "filings": recent, "coverage": "Submission metadata only; filing content and market prices were not retrieved."}, final, nil
}
func (f *fetcher) fx(ctx context.Context, currency string) (any, string, error) {
	currency = strings.ToUpper(currency)
	if !strings.Contains("|USD|GBP|JPY|CHF|CNY|", "|"+currency+"|") || len(currency) != 3 {
		return nil, "", errors.New("Supported currencies: USD, GBP, JPY, CHF, CNY")
	}
	raw := "https://data-api.ecb.europa.eu/service/data/EXR/D." + currency + ".EUR.SP00.A?lastNObservations=5&format=csvdata"
	b, final, _, e := f.get(ctx, raw)
	if e != nil {
		return nil, raw, e
	}
	rows, e := csv.NewReader(strings.NewReader(string(b))).ReadAll()
	if e != nil || len(rows) < 2 {
		return nil, raw, errors.New("ECB data unavailable")
	}
	ti, vi := -1, -1
	for i, h := range rows[0] {
		if h == "TIME_PERIOD" {
			ti = i
		}
		if h == "OBS_VALUE" {
			vi = i
		}
	}
	if ti < 0 || vi < 0 {
		return nil, raw, errors.New("ECB schema not recognized")
	}
	points := []map[string]string{}
	for _, row := range rows[1:] {
		if len(row) > ti && len(row) > vi {
			points = append(points, map[string]string{"date": row[ti], "value": row[vi]})
		}
	}
	return map[string]any{"unit": currency + " per EUR", "kind": "ECB daily reference rate; not an executable or live quote", "observations": points}, final, nil
}
