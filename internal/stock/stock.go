// Package stock fetches quotes and historical bars from Yahoo Finance's public
// chart API. No API key is required.
package stock

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const apiBase = "https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=%s&range=%s"

const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 stock-tracker"

var httpClient = &http.Client{Timeout: 30 * time.Second}

// Meta holds chart metadata for a symbol.
type Meta struct {
	Symbol             string
	ShortName          string
	LongName           string
	Currency           string
	RegularMarketPrice float64
	PreviousClose      float64
	ChartPreviousClose float64
}

// Name returns the best available human-readable name, falling back to symbol.
func (m Meta) Name(symbol string) string {
	if m.ShortName != "" {
		return m.ShortName
	}
	if m.LongName != "" {
		return m.LongName
	}
	return symbol
}

// PrevClose returns the previous session close, falling back to the chart's
// starting close when Yahoo omits previousClose.
func (m Meta) PrevClose() float64 {
	if m.PreviousClose != 0 {
		return m.PreviousClose
	}
	return m.ChartPreviousClose
}

// Bar is a single OHLCV bar. OHLCV pointers are nil when Yahoo reports no data
// for that field in the bar (common in pre/post-market).
type Bar struct {
	Time   time.Time
	Open   *float64
	High   *float64
	Low    *float64
	Close  *float64
	Volume *float64
}

// Quote is a lightweight current-price snapshot.
type Quote struct {
	Symbol        string
	Name          string
	Currency      string
	CurrentPrice  float64
	PreviousClose float64
}

// ChangePct returns the fractional change from the previous close.
func (q Quote) ChangePct() float64 {
	if q.PreviousClose == 0 {
		return 0
	}
	return q.CurrentPrice/q.PreviousClose - 1
}

// rawMeta is the JSON shape of the chart meta; toMeta converts it to the
// exported Meta so both GetBars and GetQuote build it the same way.
type rawMeta struct {
	Symbol             string  `json:"symbol"`
	ShortName          string  `json:"shortName"`
	LongName           string  `json:"longName"`
	Currency           string  `json:"currency"`
	RegularMarketPrice float64 `json:"regularMarketPrice"`
	PreviousClose      float64 `json:"previousClose"`
	ChartPreviousClose float64 `json:"chartPreviousClose"`
}

func (rm rawMeta) toMeta() Meta {
	return Meta{
		Symbol:             rm.Symbol,
		ShortName:          rm.ShortName,
		LongName:           rm.LongName,
		Currency:           rm.Currency,
		RegularMarketPrice: rm.RegularMarketPrice,
		PreviousClose:      rm.PreviousClose,
		ChartPreviousClose: rm.ChartPreviousClose,
	}
}

type chartResponse struct {
	Chart struct {
		Result []struct {
			Meta       rawMeta `json:"meta"`
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Open   []*float64 `json:"open"`
					High   []*float64 `json:"high"`
					Low    []*float64 `json:"low"`
					Close  []*float64 `json:"close"`
					Volume []*float64 `json:"volume"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

func fetch(symbol, interval, rng string, prepost bool) (*chartResponse, error) {
	url := fmt.Sprintf(apiBase, symbol, interval, rng)
	if prepost {
		url += "&includePrePost=true"
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("rate limited by Yahoo Finance for %s", symbol)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yahoo returned %d for %s", resp.StatusCode, symbol)
	}

	var cr chartResponse
	if err := json.Unmarshal(body, &cr); err != nil {
		return nil, fmt.Errorf("parsing yahoo response for %s: %w", symbol, err)
	}
	if cr.Chart.Error != nil {
		return nil, fmt.Errorf("yahoo error for %s: %s", symbol, cr.Chart.Error.Description)
	}
	if len(cr.Chart.Result) == 0 {
		return nil, fmt.Errorf("no data returned for %s (symbol not found?)", symbol)
	}
	return &cr, nil
}

// GetBars fetches OHLCV bars for a symbol over the given range/interval.
func GetBars(symbol, rng, interval string, prepost bool) (Meta, []Bar, error) {
	cr, err := fetch(symbol, interval, rng, prepost)
	if err != nil {
		return Meta{}, nil, err
	}
	r := cr.Chart.Result[0]
	meta := r.Meta.toMeta()

	var q struct {
		Open, High, Low, Close, Volume []*float64
	}
	if len(r.Indicators.Quote) > 0 {
		q.Open = r.Indicators.Quote[0].Open
		q.High = r.Indicators.Quote[0].High
		q.Low = r.Indicators.Quote[0].Low
		q.Close = r.Indicators.Quote[0].Close
		q.Volume = r.Indicators.Quote[0].Volume
	}

	at := func(s []*float64, i int) *float64 {
		if i < len(s) {
			return s[i]
		}
		return nil
	}

	var bars []Bar
	for i, ts := range r.Timestamp {
		bars = append(bars, Bar{
			Time:   time.Unix(ts, 0).UTC(),
			Open:   at(q.Open, i),
			High:   at(q.High, i),
			Low:    at(q.Low, i),
			Close:  at(q.Close, i),
			Volume: at(q.Volume, i),
		})
	}
	return meta, bars, nil
}

// GetQuote fetches a current-price snapshot (used by the price/research commands).
func GetQuote(symbol string) (Quote, error) {
	cr, err := fetch(symbol, "1m", "1d", false)
	if err != nil {
		return Quote{}, err
	}
	meta := cr.Chart.Result[0].Meta.toMeta()
	prev := meta.PrevClose()
	if meta.RegularMarketPrice == 0 || prev == 0 {
		return Quote{}, fmt.Errorf("incomplete price data for %s", symbol)
	}
	return Quote{
		Symbol:        symbol,
		Name:          meta.Name(symbol),
		Currency:      meta.Currency,
		CurrentPrice:  meta.RegularMarketPrice,
		PreviousClose: prev,
	}, nil
}
