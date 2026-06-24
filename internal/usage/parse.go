package usage

import (
	"encoding/json"
	"io"
	"time"
)

func readAllLimited(r io.Reader, max int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, max))
}

// apiWindow is a rate-limit window from /api/oauth/usage. Verified against the live
// response: `utilization` is already a 0–100 percentage, and `resets_at` is an
// RFC3339 timestamp STRING (e.g. "2026-06-25T01:19:59.965202+00:00") — NOT an epoch
// integer (unlike the stdin statusline payload, which uses epoch seconds).
type apiWindow struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    string  `json:"resets_at"`
}

type apiLimitEntry struct {
	Kind     string  `json:"kind"`
	Percent  float64 `json:"percent"`
	ResetsAt string  `json:"resets_at"`
	Scope    struct {
		Model struct {
			DisplayName string `json:"display_name"`
		} `json:"model"`
	} `json:"scope"`
}

type apiResponse struct {
	SevenDayOpus   *apiWindow      `json:"seven_day_opus"`
	SevenDaySonnet *apiWindow      `json:"seven_day_sonnet"`
	Limits         []apiLimitEntry `json:"limits"`
}

// parseResetTime converts the endpoint's RFC3339 timestamp to an epoch second.
// Returns 0 when empty or unparseable (so the reset countdown simply hides).
func parseResetTime(s string) int64 {
	if s == "" {
		return 0
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Unix()
		}
	}
	return 0
}

// parse extracts per-model weekly limits from the /api/oauth/usage response. It is
// defensive: unknown/null shapes yield no models rather than an error, so a schema
// change degrades gracefully. Plan detection is handled separately (planFromLocal).
func parse(body []byte) ([]PerModel, string) {
	var r apiResponse
	_ = json.Unmarshal(body, &r) // tolerate partial/unknown shapes

	// Preserve discovery order while deduping by name; limits[] overrides the
	// top-level seven_day_* values since that is the source /usage itself uses.
	order := make([]string, 0, 4)
	byName := make(map[string]PerModel, 4)
	add := func(name string, pct float64, resets int64) {
		if name == "" {
			return
		}
		if pct < 0 {
			pct = 0
		}
		if _, seen := byName[name]; !seen {
			order = append(order, name)
		}
		byName[name] = PerModel{Name: name, Percent: pct, ResetsAt: resets}
	}

	if r.SevenDayOpus != nil {
		add("Opus", r.SevenDayOpus.Utilization, parseResetTime(r.SevenDayOpus.ResetsAt))
	}
	if r.SevenDaySonnet != nil {
		add("Sonnet", r.SevenDaySonnet.Utilization, parseResetTime(r.SevenDaySonnet.ResetsAt))
	}
	for _, e := range r.Limits {
		if e.Kind == "weekly_scoped" && e.Scope.Model.DisplayName != "" {
			add(e.Scope.Model.DisplayName, e.Percent, parseResetTime(e.ResetsAt))
		}
	}

	out := make([]PerModel, 0, len(order))
	for _, n := range order {
		out = append(out, byName[n])
	}
	return out, ""
}
