// Package install registers (and removes) ccbar as the Claude Code statusLine in
// ~/.claude/settings.json. Doing this in Go — rather than a shell/python snippet —
// means every distribution channel (curl|sh, Homebrew, go install) converges on a
// single, dependency-free `ccbar install` step, and the edit preserves all other
// settings keys with a timestamped backup.
package install

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sayginsaman/ccbar/internal/config"
)

// Result describes what an install/uninstall did, for user-facing output.
type Result struct {
	Settings    string // settings.json path
	Backup      string // backup path, if one was written
	Command     string // the registered statusLine command (install only)
	PrevCommand string // a replaced previous command, if any (install only)
	Purged      string // config dir removed (uninstall --purge only)
}

func claudeDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".claude"
	}
	return filepath.Join(home, ".claude")
}

// SettingsPath is ~/.claude/settings.json (honoring CLAUDE_CONFIG_DIR).
func SettingsPath() string { return filepath.Join(claudeDir(), "settings.json") }

// StableBinPath returns the absolute path to register for the running binary. It
// prefers a stable Homebrew shim (<prefix>/bin/ccbar) over a version-pinned Cellar
// path so the status line keeps working across `brew upgrade`.
func StableBinPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if abs, err := filepath.Abs(exe); err == nil {
		exe = abs
	}
	if i := strings.Index(exe, "/Cellar/"); i >= 0 {
		shim := exe[:i] + "/bin/ccbar"
		if _, err := os.Stat(shim); err == nil {
			return shim, nil
		}
	}
	return exe, nil
}

// Register writes ccbar's statusLine into settings.json (creating the file if
// needed), preserving all other keys and backing up any existing file.
func Register(refreshInterval int) (*Result, error) {
	bin, err := StableBinPath()
	if err != nil {
		return nil, err
	}
	m, err := loadSettings()
	if err != nil {
		return nil, err
	}
	res := &Result{Settings: SettingsPath(), Command: bin}

	if prev, ok := m["statusLine"].(map[string]any); ok {
		if c, ok := prev["command"].(string); ok {
			res.PrevCommand = c
		}
	}
	if bk, err := backupIfExists(); err != nil {
		return nil, err
	} else {
		res.Backup = bk
	}

	sl := map[string]any{"type": "command", "command": bin, "padding": 0}
	if refreshInterval > 0 {
		sl["refreshInterval"] = refreshInterval
	}
	m["statusLine"] = sl
	if err := writeSettings(m); err != nil {
		return nil, err
	}
	return res, nil
}

// Unregister removes ccbar's statusLine from settings.json (backing it up). With
// purge, it also deletes the ccbar data directory.
func Unregister(purge bool) (*Result, error) {
	m, err := loadSettings()
	if err != nil {
		return nil, err
	}
	res := &Result{Settings: SettingsPath()}
	if _, ok := m["statusLine"]; ok {
		if bk, err := backupIfExists(); err != nil {
			return nil, err
		} else {
			res.Backup = bk
		}
		delete(m, "statusLine")
		if err := writeSettings(m); err != nil {
			return nil, err
		}
	}
	if purge {
		if err := os.RemoveAll(config.Dir()); err == nil {
			res.Purged = config.Dir()
		}
	}
	return res, nil
}

func loadSettings() (map[string]any, error) {
	data, err := os.ReadFile(SettingsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON (refusing to overwrite): %w", SettingsPath(), err)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

func writeSettings(m map[string]any) error {
	if err := os.MkdirAll(claudeDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := SettingsPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, SettingsPath())
}

func backupIfExists() (string, error) {
	data, err := os.ReadFile(SettingsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	bk := fmt.Sprintf("%s.bak.%d", SettingsPath(), time.Now().Unix())
	if err := os.WriteFile(bk, data, 0o644); err != nil {
		return "", err
	}
	return bk, nil
}
