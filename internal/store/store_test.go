package store

import "testing"

func TestTrackerRoundTrip(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	list, err := LoadTrackers()
	if err != nil {
		t.Fatalf("LoadTrackers: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("fresh tracker list = %v, want empty", list)
	}

	// Add lower-case; expect it stored upper-cased.
	added, err := AddTracker("slv")
	if err != nil || !added {
		t.Fatalf("AddTracker slv: added=%v err=%v", added, err)
	}
	// Duplicate add returns false.
	if added, _ := AddTracker("SLV"); added {
		t.Errorf("duplicate AddTracker returned true")
	}
	list, _ = LoadTrackers()
	if len(list) != 1 || list[0] != "SLV" {
		t.Fatalf("list = %v, want [SLV]", list)
	}

	// Remove non-existent returns false.
	if removed, _ := RemoveTracker("AAPL"); removed {
		t.Errorf("RemoveTracker of absent symbol returned true")
	}
	if removed, _ := RemoveTracker("slv"); !removed {
		t.Errorf("RemoveTracker slv returned false")
	}
	list, _ = LoadTrackers()
	if len(list) != 0 {
		t.Fatalf("list after remove = %v, want empty", list)
	}
}

func TestMonitorStateRoundTrip(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := Ensure(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	in := map[string]map[string]any{"SLV": {"last_price": 52.06}}
	if err := SaveMonitorState(in); err != nil {
		t.Fatalf("SaveMonitorState: %v", err)
	}
	out := map[string]map[string]any{}
	if err := LoadMonitorState(&out); err != nil {
		t.Fatalf("LoadMonitorState: %v", err)
	}
	if out["SLV"]["last_price"] != 52.06 {
		t.Errorf("round-trip last_price = %v, want 52.06", out["SLV"]["last_price"])
	}
}
