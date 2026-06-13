class GhostCommit < Formula
  desc "Commit virtual file contents without changing the working tree"
  homepage "https://github.com/s4na/ghost-commit"
  license "MIT"
  head "https://github.com/s4na/ghost-commit.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w")
  end

  test do
    mkdir testpath/"repo" do
      system "git", "init"
      system "git", "config", "user.name", "Homebrew Test"
      system "git", "config", "user.email", "homebrew@example.test"

      (testpath/"repo/base.txt").write "base\n"
      system "git", "add", "base.txt"
      system "git", "commit", "-m", "initial"

      (testpath/"ghost.txt").write "ghost\n"
      assert_match(/^[0-9a-f]{40}$/,
        shell_output("#{bin}/ghost-commit -m 'ghost file' --file virtual.txt=#{testpath}/ghost.txt").strip)
      assert_equal "ghost\n", shell_output("git show HEAD:virtual.txt")
      refute_path_exists testpath/"repo/virtual.txt"
    end
  end
end
