// Package pricing converts a transcript/stdin token usage breakdown into an
// API-equivalent USD cost. Rates are USD per million tokens and were cross-checked
// against the bundled claude-api skill and the official pricing page (2026-06-24).
// They can be overridden without recompiling via ~/.claude/ccbar/pricing.json.
package pricing

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/sayginsaman/ccbar/internal/payload"
)

// Rates are USD per million tokens for one model. CacheWrite5m is the default
// cache-write rate used for a plain usage object (which does not split 5m vs 1h).
// Fast* are flat fast-mode input/output rates; when set and fast mode is active
// they replace Input/Output and the cache rates are derived from FastInput.
type Rates struct {
	Input        float64 `json:"input"`
	Output       float64 `json:"output"`
	CacheWrite5m float64 `json:"cache_write_5m"`
	CacheWrite1h float64 `json:"cache_write_1h"`
	CacheRead    float64 `json:"cache_read"`
	FastInput    float64 `json:"fast_input,omitempty"`
	FastOutput   float64 `json:"fast_output,omitempty"`
}

// defaults are keyed by a canonical model fragment. Model ids are matched by
// substring against these keys (longest match wins) so that ids like
// "claude-opus-4-8", "claude-opus-4-8[1m]", or "us.anthropic.claude-opus-4-8"
// all resolve correctly.
var defaults = map[string]Rates{
	"opus-4-8":   {Input: 5, Output: 25, CacheWrite5m: 6.25, CacheWrite1h: 10, CacheRead: 0.5, FastInput: 10, FastOutput: 50},
	"opus-4-7":   {Input: 5, Output: 25, CacheWrite5m: 6.25, CacheWrite1h: 10, CacheRead: 0.5, FastInput: 30, FastOutput: 150},
	"opus-4-6":   {Input: 5, Output: 25, CacheWrite5m: 6.25, CacheWrite1h: 10, CacheRead: 0.5, FastInput: 30, FastOutput: 150},
	"sonnet-4-6": {Input: 3, Output: 15, CacheWrite5m: 3.75, CacheWrite1h: 6, CacheRead: 0.3},
	"haiku-4-5":  {Input: 1, Output: 5, CacheWrite5m: 1.25, CacheWrite1h: 2, CacheRead: 0.1},
	"fable-5":    {Input: 10, Output: 50, CacheWrite5m: 12.5, CacheWrite1h: 20, CacheRead: 1},
	"mythos-5":   {Input: 10, Output: 50, CacheWrite5m: 12.5, CacheWrite1h: 20, CacheRead: 1},
}

// Table is a resolved rate table (defaults overlaid with any user override).
type Table struct {
	rates map[string]Rates
}

// Load returns the default table merged with ~/.claude/ccbar/pricing.json if present.
func Load(dir string) *Table {
	t := &Table{rates: make(map[string]Rates, len(defaults))}
	for k, v := range defaults {
		t.rates[k] = v
	}
	b, err := os.ReadFile(filepath.Join(dir, "pricing.json"))
	if err == nil {
		var override map[string]Rates
		if json.Unmarshal(b, &override) == nil {
			for k, v := range override {
				t.rates[normalizeKey(k)] = v
			}
		}
	}
	return t
}

func normalizeKey(k string) string {
	k = strings.ToLower(k)
	k = strings.TrimPrefix(k, "claude-")
	return k
}

// lookup finds the rates for a model id by longest substring match.
func (t *Table) lookup(modelID string) (Rates, bool) {
	id := strings.ToLower(modelID)
	var best string
	for key := range t.rates {
		if strings.Contains(id, key) && len(key) > len(best) {
			best = key
		}
	}
	if best == "" {
		return Rates{}, false
	}
	return t.rates[best], true
}

// Cost returns the API-equivalent USD cost of one usage breakdown for the given
// model, and whether the model was known. When fast is true and fast rates exist,
// the flat fast-mode rates apply (cache rates derived from the fast input rate).
func (t *Table) Cost(modelID string, fast bool, u payload.Usage) (float64, bool) {
	r, ok := t.lookup(modelID)
	if !ok {
		return 0, false
	}
	inRate, outRate := r.Input, r.Output
	writeRate, readRate := r.CacheWrite5m, r.CacheRead
	if fast && r.FastInput > 0 {
		inRate, outRate = r.FastInput, r.FastOutput
		writeRate = r.FastInput * 1.25
		readRate = r.FastInput * 0.1
	}
	cost := float64(u.InputTokens)*inRate +
		float64(u.OutputTokens)*outRate +
		float64(u.CacheCreationInputTokens)*writeRate +
		float64(u.CacheReadInputTokens)*readRate
	return cost / 1_000_000, true
}

// Known reports whether the model id resolves to a rate card.
func (t *Table) Known(modelID string) bool {
	_, ok := t.lookup(modelID)
	return ok
}
