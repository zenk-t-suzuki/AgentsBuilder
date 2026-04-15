---
name: git-workflow
description: AgentsBuilder プロジェクト固有の Git 操作手順。development ブランチへの push 方法と、development → main へのマージ＋タグ付けによるリリース方法を提供する。ユーザーが「push して」「development に上げて」「リリースして」「タグ打って」「デプロイして」「マージして」「本番反映」などと言ったときは必ずこのスキルを使うこと。git 操作の指示であれば、明示的にスキルを指定していなくてもこのスキルを参照すること。
---

# AgentsBuilder Git ワークフロー

このプロジェクトは **development → main** の 2 ブランチ運用。
日常作業は `development` で行い、リリース時のみ `main` にマージしてタグを打つ。

---

## 1. development ブランチへの push

通常の開発成果を `development` に反映するときの手順。

### 手順

```bash
# 1. ブランチ確認（development でなければ切り替える）
git branch --show-current
# → "development" でない場合: git checkout development

# 2. 変更をステージング（関係するファイルだけ指定する）
git add <file1> <file2> ...

# 3. コミット（メッセージは日本語、末尾に Co-Authored-By を付ける）
git commit -m "$(cat <<'EOF'
<変更内容を日本語で簡潔に記述>

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"

# 4. push
git push origin development
```

### コミットメッセージのルール

- **1行目**: 変更内容を日本語で簡潔に（例: `自動アップデート機能を追加`）
- **本文**: 必要なら変更の背景や詳細を続ける
- **末尾必須**: `Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>`
- `git add -A` や `git add .` は使わない（機密ファイルの混入防止）

---

## 2. リリース（development → main へのマージとタグ付け）

機能が揃って公開できる状態になったときの手順。
GitHub Actions がタグを検知してバイナリビルド＆リリースを自動実行する。

### 手順

```bash
# 1. 現在の最新タグを確認してバージョンを決める
git tag --sort=-v:refname | head -1
# → 例: v0.2.0 → 次は v0.3.0（新機能）、v0.2.1（バグ修正）、v1.0.0（破壊的変更）

# 2. main に切り替えてマージ
git checkout main
git merge --no-ff development

# 3. タグを打つ（アノテーション付き）
git tag vX.Y.Z -m "vX.Y.Z: <変更概要を日本語で>"

# 4. main とタグを push
git push origin main && git push origin vX.Y.Z

# 5. development ブランチに戻る
git checkout development
```

### バージョニング規則（セマンティックバージョニング）

| 変更の種類 | 上げるもの | 例 |
|---|---|---|
| 後方互換性のない変更 | **major** | v0.2.0 → v1.0.0 |
| 新機能追加（互換あり） | **minor** | v0.2.0 → v0.3.0 |
| バグ修正のみ | **patch** | v0.2.0 → v0.2.1 |

### リリース後の確認

- GitHub Actions が `vX.Y.Z` タグで起動し `agentsbuilder-linux-amd64` / `agentsbuilder-linux-arm64` をビルド・公開する
- `install.sh` のワンライナーで最新版が取得できることを確認できる

---

## 注意事項

- `--no-verify` や `--force` は使わない
- `main` への直接 push や `git push --force` は禁止
- コミット前に `git diff HEAD` で変更内容を確認する習慣をつける
- `.env` や認証情報を含むファイルは絶対にステージングしない
