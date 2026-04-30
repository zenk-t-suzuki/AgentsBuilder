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
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Agents"), 0o644)

	// Codex / Project supports only Skills + AgentsMD — config.toml
	// (MCP/Hooks/Plugins) is global-only and there is no agents/ dir.
	assets := ScanProject(model.Codex, dir)
	if len(assets) != 2 {
		t.Fatalf("expected 2 assets (Skills, AgentsMD), got %d", len(assets))
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
	// Codex has no CLAUDE.md and no separate Agents asset.
	if found[model.ClaudeMD] {
		t.Error("ClaudeMD should not be tracked for Codex")
	}
	if found[model.Agents] {
		t.Error("Agents should not be tracked for Codex")
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
