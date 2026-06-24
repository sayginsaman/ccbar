// Command ccbar is a Claude Code statusLine program: it reads the statusline JSON
// on stdin and prints a single, modern-minimal info bar showing last-prompt
// tokens, API-equivalent cost (last + session), the 5h session limit, the weekly
// limit, and per-model ("all models") weekly limits.
//
// Subcommands:
//
//	ccbar                 render the bar (reads stdin) — this is what Claude Code calls
//	ccbar --refresh-usage refresh the cached per-model limits (run detached internally)
//	ccbar --init-config   write a default config file to ~/.claude/ccbar/config.json
//	ccbar --doctor        print diagnostics and test the usage endpoint
//	ccbar --demo          print a sample bar with synthetic data
//	ccbar --version       print the version
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/saygindoruksaman/ccbar/internal/config"
	"github.com/saygindoruksaman/ccbar/internal/install"
	"github.com/saygindoruksaman/ccbar/internal/payload"
	"github.com/saygindoruksaman/ccbar/internal/pricing"
	"github.com/saygindoruksaman/ccbar/internal/render"
	"github.com/saygindoruksaman/ccbar/internal/usage"
)

// version is overridable at build time via -ldflags "-X main.version=...".
var version = "1.0.0"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--refresh-usage":
			usage.Refresh(config.Load())
			return
		case "install":
			cmdInstall(os.Args[2:])
			return
		case "uninstall":
			cmdUninstall(os.Args[2:])
			return
		case "--init-config":
			cmdInitConfig()
			return
		case "--doctor":
			cmdDoctor()
			return
		case "--dump-usage":
			s, err := usage.DebugStructure(config.Load())
			if err != nil {
				fmt.Fprintln(os.Stderr, "ccbar:", err)
				os.Exit(1)
			}
			fmt.Print(s)
			return
		case "--demo":
			cmdDemo()
			return
		case "--version", "-v":
			fmt.Println("ccbar", version)
			return
		case "-h", "--help":
			fmt.Println(helpText)
			return
		}
	}
	cmdRender()
}

// cmdRender is the hot path. It must always exit 0 and print at most one line; on
// any problem it prints nothing (which blanks the status line) rather than erroring.
func cmdRender() {
	// Cap the read: the statusline payload is always small, and this keeps a
	// pathological producer from exhausting memory on the hot path.
	b, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	if err != nil || len(b) == 0 {
		return
	}
	p, err := payload.Parse(b)
	if err != nil {
		return
	}
	cfg := config.Load()
	now := time.Now()
	in := render.Inputs{
		Payload: p,
		Usage:   usage.Load(cfg, now),
		Prices:  pricing.Load(config.Dir()),
		Config:  cfg,
		Width:   render.TermWidth(),
		Now:     now,
		NoColor: !cfg.Color || os.Getenv("NO_COLOR") != "",
	}
	if line := render.Build(in); line != "" {
		fmt.Print(line)
	}
}

func cmdInstall(args []string) {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	ri := fs.Int("refresh-interval", 30, "status line refresh interval in seconds (0 to update on events only)")
	_ = fs.Parse(args)

	res, err := install.Register(*ri)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ccbar: install failed:", err)
		os.Exit(1)
	}
	_, _, _ = config.WriteDefault() // best-effort default config

	fmt.Println("✓ ccbar is now your Claude Code status line")
	fmt.Println("  settings:", res.Settings)
	if res.Backup != "" {
		fmt.Println("  backup:  ", res.Backup)
	}
	fmt.Println("  command: ", res.Command)
	if res.PrevCommand != "" && res.PrevCommand != res.Command {
		fmt.Println("  replaced:", res.PrevCommand)
	}
	fmt.Println("It appears on your next interaction — no restart needed. Verify with: ccbar --doctor")
}

func cmdUninstall(args []string) {
	fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
	purge := fs.Bool("purge", false, "also delete the ccbar data dir (binary, config, cache)")
	_ = fs.Parse(args)

	res, err := install.Unregister(*purge)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ccbar: uninstall failed:", err)
		os.Exit(1)
	}
	fmt.Println("✓ removed ccbar from your Claude Code status line")
	if res.Backup != "" {
		fmt.Println("  backup:", res.Backup)
	}
	if res.Purged != "" {
		fmt.Println("  purged:", res.Purged)
	}
}

func cmdInitConfig() {
	p, created, err := config.WriteDefault()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ccbar: could not write config:", err)
		os.Exit(1)
	}
	if created {
		fmt.Println("wrote default config:", p)
	} else {
		fmt.Println("config already exists (left unchanged):", p)
	}
}

func cmdDemo() {
	cu := payload.Usage{InputTokens: 2, OutputTokens: 769, CacheCreationInputTokens: 34588, CacheReadInputTokens: 19807}
	used5 := 23.5
	p := &payload.Payload{
		Model: payload.Model{ID: "claude-opus-4-8", DisplayName: "Opus 4.8"},
		Cost:  payload.Cost{TotalCostUSD: 1.84},
		ContextWindow: &payload.ContextWindow{
			ContextWindowSize: 1_000_000,
			UsedPercentage:    &used5,
			CurrentUsage:      &cu,
		},
		RateLimits: &payload.RateLimits{
			FiveHour: &payload.Window{UsedPercentage: 23, ResetsAt: time.Now().Add(2 * time.Hour).Unix()},
			SevenDay: &payload.Window{UsedPercentage: 41, ResetsAt: time.Now().Add(72 * time.Hour).Unix()},
		},
	}
	cfg := config.Default()
	in := render.Inputs{
		Payload: p,
		Usage: usage.Result{PerModel: []usage.PerModel{
			{Name: "Opus", Percent: 60}, {Name: "Sonnet", Percent: 12},
		}, Plan: "Max 20x", Have: true},
		Prices:  pricing.Load(config.Dir()),
		Config:  cfg,
		Width:   render.TermWidth(),
		Now:     time.Now(),
		NoColor: os.Getenv("NO_COLOR") != "",
	}
	fmt.Println(render.Build(in))
}

func cmdDoctor() {
	cfg := config.Load()
	fmt.Println("ccbar", version, "— doctor")
	fmt.Println("config:        ", config.Path())
	if _, err := os.Stat(config.Path()); err == nil {
		fmt.Println("config file:    present")
	} else {
		fmt.Println("config file:    absent (using built-in defaults)")
	}
	w := render.TermWidth()
	fmt.Printf("COLUMNS:        %d (0 = not set in this shell; Claude Code sets it)\n", w)
	fmt.Println("usage_endpoint:", cfg.UsageEndpoint)

	st := usage.Status(cfg)
	fmt.Printf("oauth token:    present=%v expired=%v source=%q\n", st.Present, st.Expired, st.Source)
	if !st.ExpiresAt.IsZero() {
		fmt.Println("token expires: ", st.ExpiresAt.Local().Format(time.RFC3339))
	}

	if cfg.UsageEndpoint {
		fmt.Println("\ntesting GET /api/oauth/usage (read-only)…")
		c, _ := usage.Refresh(cfg)
		if c != nil && c.OK {
			fmt.Printf("  ok: %d per-model window(s)", len(c.PerModel))
			if c.Plan != "" {
				fmt.Printf(", plan=%q", c.Plan)
			}
			fmt.Println()
			for _, m := range c.PerModel {
				fmt.Printf("    %-10s %.0f%%\n", m.Name, m.Percent)
			}
		} else {
			fmt.Println("  no data (token missing/expired, offline, or endpoint changed) — per-model segments will hide")
		}
	}
}

const helpText = `ccbar — Claude Code info bar (statusLine program)

usage:
  ccbar                 render the bar (reads the statusline JSON on stdin)
  ccbar install         register ccbar as your Claude Code status line (edits settings.json, with backup)
  ccbar uninstall       remove ccbar from settings.json (use --purge to also delete the data dir)
  ccbar --init-config   write a default config to ~/.claude/ccbar/config.json
  ccbar --doctor        print diagnostics and test the usage endpoint
  ccbar --demo          print a sample bar
  ccbar --version       print version`
