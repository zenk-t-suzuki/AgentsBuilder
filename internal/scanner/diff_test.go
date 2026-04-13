package scanner

import (
	"testing"

	"agentsbuilder/internal/model"
)

func TestComputeDiffs_BothExist(t *testing.T) {
	global := []model.Asset{
		{Type: model.Skills, Provider: model.ClaudeCode, Scope: model.Global, FilePath: "/home/.claude/commands", Exists: true},
		{Type: model.ClaudeMD, Provider: model.ClaudeCode, Scope: model.Global, FilePath: "/home/.claude/CLAUDE.md", Exists: true},
	}
	project := []model.Asset{
		{Type: model.Skills, Provider: model.ClaudeCode, Scope: model.Project, FilePath: "/proj/.claude/commands", Exists: true},
		{Type: model.ClaudeMD, Provider: model.ClaudeCode, Scope: model.Project, FilePath: "/proj/CLAUDE.md", Exists: true},
	}

	results := ComputeDiffs(global, project)

	for _, r := range results {
		if r.Provider != model.ClaudeCode {
			continue
		}
		switch r.AssetType {
		case model.Skills:
			if !r.HasDiff {
				t.Error("Skills/ClaudeCode: expected HasDiff=true when both exist")
			}
			if r.Priority != model.Project {
				t.Error("Skills/ClaudeCode: expected Project priority")
			}
			if r.GlobalPath != "/home/.claude/commands" {
				t.Errorf("Skills/ClaudeCode: wrong global path: %s", r.GlobalPath)
			}
			if r.ProjectPath != "/proj/.claude/commands" {
				t.Errorf("Skills/ClaudeCode: wrong project path: %s", r.ProjectPath)
			}
		case model.ClaudeMD:
			if !r.HasDiff {
				t.Error("ClaudeMD/ClaudeCode: expected HasDiff=true when both exist")
			}
			if r.Priority != model.Project {
				t.Error("ClaudeMD/ClaudeCode: expected Project priority")
			}
		}
	}
}

func TestComputeDiffs_OnlyGlobal(t *testing.T) {
	global := []model.Asset{
		{Type: model.MCP, Provider: model.ClaudeCode, Scope: model.Global, FilePath: "/home/.claude/settings.json", Exists: true},
	}
	project := []model.Asset{
		{Type: model.MCP, Provider: model.ClaudeCode, Scope: model.Project, FilePath: "/proj/.claude/settings.json", Exists: false},
	}

	results := ComputeDiffs(global, project)

	for _, r := range results {
		if r.AssetType == model.MCP && r.Provider == model.ClaudeCode {
			if r.HasDiff {
				t.Error("MCP/ClaudeCode: expected HasDiff=false when only global exists")
			}
			if r.Priority != model.Global {
				t.Error("MCP/ClaudeCode: expected Global priority when only global exists")
			}
		}
	}
}

func TestComputeDiffs_NeitherExists(t *testing.T) {
	global := []model.Asset{
		{Type: model.Agents, Provider: model.ClaudeCode, Scope: model.Global, FilePath: "/home/.claude/agents", Exists: false},
	}
	project := []model.Asset{
		{Type: model.Agents, Provider: model.ClaudeCode, Scope: model.Project, FilePath: "/proj/.claude/agents", Exists: false},
	}

	results := ComputeDiffs(global, project)

	for _, r := range results {
		if r.AssetType == model.Agents && r.Provider == model.ClaudeCode {
			if r.HasDiff {
				t.Error("Agents/ClaudeCode: expected HasDiff=false when neither exists")
			}
			if r.Priority != model.Project {
				t.Error("Agents/ClaudeCode: expected Project priority as default")
			}
		}
	}
}

func TestComputeDiffs_CoversAllCombinations(t *testing.T) {
	results := ComputeDiffs(nil, nil)
	expected := len(model.AssetTypes()) * len(model.Providers())

	if len(results) != expected {
		t.Fatalf("expected %d results (asset types × providers), got %d", expected, len(results))
	}

	seen := make(map[assetKey]bool)
	for _, r := range results {
		seen[assetKey{r.AssetType, r.Provider}] = true
	}
	for _, at := range model.AssetTypes() {
		for _, p := range model.Providers() {
			if !seen[assetKey{at, p}] {
				t.Errorf("missing diff result for (%v, %v)", at, p)
			}
		}
	}
}
