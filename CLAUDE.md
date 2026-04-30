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
2. **Diff/Priority visualization**: When the same asset type exists in both Global and Project scopes, show which takes precedence and display file-level diffs.
3. **Template**: Apply built-in directory scaffolds (predefined only — user-defined templates have been replaced by marketplace plugins) to bootstrap a Global or Project scope.
4. **Marketplace**: Register Claude Code-compatible plugin marketplaces (`.claude-plugin/marketplace.json`) and install plugins into Claude Code and/or Codex with a checkbox target picker. Source forms accepted: `owner/repo`, full Git URL with optional `#ref`, local directory or `marketplace.json`, and remote `marketplace.json` URL — matching `/plugin marketplace add`.
5. **Project management**: Add/remove projects from within the TUI.

### Marketplace details

- Cache: `~/.agentsbuilder/cache/marketplaces/<source-key>/`. AgentsBuilder never writes to `~/.claude/`.
- Install copies plugin components into the chosen targets:
  - `skills/<name>/` → `.claude/skills/<name>/` or `.codex/skills/<name>/`
  - `commands/*.md` → `.claude/commands/` (Claude Code only)
  - `agents/*.md` → `.claude/agents/` or `.codex/agents/`
  - `hooks/*.json` → merged into `.claude/settings.json` (Codex skipped)
  - `mcp/*.json` → merged into `.mcp.json` (Claude) or converted to TOML and merged into `.codex/config.toml`
- Plugin sources supported during marketplace load: relative paths (`./plugins/foo`), `github`, `url`, and `git-subdir`. `npm` is not yet supported.

### Out of Scope (MVP)

- In-TUI editing
- Non-interactive commands (`scan`, `validate`, etc.)
- Writing to `~/.claude/settings.json extraKnownMarketplaces` (read-only friendliness preserved)
- npm-based plugin sources
- Windows/macOS support
