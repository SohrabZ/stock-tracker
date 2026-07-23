// Package analyze computes trend, volume, and signal metrics from OHLCV bars.
// It is a faithful port of the original Python trend monitor's math.
package analyze

import (
	"fmt"
	"time"

	"stocktracker/internal/num"
	"stocktracker/internal/stock"
)

// easternTZ is US Eastern time, used for market-session math. Using a real
// location (not a fixed offset) means EST/EDT daylight-saving transitions are
// handled correctly year-round. A blank import of time/tzdata in main.go embeds
// the zone database so this works even without system tzdata.
var easternTZ = loadEastern()

func loadEastern() *time.Location {
	if loc, err := time.LoadLocation("America/New_York"); err == nil {
		return loc
	}
	// Last-resort fallback (should not happen with embedded tzdata): EST.
	return time.FixedZone("EST", -5*3600)
}

// Analysis is the full metric set for one symbol. JSON tags mirror the original
// state file so persisted state stays human-readable.
type Analysis struct {
	Ticker              string   `json:"ticker"`
	Name                string   `json:"name"`
	LastPrice           float64  `json:"last_price"`
	ChangePct           float64  `json:"change_pct"` // period (first→last) % change
	DailyChangePct      *float64 `json:"daily_change_pct"`
	DailyChangeAbs      *float64 `json:"daily_change_abs"`
	High                float64  `json:"high"`
	Low                 float64  `json:"low"`
	AvgPrice            float64  `json:"avg_price"`
	FullSlope           float64  `json:"full_slope"`
	RecentSlope         float64  `json:"recent_slope"`
	AvgVol              int64    `json:"avg_vol"`
	RecentVol           int64    `json:"recent_vol"`
	VolRatio            float64  `json:"vol_ratio"`
	IsDowntrend         bool     `json:"is_downtrend"`
	ReversalSignals     []string `json:"reversal_signals"`
	AccelerationSignals []string `json:"acceleration_signals"`
	BarCount            int      `json:"bar_count"`
	LastBarTime         string   `json:"last_bar_time"`
	DataStale           bool     `json:"data_stale"`
	Timestamp           string   `json:"timestamp"`
	PnlPct              *float64 `json:"_pnl_pct,omitempty"`
}

// HasSignal reports whether s is present in the acceleration/reversal list.
func hasSignal(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// MarketSession returns the US equity session label for the given time,
// evaluated in real US Eastern time (EST/EDT-aware).
func MarketSession(now time.Time) string {
	et := now.In(easternTZ)
	etTime := et.Hour()*60 + et.Minute()
	switch {
	case etTime >= 240 && etTime < 570:
		return "Pre-market"
	case etTime >= 570 && etTime < 960:
		return "Regular hours"
	case etTime >= 960 && etTime < 1200:
		return "After-hours"
	default:
		return "Market closed"
	}
}

func slope(vals []float64) float64 {
	n := len(vals)
	if n == 0 {
		return 0
	}
	xMean := float64(n-1) / 2.0
	var yMean float64
	for _, v := range vals {
		yMean += v
	}
	yMean /= float64(n)
	var num, den float64
	for i, v := range vals {
		dx := float64(i) - xMean
		num += dx * (v - yMean)
		den += dx * dx
	}
	if den == 0 {
		return 0
	}
	return num / den
}

func mean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	var s float64
	for _, v := range vals {
		s += v
	}
	return s / float64(len(vals))
}

func round(v float64, places int) float64 {
	return num.Round(v, places)
}

// Analyze computes the metric set for a symbol from its bars. It returns an
// error string (not a Go error) via the second return when there is too little
// data, matching how the original surfaced per-ticker problems.
func Analyze(ticker string, meta stock.Meta, bars []stock.Bar) (Analysis, string) {
	var closes, volumes []float64
	for _, b := range bars {
		if b.Close != nil {
			closes = append(closes, *b.Close)
		}
		if b.Volume != nil {
			volumes = append(volumes, *b.Volume)
		}
	}
	if len(closes) < 10 {
		return Analysis{}, fmt.Sprintf("only %d bars", len(closes))
	}

	fullSlope := slope(closes)
	recentN := 20
	if half := len(closes) / 2; half < recentN {
		recentN = half
	}
	recentSlope := slope(closes[len(closes)-recentN:])

	first := closes[0]
	last := closes[len(closes)-1]
	high, low := closes[0], closes[0]
	for _, c := range closes {
		if c > high {
			high = c
		}
		if c < low {
			low = c
		}
	}
	avg := mean(closes)

	// Daily change vs previous close.
	prev := meta.PreviousClose
	if prev == 0 {
		prev = meta.ChartPreviousClose
	}
	var dailyPct, dailyAbs *float64
	if prev > 0 {
		p := round((last-prev)/prev*100, 2)
		a := round(last-prev, 2)
		dailyPct, dailyAbs = &p, &a
	}

	// Volume trend.
	var avgVol, recentVol, volRatio float64
	if len(volumes) > 0 {
		avgVol = mean(volumes)
		rv := volumes
		if recentN < len(volumes) {
			rv = volumes[len(volumes)-recentN:]
		}
		recentVol = mean(rv)
		if avgVol != 0 {
			volRatio = recentVol / avgVol
		} else {
			volRatio = 1.0
		}
	}

	isDowntrend := fullSlope < -0.01 && last < avg

	var reversal []string
	if fullSlope < -0.01 && recentSlope > 0.02 {
		reversal = append(reversal, "slope reversal (recent turning up)")
	}
	if low > 0 && last > low*1.02 {
		reversal = append(reversal, fmt.Sprintf("bounced +%.1f%% off low", ((last/low)-1)*100))
	}
	if volRatio > 1.3 && recentSlope > 0 {
		reversal = append(reversal, fmt.Sprintf("volume spike (%.1fx avg) with rising price", volRatio))
	}
	if last > avg && first < avg {
		reversal = append(reversal, "recovered above period average")
	}

	var accel []string
	if fullSlope < -0.01 && recentSlope < fullSlope {
		accel = append(accel, "downtrend accelerating")
	}
	if volRatio > 1.5 && recentSlope < 0 {
		accel = append(accel, fmt.Sprintf("volume spike (%.1fx avg) with falling price", volRatio))
	}
	if last < low*1.005 {
		accel = append(accel, "near period low")
	}

	// Stale-data check for after-hours / pre-market.
	now := time.Now().UTC()
	session := MarketSession(now)
	dataStale := false
	if (session == "After-hours" || session == "Pre-market") && len(bars) > 0 {
		lastEt := bars[len(bars)-1].Time.In(easternTZ)
		lastEtTime := lastEt.Hour()*60 + lastEt.Minute()
		if session == "After-hours" && lastEtTime <= 960 {
			dataStale = true
		} else if session == "Pre-market" && lastEtTime < 240 {
			dataStale = true
		}
	}

	changePct := 0.0
	if first != 0 {
		changePct = round((last-first)/first*100, 2)
	}
	lastBarTime := ""
	if len(bars) > 0 {
		lastBarTime = bars[len(bars)-1].Time.UTC().Format(time.RFC3339)
	}
	if reversal == nil {
		reversal = []string{}
	}
	if accel == nil {
		accel = []string{}
	}

	return Analysis{
		Ticker:              ticker,
		Name:                meta.Name(ticker),
		LastPrice:           round(last, 2),
		ChangePct:           changePct,
		DailyChangePct:      dailyPct,
		DailyChangeAbs:      dailyAbs,
		High:                round(high, 2),
		Low:                 round(low, 2),
		AvgPrice:            round(avg, 2),
		FullSlope:           round(fullSlope, 4),
		RecentSlope:         round(recentSlope, 4),
		AvgVol:              int64(avgVol),
		RecentVol:           int64(recentVol),
		VolRatio:            round(volRatio, 2),
		IsDowntrend:         isDowntrend,
		ReversalSignals:     reversal,
		AccelerationSignals: accel,
		BarCount:            len(closes),
		LastBarTime:         lastBarTime,
		DataStale:           dataStale,
		Timestamp:           now.Format(time.RFC3339Nano),
	}, ""
}

// HasAccel reports whether the analysis carries a specific acceleration signal.
func (a Analysis) HasAccel(s string) bool { return hasSignal(a.AccelerationSignals, s) }
