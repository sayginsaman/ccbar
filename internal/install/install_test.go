package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func readSettings(t *testing.T) map[string]any {
	t.Helper()
	b, err := os.ReadFile(SettingsPath())
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestRegisterCreatesStatusLine(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	res, err := Register(30)
	if err != nil {
		t.Fatal(err)
	}
	m := readSettings(t)
	sl, ok := m["statusLine"].(map[string]any)
	if !ok {
		t.Fatalf("statusLine missing: %v", m)
	}
	if sl["type"] != "command" {
		t.Errorf("type = %v", sl["type"])
	}
	if sl["command"] != res.Command || res.Command == "" {
		t.Errorf("command = %v (res %q)", sl["command"], res.Command)
	}
	if sl["refreshInterval"].(float64) != 30 {
		t.Errorf("refreshInterval = %v", sl["refreshInterval"])
	}
	if res.Backup != "" {
		t.Errorf("no backup expected when file is new, got %q", res.Backup)
	}
}

func TestRegisterPreservesOtherKeysAndBacksUp(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	// Pre-existing settings with an unrelated key and a stale statusLine.
	existing := `{"model":"opus","statusLine":{"type":"command","command":"/old/path"}}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Register(0) // 0 => no refreshInterval key
	if err != nil {
		t.Fatal(err)
	}
	if res.Backup == "" {
		t.Error("expected a backup of the existing file")
	}
	if res.PrevCommand != "/old/path" {
		t.Errorf("prev command = %q", res.PrevCommand)
	}
	m := readSettings(t)
	if m["model"] != "opus" {
		t.Errorf("unrelated key not preserved: %v", m["model"])
	}
	sl := m["statusLine"].(map[string]any)
	if _, has := sl["refreshInterval"]; has {
		t.Errorf("refreshInterval should be omitted when 0")
	}
}

func TestUnregister(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	if _, err := Register(30); err != nil {
		t.Fatal(err)
	}
	if _, err := Unregister(false); err != nil {
		t.Fatal(err)
	}
	m := readSettings(t)
	if _, ok := m["statusLine"]; ok {
		t.Error("statusLine should be removed")
	}
}

func TestRefusesInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Register(30); err == nil {
		t.Error("expected Register to refuse invalid settings.json rather than clobber it")
	}
}
