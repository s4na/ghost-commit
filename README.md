# ghost-commit

[日本語](README.ja.md)

`ghost-commit` is a CLI for committing alternate file contents without rewriting the actual files in your working tree.

It lets you commit intermediate files produced by an LLM, or files that do not exist in your working tree yet, as content at the paths you choose.

## Install

Install with Homebrew:

```sh
brew tap s4na/ghost-commit https://github.com/s4na/ghost-commit
brew install --HEAD ghost-commit
```

## Usage

Commit another file's contents as `README.md`.

```sh
ghost-commit -m "Update README" --file README.md=/tmp/llm-readme.md
```

Commit content from stdin as a new file.

```sh
cat /tmp/generated-config.yml | ghost-commit -m "Add config" --file config.yml=-
```

Commit a state where a file does not exist.

```sh
ghost-commit -m "Remove old config" --delete old-config.yml
```

`ghost-commit` commits only the virtual files you specify. It does not rewrite your working files or alter unrelated staged changes.

After a ghost commit, `git status` may show differences if your working files differ from the new commit. That happens because the commit changed, not because `ghost-commit` rewrote those files.

## Build from source

```sh
go build -o ghost-commit .
./ghost-commit --help
```
