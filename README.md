# ghost-commit

`ghost-commit` と入力すると `hello world` と表示されるだけの Go 製アプリケーション。

## Install

Homebrew でインストールできます。

```sh
brew tap s4na/ghost-commit https://github.com/s4na/ghost-commit
brew install --HEAD ghost-commit
```

## Usage

```sh
$ ghost-commit
hello world
```

## Build from source

```sh
go build -o ghost-commit .
./ghost-commit
```
