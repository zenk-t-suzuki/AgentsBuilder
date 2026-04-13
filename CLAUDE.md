# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

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
3. **Template creation**: Create new asset directory structures from named, reusable templates (predefined only — user-defined templates excluded from MVP). Selection via checkboxes (assets by type, provider). Results are written immediately to the selected scope.
4. **Project management**: Add/remove projects from within the TUI.

### Out of Scope (MVP)

- Marketplace/registry integration
- In-TUI editing
- Non-interactive commands (`scan`, `validate`, etc.)
- Team sharing/distribution
- Windows/macOS support
