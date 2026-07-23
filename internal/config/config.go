// Package config loads simple KEY=VALUE settings from a .env file into the
// process environment. It is intentionally tiny and dependency-free.
package config

import (
	"bufio"
	"os"
	"strings"
)

// LoadDotenv reads the given file (if it exists) and sets any KEY=VALUE pairs
// into the environment, without overwriting variables already set in the shell.
// A missing file is not an error — the app runs fine without one.
func LoadDotenv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, val)
		}
	}
}
