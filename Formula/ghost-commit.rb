class GhostCommit < Formula
  desc "Prints hello world"
  homepage "https://github.com/s4na/ghost-commit"
  url "https://github.com/s4na/ghost-commit.git",
      tag:      "v0.1.0",
      revision: "HEAD"
  license "MIT"
  head "https://github.com/s4na/ghost-commit.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w")
  end

  test do
    assert_equal "hello world", shell_output("#{bin}/ghost-commit").strip
  end
end
