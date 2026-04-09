# ghost-commit

`ghost-commit` と入力すると `hello world` と表示されるだけの Go 製アプリケーション。

## Install

Homebrew でインストールできます。

```sh
brew install s4na/ghost-commit/ghost-commit
```

上記はこのリポジトリを tap として利用する形式です。初回のみ tap の追加が必要です。

```sh
brew tap s4na/ghost-commit https://github.com/s4na/ghost-commit
brew install ghost-commit
```

または Formula を直接指定してインストールすることもできます。

```sh
brew install --HEAD https://raw.githubusercontent.com/s4na/ghost-commit/main/Formula/ghost-commit.rb
```

## Usage

```sh
$ ghost-commit
hello world
```

## Build from source

```sh
go build ./...
./ghost-commit
```
