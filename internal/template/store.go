package template

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"agentsbuilder/internal/config"
	"agentsbuilder/internal/model"
)

// DefaultTemplateDirName is the name of the built-in default user template.
// The leading '*' causes it to sort first alphabetically in the directory listing.
const DefaultTemplateDirName = "*default"

// templateFileItem describes one file bundled inside a user-created template.
type templateFileItem struct {
	RelPath string `json:"relPath"` // path relative to the template's files/ directory
}

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
//
// Templates created via the in-app wizard may also include a "items" array and
// a corresponding files/ sub-directory containing the actual files to copy.
type templateFile struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Assets      []string           `json:"assets"`
	Providers   []string           `json:"providers"`
	Items       []templateFileItem `json:"items,omitempty"`
}

// FileRef holds the data needed to bundle one file into a user template.
// It is accepted by SaveUserTemplate and mirrors tui.TmplItem without
// creating a cross-package import cycle.
type FileRef struct {
	SrcPath    string          // absolute source file path
	DestRelDir string          // relative destination dir for project scope
	Filename   string          // filename at destination
	AssetType  model.AssetType
	Provider   model.Provider
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
		tmplDir := filepath.Join(dir, entry.Name())
		tmplPath := filepath.Join(tmplDir, "template.json")
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
		tmpl.TemplateDir = tmplDir
		templates = append(templates, tmpl)
	}
	return templates
}

// SaveUserTemplate creates a new user template from the given file refs.
// It writes a template.json manifest and copies all referenced source files
// into a files/ sub-directory within the template directory.
func SaveUserTemplate(name string, refs []FileRef) error {
	if name == "" {
		return errors.New("template name cannot be empty")
	}
	if strings.ContainsAny(name, "/\\") {
		return errors.New("template name must not contain path separators")
	}
	if len(refs) == 0 {
		return errors.New("at least one file must be selected")
	}

	dir, err := config.TemplatesDir()
	if err != nil {
		return err
	}
	tmplDir := filepath.Join(dir, name)
	if err := os.MkdirAll(tmplDir, 0o755); err != nil {
		return fmt.Errorf("creating template directory: %w", err)
	}

	// Derive asset types and providers from refs (for the legacy assets/providers fields).
	assetSet := make(map[string]bool)
	providerSet := make(map[string]bool)
	for _, r := range refs {
		assetSet[r.AssetType.String()] = true
		providerSet[r.Provider.String()] = true
	}
	var assets, providers []string
	for _, at := range model.AssetTypes() {
		if assetSet[at.String()] {
			assets = append(assets, at.String())
		}
	}
	for _, p := range model.Providers() {
		if providerSet[p.String()] {
			providers = append(providers, p.String())
		}
	}

	// Copy each file into the template's files/ directory.
	var items []templateFileItem
	for _, r := range refs {
		var relPath string
		if r.DestRelDir != "" {
			relPath = filepath.Join(r.DestRelDir, r.Filename)
		} else {
			relPath = r.Filename
		}
		destPath := filepath.Join(tmplDir, "files", relPath)
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return fmt.Errorf("creating files directory: %w", err)
		}
		if err := copyFile(r.SrcPath, destPath); err != nil {
			return fmt.Errorf("copying %s: %w", r.Filename, err)
		}
		items = append(items, templateFileItem{RelPath: relPath})
	}

	f := templateFile{
		Name:        name,
		Description: fmt.Sprintf("Created from %d selected item(s)", len(refs)),
		Assets:      assets,
		Providers:   providers,
		Items:       items,
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(tmplDir, "template.json"), data, 0o644)
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

	// File-based templates (created via the wizard) may have empty assets/providers
	// when all content is expressed through the items array. Only enforce the
	// requirements for manifest-only templates.
	hasFiles := len(f.Items) > 0
	if !hasFiles {
		if len(assets) == 0 {
			return model.Template{}, errors.New("no assets defined")
		}
		if len(providers) == 0 {
			return model.Template{}, errors.New("no providers defined")
		}
	}

	return model.Template{
		Name:        f.Name,
		Description: f.Description,
		Assets:      assets,
		Providers:   providers,
	}, nil
}

// copyFile copies src to dst, creating dst if necessary.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
