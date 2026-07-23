// Package cache stores AI explanations so the LLM is billed at most once per
// ticker per day (re-billed only when a move materially changes). This keeps
// cost bounded even though the monitor runs every 30 minutes.
package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const file = "resources/explain_cache.json"

// Entry is a cached explanation for one ticker.
type Entry struct {
	Date      string  `json:"date"`       // YYYY-MM-DD the explanation was generated
	ChangePct float64 `json:"change_pct"` // daily % move at generation time
	LastPrice float64 `json:"last_price"` // price at generation time
	Why       string  `json:"why"`        // the cached explanation text
	Timestamp string  `json:"timestamp"`  // when it was generated (RFC3339)
}

// Load reads the explanation cache. A missing file yields an empty map.
func Load() (map[string]Entry, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Entry{}, nil
		}
		return nil, err
	}
	m := map[string]Entry{}
	if len(data) == 0 {
		return m, nil
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// Save writes the explanation cache.
func Save(m map[string]Entry) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(file, data, 0o644)
}
