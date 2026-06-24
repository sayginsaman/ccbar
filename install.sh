#!/usr/bin/env bash
# ccbar installer: builds the binary into ~/.claude/ccbar and registers it as the
# Claude Code statusLine in ~/.claude/settings.json (with a timestamped backup).
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLAUDE_DIR="${CLAUDE_CONFIG_DIR:-$HOME/.claude}"
INSTALL_DIR="$CLAUDE_DIR/ccbar"
BIN="$INSTALL_DIR/ccbar"
SETTINGS="$CLAUDE_DIR/settings.json"
REFRESH_INTERVAL="${CCBAR_REFRESH_INTERVAL:-30}"

echo "ccbar installer"
echo "  repo:     $REPO_DIR"
echo "  install:  $BIN"
echo "  settings: $SETTINGS"
echo

if ! command -v go >/dev/null 2>&1; then
  echo "error: Go is required to build ccbar (https://go.dev/dl/). Aborting." >&2
  exit 1
fi

echo "==> building (native, static)…"
mkdir -p "$INSTALL_DIR"
( cd "$REPO_DIR" && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$BIN.new" . )
mv -f "$BIN.new" "$BIN"        # atomic replace, safe even if a session is running
chmod +x "$BIN"
echo "    built $("$BIN" --version)"

echo "==> writing default config (if absent)…"
"$BIN" --init-config || true

echo "==> registering statusLine in settings.json…"
if ! command -v python3 >/dev/null 2>&1; then
  echo "error: python3 is required to safely edit settings.json." >&2
  echo "Add this manually to $SETTINGS:" >&2
  echo "  \"statusLine\": { \"type\": \"command\", \"command\": \"$BIN\", \"padding\": 0, \"refreshInterval\": $REFRESH_INTERVAL }" >&2
  exit 1
fi

BIN="$BIN" SETTINGS="$SETTINGS" REFRESH_INTERVAL="$REFRESH_INTERVAL" python3 - <<'PY'
import json, os, shutil, time, sys

settings = os.environ["SETTINGS"]
binpath  = os.environ["BIN"]
interval = int(os.environ["REFRESH_INTERVAL"])

data = {}
if os.path.exists(settings):
    try:
        with open(settings) as f:
            data = json.load(f)
    except Exception as e:
        print(f"  warning: could not parse existing settings.json ({e}); starting fresh", file=sys.stderr)
        data = {}
    backup = f"{settings}.bak.{int(time.time())}"
    shutil.copy2(settings, backup)
    print(f"    backup: {backup}")

prev = data.get("statusLine")
if prev and prev.get("command") != binpath:
    print(f"    note: replacing existing statusLine command: {prev.get('command')!r}")

data["statusLine"] = {
    "type": "command",
    "command": binpath,
    "padding": 0,
    "refreshInterval": interval,
}

os.makedirs(os.path.dirname(settings), exist_ok=True)
tmp = settings + ".tmp"
with open(tmp, "w") as f:
    json.dump(data, f, indent=2)
    f.write("\n")
os.replace(tmp, settings)
print("    statusLine registered")
PY

echo
echo "✅ Installed. The bar appears on your next interaction in any Claude Code session"
echo "   (no restart needed). Tip: run '$BIN --doctor' to verify the usage endpoint,"
echo "   and edit $INSTALL_DIR/config.json to customize."
