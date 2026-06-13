package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type fileSpec struct {
	path   string
	source string
}

type fileSpecs []fileSpec

func (s *fileSpecs) String() string {
	return fmt.Sprint([]fileSpec(*s))
}

func (s *fileSpecs) Set(value string) error {
	path, source, ok := strings.Cut(value, "=")
	if !ok {
		return fmt.Errorf("expected PATH=SOURCE, got %q", value)
	}
	cleanPath, err := cleanUserPath(path)
	if err != nil {
		return err
	}
	if source == "" {
		return fmt.Errorf("source for %q is empty", cleanPath)
	}
	*s = append(*s, fileSpec{path: cleanPath, source: source})
	return nil
}

type deletePaths []string

func (p *deletePaths) String() string {
	return strings.Join(*p, ",")
}

func (p *deletePaths) Set(value string) error {
	cleanPath, err := cleanUserPath(value)
	if err != nil {
		return err
	}
	*p = append(*p, cleanPath)
	return nil
}

type options struct {
	message string
	files   fileSpecs
	deletes deletePaths
}

type virtualEntry struct {
	path   string
	mode   string
	blob   string
	delete bool
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, ""); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer, workdir string) error {
	opts, err := parseOptions(args, stderr)
	if err != nil {
		return err
	}
	if workdir == "" {
		workdir, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	g := gitRunner{workdir: workdir}
	repoRoot, err := g.output(nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	prefix, err := g.output(nil, "rev-parse", "--show-prefix")
	if err != nil {
		return fmt.Errorf("resolve repository prefix: %w", err)
	}
	opts, err = normalizeOptions(opts, workdir, strings.TrimSpace(repoRoot), strings.TrimSpace(prefix))
	if err != nil {
		return err
	}
	if err := rejectDuplicatePaths(opts); err != nil {
		return err
	}
	g = gitRunner{workdir: strings.TrimSpace(repoRoot)}

	base, err := g.output(nil, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return fmt.Errorf("resolve HEAD: %w", err)
	}
	baseCommit := strings.TrimSpace(base)

	if err := rejectOverlappingStagedChanges(g, opts); err != nil {
		return err
	}

	tmp, err := os.CreateTemp("", "ghost-commit-index-*")
	if err != nil {
		return err
	}
	indexPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Remove(indexPath); err != nil {
		return err
	}
	defer os.Remove(indexPath)

	indexEnv := []string{"GIT_INDEX_FILE=" + indexPath}
	if err := g.run(indexEnv, "read-tree", baseCommit); err != nil {
		return fmt.Errorf("prepare virtual stage: %w", err)
	}

	stdinUsed := false
	var entries []virtualEntry
	for _, spec := range opts.files {
		var r io.Reader
		if spec.source == "-" {
			if stdinUsed {
				return errors.New("only one --file entry can read from stdin")
			}
			stdinUsed = true
			r = stdin
		} else {
			source, err := os.Open(spec.source)
			if err != nil {
				return fmt.Errorf("open source for %q: %w", spec.path, err)
			}
			defer source.Close()
			r = source
		}

		blob, err := g.outputWithStdin(nil, r, "hash-object", "-w", "--stdin")
		if err != nil {
			return fmt.Errorf("store blob for %q: %w", spec.path, err)
		}
		mode, err := modeForPath(g, baseCommit, spec.path)
		if err != nil {
			return err
		}
		if err := removeIndexConflicts(g, indexEnv, spec.path); err != nil {
			return fmt.Errorf("clear virtual index conflicts for %q: %w", spec.path, err)
		}
		if err := g.run(indexEnv, "update-index", "--add", "--cacheinfo", mode, strings.TrimSpace(blob), spec.path); err != nil {
			return fmt.Errorf("stage virtual file %q: %w", spec.path, err)
		}
		entries = append(entries, virtualEntry{path: spec.path, mode: mode, blob: strings.TrimSpace(blob)})
	}

	for _, path := range opts.deletes {
		if err := removeIndexConflicts(g, indexEnv, path); err != nil {
			return fmt.Errorf("stage virtual delete %q: %w", path, err)
		}
		entries = append(entries, virtualEntry{path: path, delete: true})
	}

	tree, err := g.output(indexEnv, "write-tree")
	if err != nil {
		return fmt.Errorf("write virtual tree: %w", err)
	}
	tree = strings.TrimSpace(tree)

	baseTree, err := g.output(nil, "rev-parse", baseCommit+"^{tree}")
	if err != nil {
		return fmt.Errorf("resolve base tree: %w", err)
	}
	if tree == strings.TrimSpace(baseTree) {
		return errors.New("ghost changes produced no commit")
	}

	commit, err := g.outputWithStdin(nil, strings.NewReader(opts.message), "commit-tree", tree, "-p", baseCommit, "-F", "-")
	if err != nil {
		return fmt.Errorf("create commit: %w", err)
	}
	newCommit := strings.TrimSpace(commit)

	if err := g.run(nil, "update-ref", "-m", "ghost-commit", "HEAD", newCommit, baseCommit); err != nil {
		return fmt.Errorf("move HEAD to ghost commit: %w", err)
	}
	if err := syncIndexToGhostEntries(g, entries); err != nil {
		return fmt.Errorf("sync regular index to ghost commit: %w", err)
	}

	fmt.Fprintf(stdout, "%s\n", newCommit)
	return nil
}

func parseOptions(args []string, stderr io.Writer) (options, error) {
	var opts options
	fs := flag.NewFlagSet("ghost-commit", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&opts.message, "m", "", "commit message")
	fs.StringVar(&opts.message, "message", "", "commit message")
	fs.Var(&opts.files, "file", "stage virtual file as PATH=SOURCE; use SOURCE=- to read stdin")
	fs.Var(&opts.deletes, "delete", "stage virtual deletion for PATH")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if fs.NArg() != 0 {
		return opts, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if strings.TrimSpace(opts.message) == "" {
		return opts, errors.New("commit message is required: use -m or --message")
	}
	if len(opts.files) == 0 && len(opts.deletes) == 0 {
		return opts, errors.New("at least one --file or --delete is required")
	}
	return opts, nil
}

func normalizeOptions(opts options, workdir, repoRoot, prefix string) (options, error) {
	var normalized options
	normalized.message = opts.message
	for _, spec := range opts.files {
		path, err := cleanGitPath(prefix + spec.path)
		if err != nil {
			return normalized, err
		}
		source := spec.source
		if source != "-" && !filepath.IsAbs(source) {
			source = filepath.Join(workdir, source)
		}
		normalized.files = append(normalized.files, fileSpec{path: path, source: source})
	}
	for _, path := range opts.deletes {
		normalizedPath, err := cleanGitPath(prefix + path)
		if err != nil {
			return normalized, err
		}
		normalized.deletes = append(normalized.deletes, normalizedPath)
	}
	if repoRoot == "" {
		return normalized, errors.New("repository root is empty")
	}
	return normalized, nil
}

func rejectDuplicatePaths(opts options) error {
	seen := make(map[string]struct{})
	for _, spec := range opts.files {
		if _, ok := seen[spec.path]; ok {
			return fmt.Errorf("path %q was specified more than once", spec.path)
		}
		seen[spec.path] = struct{}{}
	}
	for _, path := range opts.deletes {
		if _, ok := seen[path]; ok {
			return fmt.Errorf("path %q was specified more than once", path)
		}
		seen[path] = struct{}{}
	}
	return nil
}

func cleanGitPath(path string) (string, error) {
	if path == "" {
		return "", errors.New("path is empty")
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("path %q must be relative", path)
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("path %q must stay inside the repository", path)
	}
	if strings.Contains(clean, "\x00") {
		return "", errors.New("path contains NUL")
	}
	return clean, nil
}

func cleanUserPath(path string) (string, error) {
	if path == "" {
		return "", errors.New("path is empty")
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("path %q must be relative", path)
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean == "." {
		return "", fmt.Errorf("path %q must name a file", path)
	}
	if strings.Contains(clean, "\x00") {
		return "", errors.New("path contains NUL")
	}
	return clean, nil
}

func modeForPath(g gitRunner, baseCommit, path string) (string, error) {
	out, err := g.output(nil, "ls-tree", "-z", baseCommit, "--", path)
	if err != nil {
		return "", fmt.Errorf("read mode for %q: %w", path, err)
	}
	if out == "" {
		return "100644", nil
	}
	mode, _, ok := strings.Cut(out, " ")
	if !ok || mode == "" {
		return "", fmt.Errorf("could not parse git mode for %q", path)
	}
	if mode == "040000" {
		return "100644", nil
	}
	return mode, nil
}

func rejectOverlappingStagedChanges(g gitRunner, opts options) error {
	seen := make(map[string]struct{})
	for _, spec := range opts.files {
		seen[spec.path] = struct{}{}
	}
	for _, path := range opts.deletes {
		seen[path] = struct{}{}
	}
	staged, err := g.stagedPaths()
	if err != nil {
		return err
	}
	for path := range seen {
		for _, stagedPath := range staged {
			if pathsOverlap(path, stagedPath) {
				return fmt.Errorf("%q overlaps staged changes at %q; unstage them before ghost-commit", path, stagedPath)
			}
		}
	}
	return nil
}

func pathsOverlap(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

func syncIndexToGhostEntries(g gitRunner, entries []virtualEntry) error {
	for _, entry := range entries {
		if err := removeIndexConflicts(g, nil, entry.path); err != nil {
			return err
		}
		if entry.delete {
			continue
		}
		if err := g.run(nil, "update-index", "--add", "--cacheinfo", entry.mode, entry.blob, entry.path); err != nil {
			return err
		}
	}
	return nil
}

func removeIndexConflicts(g gitRunner, extraEnv []string, path string) error {
	paths, err := g.indexPaths(extraEnv, path)
	if err != nil {
		return err
	}
	paths = append(paths, parentPaths(path)...)
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"update-index", "--force-remove", "--"}, paths...)
	return g.run(extraEnv, args...)
}

func parentPaths(path string) []string {
	var parents []string
	for dir := filepath.ToSlash(filepath.Dir(path)); dir != "." && dir != "/"; dir = filepath.ToSlash(filepath.Dir(dir)) {
		parents = append(parents, dir)
	}
	return parents
}

type gitRunner struct {
	workdir string
}

func (g gitRunner) run(extraEnv []string, args ...string) error {
	out, err := g.command(extraEnv, nil, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, string(out))
	}
	return nil
}

func (g gitRunner) output(extraEnv []string, args ...string) (string, error) {
	return g.outputWithStdin(extraEnv, nil, args...)
}

func (g gitRunner) outputWithStdin(extraEnv []string, stdin io.Reader, args ...string) (string, error) {
	out, err := g.command(extraEnv, stdin, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, string(out))
	}
	return string(out), nil
}

func (g gitRunner) stagedPaths() ([]string, error) {
	out, err := g.output(nil, "diff", "--cached", "--name-only", "-z")
	if err != nil {
		return nil, fmt.Errorf("list staged paths: %w", err)
	}
	if out == "" {
		return nil, nil
	}
	parts := strings.Split(out, "\x00")
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			paths = append(paths, part)
		}
	}
	return paths, nil
}

func (g gitRunner) indexPaths(extraEnv []string, path string) ([]string, error) {
	out, err := g.output(extraEnv, "ls-files", "-z", "--", path)
	if err != nil {
		return nil, fmt.Errorf("list index paths for %q: %w", path, err)
	}
	if out == "" {
		return nil, nil
	}
	parts := strings.Split(out, "\x00")
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			paths = append(paths, part)
		}
	}
	return paths, nil
}

func (g gitRunner) command(extraEnv []string, stdin io.Reader, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.workdir
	cmd.Env = append(os.Environ(), extraEnv...)
	if stdin != nil {
		cmd.Stdin = stdin
	}
	return cmd
}
