package payload

import "testing"

func TestParseFull(t *testing.T) {
	in := `{
	  "model":{"id":"claude-opus-4-8","display_name":"Opus 4.8"},
	  "cost":{"total_cost_usd":1.84,"total_lines_added":10},
	  "fast_mode":false,
	  "context_window":{"context_window_size":1000000,"used_percentage":8,
	    "current_usage":{"input_tokens":2,"output_tokens":769,"cache_creation_input_tokens":34588,"cache_read_input_tokens":19807}},
	  "rate_limits":{"five_hour":{"used_percentage":23,"resets_at":1782496799},
	    "seven_day":{"used_percentage":41,"resets_at":1782496799}}
	}`
	p, err := Parse([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if p.Model.ID != "claude-opus-4-8" {
		t.Errorf("model id = %q", p.Model.ID)
	}
	if p.ContextWindow == nil || p.ContextWindow.CurrentUsage == nil {
		t.Fatal("expected current_usage")
	}
	if got := p.ContextWindow.CurrentUsage.Total(); got != 2+769+34588+19807 {
		t.Errorf("usage total = %d", got)
	}
	if p.RateLimits == nil || p.RateLimits.FiveHour == nil || p.RateLimits.FiveHour.UsedPercentage != 23 {
		t.Error("expected five_hour 23%")
	}
}

func TestParseMissingOptionals(t *testing.T) {
	p, err := Parse([]byte(`{"model":{"id":"x"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if p.ContextWindow != nil {
		t.Error("context_window should be nil")
	}
	if p.RateLimits != nil {
		t.Error("rate_limits should be nil")
	}
}

func TestParseNullCurrentUsage(t *testing.T) {
	p, err := Parse([]byte(`{"context_window":{"current_usage":null,"used_percentage":null}}`))
	if err != nil {
		t.Fatal(err)
	}
	if p.ContextWindow == nil {
		t.Fatal("context_window should be present")
	}
	if p.ContextWindow.CurrentUsage != nil {
		t.Error("current_usage should be nil")
	}
}

func TestParseMalformed(t *testing.T) {
	if _, err := Parse([]byte("not json")); err == nil {
		t.Error("expected error for malformed input")
	}
}
