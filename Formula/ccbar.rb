# Prebuilt Homebrew formula for ccbar v1.0.0 (Claude Code info bar).
# Regenerate per release (or let .goreleaser.yaml's brews: block maintain it).
class Ccbar < Formula
  desc "Claude Code status-line info bar: tokens, cost, session/weekly/per-model limits"
  homepage "https://github.com/sayginsaman/ccbar"
  version "1.0.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/sayginsaman/ccbar/releases/download/v1.0.0/ccbar_darwin_arm64.tar.gz"
      sha256 "5fc77199695bf94f67759653f7d46a8b8647d1d4b5f4b2bf233eb401ea2f68d1"
    else
      url "https://github.com/sayginsaman/ccbar/releases/download/v1.0.0/ccbar_darwin_amd64.tar.gz"
      sha256 "a40aa1a616d3b20b90817b5fe2ca4b2b0b507c08affaed7f640d2edc62b13ef8"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/sayginsaman/ccbar/releases/download/v1.0.0/ccbar_linux_arm64.tar.gz"
      sha256 "18ae21c4744e471b9bcb25eb560fe96e80532e10f5be85c60e159798d09a7d88"
    else
      url "https://github.com/sayginsaman/ccbar/releases/download/v1.0.0/ccbar_linux_amd64.tar.gz"
      sha256 "cccc91acca4066ffaa916d387993c67f67a96ad2b35427bb848fe3a627306113"
    end
  end

  def install
    bin.install "ccbar"
  end

  def caveats
    <<~EOS
      Activate the status line (edits ~/.claude/settings.json, with a backup):
        ccbar install
      Remove with:
        ccbar uninstall      # then: brew uninstall ccbar
    EOS
  end

  test do
    assert_match "ccbar", shell_output("#{bin}/ccbar --version")
  end
end
