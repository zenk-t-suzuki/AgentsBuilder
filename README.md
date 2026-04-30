# AgentsBuilder

Claude Code と Codex の設定アセットをターミナル上で一元管理する TUI ツールです。

![見た目](docs/imgs/image.png)

## ユースケース

- **設定の全体像把握** — Global スコープと Project スコープそれぞれのアセット（Skills・Agents・MCP・Hooks・CLAUDE.md など）を一覧表示
- **優先度の確認** — Global と Project で同じ種類や同名アイテムが存在する場合、どちらが優先されるかを表示
- **テンプレート適用** — 定番の設定セット（`claude-code-basic`、`full-claude` など）を一発でディレクトリ構造ごと展開
- **Marketplace 管理** — Claude Code / Codex 形式の marketplace を登録し、プラグインを対象プロバイダへインストール
- **プロジェクト管理** — 複数プロジェクトをアプリ内に登録し、サイドバーで切り替えながら各設定を確認

## 管理対象アセット

| アセット | Claude Code | Codex |
|---------|-------------|-------|
| Skills | Global: `~/.claude/commands/`, `~/.claude/skills/` / Project: `<root>/.claude/commands/`, `<root>/.claude/skills/` | Global: `~/.agents/skills/`, `~/.codex/skills/` / Project: `<root>/.agents/skills/`（祖先ディレクトリも探索） |
| Agents | `.claude/agents/*.md` | `.codex/agents/*.toml` と `.codex/config.toml` の `[agents.roles.<name>]` |
| MCP 設定 | Global: `~/.claude.json`, `~/.claude/settings.json` / Project: `<root>/.mcp.json`, `<root>/.claude/settings.json` | `~/.codex/config.toml` と `<root>/.codex/config.toml` の `[mcp_servers.<name>]` |
| Plugins | `.claude/settings.json` の `enabledPlugins` | `~/.codex/config.toml` の `[plugins."<name>@<marketplace>"]` |
| Hooks | `.claude/settings.json` の `hooks` | `~/.codex/config.toml` と `<root>/.codex/config.toml` の `[hooks.<name>]` |
| AGENTS.md | `~/.claude/AGENTS.md`, `<root>/AGENTS.md` | `~/.codex/AGENTS.override.md` > `~/.codex/AGENTS.md`, `<root>/AGENTS.override.md` > `<root>/AGENTS.md` |
| CLAUDE.md | `~/.claude/CLAUDE.md`, `<root>/CLAUDE.md` | 対象外 |

Global スコープはユーザーのホームディレクトリ配下、Project スコープは登録した各プロジェクトルート配下を参照します。Claude Code と Codex が祖先ディレクトリを探索する設定は、AgentsBuilder でも同じ範囲をスキャンします。

## Marketplace

Claude Code 形式と Codex 形式の marketplace を登録し、対象プロバイダへインストールできます。

- Marketplace manifest: `.claude-plugin/marketplace.json` / `.agents/plugins/marketplace.json`
- Plugin manifest: `.claude-plugin/plugin.json` / `.codex-plugin/plugin.json` / `plugin.json`
- Skills: `skills/<name>/SKILL.md` または Codex の `skills/SKILL.md`
- Agents: Claude Code は `agents/*.md`、Codex は `agents/*.toml`
- MCP: `mcp/*.json` または Codex の `.mcp.json`
- Hooks: JSON の内容から Claude Code / Codex 形式を判定して、対応する設定ファイルへマージします。

## インストール

### ワンライナー（最も簡単）

```bash
curl -fsSL https://raw.githubusercontent.com/zenk-t-suzuki/AgentsBuilder/main/install.sh | sh
```

アーキテクチャ（amd64 / arm64）を自動判別し、最新リリースを `/usr/local/bin/agentsbuilder` にインストールします。

### Go で直接ビルド

```bash
git clone https://github.com/<owner>/AgentsBuilder
cd AgentsBuilder
go build -o agentsbuilder ./cmd/agentsbuilder
sudo mv agentsbuilder /usr/local/bin/
```

> **注意**: `go build` で直接ビルドした場合はバージョン情報が埋め込まれないため、起動時の自動アップデートチェックはスキップされます。
> リリースビルドは以下のように `-ldflags` でバージョンを指定してください。
>
> ```bash
> go build -ldflags="-X main.Version=v1.0.0" -o agentsbuilder ./cmd/agentsbuilder
> ```

Go 1.24 以上が必要です。

### Docker（開発用）

```bash
# ソースを直接マウントして起動
docker compose run --rm dev
```

### Docker（プロダクション）

```bash
# イメージをビルドして起動
docker compose run --rm app

# またはイメージ単体でビルド
docker build -t agentsbuilder .
docker run -it --rm \
  -v ~/.claude.json:/root/.claude.json:ro \
  -v ~/.claude:/root/.claude:ro \
  -v ~/.codex:/root/.codex:ro \
  agentsbuilder
```

## 起動

```bash
agentsbuilder
```

設定ファイルは `~/.agentsbuilder/config.json` に自動生成されます。

## 使い方

### 画面構成

```
┌──────────────┐ ┌─────────────────────────────────────────────────┐
│   Sidebar    │ │              Main Area                          │
│              │ │  [1] Browse  [2] Template  [3] Marketplace      │
│  > Global    │ │  ┌─ Browse Tabs ────────────────────────────┐   │
│    project-A │ │  │ [A]ll [S]kills [C]ustom Agents [M]CP ... │   │
│    project-B │ │  ├─ Asset List ──────────┬─ Detail ─────────┤   │
│              │ │  │                       │                   │   │
│              │ │  │                       │                   │   │
└──────────────┘ └─────────────────────────────────────────────────┘
```

### キーバインド

#### ペイン間移動

| キー | 動作 |
|------|------|
| `Tab` | サイドバー ↔ 直前のメイン要素 を切り替え |
| `←` / `h` | 現在の要素から左隣の要素へ（リスト→サイドバーなど） |
| `→` / `l` | 現在の要素から右隣の要素へ（サイドバー→元の位置など） |
| `↑` / `k` | 上の要素へ（リスト先頭→Browse タブ→Mode タブ） |
| `↓` / `j` | 下の要素へ（Mode タブ→Browse タブ→リスト） |
| `Ctrl+←/→/↑/↓` | 端に関わらず隣接要素へ直接ジャンプ |

#### モード切替

| キー | 動作 |
|------|------|
| `1` | Browse モード |
| `2` | Template モード（テンプレート適用） |
| `3` | Marketplace モード |

#### Browse モード

| キー | 動作 |
|------|------|
| `,` / `.` | Browse 内タブを左右に切り替え |
| `[` / `]` | 詳細パネルをスクロール |
| `Enter` | 選択 |
| `t` | Template モードへ移動 |

#### サイドバー

| キー | 動作 |
|------|------|
| `↑` / `↓` | スコープ / プロジェクトを選択 |
| `Enter` | 選択したスコープをアクティブ化 |
| `a` | プロジェクトを追加 |
| `d` | プロジェクトを削除 |

#### 共通

| キー | 動作 |
|------|------|
| `Esc` | 戻る / キャンセル |
| `q` | 終了 |
| `Ctrl+C` | 強制終了 |

## 技術スタック

- **言語**: Go 1.24
- **TUI フレームワーク**: [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- **スタイリング**: [Lip Gloss](https://github.com/charmbracelet/lipgloss)
- **対応 OS**: Linux（MVP）
