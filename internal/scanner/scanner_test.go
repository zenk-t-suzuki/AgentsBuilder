package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"agentsbuilder/internal/model"
)

func TestScanProject_ClaudeCode(t *testing.T) {
	dir := t.TempDir()

	// Create some Claude Code project assets.
	os.MkdirAll(filepath.Join(dir, ".claude", "commands"), 0o755)
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Claude"), 0o644)

	assets := ScanProject(model.ClaudeCode, dir)
	if len(assets) != 6 {
		t.Fatalf("expected 6 assets, got %d", len(assets))
	}

	found := map[model.AssetType]bool{}
	for _, a := range assets {
		if a.Provider != model.ClaudeCode {
			t.Errorf("expected ClaudeCode provider, got %v", a.Provider)
		}
		if a.Scope != model.Project {
			t.Errorf("expected Project scope, got %v", a.Scope)
		}
		found[a.Type] = a.Exists
	}

	if !found[model.Skills] {
		t.Error("Skills should exist")
	}
	if !found[model.ClaudeMD] {
		t.Error("CLAUDE.md should exist")
	}
	if found[model.MCP] {
		t.Error("MCP should not exist (settings.json not created)")
	}
	if found[model.Hooks] {
		t.Error("Hooks should not exist (settings.json not created)")
	}
	if found[model.AgentsMD] {
		t.Error("AGENTS.md should not exist")
	}
}

func TestScanProject_Codex(t *testing.T) {
	dir := t.TempDir()

	// Codex skills live at .agents/skills/ (not .codex/skills/) per
	// codex-rs/core-skills/src/loader.rs.
	os.MkdirAll(filepath.Join(dir, ".agents", "skills"), 0o755)
	os.MkdirAll(filepath.Join(dir, ".codex"), 0o755)
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Agents"), 0o644)
	os.WriteFile(filepath.Join(dir, ".codex", "config.toml"), []byte(`[mcp_servers.docs]
command = "npx"

[hooks.lint]
event = "after_tool_use"
command = "make lint"

[agents.roles.reviewer]
description = "Reviews changes"
`), 0o644)

	// Codex / Project supports Skills, Agents, MCP, Hooks and AgentsMD.
	assets := ScanProject(model.Codex, dir)
	if len(assets) != 5 {
		t.Fatalf("expected 5 assets (Skills, Agents, MCP, Hooks, AgentsMD), got %d", len(assets))
	}

	found := map[model.AssetType]bool{}
	for _, a := range assets {
		found[a.Type] = a.Exists
	}

	if !found[model.Skills] {
		t.Error("Skills should exist (.agents/skills was created)")
	}
	if !found[model.AgentsMD] {
		t.Error("AGENTS.md should exist")
	}
	if !found[model.Agents] {
		t.Error("Agents should exist (.codex/config.toml has [agents.roles])")
	}
	if !found[model.MCP] {
		t.Error("MCP should exist (.codex/config.toml has [mcp_servers])")
	}
	if !found[model.Hooks] {
		t.Error("Hooks should exist (.codex/config.toml has [hooks])")
	}
	// Codex has no CLAUDE.md.
	if found[model.ClaudeMD] {
		t.Error("ClaudeMD should not be tracked for Codex")
	}
}

func TestScanProject_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	assets := ScanProject(model.ClaudeCode, dir)
	for _, a := range assets {
		if a.Exists {
			t.Errorf("asset %v should not exist in empty dir", a.Type)
		}
		if a.Active {
			t.Errorf("asset %v should not be active in empty dir", a.Type)
		}
	}
}

func TestScanGlobal_CodexHome(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	codexHome := filepath.Join(dir, "custom-codex")
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", codexHome)

	os.MkdirAll(filepath.Join(codexHome, "agents"), 0o755)
	os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte("[mcp_servers.docs]\ncommand = \"npx\"\n"), 0o644)
	os.WriteFile(filepath.Join(codexHome, "agents", "reviewer.toml"), []byte("name = \"reviewer\"\n"), 0o644)

	assets := ScanGlobal(model.Codex)
	found := map[model.AssetType]model.Asset{}
	for _, a := range assets {
		found[a.Type] = a
	}
	if !found[model.MCP].Exists || found[model.MCP].FilePath != filepath.Join(codexHome, "config.toml") {
		t.Fatalf("expected MCP from CODEX_HOME, got %#v", found[model.MCP])
	}
	if !found[model.Agents].Exists || found[model.Agents].FilePath != filepath.Join(codexHome, "agents") {
		t.Fatalf("expected agents from CODEX_HOME, got %#v", found[model.Agents])
	}
}

func TestPathExists(t *testing.T) {
	dir := t.TempDir()

	filePath := filepath.Join(dir, "test.txt")
	os.WriteFile(filePath, []byte("hello"), 0o644)

	subDir := filepath.Join(dir, "subdir")
	os.MkdirAll(subDir, 0o755)

	if !pathExists(filePath, true) {
		t.Error("file should exist as file")
	}
	if pathExists(filePath, false) {
		t.Error("file should not exist as directory")
	}
	if !pathExists(subDir, false) {
		t.Error("directory should exist as directory")
	}
	if pathExists(subDir, true) {
		t.Error("directory should not exist as file")
	}
	if pathExists(filepath.Join(dir, "nonexistent"), true) {
		t.Error("nonexistent path should not exist")
	}
}
