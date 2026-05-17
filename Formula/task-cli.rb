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
  version "0.1.0"  # TODO: update on each release

  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/pooch1e/task-cli/releases/download/v#{version}/task-darwin-arm64"
      # TODO: replace with real sha256 from dist/checksums.txt
      sha256 "0000000000000000000000000000000000000000000000000000000000000000"

      def install
        bin.install "task-darwin-arm64" => "task"
      end
    else
      url "https://github.com/pooch1e/task-cli/releases/download/v#{version}/task-darwin-amd64"
      # TODO: replace with real sha256 from dist/checksums.txt
      sha256 "0000000000000000000000000000000000000000000000000000000000000000"

      def install
        bin.install "task-darwin-amd64" => "task"
      end
    end
  end

  on_linux do
    url "https://github.com/pooch1e/task-cli/releases/download/v#{version}/task-linux-amd64"
    # TODO: replace with real sha256 from dist/checksums.txt
    sha256 "0000000000000000000000000000000000000000000000000000000000000000"

    def install
      bin.install "task-linux-amd64" => "task"
    end
  end

  test do
    system "#{bin}/task", "version"
  end
end
