// Package usage retrieves per-model weekly limits ("all models limits") that are
// NOT present in the statusLine stdin payload. The only programmatic source is the
// authenticated endpoint GET https://api.anthropic.com/api/oauth/usage — the same
// call Claude Code's /usage command makes.
//
// Design constraints, in order of importance:
//  1. Never block or slow the bar. The render path only ever reads a local cache;
//     a stale cache triggers a DETACHED background refresh (`ccbar --refresh-usage`)
//     and the current render uses whatever cache already exists.
//  2. Read-only on credentials. We read the existing OAuth access token but never
//     write it, never refresh it (that is Claude Code's job), and never log it.
//  3. Degrade gracefully. No token / offline / 401 / schema drift => the per-model
//     segments simply do not appear; everything else keeps working.
package usage

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/saygindoruksaman/ccbar/internal/config"
)

// httpClient gives the refresh a hard total-time backstop in addition to the
// per-request context timeout, and never follows the network onto the render path.
var httpClient = &http.Client{Timeout: httpTO}

const (
	endpoint   = "https://api.anthropic.com/api/oauth/usage"
	betaHeader = "oauth-2025-04-20"
	cacheFile  = "usage-cache.json"
	lockFile   = "refresh.lock"
	lockTTL    = 15 * time.Second
	httpTO     = 4 * time.Second
)

// PerModel is one model's weekly limit usage.
type PerModel struct {
	Name     string  `json:"name"`
	Percent  float64 `json:"percent"`
	ResetsAt int64   `json:"resets_at"`
}

// Cache is the on-disk snapshot the render path reads. It never contains secrets.
type Cache struct {
	FetchedAtMs int64      `json:"fetched_at_ms"`
	OK          bool       `json:"ok"`
	PerModel    []PerModel `json:"per_model"`
	Plan        string     `json:"plan,omitempty"`
}

// Result is what the render path consumes.
type Result struct {
	PerModel []PerModel
	Plan     string
	Stale    bool // cache older than TTL (a refresh has been kicked off)
	Have     bool // a cache existed at all
}

func cachePath() string { return filepath.Join(config.Dir(), cacheFile) }
func lockPath() string  { return filepath.Join(config.Dir(), lockFile) }

// Load returns cached per-model data for rendering and, when the cache is stale or
// missing and the endpoint is enabled, kicks off a detached background refresh.
// It performs NO network I/O itself.
func Load(cfg config.Config, now time.Time) Result {
	c, err := readCache()
	res := Result{}
	if err == nil {
		res.Have = true
		res.PerModel = filterModels(c.PerModel, cfg.Segments.PerModel)
		res.Plan = c.Plan
		age := now.Sub(time.UnixMilli(c.FetchedAtMs))
		res.Stale = age > time.Duration(cfg.CacheTTLSeconds)*time.Second
	} else {
		res.Stale = true
	}
	if cfg.UsageEndpoint && (res.Stale || !res.Have) {
		spawnBackgroundRefresh(now)
	}
	return res
}

func readCache() (*Cache, error) {
	b, err := os.ReadFile(cachePath())
	if err != nil {
		return nil, err
	}
	var c Cache
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// filterModels keeps only the requested model names, in the requested order
// (case-insensitive). An empty request keeps everything as returned.
func filterModels(all []PerModel, want []string) []PerModel {
	if len(want) == 0 {
		return all
	}
	out := make([]PerModel, 0, len(want))
	for _, w := range want {
		for _, m := range all {
			if strings.EqualFold(m.Name, w) {
				out = append(out, m)
				break
			}
		}
	}
	return out
}

// spawnBackgroundRefresh starts `ccbar --refresh-usage` detached, but only if it
// can atomically acquire the lock — so a burst of concurrent renders produces at
// most one refresh per lockTTL. The lock doubles as the "last attempt" timestamp
// (it is never deleted; it expires by mtime), which avoids lock-theft races.
func spawnBackgroundRefresh(now time.Time) {
	if !acquireLock(now) {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exe, "--refresh-usage")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	cmd.SysProcAttr = detachSysProcAttr() // fully detach (unix); nil elsewhere
	if err := cmd.Start(); err == nil && cmd.Process != nil {
		_ = cmd.Process.Release()
	}
}

// acquireLock returns true iff this caller wins the right to start a refresh. It
// creates the lock atomically (O_EXCL); if a lock already exists it is honored
// only while fresh (mtime within lockTTL and not future-dated, guarding against
// clock skew). A stale/orphaned lock is reclaimed. Any filesystem error fails
// CLOSED (no spawn) so a broken cache dir can never cause a stampede.
func acquireLock(now time.Time) bool {
	if err := os.MkdirAll(config.Dir(), 0o755); err != nil {
		return false
	}
	if tryCreateLock() {
		return true
	}
	fi, err := os.Stat(lockPath())
	if err != nil {
		return tryCreateLock() // vanished between create and stat
	}
	age := now.Sub(fi.ModTime())
	// Treat the lock as owned while its age is within ±lockTTL: this covers normal
	// throttling and tolerates sub-second mtime jitter without spamming reclaims.
	// Only a clearly stale (age >= lockTTL) or far-future (age <= -lockTTL, i.e. a
	// large backward clock jump) lock is reclaimed.
	if age > -lockTTL && age < lockTTL {
		return false
	}
	_ = os.Remove(lockPath())
	return tryCreateLock()
}

func tryCreateLock() bool {
	f, err := os.OpenFile(lockPath(), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return false
	}
	_, _ = f.WriteString(strconv.Itoa(os.Getpid()))
	_ = f.Close()
	return true
}

// Refresh performs the authenticated fetch and writes the cache. It is the body of
// `ccbar --refresh-usage` and is also reused by --doctor. It never returns an error
// that should crash anything; failures preserve the previous cache values.
func Refresh(cfg config.Config) (*Cache, error) {
	// The lock is intentionally NOT removed here: it is the "last attempt"
	// timestamp and expires by mtime (lockTTL), which avoids a fast refresh
	// deleting a newer caller's lock.
	prev, _ := readCache()
	if !cfg.UsageEndpoint {
		return prev, nil
	}

	tok, ok := readToken(cfg)
	if !ok {
		return writeFailure(prev), nil
	}

	body, ok := fetch(tok)
	if !ok {
		return writeFailure(prev), nil
	}

	per, _ := parse(body)
	c := &Cache{
		FetchedAtMs: time.Now().UnixMilli(),
		OK:          true,
		PerModel:    per,
		Plan:        planFromLocal(),
	}
	_ = writeCache(c)
	return c, nil
}

// writeFailure re-stamps the cache so we don't retry until the next TTL, while
// preserving any previously fetched (slow-moving weekly) values.
func writeFailure(prev *Cache) *Cache {
	c := &Cache{FetchedAtMs: time.Now().UnixMilli(), OK: false}
	if prev != nil {
		c.PerModel = prev.PerModel
		c.Plan = prev.Plan
	}
	_ = writeCache(c)
	return c
}

func writeCache(c *Cache) error {
	if err := os.MkdirAll(config.Dir(), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	tmp := cachePath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, cachePath())
}

func fetch(token string) ([]byte, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), httpTO)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, false
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", betaHeader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ccbar")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	b, err := readAllLimited(resp.Body, 1<<20)
	if err != nil {
		return nil, false
	}
	return b, true
}
