# typed: false
# frozen_string_literal: true

class LetGo < Formula
  desc "A Clojure dialect implemented as a bytecode VM in Go"
  homepage "https://github.com/nooga/let-go"
  license "MIT"
  version "1.7.0"

  on_macos do
    on_intel do
      url "https://github.com/nooga/let-go/releases/download/v1.7.0/let-go_1.7.0_darwin_amd64.tar.gz"
      sha256 "9ba49d1dfef8f35b525ede9c65695899d03c4958e39cb53fd1b38bb70d9c2c84"
    end
    on_arm do
      url "https://github.com/nooga/let-go/releases/download/v1.7.0/let-go_1.7.0_darwin_arm64.tar.gz"
      sha256 "34b63e15b5a599d9d347358a17437fe1f1d2cf1a83ec552c086cc44e7da2d60b"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/nooga/let-go/releases/download/v1.7.0/let-go_1.7.0_linux_amd64.tar.gz"
      sha256 "4acded23f687bb7b6d320be67b12d1938525a5ee6f7ba193ba58db9900185840"
    end
    on_arm do
      url "https://github.com/nooga/let-go/releases/download/v1.7.0/let-go_1.7.0_linux_arm64.tar.gz"
      sha256 "7b7d7e9a7dbe9d184d2042cc4895ceda4b0a475aee17f663e9bb4661ed1af11e"
    end
  end

  def install
    bin.install "lg"
  end

  test do
    assert_equal "2", shell_output("#{bin}/lg -e '(+ 1 1)'").strip
  end
end
