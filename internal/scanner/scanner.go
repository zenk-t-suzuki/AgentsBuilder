package scanner

import (
	"os"
	"path/filepath"

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
	return scanAssets(home, assetdef.ForProviderScope(provider, model.Global))
}

// ScanProject scans a project directory for provider-specific assets.
func ScanProject(provider model.Provider, projectPath string) []model.Asset {
	return scanAssets(projectPath, assetdef.ForProviderScope(provider, model.Project))
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
			fullPath := filepath.Join(basePath, relPath)
			if i == 0 {
				primaryPath = fullPath // default display path
			}
			if !pathExists(fullPath, def.IsFile()) {
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
			scanItems(&tmp, &def)
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
