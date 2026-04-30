# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Communication Language

Always respond in **Japanese** regardless of the language used in the question.

## Project Overview

A TUI-based agent configuration management tool for **Claude Code** and **Codex**. Built with **Go + Bubble Tea**. No LLM is embedded — this tool only browses, diffs, and manages configuration assets via a terminal UI. Target platform: **Linux only** (MVP).

## Development (Docker)

```bash
# Run in dev mode (live source mount, go run)
docker compose run --rm dev

# Build production image and run
docker compose run --rm app

# Build only
docker build -t agentsbuilder .
docker run -it --rm agentsbuilder
```

The TUI requires a real TTY — always use `docker run -it` or `docker compose run` (not `up`).
Config dirs (`~/.claude`, `~/.codex`) are mounted read-only so the tool can read real assets.

## Tech Stack

- Language: **Go**
- TUI framework: **Bubble Tea** (github.com/charmbracelet/bubbletea)

## Managed Assets

The tool manages the following configuration assets for each provider (Claude Code / Codex):

- Skills
- Agents
- MCP configurations
- `AGENTS.md`
- `CLAUDE.md`

Settings/profile files are explicitly out of scope for MVP.

## Architecture

### Scopes

Two scopes: **Global** and **Project**. Projects are registered explicitly inside the app (not auto-detected from CWD). Users can manage multiple registered projects and switch between them — similar to VS Code workspace management.

### UI Layout

- **Sidebar**: Global at top, registered Projects listed below. Selecting an item switches the main area content.
- **Main area**: Displays assets for the selected scope, grouped by type. Includes a persistent detail panel showing: asset type, provider, scope, file path, active state, and diff/priority info.
- **Provider tabs**: Toggle between Claude Code and Codex within the main area.
- No tab-based Global/Project switching — sidebar handles this exclusively.

### Key Features (MVP)

1. **Browse**: View all managed assets for the selected Global or Project scope.
2. **Diff/Priority visualization**: When the same asset type or item name exists in both Global and Project scopes, show which scope takes precedence.
3. **Template**: Apply built-in directory scaffolds (predefined only) to bootstrap a Global or Project scope.
4. **Marketplace**: Register plugin marketplaces and install plugins into Claude Code and/or Codex with a checkbox target picker. Source forms accepted: `owner/repo`, full Git URL with optional `#ref`, local directory or `marketplace.json`, and remote `marketplace.json` URL — matching `/plugin marketplace add` (Claude Code) and `.agents/plugins/marketplace.json` (Codex).
5. **Project management**: Add/remove projects from within the TUI.

### Provider asset paths

Paths are derived from the actual provider sources, not inferred:

| Asset | Claude Code | Codex |
|---|---|---|
| Skills (Global) | `~/.claude/commands/`, `~/.claude/skills/` | `~/.agents/skills/`, `~/.codex/skills/` |
| Skills (Project) | `<root>/.claude/commands/`, `<root>/.claude/skills/` (ancestor walk) | `<root>/.agents/skills/` (ancestor walk) |
| Agents | `.claude/agents/<name>.md` | `.codex/agents/<name>.toml` and `.codex/config.toml` `[agents.roles.<name>]` |
| MCP | `~/.claude.json` / `<root>/.mcp.json` (`mcpServers`) | `~/.codex/config.toml` and `<root>/.codex/config.toml` `[mcp_servers.<name>]` |
| Hooks | `~/.claude/settings.json` (`hooks`) | `~/.codex/config.toml` and `<root>/.codex/config.toml` `[hooks.<name>]` |
| Plugins | `.claude/settings.json` (`enabledPlugins`) | `~/.codex/config.toml` `[plugins.<name>]` |
| AGENTS.md | `~/.claude/AGENTS.md`, `<root>/AGENTS.md` | `~/.codex/AGENTS.override.md` > `~/.codex/AGENTS.md`, `<root>/AGENTS.override.md` > `<root>/AGENTS.md` |

Codex project-local `.codex/config.toml` is loaded for trusted projects. Codex plugins remain user-config only.

### Marketplace details

- **Manifest formats supported** (auto-detected by content):
  - Claude Code: `<root>/.claude-plugin/marketplace.json` (requires `owner.name`)
  - Codex: `<root>/.agents/plugins/marketplace.json` (requires per-plugin `policy`)
- **Plugin manifest paths** (auto-detected): `<plugin>/.claude-plugin/plugin.json` (Claude), `<plugin>/.codex-plugin/plugin.json` (Codex), or legacy `<plugin>/plugin.json`. Codex's plugin.json `skills` / `mcpServers` / `hooks` path fields are honoured when present.
- **Cache**: `~/.agentsbuilder/cache/marketplaces/<source-key>/`. AgentsBuilder never writes to `~/.claude/` or `~/.codex/plugins/cache/`.
- **Install mapping**:
  - `skills/<name>/` or Codex `skills/SKILL.md` → `.claude/skills/<name>/` (Claude) or `.agents/skills/<name>/` (Codex)
  - `commands/*.md` → `.claude/commands/` (Claude Code only)
  - `agents/*.md` → `.claude/agents/`; `agents/*.toml` → `.codex/agents/`
  - `hooks/*.json` → format auto-detected. Claude-format snippets (`PreToolUse`/`PostToolUse`/etc.) merge into `.claude/settings.json`; Codex-format snippets (events `after_agent`/`after_tool_use`) merge into `~/.codex/config.toml` `[hooks.<name>]`. Mismatched formats are skipped per-target.
  - `mcp/*.json` → merged into `.mcp.json` (Claude) or converted to TOML and merged into `~/.codex/config.toml` `[mcp_servers.<name>]`. HTTP-transport entries are passed through verbatim — Claude's `bearer_token` / `headers` are written as-is; Codex expects `bearer_token_env_var` / `http_headers` and may not accept untranslated entries.
- **Plugin sources supported**: relative paths (`./plugins/foo`), `github`, `url`, `git-subdir`, plus Codex's tagless `{path}` / `{url}` / `{url, path}` shapes. `npm` is not yet supported.

### Out of Scope (MVP)

- In-TUI editing
- Non-interactive commands (`scan`, `validate`, etc.)
- Writing to `~/.claude/settings.json` `extraKnownMarketplaces` or `~/.codex/config.toml` `[marketplaces]`
- Translating MCP HTTP-transport field names between Claude and Codex
- npm-based plugin sources
- Windows/macOS support
