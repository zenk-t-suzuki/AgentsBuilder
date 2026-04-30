// Package template applies built-in directory scaffolds (model.PredefinedTemplates)
// to a target path. User-defined templates and embedded-item merge logic have
// been removed in favour of the Claude Code-compatible marketplace plugin
// system (see internal/marketplace).
package template

import (
	"fmt"
	"os"
	"path/filepath"

	"agentsbuilder/internal/assetdef"
	"agentsbuilder/internal/model"
)

// ApplyTemplate creates the directory structure defined by tmpl in targetPath.
// For Global scope, targetPath should be the user's home directory; for
// Project scope, the project root.
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
