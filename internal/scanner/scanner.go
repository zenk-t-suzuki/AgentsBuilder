package scanner

import (
	"os"
	"path/filepath"

	"agentsbuilder/internal/model"
)

// assetLocation maps asset types to their relative paths for each provider.
// RelPaths lists one or more candidate paths in priority order; items are
// merged from every path that exists so that legacy and new locations are
// both surfaced in a single Asset entry.
type assetLocation struct {
	Type     model.AssetType
	RelPaths []string // relative to scope root; all existing paths are merged
	IsFile   bool     // true for single files (e.g. CLAUDE.md), false for directories
}

// claudeCodeGlobalAssets defines where Claude Code stores global assets
// relative to the user's home directory.
func claudeCodeGlobalAssets() []assetLocation {
	// MCP: ~/.claude.json holds mcpServers (primary); ~/.claude/settings.json is legacy fallback.
	// Plugins: ~/.claude/settings.json holds the enabledPlugins map.
	// Skills: ~/.claude/skills/ is the current location; ~/.claude/commands/ is the
	// legacy location — both are scanned and merged.
	return []assetLocation{
		{Type: model.Skills, RelPaths: []string{".claude/skills", ".claude/commands"}, IsFile: false},
		{Type: model.Agents, RelPaths: []string{".claude/agents"}, IsFile: false},
		{Type: model.MCP, RelPaths: []string{".claude.json", ".claude/settings.json"}, IsFile: true},
		{Type: model.Plugins, RelPaths: []string{".claude/settings.json"}, IsFile: true},
		{Type: model.Hooks, RelPaths: []string{".claude/settings.json"}, IsFile: true},
		{Type: model.AgentsMD, RelPaths: []string{".claude/AGENTS.md"}, IsFile: true},
		{Type: model.ClaudeMD, RelPaths: []string{".claude/CLAUDE.md"}, IsFile: true},
	}
}

// claudeCodeProjectAssets defines where Claude Code stores project-level assets
// relative to the project root.
func claudeCodeProjectAssets() []assetLocation {
	// MCP: .mcp.json is the standard project-level file; .claude/settings.json is a fallback.
	// Plugins are user-level (global only) — not included here.
	return []assetLocation{
		{Type: model.Skills, RelPaths: []string{".claude/skills", ".claude/commands"}, IsFile: false},
		{Type: model.Agents, RelPaths: []string{".claude/agents"}, IsFile: false},
		{Type: model.MCP, RelPaths: []string{".mcp.json", ".claude/settings.json"}, IsFile: true},
		{Type: model.Hooks, RelPaths: []string{".claude/settings.json"}, IsFile: true},
		{Type: model.AgentsMD, RelPaths: []string{"AGENTS.md"}, IsFile: true},
		{Type: model.ClaudeMD, RelPaths: []string{"CLAUDE.md"}, IsFile: true},
	}
}

// codexGlobalAssets defines where Codex stores global assets
// relative to the user's home directory.
//
// Codex path notes:
//   - Skills: ~/.codex/skills/ and ~/.agents/skills/ are both scanned and merged.
//   - MCP is configured in ~/.codex/config.toml under [mcp_servers.*] sections.
//   - Plugins are cached in ~/.codex/.tmp/plugins/plugins/; each subdir is a plugin.
//   - AGENTS.md is read from ~/.codex/AGENTS.md for global instructions.
//   - CLAUDE.md has no Codex equivalent and is intentionally omitted.
func codexGlobalAssets() []assetLocation {
	return []assetLocation{
		{Type: model.Skills, RelPaths: []string{".codex/skills", ".agents/skills"}, IsFile: false},
		{Type: model.Agents, RelPaths: []string{".codex/agents"}, IsFile: false},
		{Type: model.MCP, RelPaths: []string{".codex/config.toml"}, IsFile: true},
		{Type: model.Plugins, RelPaths: []string{".codex/.tmp/plugins/plugins"}, IsFile: false},
		{Type: model.AgentsMD, RelPaths: []string{".codex/AGENTS.md"}, IsFile: true},
	}
}

// codexProjectAssets defines where Codex stores project-level assets
// relative to the project root.
//
// Codex path notes:
//   - Skills: .codex/skills/ and .agents/skills/ are both scanned and merged.
//   - MCP/config overrides go in .codex/config.toml (trusted projects only).
//   - Plugins are user-level (global only) — not included here.
//   - AGENTS.md is read from the repo root (Codex walks up from CWD).
//   - CLAUDE.md has no Codex equivalent and is intentionally omitted.
func codexProjectAssets() []assetLocation {
	return []assetLocation{
		{Type: model.Skills, RelPaths: []string{".codex/skills", ".agents/skills"}, IsFile: false},
		{Type: model.Agents, RelPaths: []string{".codex/agents"}, IsFile: false},
		{Type: model.MCP, RelPaths: []string{".codex/config.toml"}, IsFile: true},
		{Type: model.AgentsMD, RelPaths: []string{"AGENTS.md"}, IsFile: true},
	}
}

// globalAssetsFor returns the asset location definitions for a provider at global scope.
func globalAssetsFor(provider model.Provider) []assetLocation {
	switch provider {
	case model.ClaudeCode:
		return claudeCodeGlobalAssets()
	case model.Codex:
		return codexGlobalAssets()
	default:
		return nil
	}
}

// projectAssetsFor returns the asset location definitions for a provider at project scope.
func projectAssetsFor(provider model.Provider) []assetLocation {
	switch provider {
	case model.ClaudeCode:
		return claudeCodeProjectAssets()
	case model.Codex:
		return codexProjectAssets()
	default:
		return nil
	}
}

// ScanGlobal scans the global configuration locations for a given provider
// and returns all assets with their existence and path metadata.
func ScanGlobal(provider model.Provider) []model.Asset {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return scanAssets(provider, model.Global, home, globalAssetsFor(provider))
}

// ScanProject scans a project directory for provider-specific assets.
func ScanProject(provider model.Provider, projectPath string) []model.Asset {
	return scanAssets(provider, model.Project, projectPath, projectAssetsFor(provider))
}

// scanAssets checks each known asset location under basePath and returns
// populated Asset structs, including individual Items for directory/config assets.
//
// For locations with multiple RelPaths, items are merged from every path that
// exists. The primary display path (Asset.FilePath) is set to the first path
// that exists, falling back to the first listed path when none exist.
func scanAssets(provider model.Provider, scope model.Scope, basePath string, locs []assetLocation) []model.Asset {
	assets := make([]model.Asset, 0, len(locs))
	for _, loc := range locs {
		var primaryPath string
		var exists bool
		var mergedItems []model.AssetItem

		for i, relPath := range loc.RelPaths {
			fullPath := filepath.Join(basePath, relPath)
			if i == 0 {
				primaryPath = fullPath // default display path
			}
			if !pathExists(fullPath, loc.IsFile) {
				continue
			}
			if !exists {
				primaryPath = fullPath // first existing path wins for display
				exists = true
			}
			// Scan items from this path and merge them in.
			tmp := model.Asset{
				Type:     loc.Type,
				Provider: provider,
				Scope:    scope,
				FilePath: fullPath,
				Exists:   true,
				Active:   true,
			}
			scanItems(&tmp)
			mergedItems = append(mergedItems, tmp.Items...)
		}

		assets = append(assets, model.Asset{
			Type:     loc.Type,
			Provider: provider,
			Scope:    scope,
			FilePath: primaryPath,
			Exists:   exists,
			Active:   exists,
			Items:    mergedItems,
		})
	}
	return assets
}

// ScanAllGlobal scans global assets for all providers, ordered by AssetType then Provider.
func ScanAllGlobal() []model.Asset {
	return mergeByType(func(p model.Provider) []model.Asset { return ScanGlobal(p) })
}

// ScanAllProject scans project assets for all providers, ordered by AssetType then Provider.
func ScanAllProject(projectPath string) []model.Asset {
	return mergeByType(func(p model.Provider) []model.Asset { return ScanProject(p, projectPath) })
}

// mergeByType collects assets from all providers and returns them sorted
// by AssetType first, then by Provider, so the UI can group by type naturally.
func mergeByType(scan func(model.Provider) []model.Asset) []model.Asset {
	type key struct {
		t model.AssetType
		p model.Provider
	}
	byKey := make(map[key]model.Asset)
	for _, p := range model.Providers() {
		for _, a := range scan(p) {
			byKey[key{a.Type, a.Provider}] = a
		}
	}
	var all []model.Asset
	for _, at := range model.AssetTypes() {
		for _, p := range model.Providers() {
			if a, ok := byKey[key{at, p}]; ok {
				all = append(all, a)
			}
		}
	}
	return all
}

// pathExists checks whether a filesystem path exists and matches the expected type.
func pathExists(path string, isFile bool) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if isFile {
		return !info.IsDir()
	}
	return info.IsDir()
}
