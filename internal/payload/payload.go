// Package payload models the JSON object Claude Code pipes to a statusLine
// command on stdin. Field names and shapes were verified against the installed
// Claude Code 2.1.190 binary. Optional objects are pointers so absence can be
// detected (e.g. rate_limits only appears after the first API response, and only
// for Pro/Max OAuth subscribers).
package payload

import (
	"encoding/json"
	"fmt"
)

// Payload is the top-level stdin object. Only the fields the info bar uses are
// modeled; unknown fields are ignored by encoding/json.
type Payload struct {
	SessionID      string         `json:"session_id"`
	TranscriptPath string         `json:"transcript_path"`
	Cwd            string         `json:"cwd"`
	Version        string         `json:"version"`
	Model          Model          `json:"model"`
	Workspace      Workspace      `json:"workspace"`
	Cost           Cost           `json:"cost"`
	ContextWindow  *ContextWindow `json:"context_window"`
	Exceeds200k    bool           `json:"exceeds_200k_tokens"`
	FastMode       bool           `json:"fast_mode"`
	Effort         *Effort        `json:"effort"`
	RateLimits     *RateLimits    `json:"rate_limits"`
	OutputStyle    *OutputStyle   `json:"output_style"`
}

type Model struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

type Workspace struct {
	CurrentDir string `json:"current_dir"`
	ProjectDir string `json:"project_dir"`
}

type OutputStyle struct {
	Name string `json:"name"`
}

// Cost holds Claude Code's own client-side, API-equivalent cost estimate for the
// session. total_cost_usd is exactly "what this session would have cost on an API
// key" — the metric the brief asked for as the session total.
type Cost struct {
	TotalCostUSD       float64 `json:"total_cost_usd"`
	TotalDurationMs    int64   `json:"total_duration_ms"`
	TotalAPIDurationMs int64   `json:"total_api_duration_ms"`
	TotalLinesAdded    int     `json:"total_lines_added"`
	TotalLinesRemoved  int     `json:"total_lines_removed"`
}

// ContextWindow describes what is currently in context. CurrentUsage is the token
// breakdown of the most recent response and is the source for "last-prompt tokens".
// It is null before the first API call and immediately after /compact.
type ContextWindow struct {
	TotalInputTokens    int      `json:"total_input_tokens"`
	TotalOutputTokens   int      `json:"total_output_tokens"`
	ContextWindowSize   int      `json:"context_window_size"`
	UsedPercentage      *float64 `json:"used_percentage"`
	RemainingPercentage *float64 `json:"remaining_percentage"`
	CurrentUsage        *Usage   `json:"current_usage"`
}

// Usage is the per-response token breakdown. The four counts are mutually
// exclusive: input_tokens (uncached, full rate), cache_creation (cache write),
// cache_read (cache hit), output_tokens.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// Total returns the full token count attributable to the last prompt.
func (u Usage) Total() int {
	return u.InputTokens + u.OutputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
}

type Effort struct {
	Level string `json:"level"`
}

// RateLimits carries the two windows Claude Code derives from the
// anthropic-ratelimit-unified-* response headers. Each window is independently
// optional.
type RateLimits struct {
	FiveHour *Window `json:"five_hour"`
	SevenDay *Window `json:"seven_day"`
}

// Window is a single rate-limit window. UsedPercentage is 0–100. ResetsAt is a
// Unix epoch in SECONDS.
type Window struct {
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       int64   `json:"resets_at"`
}

// Parse decodes the stdin payload. It is tolerant of unknown/missing fields.
func Parse(b []byte) (*Payload, error) {
	var p Payload
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("decode statusline payload: %w", err)
	}
	return &p, nil
}
