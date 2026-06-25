# Releasing ccbar

Distribution has two front doors that both end at `ccbar install` (which registers
the binary in `~/.claude/settings.json`):

- **`curl | sh`** → `install.sh` downloads a prebuilt binary from a GitHub Release.
- **Homebrew** → a formula in a `homebrew-tap` repo.

> Repo references are set to `sayginsaman/ccbar` (module path, installer, formula,
> and release config). If you fork, update those in `go.mod`, `install.sh`,
> `Formula/ccbar.rb`, and `.goreleaser.yaml`.

## 1. Cut a release (prebuilt binaries for the curl installer)

The curl installer expects assets named `ccbar_<os>_<arch>.tar.gz`. The easiest way
to produce them is **GoReleaser** (config included):

```sh
brew install goreleaser            # once
git tag v1.0.0 && git push origin v1.0.0
GITHUB_TOKEN=… goreleaser release --clean
```

This builds darwin/linux/windows × amd64/arm64, creates the GitHub Release with the
correctly-named archives + `checksums.txt`, and (via the `brews:` block) pushes a
prebuilt Homebrew formula to `<owner>/homebrew-tap`.

Prefer no extra tooling? Build and upload manually with `go` + `gh`:

```sh
VER=v1.0.0
for t in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64; do
  os=${t%/*}; arch=${t#*/}
  GOOS=$os GOARCH=$arch CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=$VER" -o ccbar .
  tar -czf "ccbar_${os}_${arch}.tar.gz" ccbar README.md LICENSE
done
gh release create "$VER" ccbar_*.tar.gz --generate-notes
```

After this, the one-liner works:

```sh
curl -fsSL https://raw.githubusercontent.com/<owner>/ccbar/main/install.sh | sh
```

(Before any release exists, `install.sh` automatically falls back to building from
source if Go + git are present.)

## 2. Homebrew tap

Create a repo `github.com/<owner>/homebrew-tap`. Then either:

- **Automated:** let GoReleaser's `brews:` block write `Formula/ccbar.rb` there on
  each release (keeps the version/sha in sync for you), or
- **Manual:** copy this repo's `Formula/ccbar.rb` into the tap, set `url` to the
  release source tarball and fill `sha256` (`shasum -a 256`).

Users then run:

```sh
brew install <owner>/tap/ccbar
ccbar install
```
