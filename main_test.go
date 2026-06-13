package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommitsVirtualFileWithoutChangingWorkingTree(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "README.md", "base\n")
	git(t, repo, "add", "README.md")
	git(t, repo, "commit", "-m", "initial")

	writeFile(t, repo, "README.md", "local draft\n")
	source := filepath.Join(t.TempDir(), "README.llm.md")
	writeFile(t, filepath.Dir(source), filepath.Base(source), "ghost content\n")

	var stdout bytes.Buffer
	err := run([]string{"-m", "commit virtual README", "--file", "README.md=" + source}, strings.NewReader(""), &stdout, &bytes.Buffer{}, repo)
	if err != nil {
		t.Fatalf("run ghost-commit: %v", err)
	}

	if got := readFile(t, repo, "README.md"); got != "local draft\n" {
		t.Fatalf("working tree changed: got %q", got)
	}
	if got := gitOutput(t, repo, "show", "HEAD:README.md"); got != "ghost content\n" {
		t.Fatalf("committed README.md = %q", got)
	}
	if got := strings.TrimSpace(gitOutput(t, repo, "log", "-1", "--format=%s")); got != "commit virtual README" {
		t.Fatalf("commit subject = %q", got)
	}
	if strings.TrimSpace(stdout.String()) == "" {
		t.Fatal("expected new commit SHA on stdout")
	}
}

func TestAddsAndDeletesVirtualFilesWithoutTouchingWorkingTree(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "old.txt", "still on disk\n")
	git(t, repo, "add", "old.txt")
	git(t, repo, "commit", "-m", "initial")

	sourceDir := t.TempDir()
	writeFile(t, sourceDir, "new.txt", "new virtual file\n")

	err := run(
		[]string{
			"-m", "replace files virtually",
			"--file", "docs/new.txt=" + filepath.Join(sourceDir, "new.txt"),
			"--delete", "old.txt",
		},
		strings.NewReader(""),
		&bytes.Buffer{},
		&bytes.Buffer{},
		repo,
	)
	if err != nil {
		t.Fatalf("run ghost-commit: %v", err)
	}

	if got := readFile(t, repo, "old.txt"); got != "still on disk\n" {
		t.Fatalf("working tree deletion touched disk: got %q", got)
	}
	if got := gitOutput(t, repo, "show", "HEAD:docs/new.txt"); got != "new virtual file\n" {
		t.Fatalf("committed docs/new.txt = %q", got)
	}
	if err := exec.Command("git", "-C", repo, "show", "HEAD:old.txt").Run(); err == nil {
		t.Fatal("old.txt still exists in HEAD")
	}
}

func TestReadsOneVirtualFileFromStdin(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "base.txt", "base\n")
	git(t, repo, "add", "base.txt")
	git(t, repo, "commit", "-m", "initial")

	err := run(
		[]string{"-m", "add stdin file", "--file", "from-stdin.txt=-"},
		strings.NewReader("stdin content\n"),
		&bytes.Buffer{},
		&bytes.Buffer{},
		repo,
	)
	if err != nil {
		t.Fatalf("run ghost-commit: %v", err)
	}

	if got := gitOutput(t, repo, "show", "HEAD:from-stdin.txt"); got != "stdin content\n" {
		t.Fatalf("committed stdin file = %q", got)
	}
	if _, err := os.Stat(filepath.Join(repo, "from-stdin.txt")); !os.IsNotExist(err) {
		t.Fatalf("stdin file should not be created in working tree, stat err = %v", err)
	}
}

func TestDoesNotCommitOrClearExistingStagedChanges(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "base.txt", "base\n")
	git(t, repo, "add", "base.txt")
	git(t, repo, "commit", "-m", "initial")

	writeFile(t, repo, "already-staged.txt", "keep staged\n")
	git(t, repo, "add", "already-staged.txt")

	source := filepath.Join(t.TempDir(), "ghost.txt")
	writeFile(t, filepath.Dir(source), filepath.Base(source), "ghost\n")

	err := run(
		[]string{"-m", "ghost only", "--file", "ghost.txt=" + source},
		strings.NewReader(""),
		&bytes.Buffer{},
		&bytes.Buffer{},
		repo,
	)
	if err != nil {
		t.Fatalf("run ghost-commit: %v", err)
	}

	if got := gitOutput(t, repo, "show", "HEAD:ghost.txt"); got != "ghost\n" {
		t.Fatalf("committed ghost.txt = %q", got)
	}
	if err := exec.Command("git", "-C", repo, "show", "HEAD:already-staged.txt").Run(); err == nil {
		t.Fatal("pre-existing staged file was included in ghost commit")
	}
	if got := strings.TrimSpace(gitOutput(t, repo, "diff", "--cached", "--name-only")); got != "already-staged.txt" {
		t.Fatalf("pre-existing staged file was not preserved, got %q", got)
	}
}

func TestPathsAreRelativeToInvocationDirectory(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "sub/a.txt", "base\n")
	git(t, repo, "add", "sub/a.txt")
	git(t, repo, "commit", "-m", "initial")

	source := filepath.Join(t.TempDir(), "a.txt")
	writeFile(t, filepath.Dir(source), filepath.Base(source), "ghost from subdir\n")

	err := run(
		[]string{"-m", "ghost from subdir", "--file", "a.txt=" + source},
		strings.NewReader(""),
		&bytes.Buffer{},
		&bytes.Buffer{},
		filepath.Join(repo, "sub"),
	)
	if err != nil {
		t.Fatalf("run ghost-commit: %v", err)
	}

	if got := gitOutput(t, repo, "show", "HEAD:sub/a.txt"); got != "ghost from subdir\n" {
		t.Fatalf("committed sub/a.txt = %q", got)
	}
	if err := exec.Command("git", "-C", repo, "show", "HEAD:a.txt").Run(); err == nil {
		t.Fatal("ghost file was committed at repository root instead of invocation directory")
	}
}

func TestParentPathsCanStayInsideRepositoryFromSubdirectory(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "README.md", "base\n")
	git(t, repo, "add", "README.md")
	git(t, repo, "commit", "-m", "initial")
	if err := os.Mkdir(filepath.Join(repo, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	source := filepath.Join(t.TempDir(), "README.md")
	writeFile(t, filepath.Dir(source), filepath.Base(source), "ghost parent\n")

	err := run(
		[]string{"-m", "ghost parent", "--file", "../README.md=" + source},
		strings.NewReader(""),
		&bytes.Buffer{},
		&bytes.Buffer{},
		filepath.Join(repo, "sub"),
	)
	if err != nil {
		t.Fatalf("run ghost-commit: %v", err)
	}
	if got := gitOutput(t, repo, "show", "HEAD:README.md"); got != "ghost parent\n" {
		t.Fatalf("committed README.md = %q", got)
	}
}

func TestRejectsParentPathsEscapingRepositoryFromRoot(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "base.txt", "base\n")
	git(t, repo, "add", "base.txt")
	git(t, repo, "commit", "-m", "initial")

	source := filepath.Join(t.TempDir(), "outside.txt")
	writeFile(t, filepath.Dir(source), filepath.Base(source), "outside\n")

	err := run(
		[]string{"-m", "outside", "--file", "../outside.txt=" + source},
		strings.NewReader(""),
		&bytes.Buffer{},
		&bytes.Buffer{},
		repo,
	)
	if err == nil || !strings.Contains(err.Error(), "must stay inside the repository") {
		t.Fatalf("expected escape rejection, got %v", err)
	}
}

func TestReplacesDirectoryWithVirtualFile(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "dir/a.txt", "child\n")
	git(t, repo, "add", "dir/a.txt")
	git(t, repo, "commit", "-m", "initial")

	source := filepath.Join(t.TempDir(), "dir")
	writeFile(t, filepath.Dir(source), filepath.Base(source), "file now\n")

	err := run(
		[]string{"-m", "replace dir with file", "--file", "dir=" + source},
		strings.NewReader(""),
		&bytes.Buffer{},
		&bytes.Buffer{},
		repo,
	)
	if err != nil {
		t.Fatalf("run ghost-commit: %v", err)
	}
	if got := gitOutput(t, repo, "show", "HEAD:dir"); got != "file now\n" {
		t.Fatalf("committed dir file = %q", got)
	}
	if err := exec.Command("git", "-C", repo, "show", "HEAD:dir/a.txt").Run(); err == nil {
		t.Fatal("directory child still exists in HEAD")
	}
}

func TestReplacesFileWithVirtualDirectoryChild(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "dir", "file first\n")
	git(t, repo, "add", "dir")
	git(t, repo, "commit", "-m", "initial")

	source := filepath.Join(t.TempDir(), "a.txt")
	writeFile(t, filepath.Dir(source), filepath.Base(source), "child now\n")

	err := run(
		[]string{"-m", "replace file with dir child", "--file", "dir/a.txt=" + source},
		strings.NewReader(""),
		&bytes.Buffer{},
		&bytes.Buffer{},
		repo,
	)
	if err != nil {
		t.Fatalf("run ghost-commit: %v", err)
	}
	if got := gitOutput(t, repo, "show", "HEAD:dir/a.txt"); got != "child now\n" {
		t.Fatalf("committed dir/a.txt = %q", got)
	}
	if got := strings.TrimSpace(gitOutput(t, repo, "cat-file", "-t", "HEAD:dir")); got != "tree" {
		t.Fatalf("dir should be a tree after replacement, got %q", got)
	}
}

func TestRejectsGhostPathWithExistingStagedChanges(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "target.txt", "base\n")
	git(t, repo, "add", "target.txt")
	git(t, repo, "commit", "-m", "initial")

	writeFile(t, repo, "target.txt", "already staged\n")
	git(t, repo, "add", "target.txt")
	source := filepath.Join(t.TempDir(), "target.txt")
	writeFile(t, filepath.Dir(source), filepath.Base(source), "ghost\n")

	err := run(
		[]string{"-m", "ghost target", "--file", "target.txt=" + source},
		strings.NewReader(""),
		&bytes.Buffer{},
		&bytes.Buffer{},
		repo,
	)
	if err == nil || !strings.Contains(err.Error(), "overlaps staged changes") {
		t.Fatalf("expected staged-overlap error, got %v", err)
	}
	if got := gitOutput(t, repo, "show", "HEAD:target.txt"); got != "base\n" {
		t.Fatalf("HEAD changed despite rejection: %q", got)
	}
}

func TestRejectsParentChildStagedOverlapBeforeMovingHead(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "base.txt", "base\n")
	git(t, repo, "add", "base.txt")
	git(t, repo, "commit", "-m", "initial")
	before := strings.TrimSpace(gitOutput(t, repo, "rev-parse", "HEAD"))

	writeFile(t, repo, "dir", "already staged parent\n")
	git(t, repo, "add", "dir")
	source := filepath.Join(t.TempDir(), "child.txt")
	writeFile(t, filepath.Dir(source), filepath.Base(source), "ghost child\n")

	err := run(
		[]string{"-m", "ghost child", "--file", "dir/a.txt=" + source},
		strings.NewReader(""),
		&bytes.Buffer{},
		&bytes.Buffer{},
		repo,
	)
	if err == nil || !strings.Contains(err.Error(), "overlaps staged changes") {
		t.Fatalf("expected staged parent/child overlap error, got %v", err)
	}
	after := strings.TrimSpace(gitOutput(t, repo, "rev-parse", "HEAD"))
	if after != before {
		t.Fatalf("HEAD moved despite rejection: before %s after %s", before, after)
	}
	if got := strings.TrimSpace(gitOutput(t, repo, "diff", "--cached", "--name-only")); got != "dir" {
		t.Fatalf("staged parent was not preserved, got %q", got)
	}
}

func TestRejectsNoopGhostCommit(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "same.txt", "same\n")
	git(t, repo, "add", "same.txt")
	git(t, repo, "commit", "-m", "initial")

	source := filepath.Join(t.TempDir(), "same.txt")
	writeFile(t, filepath.Dir(source), filepath.Base(source), "same\n")

	err := run([]string{"-m", "same", "--file", "same.txt=" + source}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, repo)
	if err == nil || !strings.Contains(err.Error(), "no commit") {
		t.Fatalf("expected no-op error, got %v", err)
	}
}

func newRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init")
	git(t, repo, "config", "user.name", "Ghost Commit Test")
	git(t, repo, "config", "user.email", "ghost-commit@example.test")
	return repo
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func git(t *testing.T, repo string, args ...string) {
	t.Helper()
	if out, err := gitCmd(repo, args...).CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func gitOutput(t *testing.T, repo string, args ...string) string {
	t.Helper()
	out, err := gitCmd(repo, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func gitCmd(repo string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	return cmd
}
