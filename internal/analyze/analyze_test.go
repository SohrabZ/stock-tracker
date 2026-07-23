package analyze

import (
	"testing"
	"time"

	"stocktracker/internal/stock"
)

func ptr(f float64) *float64 { return &f }

// makeBars builds hourly bars from parallel close/volume slices.
func makeBars(closes, vols []float64) []stock.Bar {
	base := time.Date(2026, 7, 20, 14, 0, 0, 0, time.UTC)
	var bars []stock.Bar
	for i := range closes {
		var v *float64
		if i < len(vols) {
			v = ptr(vols[i])
		}
		bars = append(bars, stock.Bar{
			Time:   base.Add(time.Duration(i) * time.Hour),
			Close:  ptr(closes[i]),
			Volume: v,
		})
	}
	return bars
}

func TestSlope(t *testing.T) {
	// A perfectly linear rising series [1,2,3,4] has slope 1.
	if got := slope([]float64{1, 2, 3, 4}); got != 1 {
		t.Fatalf("slope rising = %v, want 1", got)
	}
	// Falling series has slope -1.
	if got := slope([]float64{4, 3, 2, 1}); got != -1 {
		t.Fatalf("slope falling = %v, want -1", got)
	}
	// Flat series has slope 0.
	if got := slope([]float64{5, 5, 5}); got != 0 {
		t.Fatalf("slope flat = %v, want 0", got)
	}
	// Empty is defined as 0.
	if got := slope(nil); got != 0 {
		t.Fatalf("slope empty = %v, want 0", got)
	}
}

func TestMarketSession(t *testing.T) {
	// UTC hour -> expected ET session (ET = UTC-4).
	cases := []struct {
		utcHour, utcMin int
		want            string
	}{
		{8, 30, "Pre-market"},     // 04:30 ET
		{13, 0, "Pre-market"},     // 09:00 ET
		{14, 0, "Regular hours"},  // 10:00 ET
		{19, 59, "Regular hours"}, // 15:59 ET
		{20, 0, "After-hours"},    // 16:00 ET
		{23, 59, "After-hours"},   // 19:59 ET
		{2, 0, "Market closed"},   // 22:00 ET
		{7, 59, "Market closed"},  // 03:59 ET
	}
	for _, c := range cases {
		now := time.Date(2026, 7, 20, c.utcHour, c.utcMin, 0, 0, time.UTC)
		if got := MarketSession(now); got != c.want {
			t.Errorf("MarketSession(%02d:%02dZ) = %q, want %q", c.utcHour, c.utcMin, got, c.want)
		}
	}
}

func TestMarketSessionWinterDST(t *testing.T) {
	// January is EST (UTC-5). 14:00 UTC = 09:00 ET = pre-market.
	// (A fixed -4 offset would wrongly call this 10:00 ET / regular hours.)
	winter := time.Date(2026, 1, 15, 14, 0, 0, 0, time.UTC)
	if got := MarketSession(winter); got != "Pre-market" {
		t.Errorf("winter 14:00Z = %q, want Pre-market (EST-aware)", got)
	}
	// 15:00 UTC = 10:00 EST = regular hours.
	if got := MarketSession(time.Date(2026, 1, 15, 15, 0, 0, 0, time.UTC)); got != "Regular hours" {
		t.Errorf("winter 15:00Z = %q, want Regular hours", got)
	}
	// Summer sanity: July is EDT (UTC-4). 13:30 UTC = 09:30 ET = regular hours.
	if got := MarketSession(time.Date(2026, 7, 15, 13, 30, 0, 0, time.UTC)); got != "Regular hours" {
		t.Errorf("summer 13:30Z = %q, want Regular hours (EDT-aware)", got)
	}
}

func TestAnalyzeDowntrend(t *testing.T) {
	// 20 bars falling 100 -> 81.
	var closes, vols []float64
	for i := 0; i < 20; i++ {
		closes = append(closes, float64(100-i))
		vols = append(vols, 1000)
	}
	meta := stock.Meta{Symbol: "TEST", ShortName: "Test Co"}
	a, errStr := Analyze("TEST", meta, makeBars(closes, vols))
	if errStr != "" {
		t.Fatalf("unexpected error: %s", errStr)
	}
	if !a.IsDowntrend {
		t.Errorf("IsDowntrend = false, want true (falling series)")
	}
	if a.FullSlope >= 0 {
		t.Errorf("FullSlope = %v, want negative", a.FullSlope)
	}
	if a.LastPrice != 81 {
		t.Errorf("LastPrice = %v, want 81", a.LastPrice)
	}
	if a.High != 100 || a.Low != 81 {
		t.Errorf("High/Low = %v/%v, want 100/81", a.High, a.Low)
	}
	// change_pct = (81-100)/100 = -19%
	if a.ChangePct != -19 {
		t.Errorf("ChangePct = %v, want -19", a.ChangePct)
	}
	if a.Name != "Test Co" {
		t.Errorf("Name = %q, want %q", a.Name, "Test Co")
	}
}

func TestAnalyzeUptrend(t *testing.T) {
	var closes, vols []float64
	for i := 0; i < 20; i++ {
		closes = append(closes, float64(80+i))
		vols = append(vols, 1000)
	}
	a, errStr := Analyze("UP", stock.Meta{}, makeBars(closes, vols))
	if errStr != "" {
		t.Fatalf("unexpected error: %s", errStr)
	}
	if a.IsDowntrend {
		t.Errorf("IsDowntrend = true, want false (rising series)")
	}
	if a.FullSlope <= 0 {
		t.Errorf("FullSlope = %v, want positive", a.FullSlope)
	}
}

func TestAnalyzeDailyChange(t *testing.T) {
	closes := make([]float64, 12)
	for i := range closes {
		closes[i] = 100
	}
	closes[len(closes)-1] = 95 // last bar = 95
	meta := stock.Meta{PreviousClose: 100, ChartPreviousClose: 50}
	a, errStr := Analyze("D", meta, makeBars(closes, nil))
	if errStr != "" {
		t.Fatalf("unexpected error: %s", errStr)
	}
	if a.DailyChangePct == nil || *a.DailyChangePct != -5 {
		t.Errorf("DailyChangePct = %v, want -5 (uses previousClose, not chartPreviousClose)", a.DailyChangePct)
	}
	if a.DailyChangeAbs == nil || *a.DailyChangeAbs != -5 {
		t.Errorf("DailyChangeAbs = %v, want -5", a.DailyChangeAbs)
	}
}

func TestAnalyzeDailyChangeFallback(t *testing.T) {
	closes := make([]float64, 12)
	for i := range closes {
		closes[i] = 100
	}
	closes[len(closes)-1] = 90
	// No previousClose -> falls back to chartPreviousClose (matches original).
	meta := stock.Meta{ChartPreviousClose: 100}
	a, _ := Analyze("D", meta, makeBars(closes, nil))
	if a.DailyChangePct == nil || *a.DailyChangePct != -10 {
		t.Errorf("DailyChangePct = %v, want -10 via chartPreviousClose fallback", a.DailyChangePct)
	}
}

func TestAnalyzeInsufficientBars(t *testing.T) {
	closes := []float64{1, 2, 3, 4, 5} // only 5 < 10
	_, errStr := Analyze("X", stock.Meta{}, makeBars(closes, nil))
	if errStr == "" {
		t.Fatalf("expected error for <10 bars, got none")
	}
}

func TestAnalyzeSkipsNilCloses(t *testing.T) {
	// Interleave nil closes; 24 bars with closes on even indices -> 12 valid.
	base := time.Date(2026, 7, 20, 14, 0, 0, 0, time.UTC)
	var bars []stock.Bar
	valid := 0
	for i := 0; i < 24; i++ {
		b := stock.Bar{Time: base.Add(time.Duration(i) * time.Hour)}
		if i%2 == 0 {
			b.Close = ptr(float64(100 - valid))
			valid++
		}
		bars = append(bars, b)
	}
	a, errStr := Analyze("N", stock.Meta{}, bars)
	if errStr != "" {
		t.Fatalf("unexpected error: %s", errStr)
	}
	if a.BarCount != 12 {
		t.Errorf("BarCount = %d, want 12 (nil closes skipped)", a.BarCount)
	}
}
