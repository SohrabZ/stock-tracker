// Package monitor runs a trend-monitoring pass: for each tracked symbol it
// fetches 5-day hourly bars, computes trend/volume/signal metrics and position
// P&L, detects changes versus the previous run, and prints a compact,
// Telegram-ready report to stdout (no LLM involved).
package monitor

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"stocktracker/internal/ai"
	"stocktracker/internal/analyze"
	"stocktracker/internal/cache"
	"stocktracker/internal/positions"
	"stocktracker/internal/stock"
	"stocktracker/internal/store"
)

// Run executes one monitoring pass. It returns true if any ticker errored, so
// the caller can exit non-zero for cron error-alerting.
func Run() (bool, error) {
	tickers, err := store.LoadTrackers()
	if err != nil {
		return false, err
	}
	pos, err := positions.Load()
	if err != nil {
		return false, err
	}

	state := map[string]analyze.Analysis{}
	if err := store.LoadMonitorState(&state); err != nil {
		return false, err
	}

	explainCache, err := cache.Load()
	if err != nil {
		return false, err
	}
	today := time.Now().Format("2006-01-02")

	var alerts []string
	anyError := false

	for _, ticker := range tickers {
		meta, bars, err := stock.GetBars(ticker, "5d", "1h", true)
		if err != nil {
			alerts = append(alerts, fmt.Sprintf("*%s* ERROR: %v", ticker, err))
			anyError = true
			continue
		}
		newData, errStr := analyze.Analyze(ticker, meta, bars)
		if errStr != "" {
			alerts = append(alerts, fmt.Sprintf("*%s* ERROR: %s", ticker, errStr))
			anyError = true
			continue
		}

		var old *analyze.Analysis
		if o, ok := state[ticker]; ok {
			old = &o
		}
		position, hasPos := pos[ticker]
		msg, changeAlerted := formatAlert(&newData, old, position, hasPos)

		// Optional AI news context — gated and cached to control cost.
		if why := explanation(ticker, &newData, changeAlerted, today, explainCache); why != "" {
			msg += "\n• 🧠 News: " + why
		}

		alerts = append(alerts, msg)
		state[ticker] = newData
	}

	if err := store.SaveMonitorState(state); err != nil {
		return anyError, err
	}
	if err := cache.Save(explainCache); err != nil {
		return anyError, err
	}

	// Portfolio totals across tracked tickers with positions.
	var totalCost, totalValue float64
	for _, ticker := range tickers {
		d, ok := state[ticker]
		if !ok || d.LastPrice == 0 {
			continue
		}
		if p, ok := pos[ticker]; ok {
			totalCost += p.Qty * p.Avg
			totalValue += p.Qty * d.LastPrice
		}
	}

	// Print for cron delivery — clean Telegram format, no headers.
	var b strings.Builder
	for i, a := range alerts {
		b.WriteString(a)
		b.WriteString("\n")
		if i < len(alerts)-1 {
			b.WriteString("\n---\n\n")
		} else {
			b.WriteString("\n")
		}
	}
	if totalCost > 0 {
		portPnl := totalValue - totalCost
		portPnlPct := portPnl / totalCost * 100
		b.WriteString(fmt.Sprintf("*Portfolio*: $%s  |  P&L: **$%s (%+.1f%%)**\n\n",
			comma0(totalValue), comma0(portPnl), portPnlPct))
	}
	fmt.Print(b.String())

	return anyError, nil
}

// formatAlert renders one ticker's report and (when a position exists) stamps
// new.PnlPct so the next run can detect P&L threshold crossings. The bool return
// reports whether a change-detection ALERT line fired this run.
func formatAlert(new, old *analyze.Analysis, pos positions.Position, hasPos bool) (string, bool) {
	t := new.Ticker
	lp := new.LastPrice
	session := analyze.MarketSession(time.Now().UTC())

	sessionTag := fmt.Sprintf("[🕐 %s]", session)
	if new.DataStale {
		sessionTag = fmt.Sprintf("[🕐 %s — stale data (no trades)]", session)
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("**%s**  %s", t, sessionTag))

	var pnl positions.PnL
	if hasPos {
		pnl = positions.Compute(pos, lp)
	}

	// One-line assessment.
	var assess []string
	if new.IsDowntrend {
		switch {
		case len(new.AccelerationSignals) > 0:
			assess = append(assess, "downtrend worsening")
		case len(new.ReversalSignals) > 0:
			assess = append(assess, "downtrend but reversal signals appearing")
		default:
			assess = append(assess, "downtrend intact")
		}
	} else {
		if len(new.ReversalSignals) > 0 {
			assess = append(assess, "potential bottom forming")
		} else {
			assess = append(assess, "not in downtrend")
		}
	}
	if hasPos {
		dist := pnl.DistToBreakevenPct
		switch {
		case dist <= -20:
			assess = append(assess, fmt.Sprintf("deep loss (%+.1f%%)", pnl.PnlPct))
		case dist <= -10:
			assess = append(assess, fmt.Sprintf("significant loss (%+.1f%%)", pnl.PnlPct))
		case dist >= -2:
			assess = append(assess, "near breakeven")
		}
	}
	if new.VolRatio > 1.5 {
		assess = append(assess, "heavy volume")
	} else if new.VolRatio < 0.7 {
		assess = append(assess, "very quiet")
	}
	if new.HasAccel("near period low") {
		assess = append(assess, "at period low")
	}
	lines = append(lines, "*"+strings.Join(assess, " / ")+"*")

	// Price line.
	dc := 0.0
	if new.DailyChangePct != nil {
		dc = *new.DailyChangePct
	}
	lines = append(lines, fmt.Sprintf("• 💵 **$%.2f**  |  %s Today **%+.2f%%**  |  %s 5d **%+.2f%%**",
		lp, emoji(dc >= 0), dc, emoji(new.ChangePct >= 0), new.ChangePct))

	// Position details.
	if hasPos {
		totalVal := lp * pnl.Qty
		lines = append(lines, fmt.Sprintf("• 📦 Position: %d @ **$%.2f**  |  Total: **$%s**",
			int64(pnl.Qty), pnl.Avg, comma0(totalVal)))
		lines = append(lines, fmt.Sprintf("• %s 💰 P&L: **$%s (%+.1f%%)**  |  🎯 BE: **%+.1f%%**",
			emoji(pnl.Pnl >= 0), comma0(pnl.Pnl), pnl.PnlPct, pnl.DistToBreakevenPct))
	}

	// Trend + volume.
	statusEmoji := "📈"
	status := "**UP**"
	if new.IsDowntrend {
		statusEmoji = "📉"
		status = "**DOWN**"
	}
	lines = append(lines, fmt.Sprintf("• %s Status: %s", statusEmoji, status))
	lines = append(lines, fmt.Sprintf("• 📊 Vol: **%.2fx** (%s)  |  Recent: %s  |  Avg: %s",
		new.VolRatio, volDesc(new.VolRatio), fmtBig(new.RecentVol), fmtBig(new.AvgVol)))

	// The analysis signals ARE the "why" — surface them in the alert.
	signals := append(append([]string{}, new.ReversalSignals...), new.AccelerationSignals...)
	if len(signals) > 0 {
		lines = append(lines, "• 🔎 Why: "+strings.Join(signals, ", "))
	}

	// Change detection vs previous run.
	fired := false
	if old != nil {
		switch {
		case old.IsDowntrend && !new.IsDowntrend:
			lines = append(lines, "🚨 **ALERT: Downtrend ending**")
			fired = true
		case !old.IsDowntrend && new.IsDowntrend:
			lines = append(lines, "🚨 **ALERT: Downtrend beginning**")
			fired = true
		case len(new.ReversalSignals) > 0 && len(old.ReversalSignals) == 0:
			lines = append(lines, "🚨 **ALERT: Reversal signal**")
			fired = true
		case len(new.AccelerationSignals) > 0 && len(old.AccelerationSignals) == 0:
			lines = append(lines, "🚨 **ALERT: Downtrend accelerating**")
			fired = true
		}

		if hasPos {
			var oldPnlVal float64
			hasOldPnl := false
			if old.LastPrice > 0 {
				oldPnlVal = positions.Compute(pos, old.LastPrice).Pnl
				hasOldPnl = true
			}
			dist := pnl.DistToBreakevenPct
			if hasOldPnl && oldPnlVal > -10000 && pnl.Pnl <= -10000 {
				lines = append(lines, "🚨 **ALERT: Loss crossed -$10K**")
			}
			if dist >= -2.0 && dist < 0 {
				oldPrice := old.LastPrice
				if oldPrice == 0 {
					oldPrice = lp
				}
				if positions.Compute(pos, oldPrice).DistToBreakevenPct < -2.0 {
					lines = append(lines, fmt.Sprintf("🚨 **ALERT: Near breakeven (%.1f%%)**", dist))
				}
			}
			if hasOldPnl && old.PnlPct != nil && *old.PnlPct > -20 && pnl.PnlPct <= -20 {
				lines = append(lines, "🚨 **ALERT: Loss exceeded -20%**")
			}
		}
	}

	// Stamp P&L for next run's threshold comparisons.
	if hasPos {
		p := pnl.PnlPct
		new.PnlPct = &p
	}

	return strings.Join(lines, "\n"), fired
}

// explanation returns an optional AI news "why" line, gated and cached to keep
// LLM cost bounded across the many runs in a day:
//   - only when OPENAI_API_KEY is set and AI_EXPLAIN != "false"
//   - only when the move is meaningful (a change-alert fired OR the daily move
//     magnitude >= AI_EXPLAIN_MIN_MOVE_PCT)
//   - at most once per ticker per day, re-billed only if the move materially
//     shifts (sign flip or >= AI_CACHE_REFRESH_PCT jump); every other run reuses
//     the cached text for free.
//
// It mutates the cache map in place; the caller persists it.
func explanation(ticker string, new *analyze.Analysis, changeAlerted bool, today string, c map[string]cache.Entry) string {
	if !ai.Enabled() || !getenvBool("AI_EXPLAIN", true) {
		return ""
	}

	dc := 0.0
	if new.DailyChangePct != nil {
		dc = *new.DailyChangePct
	}
	minMove := getenvFloat("AI_EXPLAIN_MIN_MOVE_PCT", 3.0)
	refresh := getenvFloat("AI_CACHE_REFRESH_PCT", 2.0)
	need := changeAlerted || absf(dc) >= minMove

	entry, ok := c[ticker]
	validToday := ok && entry.Date == today

	generate := func() string {
		why, err := ai.WhyMoved(ticker, dc, new.LastPrice)
		if err != nil {
			fmt.Fprintf(os.Stderr, "(AI explain failed for %s: %v)\n", ticker, err)
			return ""
		}
		c[ticker] = cache.Entry{
			Date:      today,
			ChangePct: dc,
			LastPrice: new.LastPrice,
			Why:       why,
			Timestamp: time.Now().Format(time.RFC3339),
		}
		return why
	}

	if validToday {
		stale := (entry.ChangePct < 0) != (dc < 0) || absf(dc-entry.ChangePct) >= refresh
		if need && stale {
			if why := generate(); why != "" {
				return why
			}
		}
		return entry.Why // free reuse of today's cached explanation
	}

	if need {
		return generate()
	}
	return ""
}

func getenvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func getenvFloat(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

func absf(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func emoji(green bool) string {
	if green {
		return "🟢"
	}
	return "🔴"
}

func volDesc(vr float64) string {
	switch {
	case vr < 0.7:
		return "low volume (quiet)"
	case vr < 0.9:
		return "light volume (below avg)"
	case vr <= 1.1:
		return "normal volume"
	case vr <= 1.5:
		return "elevated volume (active)"
	case vr <= 2.5:
		return "high volume (spike)"
	default:
		return "very heavy volume (unusual)"
	}
}

// fmtBig formats large counts as 6.8M, 1.2B, etc.
func fmtBig(n int64) string {
	f := float64(n)
	switch {
	case absf(f) >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", f/1_000_000_000)
	case absf(f) >= 1_000_000:
		return fmt.Sprintf("%.1fM", f/1_000_000)
	case absf(f) >= 1_000:
		return fmt.Sprintf("%.1fK", f/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// comma0 formats a number with thousands separators and no decimals.
func comma0(f float64) string {
	neg := f < 0
	if neg {
		f = -f
	}
	n := int64(f + 0.5)
	s := fmt.Sprintf("%d", n)
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	out := strings.Join(parts, ",")
	if neg {
		out = "-" + out
	}
	return out
}
