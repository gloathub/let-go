# typed: false
# frozen_string_literal: true

class LetGo < Formula
  desc "A Clojure dialect implemented as a bytecode VM in Go"
  homepage "https://github.com/nooga/let-go"
  license "MIT"
  version "1.5.0"

  on_macos do
    on_intel do
      url "https://github.com/nooga/let-go/releases/download/v1.5.0/let-go_1.5.0_darwin_amd64.tar.gz"
      sha256 "1da75fe6d50834930ec22f64476a1707f81bdfb0db38e87ad447494957607484"
    end
    on_arm do
      url "https://github.com/nooga/let-go/releases/download/v1.5.0/let-go_1.5.0_darwin_arm64.tar.gz"
      sha256 "e87502b4d4dca1715ff360e837c83df88819ccdad069fa77f8aeedb86776342f"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/nooga/let-go/releases/download/v1.5.0/let-go_1.5.0_linux_amd64.tar.gz"
      sha256 "49cc2f3fb6156d443a978e8532c302d5066163029506de98f5315f57d719b3d2"
    end
    on_arm do
      url "https://github.com/nooga/let-go/releases/download/v1.5.0/let-go_1.5.0_linux_arm64.tar.gz"
      sha256 "8d281dbad80f717e5c3e0ea0a53da9007c0c27893d77f52e1dadc4915ab20e9c"
    end
  end

  def install
    bin.install "lg"
  end

  test do
    assert_equal "2", shell_output("#{bin}/lg -e '(+ 1 1)'").strip
  end
end
