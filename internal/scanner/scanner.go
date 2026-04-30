package scanner

import (
	"os"
	"path/filepath"
	"strings"

	"agentsbuilder/internal/assetdef"
	"agentsbuilder/internal/model"
)

// ScanGlobal scans the global configuration locations for a given provider
// and returns all assets with their existence and path metadata.
func ScanGlobal(provider model.Provider) []model.Asset {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	defs := assetdef.ForProviderScope(provider, model.Global)
	if provider == model.Codex {
		defs = expandCodexHomePaths(defs)
	}
	return scanAssets(home, defs)
}

// ScanProject scans a project directory for provider-specific assets.
func ScanProject(provider model.Provider, projectPath string) []model.Asset {
	defs := assetdef.ForProviderScope(provider, model.Project)
	if provider == model.Codex {
		defs = expandProjectScanPaths(projectPath, defs, ".agents", "skills")
		defs = expandProjectScanPaths(projectPath, defs, ".codex", "config.toml")
		defs = expandProjectScanPaths(projectPath, defs, ".codex", "agents")
	} else if provider == model.ClaudeCode {
		defs = expandProjectScanPaths(projectPath, defs, ".claude", "skills")
		defs = expandProjectScanPaths(projectPath, defs, ".claude", "commands")
		defs = expandProjectScanPaths(projectPath, defs, ".claude", "agents")
	}
	return scanAssets(projectPath, defs)
}

// scanAssets checks each known asset definition under basePath and returns
// populated Asset structs, including individual Items for directory/config assets.
//
// For definitions with multiple ScanPaths, items are merged from every path that
// exists. The primary display path (Asset.FilePath) is set to the first path
// that exists, falling back to the first listed path when none exist.
func scanAssets(basePath string, defs []assetdef.AssetDef) []model.Asset {
	assets := make([]model.Asset, 0, len(defs))
	for _, def := range defs {
		var primaryPath string
		var exists bool
		var mergedItems []model.AssetItem

		for i, relPath := range def.ScanPaths {
			fullPath := scopedPath(basePath, relPath)
			if i == 0 {
				primaryPath = fullPath // default display path
			}
			if !pathExistsForDef(fullPath, def) {
				continue
			}
			if !exists {
				primaryPath = fullPath // first existing path wins for display
				exists = true
			}
			// Scan items from this path and merge them in.
			tmp := model.Asset{
				Type:     def.Type,
				Provider: def.Provider,
				Scope:    def.Scope,
				FilePath: fullPath,
				Exists:   true,
				Active:   true,
			}
			scanItemsForPath(&tmp, &def, relPath)
			mergedItems = append(mergedItems, tmp.Items...)
		}

		assets = append(assets, model.Asset{
			Type:     def.Type,
			Provider: def.Provider,
			Scope:    def.Scope,
			FilePath: primaryPath,
			Exists:   exists,
			Active:   exists,
			Items:    mergedItems,
		})
	}
	return assets
}

func scopedPath(basePath, relPath string) string {
	if filepath.IsAbs(relPath) {
		return relPath
	}
	return filepath.Join(basePath, relPath)
}

func expandCodexHomePaths(defs []assetdef.AssetDef) []assetdef.AssetDef {
	codexHome := os.Getenv("CODEX_HOME")
	if codexHome == "" {
		return defs
	}
	codexHome = filepath.Clean(codexHome)
	if !filepath.IsAbs(codexHome) {
		if abs, err := filepath.Abs(codexHome); err == nil {
			codexHome = abs
		}
	}
	defaultCodexHome := ""
	if home, err := os.UserHomeDir(); err == nil {
		defaultCodexHome = filepath.Join(home, ".codex")
	}
	if codexHome == defaultCodexHome {
		return defs
	}
	for i := range defs {
		var expanded []string
		for _, p := range defs[i].ScanPaths {
			if strings.HasPrefix(p, ".codex/") {
				expanded = append(expanded, filepath.Join(codexHome, strings.TrimPrefix(p, ".codex/")))
			}
			expanded = append(expanded, p)
		}
		defs[i].ScanPaths = dedupePaths(expanded)
	}
	return defs
}

func dedupePaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func pathExistsForDef(path string, def assetdef.AssetDef) bool {
	if def.Storage == assetdef.CodexAgentRoles {
		_, err := os.Stat(path)
		return err == nil
	}
	return pathExists(path, def.IsFile())
}

func expandProjectScanPaths(projectPath string, defs []assetdef.AssetDef, configDir, leaf string) []assetdef.AssetDef {
	extra := discoverProjectConfigPaths(projectPath, configDir, leaf)
	if len(extra) == 0 {
		return defs
	}
	for i := range defs {
		if !definitionUsesPath(defs[i], filepath.Join(configDir, leaf)) {
			continue
		}
		seen := make(map[string]bool, len(defs[i].ScanPaths)+len(extra))
		for _, p := range defs[i].ScanPaths {
			seen[p] = true
		}
		for _, p := range extra {
			if !seen[p] {
				defs[i].ScanPaths = append(defs[i].ScanPaths, p)
				seen[p] = true
			}
		}
	}
	return defs
}

func definitionUsesPath(def assetdef.AssetDef, relPath string) bool {
	for _, p := range def.ScanPaths {
		if p == relPath {
			return true
		}
	}
	return false
}

func discoverProjectConfigPaths(projectPath, configDir, leaf string) []string {
	var paths []string
	root := filepath.Clean(projectPath)
	targetSuffix := string(filepath.Separator) + filepath.Join(configDir, leaf)
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			if leaf != filepath.Base(path) || !strings.HasSuffix(path, targetSuffix) {
				return nil
			}
		}
		name := d.Name()
		if d.IsDir() && (name == ".git" || name == "node_modules" || name == "vendor") {
			return filepath.SkipDir
		}
		if !strings.HasSuffix(path, targetSuffix) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err == nil && rel != "." {
			paths = append(paths, rel)
		}
		return nil
	})
	return paths
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
