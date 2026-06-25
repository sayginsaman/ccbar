package usage

import (
	"encoding/json"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/sayginsaman/ccbar/internal/config"
)

// credFile mirrors ~/.claude/.credentials.json. expiresAt is a Unix epoch in ms.
type credFile struct {
	ClaudeAiOauth struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresAt    int64  `json:"expiresAt"`
	} `json:"claudeAiOauth"`
}

const expiryMarginMs = 60_000 // treat tokens expiring within 60s as expired

// TokenStatus is a non-secret description of the credential state, for --doctor.
type TokenStatus struct {
	Present   bool
	Expired   bool
	Source    string // "file" | "keychain" | ""
	ExpiresAt time.Time
}

func credentialsFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", ".credentials.json")
}

// readToken returns a usable OAuth access token, preferring the credentials file
// (no prompt). If that token is missing/expired and cfg.Keychain is set, it falls
// back to the macOS Keychain (which may prompt once). The token is never logged.
func readToken(cfg config.Config) (string, bool) {
	if tok, ok := parseCred(readCredFileBytes()); ok {
		return tok, true
	}
	if cfg.Keychain {
		if tok, ok := parseCred(readKeychainBytes()); ok {
			return tok, true
		}
	}
	return "", false
}

func readCredFileBytes() []byte {
	p := credentialsFilePath()
	if p == "" {
		return nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	return b
}

func readKeychainBytes() []byte {
	u := os.Getenv("USER")
	if u == "" {
		if cu, err := user.Current(); err == nil {
			u = cu.Username
		}
	}
	out, err := exec.Command("security", "find-generic-password",
		"-s", "Claude Code-credentials", "-a", u, "-w").Output()
	if err != nil {
		return nil
	}
	return out
}

// parseCred extracts a non-expired access token from a credentials blob. The blob
// is normally the {"claudeAiOauth":{...}} JSON; as a fallback it also accepts a
// bare token string (some Keychain items store just the token).
func parseCred(b []byte) (string, bool) {
	if len(b) == 0 {
		return "", false
	}
	var c credFile
	if json.Unmarshal(b, &c) == nil && c.ClaudeAiOauth.AccessToken != "" {
		if tokenExpired(c.ClaudeAiOauth.ExpiresAt) {
			return "", false
		}
		return c.ClaudeAiOauth.AccessToken, true
	}
	if s := strings.TrimSpace(string(b)); strings.HasPrefix(s, "sk-ant-oat") {
		return s, true
	}
	return "", false
}

func tokenExpired(expiresAtMs int64) bool {
	if expiresAtMs <= 0 {
		return false // unknown expiry: assume usable, let a 401 handle it
	}
	return time.Now().UnixMilli() > expiresAtMs-expiryMarginMs
}

// Status reports the credential state without exposing the token, for diagnostics.
func Status(cfg config.Config) TokenStatus {
	var st TokenStatus
	if b := readCredFileBytes(); len(b) > 0 {
		var c credFile
		if json.Unmarshal(b, &c) == nil && c.ClaudeAiOauth.AccessToken != "" {
			st.Present = true
			st.Source = "file"
			if c.ClaudeAiOauth.ExpiresAt > 0 {
				st.ExpiresAt = time.UnixMilli(c.ClaudeAiOauth.ExpiresAt)
			}
			st.Expired = tokenExpired(c.ClaudeAiOauth.ExpiresAt)
			if !st.Expired {
				return st
			}
		}
	}
	if cfg.Keychain {
		if _, ok := parseCred(readKeychainBytes()); ok {
			return TokenStatus{Present: true, Source: "keychain"}
		}
	}
	return st
}
