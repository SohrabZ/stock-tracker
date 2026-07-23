// Package ai provides an optional AI news-research step. When OPENAI_API_KEY is
// set it uses the OpenAI Responses API (with the web_search tool) to explain
// why a stock moved. Without a key, callers fall back to no explanation.
//
// Cost note: every call here hits a paid API. Callers are responsible for
// gating how often this runs (see the monitor's AI_EXPLAIN_* controls).
package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	responsesURL = "https://api.openai.com/v1/responses"
	defaultModel = "gpt-5.6-luna"
)

// Enabled reports whether an OpenAI API key is configured.
func Enabled() bool {
	return os.Getenv("OPENAI_API_KEY") != ""
}

func model() string {
	if m := os.Getenv("OPENAI_MODEL"); m != "" {
		return m
	}
	return defaultModel
}

var httpClient = &http.Client{Timeout: 120 * time.Second}

type request struct {
	Model string        `json:"model"`
	Input string        `json:"input"`
	Tools []interface{} `json:"tools,omitempty"`
}

type response struct {
	OutputText string `json:"output_text"`
	Output     []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// callResponses sends a single prompt (with web search enabled) and returns the
// model's text output.
func callResponses(prompt string) (string, error) {
	payload, err := json.Marshal(request{
		Model: model(),
		Input: prompt,
		Tools: []interface{}{map[string]string{"type": "web_search"}},
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, responsesURL, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+os.Getenv("OPENAI_API_KEY"))

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var r response
	if err := json.Unmarshal(body, &r); err != nil {
		return "", fmt.Errorf("parsing OpenAI response: %w (body: %s)", err, truncate(string(body), 300))
	}
	if r.Error != nil {
		return "", fmt.Errorf("OpenAI error: %s", r.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenAI returned %d: %s", resp.StatusCode, truncate(string(body), 300))
	}

	text := extractText(r)
	if text == "" {
		return "", fmt.Errorf("OpenAI returned no text output")
	}
	return text, nil
}

// WhyMoved returns a concise, prefix-free explanation (<=160 chars) of why a
// symbol moved, suitable to append to a monitor alert line.
func WhyMoved(symbol string, dailyChangePct, lastPrice float64) (string, error) {
	dir := "up"
	if dailyChangePct < 0 {
		dir = "down"
	}
	mag := dailyChangePct
	if mag < 0 {
		mag = -mag
	}
	prompt := fmt.Sprintf(`The stock/ETF %s is %s %.2f%% today (now %.2f). Use web search to find the most likely reason for this move in the last 24 hours (news, earnings, macro, sector). Respond with ONLY a concise reason, no more than 160 characters. No ticker prefix, no percentage, no quotes — just the reason.`,
		symbol, dir, mag, lastPrice)
	return callResponses(prompt)
}

// Research returns a full one-line alert ("SYM UP/DOWN X.XX%: reason") for the
// standalone `research` command.
func Research(symbol string, currentPrice, prevClose float64) (string, error) {
	pct := (currentPrice/prevClose - 1) * 100
	prompt := fmt.Sprintf(`You are a stock market researcher. The stock %s has moved %.2f%% from its previous close (now %.2f, previous close %.2f) within the past 24 hours.

Use web search to find the most likely reason for this move in the last 24 hours. Then respond with a SINGLE line, no more than 160 characters, in exactly this format:

%s UP/DOWN X.XX%%: <concise reason>

Pick UP or DOWN based on the sign of the move. Do not add anything else.`,
		symbol, pct, currentPrice, prevClose, symbol)
	return callResponses(prompt)
}

func extractText(r response) string {
	if r.OutputText != "" {
		return r.OutputText
	}
	for _, item := range r.Output {
		if item.Type != "message" {
			continue
		}
		for _, c := range item.Content {
			if c.Type == "output_text" && c.Text != "" {
				return c.Text
			}
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
