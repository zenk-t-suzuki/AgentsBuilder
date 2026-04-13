package template

import (
	"fmt"
	"os"
	"path/filepath"

	"agentsbuilder/internal/model"
)

// dirMappings maps asset types to their directory paths for each provider.
// These mirror the scanner's assetLocation definitions.
var dirMappings = map[model.Provider]map[model.AssetType]string{
	model.ClaudeCode: {
		model.Skills:   ".claude/commands",
		model.Agents:   ".claude/agents",
		model.MCP:      ".claude",
		model.AgentsMD: "", // single file, just needs parent dir
		model.ClaudeMD: "", // single file, just needs parent dir
	},
	model.Codex: {
		model.Skills:   ".codex/commands",
		model.Agents:   ".codex/agents",
		model.MCP:      ".codex",
		model.AgentsMD: "",
		model.ClaudeMD: ".codex",
	},
}

// globalDirMappings maps asset types to global-scope directory paths.
var globalDirMappings = map[model.Provider]map[model.AssetType]string{
	model.ClaudeCode: {
		model.Skills:   ".claude/commands",
		model.Agents:   ".claude/agents",
		model.MCP:      ".claude",
		model.AgentsMD: ".claude",
		model.ClaudeMD: ".claude",
	},
	model.Codex: {
		model.Skills:   ".codex/commands",
		model.Agents:   ".codex/agents",
		model.MCP:      ".codex",
		model.AgentsMD: ".codex",
		model.ClaudeMD: ".codex",
	},
}

// ApplyTemplate creates the directory structure defined by the template
// in the target path. For Global scope, targetPath should be the user's
// home directory. For Project scope, it should be the project root.
func ApplyTemplate(tmpl model.Template, targetPath string, scope model.Scope) error {
	if err := ValidateTemplate(tmpl); err != nil {
		return err
	}

	mappings := dirMappings
	if scope == model.Global {
		mappings = globalDirMappings
	}

	for _, provider := range tmpl.Providers {
		providerMap, ok := mappings[provider]
		if !ok {
			return fmt.Errorf("unsupported provider: %v", provider)
		}

		for _, asset := range tmpl.Assets {
			relDir, ok := providerMap[asset]
			if !ok {
				continue
			}
			if relDir == "" {
				// Single-file asset type in project scope; no directory to create
				continue
			}

			fullPath := filepath.Join(targetPath, relDir)
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
