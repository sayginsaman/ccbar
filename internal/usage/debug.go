package usage

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/sayginsaman/ccbar/internal/config"
)

// DebugStructure fetches /api/oauth/usage and returns a SANITIZED description of
// its shape: top-level keys with their JSON type, the numeric fields of any
// rate-limit window objects, and the limits[] entries (kind, model display name,
// percent, resets_at). It deliberately does NOT print string values of unknown
// keys, so account identifiers / emails are never echoed. Used by `ccbar
// --dump-usage` to diagnose schema drift.
func DebugStructure(cfg config.Config) (string, error) {
	tok, ok := readToken(cfg)
	if !ok {
		return "", fmt.Errorf("no usable OAuth token")
	}
	body, ok := fetch(tok)
	if !ok {
		return "", fmt.Errorf("request failed (offline, 401, or non-200)")
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal(body, &top); err != nil {
		return "", fmt.Errorf("response is not a JSON object: %w", err)
	}

	var sb strings.Builder
	keys := make([]string, 0, len(top))
	for k := range top {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Fprintf(&sb, "top-level keys (%d):\n", len(keys))
	for _, k := range keys {
		fmt.Fprintf(&sb, "  %-28s %s\n", k, jsonKind(top[k]))
		// Expand objects that look like rate-limit windows.
		if looksLikeWindow(top[k]) {
			var w map[string]json.RawMessage
			_ = json.Unmarshal(top[k], &w)
			wk := make([]string, 0, len(w))
			for kk := range w {
				wk = append(wk, kk)
			}
			sort.Strings(wk)
			for _, kk := range wk {
				// Only echo the raw value when it is genuinely a scalar number;
				// never print objects/strings (which may hold account data) even
				// under a number-like key name.
				if isNumberKey(kk) && jsonKind(w[kk]) == "number" {
					fmt.Fprintf(&sb, "      .%-22s = %s\n", kk, string(w[kk]))
				} else {
					fmt.Fprintf(&sb, "      .%-22s %s\n", kk, jsonKind(w[kk]))
				}
			}
		}
	}

	// limits[] detail (safe fields only).
	if raw, ok := top["limits"]; ok {
		var entries []apiLimitEntry
		if json.Unmarshal(raw, &entries) == nil {
			fmt.Fprintf(&sb, "\nlimits[] entries (%d):\n", len(entries))
			for i, e := range entries {
				model := e.Scope.Model.DisplayName
				if model == "" {
					model = "-"
				}
				fmt.Fprintf(&sb, "  [%d] kind=%-16q model=%-10q percent=%.3f resets_at=%q\n",
					i, e.Kind, model, e.Percent, e.ResetsAt)
			}
		}
	}

	per, _ := parse(body)
	fmt.Fprintf(&sb, "\nparsed per-model: %d\n", len(per))
	for _, m := range per {
		fmt.Fprintf(&sb, "  %-10s %.3f%% resets_at=%d\n", m.Name, m.Percent, m.ResetsAt)
	}
	fmt.Fprintf(&sb, "detected plan (local): %q\n", planFromLocal())
	return sb.String(), nil
}

func jsonKind(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return "empty"
	}
	switch s[0] {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "bool"
	case 'n':
		return "null"
	default:
		return "number"
	}
}

func looksLikeWindow(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	if len(s) == 0 || s[0] != '{' {
		return false
	}
	return strings.Contains(s, "utilization") || strings.Contains(s, "resets_at") || strings.Contains(s, "percent")
}

func isNumberKey(k string) bool {
	switch k {
	case "utilization", "resets_at", "percent", "remaining", "used", "limit":
		return true
	}
	return false
}
