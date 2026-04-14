package template

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"agentsbuilder/internal/config"
	"agentsbuilder/internal/model"
)

// DefaultTemplateDirName is the name of the built-in default user template.
// The leading '*' causes it to sort first alphabetically in the directory listing.
const DefaultTemplateDirName = "*default"

// templateFile is the on-disk JSON representation of a user-defined template.
// Create ~/.agentsbuilder/templates/<name>/template.json to add a custom template.
//
// Example template.json:
//
//	{
//	  "name": "my-template",
//	  "description": "My custom setup",
//	  "assets": ["Skills", "ClaudeMD"],
//	  "providers": ["ClaudeCode"]
//	}
//
// Valid asset values:  Skills, Agents, MCP, Plugins, Hooks, AgentsMD, ClaudeMD
// Valid provider values: ClaudeCode, Codex
type templateFile struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Assets      []string `json:"assets"`
	Providers   []string `json:"providers"`
}

// EnsureDefaultTemplate creates ~/.agentsbuilder/templates/*default/template.json
// on first run so users have a concrete example to copy from.
// Errors are non-fatal — built-in predefined templates remain available.
func EnsureDefaultTemplate() error {
	dir, err := config.TemplatesDir()
	if err != nil {
		return err
	}
	defaultDir := filepath.Join(dir, DefaultTemplateDirName)
	if err := os.MkdirAll(defaultDir, 0o755); err != nil {
		return fmt.Errorf("creating templates directory: %w", err)
	}

	tmplPath := filepath.Join(defaultDir, "template.json")
	if _, err := os.Stat(tmplPath); err == nil {
		return nil // already exists
	}

	f := templateFile{
		Name:        DefaultTemplateDirName,
		Description: "Basic Claude Code setup with Skills and CLAUDE.md",
		Assets:      []string{"Skills", "ClaudeMD"},
		Providers:   []string{"ClaudeCode"},
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(tmplPath, data, 0o644)
}

// LoadUserTemplates reads all valid template subdirectories from
// ~/.agentsbuilder/templates/ and returns the parsed templates.
// Subdirectories that lack a template.json or contain invalid JSON are skipped silently.
func LoadUserTemplates() []model.Template {
	dir, err := config.TemplatesDir()
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var templates []model.Template
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		tmplPath := filepath.Join(dir, entry.Name(), "template.json")
		data, err := os.ReadFile(tmplPath)
		if err != nil {
			continue
		}
		var f templateFile
		if err := json.Unmarshal(data, &f); err != nil {
			continue
		}
		tmpl, err := fileToTemplate(f)
		if err != nil {
			continue
		}
		tmpl.UserDefined = true
		templates = append(templates, tmpl)
	}
	return templates
}

// fileToTemplate converts the on-disk templateFile to a model.Template.
func fileToTemplate(f templateFile) (model.Template, error) {
	if f.Name == "" {
		return model.Template{}, errors.New("name is empty")
	}

	assetNames := map[string]model.AssetType{
		"Skills":       model.Skills,
		"Agents":       model.Agents,
		"CustomAgents": model.Agents,
		"MCP":          model.MCP,
		"Plugins":      model.Plugins,
		"Hooks":        model.Hooks,
		"AgentsMD":     model.AgentsMD,
		"ClaudeMD":     model.ClaudeMD,
	}
	providerNames := map[string]model.Provider{
		"ClaudeCode": model.ClaudeCode,
		"Codex":      model.Codex,
	}

	var assets []model.AssetType
	for _, a := range f.Assets {
		at, ok := assetNames[a]
		if !ok {
			return model.Template{}, fmt.Errorf("unknown asset %q", a)
		}
		assets = append(assets, at)
	}

	var providers []model.Provider
	for _, p := range f.Providers {
		pv, ok := providerNames[p]
		if !ok {
			return model.Template{}, fmt.Errorf("unknown provider %q", p)
		}
		providers = append(providers, pv)
	}

	if len(assets) == 0 {
		return model.Template{}, errors.New("no assets defined")
	}
	if len(providers) == 0 {
		return model.Template{}, errors.New("no providers defined")
	}

	return model.Template{
		Name:        f.Name,
		Description: f.Description,
		Assets:      assets,
		Providers:   providers,
	}, nil
}
