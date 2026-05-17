# typed: false
# frozen_string_literal: true

class LetGo < Formula
  desc "A Clojure dialect implemented as a bytecode VM in Go"
  homepage "https://github.com/nooga/let-go"
  license "MIT"
  version "2.0.0"

  on_macos do
    on_intel do
      url "https://github.com/nooga/let-go/releases/download/v2.0.0/let-go_2.0.0_darwin_amd64.tar.gz"
      sha256 "48f1224d45771e299e33fa1acaebc2fe9458524d5a6751dc84d9683350a9b566"
    end
    on_arm do
      url "https://github.com/nooga/let-go/releases/download/v2.0.0/let-go_2.0.0_darwin_arm64.tar.gz"
      sha256 "33f43d3baf3a7d3f39195ec37a5085277e7ed709b83b48c389ea1bce6587ba16"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/nooga/let-go/releases/download/v2.0.0/let-go_2.0.0_linux_amd64.tar.gz"
      sha256 "0078f35a4a8a2e7e31da193eeecf3cbe6a4939a97cac693ef97715f0a6cd784b"
    end
    on_arm do
      url "https://github.com/nooga/let-go/releases/download/v2.0.0/let-go_2.0.0_linux_arm64.tar.gz"
      sha256 "3d63ff157dc3db07e5bf8ad6b8bcbd8facf39658d784c90c73e6695d4555c227"
    end
  end

  def install
    bin.install "lg"
  end

  test do
    assert_equal "2", shell_output("#{bin}/lg -e '(+ 1 1)'").strip
  end
end
