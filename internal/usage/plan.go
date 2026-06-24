package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// planFromLocal auto-detects the subscription tier from ~/.claude.json, where
// Claude Code records oauthAccount.{userRateLimitTier,organizationRateLimitTier}
// (e.g. "default_claude_max_20x"). This is a local read (no network) and is done
// off the hot path, during a background refresh. Returns "" if unknown.
func planFromLocal() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(home, ".claude.json"))
	if err != nil {
		return ""
	}
	var d struct {
		OauthAccount struct {
			OrganizationRateLimitTier string `json:"organizationRateLimitTier"`
			UserRateLimitTier         string `json:"userRateLimitTier"`
		} `json:"oauthAccount"`
	}
	if json.Unmarshal(b, &d) != nil {
		return ""
	}
	tier := d.OauthAccount.UserRateLimitTier
	if tier == "" {
		tier = d.OauthAccount.OrganizationRateLimitTier
	}
	return PrettyTier(tier)
}

// PrettyTier maps an internal rate-limit tier string to a display label.
func PrettyTier(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "default_")
	s = strings.TrimPrefix(s, "claude_")
	switch s {
	case "":
		return ""
	case "max_20x", "max20x":
		return "Max 20x"
	case "max_5x", "max5x":
		return "Max 5x"
	case "pro":
		return "Pro"
	case "free":
		return "Free"
	case "team":
		return "Team"
	case "enterprise":
		return "Enterprise"
	}
	// Fallback: turn "some_tier" into "Some Tier".
	words := strings.FieldsFunc(s, func(r rune) bool { return r == '_' || r == '-' })
	for i, w := range words {
		if w != "" {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
