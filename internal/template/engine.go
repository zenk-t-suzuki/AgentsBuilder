package template

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agentsbuilder/internal/assetdef"
	"agentsbuilder/internal/model"
)

// copyTemplateFiles copies every file from srcDir into dstDir, preserving the
// relative directory structure. Files under __merge__/ are skipped during the
// walk and handled separately via mergeEmbeddedItems.
// Missing srcDir is silently ignored.
func copyTemplateFiles(srcDir, dstDir string) error {
	info, err := os.Stat(srcDir)
	if err != nil || !info.IsDir() {
		return nil // no files bundled — not an error
	}
	return filepath.Walk(srcDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		// Skip merge items — they are handled by mergeEmbeddedItems.
		if strings.HasPrefix(rel, "__merge__") {
			return nil
		}
		dst := filepath.Join(dstDir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return copyFile(path, dst)
	})
}

// mergeEmbeddedItems processes embedded (merge) items from the template manifest.
// Each merge item's snippet file is merged into the appropriate config file
// at the target path, determined by asset type, provider, and scope.
func mergeEmbeddedItems(filesDir, dstDir string, items []templateFileItem, scope model.Scope) error {
	for _, item := range items {
		if item.Merge == nil {
			continue
		}
		snippetPath := filepath.Join(filesDir, item.RelPath)
		targetRel := mergeTargetPath(item.Merge.AssetType, item.Merge.Provider, scope, dstDir)
		if targetRel == "" {
			continue
		}
		targetPath := filepath.Join(dstDir, targetRel)

		if strings.HasSuffix(item.RelPath, ".toml") {
			if err := mergeTOMLSnippet(snippetPath, targetPath); err != nil {
				return fmt.Errorf("merging %s: %w", item.RelPath, err)
			}
		} else {
			if err := mergeJSONSnippet(snippetPath, targetPath); err != nil {
				return fmt.Errorf("merging %s: %w", item.RelPath, err)
			}
		}
	}
	return nil
}

// mergeTargetPath determines the config file to merge into.
// It uses assetdef.Lookup to find the ScanPaths for the given asset type,
// preferring an existing file, falling back to the PrimaryPath.
func mergeTargetPath(assetType, provider string, scope model.Scope, targetRoot string) string {
	at, ok := model.ParseAssetType(assetType)
	if !ok {
		return ""
	}
	pv, ok := model.ParseProvider(provider)
	if !ok {
		return ""
	}
	def, ok := assetdef.Lookup(pv, scope, at)
	if !ok || !def.IsEmbedded() {
		return ""
	}
	// Prefer an existing file from the scan paths.
	for _, c := range def.ScanPaths {
		if _, err := os.Stat(filepath.Join(targetRoot, c)); err == nil {
			return c
		}
	}
	return def.PrimaryPath
}

// mergeJSONSnippet reads a JSON snippet and merges its top-level keys into
// the target JSON file. If the target doesn't exist, it is created.
// For object-valued keys, entries are merged (added/overwritten); for other
// types, the snippet value replaces the existing one.
func mergeJSONSnippet(snippetPath, targetPath string) error {
	snippetData, err := os.ReadFile(snippetPath)
	if err != nil {
		return err
	}
	var snippet map[string]json.RawMessage
	if err := json.Unmarshal(snippetData, &snippet); err != nil {
		return fmt.Errorf("parsing snippet: %w", err)
	}

	var target map[string]json.RawMessage
	if data, err := os.ReadFile(targetPath); err == nil {
		if err := json.Unmarshal(data, &target); err != nil {
			return fmt.Errorf("parsing target %s: %w", targetPath, err)
		}
	} else {
		target = make(map[string]json.RawMessage)
	}

	for key, value := range snippet {
		existing, ok := target[key]
		if !ok {
			target[key] = value
			continue
		}
		// Try to merge as objects.
		var existingMap, snippetMap map[string]json.RawMessage
		if json.Unmarshal(existing, &existingMap) == nil && json.Unmarshal(value, &snippetMap) == nil {
			for k, v := range snippetMap {
				existingMap[k] = v
			}
			merged, err := json.Marshal(existingMap)
			if err != nil {
				return err
			}
			target[key] = merged
		} else {
			target[key] = value
		}
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	result, err := json.MarshalIndent(target, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(targetPath, append(result, '\n'), 0o644)
}

// mergeTOMLSnippet appends a TOML snippet to the target file.
func mergeTOMLSnippet(snippetPath, targetPath string) error {
	snippetData, err := os.ReadFile(snippetPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(targetPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.WriteString("\n"); err != nil {
		return err
	}
	_, err = f.Write(snippetData)
	return err
}

// ApplyTemplate creates the directory structure defined by the template
// in the target path. For Global scope, targetPath should be the user's
// home directory. For Project scope, it should be the project root.
func ApplyTemplate(tmpl model.Template, targetPath string, scope model.Scope) error {
	if err := ValidateTemplate(tmpl); err != nil {
		return err
	}

	for _, provider := range tmpl.Providers {
		for _, assetType := range tmpl.Assets {
			def, ok := assetdef.Lookup(provider, scope, assetType)
			if !ok {
				continue
			}
			dir := def.ParentDir()
			if dir == "" {
				continue
			}
			fullPath := filepath.Join(targetPath, dir)
			if err := os.MkdirAll(fullPath, 0o755); err != nil {
				return fmt.Errorf("creating directory %s: %w", fullPath, err)
			}
		}
	}

	// If this template was created via the in-app wizard it bundles actual files.
	// Copy them into the target directory tree and merge embedded items.
	if tmpl.TemplateDir != "" {
		filesDir := filepath.Join(tmpl.TemplateDir, "files")
		_ = copyTemplateFiles(filesDir, targetPath)

		items := loadManifestItems(tmpl.TemplateDir)
		_ = mergeEmbeddedItems(filesDir, targetPath, items, scope)
	}

	return nil
}

// ValidateTemplate checks that a template has valid, non-empty fields.
func ValidateTemplate(tmpl model.Template) error {
	if tmpl.Name == "" {
		return fmt.Errorf("template name cannot be empty")
	}
	if len(tmpl.Assets) == 0 {
		return fmt.Errorf("template must include at least one asset type")
	}
	if len(tmpl.Providers) == 0 {
		return fmt.Errorf("template must include at least one provider")
	}
	return nil
}

// ListTemplates returns all predefined templates.
func ListTemplates() []model.Template {
	return model.PredefinedTemplates()
}
