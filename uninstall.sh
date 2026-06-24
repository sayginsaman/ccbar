#!/bin/sh
# Removes ccbar from your Claude Code status line. Pass --purge to also delete the
# ccbar data dir (binary, config, cache). Delegates to `ccbar uninstall`.
set -eu

DIR="${CCBAR_INSTALL_DIR:-${CLAUDE_CONFIG_DIR:-$HOME/.claude}/ccbar}"

if [ -x "$DIR/ccbar" ]; then
  exec "$DIR/ccbar" uninstall "$@"
elif command -v ccbar >/dev/null 2>&1; then
  exec ccbar uninstall "$@"
else
  echo "ccbar binary not found; remove the \"statusLine\" entry from settings.json manually." >&2
  exit 1
fi
