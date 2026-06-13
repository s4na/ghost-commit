# ghost-commit

`ghost-commit` は、実際のファイルを書き換えずに「別の内容のファイル」をコミットできる CLI です。

LLM が作った中間ファイルや、まだ作業ツリーに存在しないファイルを、そのまま指定したパスの内容としてコミットできます。

## Install

Homebrew でインストールできます。

```sh
brew tap s4na/ghost-commit https://github.com/s4na/ghost-commit
brew install --HEAD ghost-commit
```

## Usage

別ファイルの内容を `README.md` としてコミットします。

```sh
ghost-commit -m "README を更新" --file README.md=/tmp/llm-readme.md
```

標準入力から受け取った内容を、新しいファイルとしてコミットします。

```sh
cat /tmp/generated-config.yml | ghost-commit -m "設定を追加" --file config.yml=-
```

ファイルが存在しない状態もコミットできます。

```sh
ghost-commit -m "古い設定を削除" --delete old-config.yml
```

`ghost-commit` は指定された仮想ファイルだけをコミットします。手元のファイルや、指定していない staging 済みの変更は変更しません。

コミット後に、手元のファイルが ghost commit の内容と違う場合は `git status` に差分として表示されます。これはファイルを書き換えたわけではなく、新しいコミットの内容と手元の状態が違うためです。

## Build from source

```sh
go build -o ghost-commit .
./ghost-commit --help
```
