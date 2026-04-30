package marketplace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPluginManifest_CodexPluginPath(t *testing.T) {
	dir := t.TempDir()
	manifestDir := filepath.Join(dir, ".codex-plugin")
	os.MkdirAll(manifestDir, 0o755)
	os.WriteFile(filepath.Join(manifestDir, "plugin.json"), []byte(`{"name":"codex-plugin","skills":"skills"}`), 0o644)

	pm, err := LoadPluginManifest(dir)
	if err != nil {
		t.Fatalf("LoadPluginManifest: %v", err)
	}
	if pm == nil || pm.Name != "codex-plugin" || pm.SkillsPath != "skills" {
		t.Fatalf("unexpected manifest: %#v", pm)
	}
}

func TestDiscoverComponents_CodexDefaults(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "skills"), 0o755)
	os.WriteFile(filepath.Join(dir, "skills", "SKILL.md"), []byte("---\nname: root\n---\n"), 0o644)
	os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(`{"mcpServers":{"docs":{"command":"npx"}}}`), 0o644)
	os.MkdirAll(filepath.Join(dir, "agents"), 0o755)
	os.WriteFile(filepath.Join(dir, "agents", "reviewer.toml"), []byte("name = \"reviewer\"\n"), 0o644)

	p := Plugin{Name: "codex-plugin", Dir: dir}
	discoverComponents(&p, nil)

	if len(p.Skills) != 1 || p.Skills[0] != filepath.Join(dir, "skills") {
		t.Fatalf("expected root skills package, got %#v", p.Skills)
	}
	if len(p.McpFiles) != 1 || p.McpFiles[0] != filepath.Join(dir, ".mcp.json") {
		t.Fatalf("expected .mcp.json, got %#v", p.McpFiles)
	}
	if len(p.Agents) != 1 || filepath.Base(p.Agents[0]) != "reviewer.toml" {
		t.Fatalf("expected Codex toml agent, got %#v", p.Agents)
	}
}
