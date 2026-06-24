package pricing

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/saygindoruksaman/ccbar/internal/payload"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestCostOpus48(t *testing.T) {
	tbl := Load(t.TempDir())
	u := payload.Usage{InputTokens: 2, OutputTokens: 769, CacheCreationInputTokens: 34588, CacheReadInputTokens: 19807}
	got, ok := tbl.Cost("claude-opus-4-8", false, u)
	if !ok {
		t.Fatal("expected opus-4-8 to be known")
	}
	// (2*5 + 769*25 + 34588*6.25 + 19807*0.5) / 1e6
	want := (2*5.0 + 769*25.0 + 34588*6.25 + 19807*0.5) / 1e6
	if !approx(got, want) {
		t.Fatalf("cost = %v, want %v", got, want)
	}
}

func TestCostFastMode(t *testing.T) {
	tbl := Load(t.TempDir())
	u := payload.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000}
	got, ok := tbl.Cost("claude-opus-4-8", true, u)
	if !ok {
		t.Fatal("known")
	}
	// fast opus-4-8 = 10 in / 50 out
	if !approx(got, 60.0) {
		t.Fatalf("fast cost = %v, want 60", got)
	}
}

func TestModelIDVariants(t *testing.T) {
	tbl := Load(t.TempDir())
	for _, id := range []string{
		"claude-opus-4-8",
		"claude-opus-4-8[1m]",
		"us.anthropic.claude-opus-4-8-v1",
		"CLAUDE-OPUS-4-8",
	} {
		if !tbl.Known(id) {
			t.Errorf("expected %q to resolve to a rate card", id)
		}
	}
}

func TestUnknownModel(t *testing.T) {
	tbl := Load(t.TempDir())
	if _, ok := tbl.Cost("claude-zzz-9", false, payload.Usage{OutputTokens: 100}); ok {
		t.Fatal("expected unknown model to be unpriced")
	}
}

func TestLongestMatchWins(t *testing.T) {
	tbl := Load(t.TempDir())
	// sonnet-4-6 must not be shadowed by a shorter accidental key
	got, ok := tbl.Cost("claude-sonnet-4-6", false, payload.Usage{OutputTokens: 1_000_000})
	if !ok || !approx(got, 15.0) {
		t.Fatalf("sonnet output cost = %v ok=%v, want 15", got, ok)
	}
}

func TestPricingOverride(t *testing.T) {
	dir := t.TempDir()
	override := `{"opus-4-8":{"input":999,"output":1,"cache_write_5m":1,"cache_write_1h":1,"cache_read":1}}`
	if err := os.WriteFile(filepath.Join(dir, "pricing.json"), []byte(override), 0o644); err != nil {
		t.Fatal(err)
	}
	tbl := Load(dir)
	got, _ := tbl.Cost("claude-opus-4-8", false, payload.Usage{InputTokens: 1_000_000})
	if !approx(got, 999.0) {
		t.Fatalf("override input cost = %v, want 999", got)
	}
}
