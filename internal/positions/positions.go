// Package positions loads held positions and computes unrealized P&L.
package positions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"stocktracker/internal/num"
)

const file = "resources/positions.json"

// Position is a held position in a single symbol.
type Position struct {
	Qty  float64      `json:"qty"`
	Avg  float64      `json:"avg"`
	Lots [][2]float64 `json:"lots,omitempty"` // optional [shares, price] lots
}

// PnL is the computed profit/loss for a position at a given price.
type PnL struct {
	Qty                float64
	Avg                float64
	Value              float64
	Cost               float64
	Pnl                float64
	PnlPct             float64
	Breakeven          float64
	DistToBreakevenPct float64
}

// Load reads resources/positions.json. A missing file yields an empty map.
func Load() (map[string]Position, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Position{}, nil
		}
		return nil, err
	}
	positions := map[string]Position{}
	if len(data) == 0 {
		return positions, nil
	}
	if err := json.Unmarshal(data, &positions); err != nil {
		return nil, err
	}
	return positions, nil
}

// Save writes the positions map to resources/positions.json.
func Save(m map[string]Position) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(file, data, 0o644)
}

// Set creates or updates a position (symbol upper-cased). It replaces qty and
// avg; any existing `lots` detail is preserved.
func Set(symbol string, qty, avg float64) error {
	symbol = strings.ToUpper(symbol)
	m, err := Load()
	if err != nil {
		return err
	}
	existing := m[symbol]
	existing.Qty = qty
	existing.Avg = avg
	m[symbol] = existing
	return Save(m)
}

// Remove deletes a position. Returns false if it was not present.
func Remove(symbol string) (bool, error) {
	symbol = strings.ToUpper(symbol)
	m, err := Load()
	if err != nil {
		return false, err
	}
	if _, ok := m[symbol]; !ok {
		return false, nil
	}
	delete(m, symbol)
	return true, Save(m)
}

// Compute returns the P&L for a position at the given last price.
func Compute(pos Position, lastPrice float64) PnL {
	value := pos.Qty * lastPrice
	cost := pos.Qty * pos.Avg
	pnl := value - cost
	pnlPct := 0.0
	if cost != 0 {
		pnlPct = pnl / cost * 100
	}
	dist := 0.0
	if pos.Avg != 0 {
		dist = (lastPrice - pos.Avg) / pos.Avg * 100
	}
	return PnL{
		Qty:                pos.Qty,
		Avg:                pos.Avg,
		Value:              round2(value),
		Cost:               round2(cost),
		Pnl:                round2(pnl),
		PnlPct:             round2(pnlPct),
		Breakeven:          pos.Avg,
		DistToBreakevenPct: round2(dist),
	}
}

func round2(v float64) float64 {
	return num.Round(v, 2)
}
