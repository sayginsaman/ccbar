# Homebrew formula for ccbar (build-from-source).
#
# Distribute via a tap repo named "homebrew-tap":
#   1. Create github.com/<you>/homebrew-tap
#   2. Put this file at Formula/ccbar.rb in that repo
#   3. Tag a release in the ccbar repo (e.g. v1.0.0) and fill in `url`/`sha256`:
#        curl -fsSL https://github.com/<you>/ccbar/archive/refs/tags/v1.0.0.tar.gz -o ccbar.tgz
#        shasum -a 256 ccbar.tgz
#
# Users then run:
#   brew install <you>/tap/ccbar
#   ccbar install
#
# (GoReleaser can also generate/update this formula automatically — see .goreleaser.yaml.)
class Ccbar < Formula
  desc "Claude Code status-line info bar: tokens, cost, session/weekly/per-model limits"
  homepage "https://github.com/saygindoruksaman/ccbar"
  url "https://github.com/saygindoruksaman/ccbar/archive/refs/tags/v1.0.0.tar.gz"
  sha256 "REPLACE_WITH_SOURCE_TARBALL_SHA256"
  license "MIT"
  head "https://github.com/saygindoruksaman/ccbar.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w -X main.version=#{version}")
  end

  def caveats
    <<~EOS
      Activate the status line (edits ~/.claude/settings.json, with a backup):
        ccbar install

      Re-run `ccbar install` after `brew upgrade ccbar` if the path changes.
      Remove with:
        ccbar uninstall      # then: brew uninstall ccbar
    EOS
  end

  test do
    assert_match "ccbar", shell_output("#{bin}/ccbar --version")
  end
end
