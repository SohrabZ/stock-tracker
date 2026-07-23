package monitor

import (
	"strings"
	"testing"

	"stocktracker/internal/analyze"
	"stocktracker/internal/cache"
	"stocktracker/internal/positions"
)

func ptr(f float64) *float64 { return &f }

func TestComma0(t *testing.T) {
	cases := map[float64]string{
		0:       "0",
		999:     "999",
		1000:    "1,000",
		50498.2: "50,498",
		-7265.3: "-7,265",
		1234567: "1,234,567",
	}
	for in, want := range cases {
		if got := comma0(in); got != want {
			t.Errorf("comma0(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestFmtBig(t *testing.T) {
	cases := map[int64]string{
		500:        "500",
		719900:     "719.9K",
		2200000:    "2.2M",
		1100000000: "1.1B",
	}
	for in, want := range cases {
		if got := fmtBig(in); got != want {
			t.Errorf("fmtBig(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestVolDesc(t *testing.T) {
	cases := map[float64]string{
		0.5: "low volume (quiet)",
		0.8: "light volume (below avg)",
		1.0: "normal volume",
		1.3: "elevated volume (active)",
		1.9: "high volume (spike)",
		3.0: "very heavy volume (unusual)",
	}
	for in, want := range cases {
		if got := volDesc(in); got != want {
			t.Errorf("volDesc(%v) = %q, want %q", in, got, want)
		}
	}
}

func baseAnalysis() analyze.Analysis {
	return analyze.Analysis{
		Ticker:              "SLV",
		Name:                "iShares Silver Trust",
		LastPrice:           52.06,
		ChangePct:           4.0,
		DailyChangePct:      ptr(-3.44),
		High:                54.5,
		Low:                 49.8,
		AvgPrice:            52.2,
		VolRatio:            0.91,
		AvgVol:              792200,
		RecentVol:           719900,
		IsDowntrend:         false,
		ReversalSignals:     []string{"bounced +4.6% off low"},
		AccelerationSignals: []string{},
	}
}

func TestFormatAlertWithPosition(t *testing.T) {
	a := baseAnalysis()
	pos := positions.Position{Qty: 970, Avg: 59.55}
	out, _ := formatAlert(&a, nil, pos, true)

	for _, want := range []string{"📦 Position: 970", "💰 P&L:", "🔎 Why: bounced +4.6% off low", "**SLV**"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n---\n%s", want, out)
		}
	}
	// PnlPct must be stamped for next-run comparisons.
	if a.PnlPct == nil {
		t.Errorf("PnlPct not stamped after formatAlert")
	}
}

func TestFormatAlertNoPosition(t *testing.T) {
	a := baseAnalysis()
	out, _ := formatAlert(&a, nil, positions.Position{}, false)

	for _, forbidden := range []string{"Position:", "P&L:", "BE:"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("output should NOT contain %q when no position\n---\n%s", forbidden, out)
		}
	}
	// Still shows price and the analysis "why".
	if !strings.Contains(out, "🔎 Why:") {
		t.Errorf("output missing analysis why line\n---\n%s", out)
	}
	if a.PnlPct != nil {
		t.Errorf("PnlPct should stay nil with no position")
	}
}

func TestFormatAlertChangeDetectionBeginning(t *testing.T) {
	old := baseAnalysis()
	old.IsDowntrend = false
	newA := baseAnalysis()
	newA.IsDowntrend = true

	out, fired := formatAlert(&newA, &old, positions.Position{}, false)
	if !fired {
		t.Errorf("fired = false, want true on downtrend beginning")
	}
	if !strings.Contains(out, "Downtrend beginning") {
		t.Errorf("missing 'Downtrend beginning'\n---\n%s", out)
	}
}

func TestFormatAlertChangeDetectionEnding(t *testing.T) {
	old := baseAnalysis()
	old.IsDowntrend = true
	newA := baseAnalysis()
	newA.IsDowntrend = false

	out, fired := formatAlert(&newA, &old, positions.Position{}, false)
	if !fired {
		t.Errorf("fired = false, want true on downtrend ending")
	}
	if !strings.Contains(out, "Downtrend ending") {
		t.Errorf("missing 'Downtrend ending'\n---\n%s", out)
	}
}

func TestFormatAlertLossExceeded20(t *testing.T) {
	// old at -19%, new at -21% -> "Loss exceeded -20%".
	old := baseAnalysis()
	old.LastPrice = 48.24 // ~ -19% from 59.55
	old.PnlPct = ptr(-19.0)
	newA := baseAnalysis()
	newA.LastPrice = 47.05 // ~ -21% from 59.55
	pos := positions.Position{Qty: 970, Avg: 59.55}

	out, _ := formatAlert(&newA, &old, pos, true)
	if !strings.Contains(out, "Loss exceeded -20%") {
		t.Errorf("missing 'Loss exceeded -20%%'\n---\n%s", out)
	}
}

func TestExplanationDisabledWithoutKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	a := baseAnalysis()
	if got := explanation("SLV", &a, true, "2026-07-23", map[string]cache.Entry{}); got != "" {
		t.Errorf("explanation = %q, want empty without API key", got)
	}
}

func TestExplanationReusesCacheWithoutCall(t *testing.T) {
	// With a valid same-day cache entry and no material move change, explanation
	// must return the cached text without any API call — even with a key set.
	t.Setenv("OPENAI_API_KEY", "sk-test-should-not-be-called")
	t.Setenv("AI_EXPLAIN_MIN_MOVE_PCT", "3.0")
	a := baseAnalysis() // DailyChangePct = -3.44
	c := map[string]cache.Entry{
		"SLV": {Date: "2026-07-23", ChangePct: -3.44, Why: "cached reason"},
	}
	if got := explanation("SLV", &a, false, "2026-07-23", c); got != "cached reason" {
		t.Errorf("explanation = %q, want cached reason (no API call)", got)
	}
}
