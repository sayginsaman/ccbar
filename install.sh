#!/bin/sh
# ccbar remote installer — downloads a prebuilt binary (no Go needed) and registers
# it as your Claude Code status line.
#
#   curl -fsSL https://raw.githubusercontent.com/saygindoruksaman/ccbar/main/install.sh | sh
#
# Environment overrides:
#   CCBAR_REPO         GitHub "owner/repo"     (default: saygindoruksaman/ccbar)
#   CCBAR_VERSION      tag, e.g. v1.2.0         (default: latest)
#   CCBAR_INSTALL_DIR  where to place the binary (default: ~/.claude/ccbar)
#   CCBAR_FROM_SOURCE  set to 1 to build from source with Go instead of downloading
#
# Falls back to building from source (needs Go + git) if no prebuilt asset is found.
set -eu

REPO="${CCBAR_REPO:-saygindoruksaman/ccbar}"
VERSION="${CCBAR_VERSION:-latest}"
INSTALL_DIR="${CCBAR_INSTALL_DIR:-${CLAUDE_CONFIG_DIR:-$HOME/.claude}/ccbar}"
BIN="$INSTALL_DIR/ccbar"

say() { printf '%s\n' "$*"; }
err() { printf 'error: %s\n' "$*" >&2; exit 1; }

# --- detect platform -------------------------------------------------------
os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$os" in
  darwin) os=darwin ;;
  linux)  os=linux ;;
  *) err "unsupported OS '$os'. On Windows, download the .exe from the Releases page; otherwise use 'go install $REPO@latest'." ;;
esac
case "$arch" in
  arm64|aarch64) arch=arm64 ;;
  x86_64|amd64)  arch=amd64 ;;
  *) err "unsupported architecture '$arch'" ;;
esac

# --- pick a downloader -----------------------------------------------------
if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -qO "$2" "$1"; }
else
  err "need curl or wget to download ccbar"
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

asset="ccbar_${os}_${arch}.tar.gz"
if [ "$VERSION" = latest ]; then
  url="https://github.com/$REPO/releases/latest/download/$asset"
else
  url="https://github.com/$REPO/releases/download/$VERSION/$asset"
fi

build_from_source() {
  command -v go  >/dev/null 2>&1 || err "Go is required to build from source (https://go.dev/dl)"
  command -v git >/dev/null 2>&1 || err "git is required to build from source"
  branch=main; [ "$VERSION" != latest ] && branch="$VERSION"
  say "Building ccbar from source ($REPO@$branch)…"
  git clone --depth 1 --branch "$branch" "https://github.com/$REPO" "$tmp/src" 2>/dev/null \
    || git clone --depth 1 "https://github.com/$REPO" "$tmp/src" \
    || err "git clone failed"
  ( cd "$tmp/src" && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$tmp/ccbar" . ) \
    || err "go build failed"
}

if [ "${CCBAR_FROM_SOURCE:-0}" = 1 ]; then
  build_from_source
else
  say "Downloading $asset ($VERSION)…"
  if fetch "$url" "$tmp/ccbar.tar.gz" && [ -s "$tmp/ccbar.tar.gz" ]; then
    tar -xzf "$tmp/ccbar.tar.gz" -C "$tmp" || err "could not extract $asset"
    [ -f "$tmp/ccbar" ] || err "archive did not contain a ccbar binary"
  else
    say "No prebuilt asset at $url — falling back to a source build."
    build_from_source
  fi
fi

# --- place the binary at a stable path & register it -----------------------
mkdir -p "$INSTALL_DIR"
mv -f "$tmp/ccbar" "$BIN"
chmod +x "$BIN"
say "Installed $("$BIN" --version) -> $BIN"
say ""

# `ccbar install` edits ~/.claude/settings.json (with a backup) to add the
# statusLine pointing at this binary.
"$BIN" install

say ""
say "All set. Tip: '$BIN --doctor' verifies the usage endpoint."
