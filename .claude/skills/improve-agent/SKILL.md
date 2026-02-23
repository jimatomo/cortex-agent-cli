---
description: フィードバックを分析してエージェントの改善案を提案し、承認があれば YAML を修正して apply する
allowed-tools:
  - Bash
  - Read
  - Edit
  - Write
  - Glob
  - AskUserQuestion
---

Analyze user feedback for a Cortex Agent, propose improvements to the agent spec, and apply them upon approval.

Usage: `/improve <agent-name> [-d DATABASE] [-s SCHEMA] [--limit N]`

Parse the arguments from: `$ARGUMENTS`

## Steps

### Step 1: フィードバック収集

引数から `<agent-name>`、`-d`/`--database`、`-s`/`--schema`、`--limit` を抽出して使用する。
`-d` / `-s` が指定されていない場合はオプションを省略して実行する（デフォルト接続設定が使われる）。

> **Note**: `feedback` コマンドはローカルキャッシュ（`~/.coragent/feedback/<agent>.json`）を使用する。
> チェック済みのレコードはデフォルトでは表示されない。
> **データ取得には必ず `--json` を付けること**（インタラクティブなプロンプトをスキップし、構造化 JSON を取得するため）。

まず全フィードバック（チェック済み含む・全センチメント）の件数を確認する：

```bash
coragent feedback <agent-name> [--database DB] [--schema SCHEMA] --all --include-checked --json [--limit N]
```

次に未チェックのネガティブフィードバックを取得する（改善分析に使用）：

```bash
coragent feedback <agent-name> [--database DB] [--schema SCHEMA] --json [--limit N]
```

取得した JSON を解析して `sentiment`、`comment`、`question`、`response` を読み取る。
未チェックのネガティブフィードバックが 0 件の場合は「現時点では改善提案に必要なフィードバックがありません。」と伝えて処理を終了する。

### Step 2: 現在のエージェントスペックの取得

Glob で `**/*.yml` と `**/*.yaml` を検索し、ローカルに `<agent-name>` に対応する YAML ファイルが存在するか確認する。

- **ローカルファイルがある場合**: `Read` でそのファイルを読み込む。ファイルパスを記録しておく。
- **ローカルファイルがない場合**: `coragent export <agent-name> [--database DB] [--schema SCHEMA]` でリモートから取得してスペック内容を確認する。この時点ではファイルは作成しない。

### Step 3: 改善案の分析・提案

収集したフィードバックとエージェントスペックを照合し、以下を分析・提案する：

1. **フィードバックのパターン分類**
   - 回答精度の問題（誤った情報、不完全な回答）
   - スコープ外の質問への対応
   - 応答スタイルの問題（長すぎる/短すぎる、わかりにくい）
   - その他のカテゴリ

2. **エージェントスペックの改善ポイント**（特に `instructions` セクション）
   - 現状の `instructions` の課題
   - 追記・修正すべき内容

3. **具体的な修正案**
   - before/after で `instructions` の変更内容を明示する

フィードバックが存在しても、スペック修正によって解決できない問題（例：データソースの不足）は、修正提案の対象外とし、別途コメントとして伝える。

### Step 4: YAML 修正の許可取得

スペック修正が効果的と判断した場合、`AskUserQuestion` でユーザーに許可を求める。

提示する情報：
- 修正箇所（例：`instructions` の何行目付近）
- before（修正前）
- after（修正後）

選択肢には必ず「修正しない」を含める。

### Step 5: YAML ファイルの修正（承認時のみ）

ユーザーが修正を承認した場合：

1. **ローカルファイルが存在する場合**: `Edit` ツールで直接修正する。
2. **ローカルファイルが存在しない場合**: まず export してファイルを生成する：
   ```bash
   coragent export <agent-name> [--database DB] [--schema SCHEMA] --out <agent-name>.yml
   ```
   その後 `Edit` ツールで修正する。

修正後、変更内容を改めて表示する。

### Step 6: apply の実行許可取得と実行

YAML 修正後、`AskUserQuestion` で apply を実行するか確認する。

承認された場合、以下を実行する：

```bash
coragent apply <path-to-yaml> [--database DB] [--schema SCHEMA]
```

apply が成功したら完了を伝える。

### Step 7: フィードバックをチェック済みにする

`AskUserQuestion` で、今回分析したフィードバックをチェック済みにするか確認する。

承認された場合、以下を実行する（`-y` で確認プロンプトを自動スキップ）：

```bash
coragent feedback <agent-name> [--database DB] [--schema SCHEMA] -y
```

これにより、今回レビューした未チェックのネガティブフィードバックが全てチェック済みになり、次回の `feedback` / `improve` 実行時には表示されなくなる。
