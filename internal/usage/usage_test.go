package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sayginsaman/ccbar/internal/config"
)

// realResponse mirrors the actual /api/oauth/usage body shape observed live:
// utilization is already 0–100, resets_at is an RFC3339 string, seven_day_opus is
// null, seven_day_sonnet is an object, and limits[] is empty.
const realResponse = `{
  "five_hour": {"utilization": 8.0, "resets_at": "2026-06-25T01:19:59.965202+00:00"},
  "seven_day": {"utilization": 19.0, "resets_at": "2026-06-26T17:59:59.965220+00:00"},
  "seven_day_opus": null,
  "seven_day_sonnet": {"utilization": 0.0, "resets_at": "2026-06-26T17:59:59.965227+00:00"},
  "limits": []
}`

func TestParseRealResponse(t *testing.T) {
	per, _ := parse([]byte(realResponse))
	if len(per) != 1 {
		t.Fatalf("expected 1 per-model window (Sonnet), got %d: %+v", len(per), per)
	}
	if per[0].Name != "Sonnet" || per[0].Percent != 0 {
		t.Fatalf("unexpected per-model: %+v", per[0])
	}
	want := time.Date(2026, 6, 26, 17, 59, 59, 0, time.UTC).Unix()
	if per[0].ResetsAt != want {
		t.Fatalf("resets_at = %d, want %d", per[0].ResetsAt, want)
	}
}

func TestParseUtilizationNotRescaled(t *testing.T) {
	// 19.0 must stay 19%, never become 1900% (the old util*100 bug).
	per, _ := parse([]byte(`{"seven_day_opus":{"utilization":19.0,"resets_at":""}}`))
	if len(per) != 1 || per[0].Percent != 19 {
		t.Fatalf("got %+v, want Opus 19%%", per)
	}
}

func TestParseLimitsArray(t *testing.T) {
	body := `{"limits":[
	  {"kind":"weekly_scoped","percent":42.5,"resets_at":"2026-06-26T00:00:00Z","scope":{"model":{"display_name":"Haiku"}}},
	  {"kind":"five_hour","percent":1,"resets_at":"","scope":{}}
	]}`
	per, _ := parse([]byte(body))
	if len(per) != 1 || per[0].Name != "Haiku" || per[0].Percent != 42.5 {
		t.Fatalf("expected Haiku 42.5%% from limits[], got %+v", per)
	}
}

func TestParseLimitsOverrideTopLevel(t *testing.T) {
	body := `{"seven_day_opus":{"utilization":10,"resets_at":""},
	  "limits":[{"kind":"weekly_scoped","percent":55,"resets_at":"","scope":{"model":{"display_name":"Opus"}}}]}`
	per, _ := parse([]byte(body))
	if len(per) != 1 || per[0].Name != "Opus" || per[0].Percent != 55 {
		t.Fatalf("limits[] should override seven_day_opus; got %+v", per)
	}
}

func TestParseResetTime(t *testing.T) {
	cases := map[string]bool{ // input -> expect non-zero
		"2026-06-25T01:19:59.965202+00:00": true,
		"2026-06-26T00:00:00Z":             true,
		"":                                 false,
		"garbage":                          false,
	}
	for in, wantNonZero := range cases {
		got := parseResetTime(in)
		if (got != 0) != wantNonZero {
			t.Errorf("parseResetTime(%q) = %d, wantNonZero=%v", in, got, wantNonZero)
		}
	}
}

func TestFilterModels(t *testing.T) {
	all := []PerModel{{Name: "Sonnet"}, {Name: "Opus"}, {Name: "Haiku"}}
	got := filterModels(all, []string{"Opus", "Sonnet"})
	if len(got) != 2 || got[0].Name != "Opus" || got[1].Name != "Sonnet" {
		t.Fatalf("filter should reorder to [Opus,Sonnet], got %+v", got)
	}
	// case-insensitive
	if got := filterModels(all, []string{"opus"}); len(got) != 1 || got[0].Name != "Opus" {
		t.Fatalf("case-insensitive match failed: %+v", got)
	}
	// empty want keeps all
	if got := filterModels(all, nil); len(got) != 3 {
		t.Fatalf("nil want should keep all, got %d", len(got))
	}
}

func TestPrettyTier(t *testing.T) {
	cases := map[string]string{
		"default_claude_max_20x": "Max 20x",
		"default_claude_max_5x":  "Max 5x",
		"claude_pro":             "Pro",
		"":                       "",
		"some_new_tier":          "Some New Tier",
	}
	for in, want := range cases {
		if got := PrettyTier(in); got != want {
			t.Errorf("PrettyTier(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCacheRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := &Cache{FetchedAtMs: time.Now().UnixMilli(), OK: true, Plan: "Max 20x",
		PerModel: []PerModel{{Name: "Opus", Percent: 60, ResetsAt: 123}}}
	if err := writeCache(c); err != nil {
		t.Fatal(err)
	}
	got, err := readCache()
	if err != nil {
		t.Fatal(err)
	}
	if got.Plan != "Max 20x" || len(got.PerModel) != 1 || got.PerModel[0].Percent != 60 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestAcquireLock(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	now := time.Now()

	// First caller wins.
	if !acquireLock(now) {
		t.Fatal("first acquireLock should win")
	}
	// A second caller within lockTTL is throttled out.
	if acquireLock(now) {
		t.Fatal("second acquireLock within lockTTL should fail")
	}
	// After the lock goes stale, a caller can reclaim it.
	if !acquireLock(now.Add(2 * lockTTL)) {
		t.Fatal("acquireLock should reclaim a stale lock")
	}
	// A future-dated lock (clock skew) is also treated as reclaimable.
	if err := os.Chtimes(lockPath(), now.Add(time.Hour), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if !acquireLock(now) {
		t.Fatal("acquireLock should reclaim a future-dated lock")
	}
}

func TestLoadStaleSpawnsAndUsesCache(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// pre-seed a stale cache; usage_endpoint off so no background spawn occurs
	old := time.Now().Add(-1 * time.Hour).UnixMilli()
	_ = os.MkdirAll(filepath.Join(home, ".claude", "ccbar"), 0o755)
	_ = writeCache(&Cache{FetchedAtMs: old, OK: true, PerModel: []PerModel{{Name: "Opus", Percent: 5}}})

	cfg := config.Default()
	cfg.UsageEndpoint = false // do not hit the network in tests
	res := Load(cfg, time.Now())
	if !res.Have || res.Stale != true {
		t.Fatalf("expected Have && Stale, got %+v", res)
	}
	if len(res.PerModel) != 1 || res.PerModel[0].Name != "Opus" {
		t.Fatalf("expected cached Opus to be used even when stale, got %+v", res.PerModel)
	}
}
