// Package config loads ccbar's optional user configuration. All settings have
// sensible defaults baked in, so the file is optional; values present in the file
// override the corresponding defaults.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Segments toggles which segments may appear in the bar (width permitting).
type Segments struct {
	Model       bool     `json:"model"`
	Tokens      bool     `json:"tokens"`
	CostLast    bool     `json:"cost_last"`
	CostSession bool     `json:"cost_session"`
	FiveHour    bool     `json:"five_hour"`
	SevenDay    bool     `json:"seven_day"`
	PerModel    []string `json:"per_model"` // display names, e.g. ["Opus","Sonnet"]
	Context     bool     `json:"context"`
}

// reserved style values; currently only "text" is implemented.
const StyleText = "text"

// Config is the full set of user-tunable options.
type Config struct {
	Style             string   `json:"style"`               // reserved; only "text" is implemented
	Color             bool     `json:"color"`               // emit ANSI color
	ASCII             bool     `json:"ascii"`               // ASCII-only separators/glyphs
	CompactLabels     bool     `json:"compact_labels"`      // terse labels (5h, wk, tok) instead of full phrases
	WarnThreshold     float64  `json:"warn_threshold"`      // percent at which a limit turns amber
	CritThreshold     float64  `json:"crit_threshold"`      // percent at which a limit turns red
	Segments          Segments `json:"segments"`            //
	HideZeroPerModel  bool     `json:"hide_zero_per_model"` // omit per-model limits sitting at 0%
	UsageEndpoint     bool     `json:"usage_endpoint"`      // allow the authenticated /api/oauth/usage call for per-model limits
	Keychain          bool     `json:"keychain"`            // fall back to macOS Keychain if the credentials file token is expired (may prompt once)
	CacheTTLSeconds   int      `json:"cache_ttl_seconds"`
	Plan              string   `json:"plan"`                 // "auto" or an explicit label, e.g. "Max 20x"
	ShowPlan          bool     `json:"show_plan"`            // render the plan label as a leading segment
	ShowResetsWhenHot bool     `json:"show_resets_when_hot"` // append a reset countdown to a limit once it crosses the warn threshold
}

// Default returns the built-in configuration.
func Default() Config {
	return Config{
		Style:         StyleText,
		Color:         true,
		ASCII:         false,
		WarnThreshold: 70,
		CritThreshold: 90,
		Segments: Segments{
			Model:       true,
			Tokens:      true,
			CostLast:    true,
			CostSession: true,
			FiveHour:    true,
			SevenDay:    true,
			PerModel:    []string{"Opus", "Sonnet"},
			Context:     false,
		},
		UsageEndpoint:     true,
		Keychain:          false,
		CacheTTLSeconds:   60,
		Plan:              "auto",
		ShowPlan:          false,
		ShowResetsWhenHot: false,
	}
}

// Dir is ~/.claude/ccbar, where the config, pricing override, and usage cache live.
func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "ccbar")
	}
	return filepath.Join(home, ".claude", "ccbar")
}

// Path is the config file location.
func Path() string { return filepath.Join(Dir(), "config.json") }

// Load reads the config file over the defaults. A missing file yields defaults; a
// malformed file also yields defaults (the bar must never fail to render).
func Load() Config {
	cfg := Default()
	b, err := os.ReadFile(Path())
	if err != nil {
		return cfg
	}
	// Unmarshalling into the defaults-populated struct means only keys present in
	// the file override defaults; absent scalars keep their default value.
	_ = json.Unmarshal(b, &cfg)
	if cfg.CacheTTLSeconds <= 0 {
		cfg.CacheTTLSeconds = 60
	}
	if cfg.Style == "" {
		cfg.Style = StyleText
	}
	return cfg
}

// WriteDefault writes a default config file (and ensures the directory exists),
// used by `ccbar --init-config`. It does not overwrite an existing file.
func WriteDefault() (string, bool, error) {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return "", false, err
	}
	p := Path()
	if _, err := os.Stat(p); err == nil {
		return p, false, nil // already exists; leave it alone
	}
	b, err := json.MarshalIndent(Default(), "", "  ")
	if err != nil {
		return "", false, err
	}
	if err := os.WriteFile(p, append(b, '\n'), 0o644); err != nil {
		return "", false, err
	}
	return p, true, nil
}
