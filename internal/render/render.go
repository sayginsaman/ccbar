// Package render turns the statusline payload plus cached per-model usage into a
// single, width-aware line. Color is used only to signal limit pressure; when the
// terminal is too narrow, the lowest-priority segments are dropped (never wrapped).
package render

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sayginsaman/ccbar/internal/config"
	"github.com/sayginsaman/ccbar/internal/payload"
	"github.com/sayginsaman/ccbar/internal/pricing"
	"github.com/sayginsaman/ccbar/internal/usage"
)

const (
	ansiReset = "\x1b[0m"
	ansiDim   = "\x1b[2m"
	ansiGreen = "\x1b[32m"
	ansiAmber = "\x1b[33m"
	ansiRed   = "\x1b[31m"
)

// drop ranks: lower = kept longer. Tokens + 5h are the last to go.
const (
	rankTokens      = 10
	rankFiveHour    = 15
	rankCostLast    = 20
	rankSevenDay    = 30
	rankCostSession = 40
	rankPlan        = 45
	rankModel       = 50
	rankPerModel    = 60 // + index
	rankContext     = 80
)

type seg struct {
	plain    string // uncolored, for width math
	rendered string // possibly colored
	rank     int
}

// Inputs bundles everything the renderer needs (keeps Build testable: callers
// inject width and now).
type Inputs struct {
	Payload *payload.Payload
	Usage   usage.Result
	Prices  *pricing.Table
	Config  config.Config
	Width   int       // terminal columns; <=0 means unknown (defaults applied)
	Now     time.Time //
	NoColor bool      //
}

// Build returns the final status line (no trailing newline).
func Build(in Inputs) string {
	segs := buildSegments(in)
	if len(segs) == 0 {
		return ""
	}
	max := in.Width
	if max <= 0 {
		max = 80
	}
	max -= 2 // safety margin against Claude Code's own right-side badges
	fit := fitSegments(segs, max)

	sep := dim(" · ", in.NoColor)
	if in.Config.ASCII {
		sep = dim(" | ", in.NoColor)
	}
	parts := make([]string, len(fit))
	for i, s := range fit {
		parts[i] = s.rendered
	}
	line := strings.Join(parts, sep)
	// Last-resort guarantee: even a single segment wider than the budget (or a
	// width miscount) can never overflow the one-line region.
	if displayWidth(line) > max {
		line = truncateToWidth(line, max)
	}
	return line
}

func buildSegments(in Inputs) []seg {
	p := in.Payload
	cfg := in.Config
	nc := in.NoColor
	pick := func(full, short string) string {
		if cfg.CompactLabels {
			return short
		}
		return full
	}
	var out []seg

	if cfg.ShowPlan {
		if label := sanitize(planLabel(cfg, in.Usage.Plan)); label != "" {
			out = append(out, plainSeg(rankPlan, dim(label, nc), label))
		}
	}

	if cfg.Segments.Model {
		if name := sanitize(p.Model.DisplayName); name != "" {
			out = append(out, metricSeg(rankModel, name, "", ansiGreen, nc))
		}
	}

	// Last-prompt tokens (from context_window.current_usage).
	if cfg.Segments.Tokens && p.ContextWindow != nil && p.ContextWindow.CurrentUsage != nil {
		t := p.ContextWindow.CurrentUsage.Total()
		out = append(out, metricSeg(rankTokens, formatTokens(t), pick(" tokens", " tok"), ansiGreen, nc))
	}

	// Cost (API-equivalent). Last prompt is computed from the rate card; session
	// total comes from Claude Code's own estimate.
	if cfg.Segments.CostLast && p.ContextWindow != nil && p.ContextWindow.CurrentUsage != nil {
		if c, ok := in.Prices.Cost(p.Model.ID, p.FastMode, *p.ContextWindow.CurrentUsage); ok {
			out = append(out, metricSeg(rankCostLast, formatMoney(c), pick(" last prompt", ""), ansiGreen, nc))
		}
	}
	if cfg.Segments.CostSession && p.Cost.TotalCostUSD > 0 {
		out = append(out, metricSeg(rankCostSession, formatMoney(p.Cost.TotalCostUSD), pick(" session", " ses"), ansiGreen, nc))
	}

	// Session (5h) and weekly (7d) limits from stdin rate_limits.
	if cfg.Segments.FiveHour && p.RateLimits != nil && p.RateLimits.FiveHour != nil {
		out = append(out, limitSeg(rankFiveHour, pick("session limit", "5h"), p.RateLimits.FiveHour.UsedPercentage, p.RateLimits.FiveHour.ResetsAt, in))
	}
	if cfg.Segments.SevenDay && p.RateLimits != nil && p.RateLimits.SevenDay != nil {
		out = append(out, limitSeg(rankSevenDay, pick("weekly limit", "wk"), p.RateLimits.SevenDay.UsedPercentage, p.RateLimits.SevenDay.ResetsAt, in))
	}

	// Per-model weekly limits ("all models limits") from the cached usage endpoint.
	for i, m := range in.Usage.PerModel {
		if cfg.HideZeroPerModel && m.Percent <= 0 {
			continue
		}
		name := sanitize(m.Name)
		out = append(out, limitSeg(rankPerModel+i, pick(name+" weekly", name), m.Percent, m.ResetsAt, in))
	}

	// Context-window fill (optional, lowest priority).
	if cfg.Segments.Context && p.ContextWindow != nil && p.ContextWindow.UsedPercentage != nil {
		label := pick("context ", "ctx ")
		value := formatPct(*p.ContextWindow.UsedPercentage)
		out = append(out, seg{plain: label + value, rank: rankContext, rendered: dim(label, nc) + colorize(value, ansiGreen, nc)})
	}

	return out
}

func plainSeg(rank int, rendered, plain string) seg {
	return seg{plain: plain, rendered: rendered, rank: rank}
}

// metricSeg renders a colored value followed by a dim descriptive label. The label
// carries its own leading space (e.g. " tokens"), or is "" for a bare value like
// the model name.
func metricSeg(rank int, value, label, valueColor string, nc bool) seg {
	rendered := colorize(value, valueColor, nc)
	if label != "" {
		rendered += dim(label, nc)
	}
	return seg{plain: value + label, rendered: rendered, rank: rank}
}

// limitSeg renders a "<label> <pct>%" segment with a dim label and a value colored
// by pressure (green / amber / red), optionally appending a reset countdown when hot.
func limitSeg(rank int, label string, pct float64, resetsAt int64, in Inputs) seg {
	nc := in.NoColor
	value := formatPct(pct)
	plain := label + " " + value
	rendered := dim(label+" ", nc) + colorize(value, limitColor(pct, in.Config), nc)
	if in.Config.ShowResetsWhenHot && pct >= in.Config.WarnThreshold {
		if r := formatReset(resetsAt, in.Now); r != "" {
			marker := " ↻"
			if in.Config.ASCII {
				marker = " r"
			}
			plain += marker + r
			rendered += dim(marker+r, nc)
		}
	}
	return seg{plain: plain, rendered: rendered, rank: rank}
}

// fitSegments drops the highest-rank segments until the line fits within max.
func fitSegments(segs []seg, max int) []seg {
	kept := make([]seg, len(segs))
	copy(kept, segs)
	for len(kept) > 1 && lineWidth(kept) > max {
		worst := 0
		for i := 1; i < len(kept); i++ {
			if kept[i].rank > kept[worst].rank {
				worst = i
			}
		}
		kept = append(kept[:worst], kept[worst+1:]...)
	}
	return kept
}

// lineWidth is the display width of the joined segments including " · " separators.
func lineWidth(segs []seg) int {
	if len(segs) == 0 {
		return 0
	}
	w := 0
	for _, s := range segs {
		w += displayWidth(s.plain)
	}
	w += 3 * (len(segs) - 1) // each separator " · " is 3 columns
	return w
}

// limitColor maps a usage percentage to a color: green when there is headroom,
// amber as it gets high, red when nearly exhausted.
func limitColor(pct float64, cfg config.Config) string {
	switch {
	case pct >= cfg.CritThreshold:
		return ansiRed
	case pct >= cfg.WarnThreshold:
		return ansiAmber
	default:
		return ansiGreen
	}
}

func colorize(s, code string, noColor bool) string {
	if noColor || code == "" {
		return s
	}
	return code + s + ansiReset
}

func dim(s string, noColor bool) string {
	if noColor {
		return s
	}
	return ansiDim + s + ansiReset
}

func planLabel(cfg config.Config, detected string) string {
	if cfg.Plan != "" && cfg.Plan != "auto" {
		return cfg.Plan
	}
	return detected
}

// --- formatting helpers ---

func formatTokens(n int) string {
	switch {
	case n < 0:
		return "0"
	case n < 1000:
		return strconv.Itoa(n)
	case n < 999_950: // above this, %.1f rounds to "1000.0k" — promote to "1.00m"
		k := float64(n) / 1000
		if k == math.Trunc(k) {
			return fmt.Sprintf("%.0fk", k)
		}
		return fmt.Sprintf("%.1fk", k)
	default:
		return fmt.Sprintf("%.2fm", float64(n)/1_000_000)
	}
}

func formatMoney(v float64) string {
	switch {
	case v <= 0:
		return "$0.00"
	case v < 0.01:
		return "<$0.01"
	default:
		return fmt.Sprintf("$%.2f", v)
	}
}

func formatPct(p float64) string {
	if p < 0 {
		p = 0
	}
	if p > 999 { // guard against schema drift / corrupt values producing a giant segment
		p = 999
	}
	return fmt.Sprintf("%.0f%%", math.Round(p))
}

// formatReset returns a compact countdown like "2h" or "14m" until resetsAt, or
// "" if it is in the past / unknown.
func formatReset(resetsAt int64, now time.Time) string {
	if resetsAt <= 0 {
		return ""
	}
	d := time.Unix(resetsAt, 0).Sub(now)
	if d <= 0 {
		return ""
	}
	if d >= time.Hour {
		return fmt.Sprintf("%dh", int(math.Round(d.Hours())))
	}
	m := int(math.Ceil(d.Minutes()))
	if m >= 60 { // 59m59s ceils to 60 — show "1h", not "60m"
		return "1h"
	}
	return fmt.Sprintf("%dm", m)
}

// displayWidth counts visible columns, ignoring ANSI SGR escape sequences. The
// text style uses only single-width runes, so a rune count is exact.
func displayWidth(s string) int {
	w := 0
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b { // ESC: skip until 'm'
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		// advance by full rune
		if s[i] < 0x80 {
			w++
			continue
		}
		w++
		// skip UTF-8 continuation bytes
		for i+1 < len(s) && s[i+1]&0xC0 == 0x80 {
			i++
		}
	}
	return w
}

// sanitize removes C0 control characters (newline, CR, tab, ESC, …) and DEL from
// stdin/endpoint-derived text. This prevents a stray newline from wrapping the
// one-line bar, blocks ANSI injection via field values, and keeps displayWidth
// exact (only intentional escapes from colorize/dim remain).
func sanitize(s string) string {
	if s == "" {
		return s
	}
	clean := true
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			clean = false
			break
		}
	}
	if clean {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// truncateToWidth hard-trims a (possibly colored) string to at most max display
// columns, never cutting inside an ANSI escape, and re-closing color if it cut
// mid-sequence. This is the last-resort guarantee that the bar fits one line.
func truncateToWidth(s string, max int) string {
	if max <= 0 {
		return ""
	}
	var b strings.Builder
	w := 0
	truncated := false
	hadEsc := false
	for i := 0; i < len(s); {
		if s[i] == 0x1b { // copy the whole escape sequence verbatim
			start := i
			for i < len(s) && s[i] != 'm' {
				i++
			}
			if i < len(s) {
				i++
			}
			b.WriteString(s[start:i])
			hadEsc = true
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		if w+1 > max {
			truncated = true
			break
		}
		b.WriteString(s[i : i+size])
		w++
		i += size
	}
	out := b.String()
	if truncated && hadEsc {
		out += ansiReset
	}
	return out
}

// TermWidth reads the terminal width from COLUMNS (set by Claude Code). Returns 0
// when unknown.
func TermWidth() int {
	if c := os.Getenv("COLUMNS"); c != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(c)); err == nil {
			return n
		}
	}
	return 0
}
