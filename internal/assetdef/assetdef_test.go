package assetdef

import (
	"testing"

	"agentsbuilder/internal/model"
)

func TestLookup(t *testing.T) {
	tests := []struct {
		provider model.Provider
		scope    model.Scope
		asset    model.AssetType
		wantOK   bool
		wantPri  string
		wantKind StorageKind
	}{
		{model.ClaudeCode, model.Global, model.Skills, true, ".claude/commands", DirListing},
		{model.ClaudeCode, model.Global, model.MCP, true, ".claude.json", EmbeddedJSON},
		{model.ClaudeCode, model.Project, model.MCP, true, ".mcp.json", EmbeddedJSON},
		// Codex skills live at .agents/skills (not .codex/skills) per
		// codex-rs/core-skills/src/loader.rs.
		{model.Codex, model.Global, model.Skills, true, ".agents/skills", DirListing},
		{model.Codex, model.Project, model.Skills, true, ".agents/skills", DirListing},
		{model.Codex, model.Global, model.MCP, true, ".codex/config.toml", EmbeddedTOML},
		{model.Codex, model.Global, model.Hooks, true, ".codex/config.toml", EmbeddedTOML},
		{model.Codex, model.Global, model.Plugins, true, ".codex/config.toml", EmbeddedTOML},
		{model.ClaudeCode, model.Project, model.ClaudeMD, true, "CLAUDE.md", SingleFile},
		// Codex has no ClaudeMD
		{model.Codex, model.Project, model.ClaudeMD, false, "", 0},
		// Codex's config.toml is global only — project has no MCP/Hooks/Plugins.
		{model.Codex, model.Project, model.MCP, false, "", 0},
		{model.Codex, model.Project, model.Hooks, false, "", 0},
		{model.Codex, model.Project, model.Plugins, false, "", 0},
		// Codex has no separate agents/ directory.
		{model.Codex, model.Global, model.Agents, false, "", 0},
		{model.Codex, model.Project, model.Agents, false, "", 0},
	}
	for _, tt := range tests {
		def, ok := Lookup(tt.provider, tt.scope, tt.asset)
		if ok != tt.wantOK {
			t.Errorf("Lookup(%v,%v,%v) ok=%v, want %v", tt.provider, tt.scope, tt.asset, ok, tt.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if def.PrimaryPath != tt.wantPri {
			t.Errorf("Lookup(%v,%v,%v).PrimaryPath=%q, want %q", tt.provider, tt.scope, tt.asset, def.PrimaryPath, tt.wantPri)
		}
		if def.Storage != tt.wantKind {
			t.Errorf("Lookup(%v,%v,%v).Storage=%v, want %v", tt.provider, tt.scope, tt.asset, def.Storage, tt.wantKind)
		}
	}
}

func TestForProviderScope(t *testing.T) {
	defs := ForProviderScope(model.ClaudeCode, model.Project)
	if len(defs) != 6 {
		t.Fatalf("ClaudeCode Project: got %d defs, want 6", len(defs))
	}
	for _, d := range defs {
		if d.Provider != model.ClaudeCode || d.Scope != model.Project {
			t.Errorf("unexpected def: %v/%v", d.Provider, d.Scope)
		}
	}

	defs = ForProviderScope(model.Codex, model.Project)
	if len(defs) != 2 {
		t.Fatalf("Codex Project: got %d defs, want 2 (Skills, AgentsMD)", len(defs))
	}

	defs = ForProviderScope(model.Codex, model.Global)
	if len(defs) != 5 {
		t.Fatalf("Codex Global: got %d defs, want 5 (Skills, MCP, Hooks, Plugins, AgentsMD)", len(defs))
	}
}

func TestIsEmbedded(t *testing.T) {
	def, _ := Lookup(model.ClaudeCode, model.Global, model.MCP)
	if !def.IsEmbedded() {
		t.Error("MCP should be embedded")
	}
	if def.Key == nil || def.Key.JSONKey != "mcpServers" {
		t.Error("MCP should have JSONKey mcpServers")
	}

	def, _ = Lookup(model.ClaudeCode, model.Global, model.Skills)
	if def.IsEmbedded() {
		t.Error("Skills should not be embedded")
	}
}

func TestLookupAny(t *testing.T) {
	// MCP exists in both Project and Global — LookupAny should return Project first.
	def, ok := LookupAny(model.ClaudeCode, model.MCP)
	if !ok {
		t.Fatal("LookupAny(ClaudeCode, MCP) not found")
	}
	if def.Scope != model.Project {
		t.Errorf("LookupAny should prefer Project scope, got %v", def.Scope)
	}
	if def.PrimaryPath != ".mcp.json" {
		t.Errorf("PrimaryPath=%q, want .mcp.json", def.PrimaryPath)
	}

	// Plugins only exists in Global for ClaudeCode — LookupAny should fall back.
	def, ok = LookupAny(model.ClaudeCode, model.Plugins)
	if !ok {
		t.Fatal("LookupAny(ClaudeCode, Plugins) not found")
	}
	if def.Scope != model.Global {
		t.Errorf("LookupAny should fall back to Global for Plugins, got %v", def.Scope)
	}

	// ClaudeMD doesn't exist for Codex at all.
	_, ok = LookupAny(model.Codex, model.ClaudeMD)
	if ok {
		t.Error("LookupAny(Codex, ClaudeMD) should not be found")
	}
}

func TestIsEmbeddedConvenience(t *testing.T) {
	if !IsEmbedded(model.ClaudeCode, model.MCP) {
		t.Error("MCP should be embedded")
	}
	if !IsEmbedded(model.Codex, model.MCP) {
		t.Error("Codex MCP should be embedded")
	}
	if IsEmbedded(model.ClaudeCode, model.Skills) {
		t.Error("Skills should not be embedded")
	}
	if IsEmbedded(model.Codex, model.ClaudeMD) {
		t.Error("non-existent combo should not be embedded")
	}
}

func TestDestDir(t *testing.T) {
	tests := []struct {
		provider model.Provider
		asset    model.AssetType
		want     string
	}{
		{model.ClaudeCode, model.Skills, ".claude/commands"},
		{model.ClaudeCode, model.MCP, ""},          // .mcp.json → parent is "." → ""
		{model.ClaudeCode, model.Agents, ".claude/agents"},
		{model.Codex, model.Skills, ".agents/skills"},
		{model.Codex, model.ClaudeMD, ""},          // not found → ""
		{model.Codex, model.Agents, ""},            // not modeled → ""
	}
	for _, tt := range tests {
		got := DestDir(tt.provider, tt.asset)
		if got != tt.want {
			t.Errorf("DestDir(%v,%v)=%q, want %q", tt.provider, tt.asset, got, tt.want)
		}
	}
}

func TestParentDir(t *testing.T) {
	tests := []struct {
		provider model.Provider
		scope    model.Scope
		asset    model.AssetType
		want     string
	}{
		{model.ClaudeCode, model.Project, model.Skills, ".claude/commands"},
		{model.ClaudeCode, model.Project, model.MCP, ""},       // .mcp.json → dir is "."  → ""
		{model.ClaudeCode, model.Global, model.MCP, ""},          // .claude.json → filepath.Dir = "." → ""
		{model.ClaudeCode, model.Global, model.Plugins, ".claude"}, // .claude/settings.json → ".claude"
		{model.ClaudeCode, model.Project, model.ClaudeMD, ""},  // CLAUDE.md → dir is "." → ""
	}
	for _, tt := range tests {
		def, ok := Lookup(tt.provider, tt.scope, tt.asset)
		if !ok {
			t.Fatalf("Lookup(%v,%v,%v) not found", tt.provider, tt.scope, tt.asset)
		}
		got := def.ParentDir()
		if got != tt.want {
			t.Errorf("ParentDir(%v,%v,%v)=%q, want %q (PrimaryPath=%q)", tt.provider, tt.scope, tt.asset, got, tt.want, def.PrimaryPath)
		}
	}
}
