package render

import (
	"strings"
	"testing"
	"time"

	"github.com/saygindoruksaman/ccbar/internal/config"
	"github.com/saygindoruksaman/ccbar/internal/payload"
	"github.com/saygindoruksaman/ccbar/internal/pricing"
	"github.com/saygindoruksaman/ccbar/internal/usage"
)

func samplePayload() *payload.Payload {
	pct := 8.0
	return &payload.Payload{
		Model: payload.Model{ID: "claude-opus-4-8", DisplayName: "Opus 4.8"},
		Cost:  payload.Cost{TotalCostUSD: 1.84},
		ContextWindow: &payload.ContextWindow{
			ContextWindowSize: 1_000_000,
			UsedPercentage:    &pct,
			CurrentUsage: &payload.Usage{
				InputTokens: 2, OutputTokens: 769,
				CacheCreationInputTokens: 34588, CacheReadInputTokens: 19807,
			},
		},
		RateLimits: &payload.RateLimits{
			FiveHour: &payload.Window{UsedPercentage: 23},
			SevenDay: &payload.Window{UsedPercentage: 41},
		},
	}
}

func baseInputs(p *payload.Payload, width int, noColor bool) Inputs {
	return Inputs{
		Payload: p,
		Usage: usage.Result{Have: true, PerModel: []usage.PerModel{
			{Name: "Opus", Percent: 60}, {Name: "Sonnet", Percent: 12},
		}},
		Prices:  pricing.Load(""),
		Config:  config.Default(),
		Width:   width,
		Now:     time.Now(),
		NoColor: noColor,
	}
}

func TestBuildFullWidthGolden(t *testing.T) {
	got := Build(baseInputs(samplePayload(), 200, true))
	want := "Opus 4.8 · 55.2k tokens · $0.25 last prompt · $1.84 session · session limit 23% · weekly limit 41% · Opus weekly 60% · Sonnet weekly 12%"
	if got != want {
		t.Fatalf("\n got: %q\nwant: %q", got, want)
	}
}

func TestBuildCompactLabelsGolden(t *testing.T) {
	in := baseInputs(samplePayload(), 200, true)
	in.Config.CompactLabels = true
	got := Build(in)
	want := "Opus 4.8 · 55.2k tok · $0.25 · $1.84 ses · 5h 23% · wk 41% · Opus 60% · Sonnet 12%"
	if got != want {
		t.Fatalf("\n got: %q\nwant: %q", got, want)
	}
}

func TestBuildNarrowDropsLowPriority(t *testing.T) {
	got := Build(baseInputs(samplePayload(), 45, true))
	want := "55.2k tokens · session limit 23%"
	if got != want {
		t.Fatalf("\n got: %q\nwant: %q", got, want)
	}
}

func TestBuildNeverWrapsVeryNarrow(t *testing.T) {
	// At several pathological widths the output must always stay on one line and
	// fit within the (width-2) budget.
	for _, width := range []int{40, 20, 12, 8, 6, 3, 1} {
		got := Build(baseInputs(samplePayload(), width, true))
		if strings.Contains(got, "\n") {
			t.Fatalf("width %d: output must be single line, got %q", width, got)
		}
		max := width - 2
		if max < 0 {
			max = 0
		}
		if w := displayWidth(got); w > max {
			t.Fatalf("width %d: visible width %d exceeds budget %d (got %q)", width, w, max, got)
		}
	}
}

func TestBuildTruncatesOversizedSingleSegment(t *testing.T) {
	// A single segment wider than the budget must be hard-truncated, not emitted whole.
	got := Build(baseInputs(samplePayload(), 6, true)) // max = 4
	if displayWidth(got) > 4 {
		t.Fatalf("oversized segment not truncated: %q (width %d)", got, displayWidth(got))
	}
}

func TestBuildSanitizesControlChars(t *testing.T) {
	p := samplePayload()
	p.Model.DisplayName = "Op\nus\t4.8" // newline + tab must never reach output
	got := Build(baseInputs(p, 200, true))
	if strings.ContainsAny(got, "\n\t") {
		t.Fatalf("control chars leaked into output: %q", got)
	}
	if !strings.Contains(got, "Opus4.8") {
		t.Fatalf("sanitized name should be Opus4.8, got %q", got)
	}
}

func TestBuildStripsAnsiInjection(t *testing.T) {
	p := samplePayload()
	p.Model.DisplayName = "\x1b[31mRED\x1b[0m" // attempt to inject color via stdin
	got := Build(baseInputs(p, 200, true))     // NoColor=true: no intentional escapes either
	if strings.Contains(got, "\x1b") {
		t.Fatalf("ESC byte must be stripped from stdin-derived text, got %q", got)
	}
}

func TestBuildHideZeroPerModel(t *testing.T) {
	in := baseInputs(samplePayload(), 200, true)
	in.Usage.PerModel = []usage.PerModel{{Name: "Opus", Percent: 60}, {Name: "Sonnet", Percent: 0}}
	in.Config.HideZeroPerModel = true
	got := Build(in)
	if strings.Contains(got, "Sonnet") {
		t.Fatalf("0%% per-model should be hidden, got %q", got)
	}
	if !strings.Contains(got, "Opus weekly 60%") {
		t.Fatalf("non-zero per-model should remain, got %q", got)
	}
}

func TestBuildColorsLimitsByPressure(t *testing.T) {
	p := samplePayload()
	p.RateLimits.FiveHour.UsedPercentage = 23 // green (healthy)
	p.RateLimits.SevenDay.UsedPercentage = 88 // amber
	in := baseInputs(p, 200, false)
	in.Usage.PerModel = []usage.PerModel{{Name: "Opus", Percent: 94}} // red
	got := Build(in)
	if !strings.Contains(got, ansiGreen+"23%"+ansiReset) {
		t.Errorf("expected green healthy value, got %q", got)
	}
	if !strings.Contains(got, ansiAmber+"88%"+ansiReset) {
		t.Errorf("expected amber value, got %q", got)
	}
	if !strings.Contains(got, ansiRed+"94%"+ansiReset) {
		t.Errorf("expected red value, got %q", got)
	}
	// labels stay dim, not colored
	if !strings.Contains(got, ansiDim+"session limit "+ansiReset) {
		t.Errorf("expected dim label, got %q", got)
	}
}

func TestBuildColorsValuesGreen(t *testing.T) {
	got := Build(baseInputs(samplePayload(), 200, false))
	if !strings.Contains(got, ansiGreen+"55.2k"+ansiReset) {
		t.Errorf("expected green token value, got %q", got)
	}
	if !strings.Contains(got, ansiGreen+"$0.25"+ansiReset) {
		t.Errorf("expected green cost value, got %q", got)
	}
}

func TestBuildNoRateLimits(t *testing.T) {
	p := samplePayload()
	p.RateLimits = nil
	got := Build(baseInputs(p, 200, true))
	if strings.Contains(got, "session limit") || strings.Contains(got, "weekly limit") {
		t.Fatalf("limits should be absent, got %q", got)
	}
	if !strings.Contains(got, "55.2k tokens") {
		t.Fatalf("tokens should still render, got %q", got)
	}
}

func TestBuildNoCurrentUsage(t *testing.T) {
	p := samplePayload()
	p.ContextWindow.CurrentUsage = nil
	got := Build(baseInputs(p, 200, true))
	if strings.Contains(got, "tokens") {
		t.Fatalf("tokens should be absent without current_usage, got %q", got)
	}
	if !strings.Contains(got, "$1.84 session") {
		t.Fatalf("session cost should still render, got %q", got)
	}
	if strings.Contains(got, "$0.25") {
		t.Fatalf("last-prompt cost should be absent, got %q", got)
	}
}

func TestBuildUnknownModelHidesLastCost(t *testing.T) {
	p := samplePayload()
	p.Model = payload.Model{ID: "claude-zzz-9", DisplayName: "Mystery"}
	got := Build(baseInputs(p, 200, true))
	if !strings.Contains(got, "$1.84 session") {
		t.Fatalf("session cost (from Claude Code) should render, got %q", got)
	}
	// the only "$" should be the session cost
	if strings.Count(got, "$") != 1 {
		t.Fatalf("expected exactly one cost segment, got %q", got)
	}
}

func TestBuildASCIISeparators(t *testing.T) {
	in := baseInputs(samplePayload(), 200, true)
	in.Config.ASCII = true
	got := Build(in)
	if !strings.Contains(got, " | ") || strings.Contains(got, " · ") {
		t.Fatalf("expected ASCII separators, got %q", got)
	}
}

func TestBuildEmptyWhenNothing(t *testing.T) {
	in := Inputs{Payload: &payload.Payload{}, Prices: pricing.Load(""), Config: config.Default(), Width: 200, Now: time.Now(), NoColor: true}
	if got := Build(in); got != "" {
		t.Fatalf("expected empty output, got %q", got)
	}
}

func TestFormatTokens(t *testing.T) {
	cases := map[int]string{-5: "0", 0: "0", 999: "999", 1000: "1k", 2000: "2k", 55166: "55.2k",
		999_949: "999.9k", 999_950: "1.00m", 999_999: "1.00m", 1_000_000: "1.00m", 1_234_567: "1.23m"}
	for in, want := range cases {
		if got := formatTokens(in); got != want {
			t.Errorf("formatTokens(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatMoney(t *testing.T) {
	cases := map[float64]string{0: "$0.00", 0.004: "<$0.01", 0.25: "$0.25", 1.8: "$1.80", 12.5: "$12.50"}
	for in, want := range cases {
		if got := formatMoney(in); got != want {
			t.Errorf("formatMoney(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatPct(t *testing.T) {
	cases := map[float64]string{0: "0%", 23.4: "23%", 23.6: "24%", -5: "0%", 1e308: "999%"}
	for in, want := range cases {
		if got := formatPct(in); got != want {
			t.Errorf("formatPct(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestDisplayWidthStripsANSI(t *testing.T) {
	if w := displayWidth(ansiAmber + "5h 88%" + ansiReset); w != 6 {
		t.Fatalf("width = %d, want 6", w)
	}
	if w := displayWidth("a · b"); w != 5 { // middot counts as one column
		t.Fatalf("width = %d, want 5", w)
	}
}

func TestFormatReset(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	if got := formatReset(now.Add(2*time.Hour).Unix(), now); got != "2h" {
		t.Errorf("2h reset = %q", got)
	}
	if got := formatReset(now.Add(30*time.Minute).Unix(), now); got != "30m" {
		t.Errorf("30m reset = %q", got)
	}
	if got := formatReset(now.Add(59*time.Minute+59*time.Second).Unix(), now); got != "1h" {
		t.Errorf("59m59s reset should round to 1h, got %q", got)
	}
	if got := formatReset(now.Add(-time.Hour).Unix(), now); got != "" {
		t.Errorf("past reset should be empty, got %q", got)
	}
	if got := formatReset(0, now); got != "" {
		t.Errorf("zero reset should be empty, got %q", got)
	}
}

func TestResetWhenHotAppended(t *testing.T) {
	in := baseInputs(samplePayload(), 200, true)
	in.Config.ShowResetsWhenHot = true
	in.Payload.RateLimits.FiveHour.UsedPercentage = 95
	in.Payload.RateLimits.FiveHour.ResetsAt = in.Now.Add(2 * time.Hour).Unix()
	got := Build(in)
	if !strings.Contains(got, "session limit 95% ↻2h") {
		t.Fatalf("expected reset countdown on hot limit, got %q", got)
	}
}
