# Formula/task-cli.rb
#
# Homebrew formula for task-cli.
#
# To install from this tap:
#   brew tap pooch1e/task-cli https://github.com/pooch1e/homebrew-task-cli
#   brew install task-cli
#
# Or run directly:
#   brew install pooch1e/task-cli/task-cli
#
# NOTE: The sha256 values and version below must be updated after each release.
# Run `make checksums` after `make release` to get fresh values, then update
# this file and commit to the homebrew-task-cli tap repo.

class TaskCli < Formula
  desc "Personal user story and task tracker with LLM generation"
  homepage "https://github.com/pooch1e/task-cli"
  version "0.1.0"

  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/pooch1e/task-cli/releases/download/v#{version}/task-darwin-arm64"
      sha256 "0c6f68fee7bf2d190189bfa8b1b6b29f764aa2f355f230d3248a15cf3d13cf91"

      def install
        bin.install "task-darwin-arm64" => "task"
      end
    else
      url "https://github.com/pooch1e/task-cli/releases/download/v#{version}/task-darwin-amd64"
      sha256 "ee994d881c2239a758565ad3f71c1a4ccbf9765ee36009637434d27f2725a612"

      def install
        bin.install "task-darwin-amd64" => "task"
      end
    end
  end

  on_linux do
    url "https://github.com/pooch1e/task-cli/releases/download/v#{version}/task-linux-amd64"
    sha256 "3e03f7545b0c66b4863ef353affa12ca19ad918d004a6d39c2b2648b4944f921"

    def install
      bin.install "task-linux-amd64" => "task"
    end
  end

  test do
    system "#{bin}/task", "version"
  end
end
