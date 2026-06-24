#!/usr/bin/env bash
# Removes the ccbar statusLine from settings.json (with a backup). Pass --purge to
# also delete ~/.claude/ccbar (binary, config, and usage cache).
set -euo pipefail

CLAUDE_DIR="${CLAUDE_CONFIG_DIR:-$HOME/.claude}"
INSTALL_DIR="$CLAUDE_DIR/ccbar"
SETTINGS="$CLAUDE_DIR/settings.json"
PURGE="${1:-}"

if [ -f "$SETTINGS" ] && command -v python3 >/dev/null 2>&1; then
  SETTINGS="$SETTINGS" python3 - <<'PY'
import json, os, shutil, time
settings = os.environ["SETTINGS"]
try:
    with open(settings) as f:
        data = json.load(f)
except Exception:
    data = {}
if "statusLine" in data:
    shutil.copy2(settings, f"{settings}.bak.{int(time.time())}")
    del data["statusLine"]
    tmp = settings + ".tmp"
    with open(tmp, "w") as f:
        json.dump(data, f, indent=2); f.write("\n")
    os.replace(tmp, settings)
    print("removed statusLine from settings.json (backup written)")
else:
    print("no statusLine entry found in settings.json")
PY
else
  echo "settings.json not found or python3 unavailable; remove the statusLine entry manually."
fi

if [ "$PURGE" = "--purge" ]; then
  rm -rf "$INSTALL_DIR"
  echo "purged $INSTALL_DIR"
else
  echo "left $INSTALL_DIR in place (run with --purge to delete it)"
fi
