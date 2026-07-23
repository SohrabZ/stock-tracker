// Package store persists the tracker list and the monitor's run-to-run state
// as JSON files under resources/.
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	dir          = "resources"
	trackerFile  = "resources/tracker_list.json"
	monitorState = "resources/monitor_state.json"
)

// Ensure creates the resources directory and empty data files if missing.
func Ensure() error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(trackerFile); os.IsNotExist(err) {
		if err := writeJSON(trackerFile, []string{}); err != nil {
			return err
		}
	}
	if _, err := os.Stat(monitorState); os.IsNotExist(err) {
		if err := writeJSON(monitorState, map[string]any{}); err != nil {
			return err
		}
	}
	return nil
}

// LoadTrackers returns the list of tracked symbols.
func LoadTrackers() ([]string, error) {
	var list []string
	if err := readJSON(trackerFile, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// SaveTrackers writes the tracker list.
func SaveTrackers(list []string) error {
	return writeJSON(trackerFile, list)
}

// AddTracker adds a symbol (upper-cased). Returns false if already present.
func AddTracker(symbol string) (bool, error) {
	symbol = strings.ToUpper(symbol)
	list, err := LoadTrackers()
	if err != nil {
		return false, err
	}
	if slices.Contains(list, symbol) {
		return false, nil
	}
	list = append(list, symbol)
	return true, SaveTrackers(list)
}

// RemoveTracker removes a symbol. Returns false if it was not present.
func RemoveTracker(symbol string) (bool, error) {
	symbol = strings.ToUpper(symbol)
	list, err := LoadTrackers()
	if err != nil {
		return false, err
	}
	idx := slices.Index(list, symbol)
	if idx == -1 {
		return false, nil
	}
	list = slices.Delete(list, idx, idx+1)
	return true, SaveTrackers(list)
}

// LoadMonitorState reads the monitor state into v (a pointer).
func LoadMonitorState(v any) error {
	return readJSON(monitorState, v)
}

// SaveMonitorState writes the monitor state.
func SaveMonitorState(v any) error {
	return writeJSON(monitorState, v)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, v)
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
