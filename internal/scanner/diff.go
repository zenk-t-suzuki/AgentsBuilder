package scanner

import (
	"sort"

	"agentsbuilder/internal/model"
)

type assetKey struct {
	t model.AssetType
	p model.Provider
}

// ComputeDiffs compares global and project assets per (AssetType, Provider) and
// returns a DiffResult for each combination, indicating scope priority.
func ComputeDiffs(global []model.Asset, project []model.Asset) []model.DiffResult {
	globalByKey := indexByTypeProvider(global)
	projectByKey := indexByTypeProvider(project)

	var results []model.DiffResult
	for _, at := range model.AssetTypes() {
		for _, p := range model.Providers() {
			k := assetKey{at, p}
			g := globalByKey[k]
			proj := projectByKey[k]

			result := model.DiffResult{
				AssetType:     at,
				Provider:      p,
				Priority:      model.Project,
				GlobalPath:    g.FilePath,
				GlobalExists:  g.Exists,
				ProjectPath:   proj.FilePath,
				ProjectExists: proj.Exists,
			}
			result.HasDiff = result.GlobalExists && result.ProjectExists
			result.ItemConflicts = sharedItemNames(g.Items, proj.Items)
			if result.GlobalExists && !result.ProjectExists {
				result.Priority = model.Global
			}
			results = append(results, result)
		}
	}
	return results
}

// indexByTypeProvider creates a lookup map from (AssetType, Provider) to Asset.
func indexByTypeProvider(assets []model.Asset) map[assetKey]model.Asset {
	m := make(map[assetKey]model.Asset, len(assets))
	for _, a := range assets {
		k := assetKey{a.Type, a.Provider}
		if _, exists := m[k]; !exists {
			m[k] = a
		}
	}
	return m
}

func sharedItemNames(global, project []model.AssetItem) []string {
	if len(global) == 0 || len(project) == 0 {
		return nil
	}
	names := make(map[string]bool, len(global))
	for _, item := range global {
		if item.Name != "" {
			names[item.Name] = true
		}
	}
	var shared []string
	seen := make(map[string]bool)
	for _, item := range project {
		if item.Name == "" || !names[item.Name] || seen[item.Name] {
			continue
		}
		seen[item.Name] = true
		shared = append(shared, item.Name)
	}
	sort.Strings(shared)
	return shared
}
