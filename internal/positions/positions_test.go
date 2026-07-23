package positions

import "testing"

func TestComputeSLV(t *testing.T) {
	// SLV: 970 @ $59.55, last $52.06.
	pos := Position{Qty: 970, Avg: 59.55}
	p := Compute(pos, 52.06)

	if p.Cost != 57763.5 {
		t.Errorf("Cost = %v, want 57763.5", p.Cost)
	}
	if p.Value != 50498.2 {
		t.Errorf("Value = %v, want 50498.2", p.Value)
	}
	if p.Pnl != -7265.3 {
		t.Errorf("Pnl = %v, want -7265.3", p.Pnl)
	}
	if p.PnlPct != -12.58 {
		t.Errorf("PnlPct = %v, want -12.58", p.PnlPct)
	}
	if p.DistToBreakevenPct != -12.58 {
		t.Errorf("DistToBreakevenPct = %v, want -12.58", p.DistToBreakevenPct)
	}
	if p.Breakeven != 59.55 {
		t.Errorf("Breakeven = %v, want 59.55", p.Breakeven)
	}
}

func TestComputeProfit(t *testing.T) {
	pos := Position{Qty: 10, Avg: 100}
	p := Compute(pos, 150)
	if p.Pnl != 500 {
		t.Errorf("Pnl = %v, want 500", p.Pnl)
	}
	if p.PnlPct != 50 {
		t.Errorf("PnlPct = %v, want 50", p.PnlPct)
	}
	if p.DistToBreakevenPct != 50 {
		t.Errorf("DistToBreakevenPct = %v, want 50", p.DistToBreakevenPct)
	}
}

func TestComputeZeroAvg(t *testing.T) {
	// Guard against divide-by-zero.
	p := Compute(Position{Qty: 5, Avg: 0}, 10)
	if p.PnlPct != 0 {
		t.Errorf("PnlPct = %v, want 0 for zero avg", p.PnlPct)
	}
	if p.DistToBreakevenPct != 0 {
		t.Errorf("DistToBreakevenPct = %v, want 0 for zero avg", p.DistToBreakevenPct)
	}
}

func TestSetPreservesLotsAndRemove(t *testing.T) {
	t.Chdir(t.TempDir())

	// Seed a position with lots, then update qty/avg via Set.
	seed := map[string]Position{"SLV": {Qty: 970, Avg: 59.55, Lots: [][2]float64{{457, 54.99}, {513, 63.61}}}}
	if err := Save(seed); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := Set("slv", 1000, 58.00); err != nil { // lower-case in -> upper-case key
		t.Fatalf("Set: %v", err)
	}

	m, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p, ok := m["SLV"]
	if !ok {
		t.Fatalf("SLV missing after Set")
	}
	if p.Qty != 1000 || p.Avg != 58.00 {
		t.Errorf("qty/avg = %v/%v, want 1000/58", p.Qty, p.Avg)
	}
	if len(p.Lots) != 2 {
		t.Errorf("lots not preserved by Set: %v", p.Lots)
	}

	// Remove semantics.
	if removed, _ := Remove("MISSING"); removed {
		t.Errorf("Remove of absent symbol returned true")
	}
	if removed, _ := Remove("SLV"); !removed {
		t.Errorf("Remove SLV returned false")
	}
	m, _ = Load()
	if _, ok := m["SLV"]; ok {
		t.Errorf("SLV still present after Remove")
	}
}

func TestRound2Negative(t *testing.T) {
	if got := round2(-12.5776); got != -12.58 {
		t.Errorf("round2(-12.5776) = %v, want -12.58", got)
	}
	if got := round2(12.5776); got != 12.58 {
		t.Errorf("round2(12.5776) = %v, want 12.58", got)
	}
}
