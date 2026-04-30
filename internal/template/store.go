package template

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"agentsbuilder/internal/assetdef"
	"agentsbuilder/internal/config"
	"agentsbuilder/internal/model"
)

// DefaultTemplateDirName is the name of the built-in default user template.
// The leading '*' causes it to sort first alphabetically in the directory listing.
const DefaultTemplateDirName = "*default"

// mergeInfo records how an embedded item should be merged into a target config file.
type mergeInfo struct {
	AssetType string `json:"assetType"` // e.g. "MCP", "Plugins"
	Provider  string `json:"provider"`  // e.g. "ClaudeCode", "Codex"
}

// templateFileItem describes one file bundled inside a user-created template.
type templateFileItem struct {
	RelPath string     `json:"relPath"`         // path relative to the template's files/ directory
	Merge   *mergeInfo `json:"merge,omitempty"` // non-nil for embedded items that require merge
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
	ItemName   string // for embedded items: the specific entry name (e.g. MCP server name)
	Embedded   bool   // true when this item is embedded in a shared config file
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
// Embedded items (MCP servers, plugins) are extracted from shared config files
// and saved as individual snippet files under files/__merge__/.
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

	// Copy/extract each item into the template's files/ directory.
	var items []templateFileItem
	for _, r := range refs {
		var relPath string
		if r.DestRelDir != "" {
			relPath = filepath.Join(r.DestRelDir, r.Filename)
		} else {
			relPath = r.Filename
		}
		destPath := filepath.Join(tmplDir, "files", relPath)

		if r.Embedded {
			// Extract the specific entry from the shared config file.
			if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
				return fmt.Errorf("creating merge directory: %w", err)
			}
			extracted, err := extractEmbeddedItem(r)
			if err != nil {
				return fmt.Errorf("extracting %s %q: %w", r.AssetType.String(), r.ItemName, err)
			}
			if err := os.WriteFile(destPath, extracted, 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", r.ItemName, err)
			}
			items = append(items, templateFileItem{
				RelPath: relPath,
				Merge: &mergeInfo{
					AssetType: r.AssetType.String(),
					Provider:  r.Provider.String(),
				},
			})
		} else {
			// Regular file or directory copy.
			info, err := os.Stat(r.SrcPath)
			if err != nil {
				return fmt.Errorf("stat %s: %w", r.Filename, err)
			}

			if info.IsDir() {
				if err := copyDir(r.SrcPath, destPath); err != nil {
					return fmt.Errorf("copying %s: %w", r.Filename, err)
				}
			} else {
				if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
					return fmt.Errorf("creating files directory: %w", err)
				}
				if err := copyFile(r.SrcPath, destPath); err != nil {
					return fmt.Errorf("copying %s: %w", r.Filename, err)
				}
			}
			items = append(items, templateFileItem{RelPath: relPath})
		}
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

// extractEmbeddedItem extracts a single item's configuration from a shared
// config file. The extraction method is determined by the AssetDef's
// StorageKind and ConfigKey.
func extractEmbeddedItem(r FileRef) ([]byte, error) {
	// Look up the asset definition to get the config key.
	def, ok := assetdef.LookupAny(r.Provider, r.AssetType)
	if !ok || def.Key == nil {
		return nil, fmt.Errorf("unsupported embedded asset type: %s", r.AssetType.String())
	}

	switch def.Storage {
	case assetdef.EmbeddedJSON:
		return extractJSONEntry(r.SrcPath, def.Key.JSONKey, r.ItemName)
	case assetdef.EmbeddedTOML:
		return extractTOMLSection(r.SrcPath, def.Key.TOMLPrefix, r.ItemName)
	default:
		return nil, fmt.Errorf("unsupported storage kind for extraction")
	}
}

// extractJSONEntry extracts a single entry from a top-level JSON object key.
// Returns a JSON snippet like {"mcpServers": {"name": {...}}}.
func extractJSONEntry(filePath, jsonKey, itemName string) ([]byte, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	section, ok := raw[jsonKey]
	if !ok {
		return nil, fmt.Errorf("%q section not found in %s", jsonKey, filePath)
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(section, &entries); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", jsonKey, err)
	}

	// For enabledPlugins, the key may include an @marketplace suffix.
	// Try exact match first, then match by name prefix.
	entryKey := itemName
	if _, ok := entries[entryKey]; !ok {
		for key := range entries {
			name := key
			if at := strings.LastIndex(key, "@"); at >= 0 {
				name = key[:at]
			}
			if name == itemName {
				entryKey = key
				break
			}
		}
	}

	entry, ok := entries[entryKey]
	if !ok {
		return nil, fmt.Errorf("%q not found in %s.%s", itemName, filePath, jsonKey)
	}
	result := map[string]map[string]json.RawMessage{
		jsonKey: {entryKey: entry},
	}
	return json.MarshalIndent(result, "", "  ")
}

// extractTOMLSection extracts a single [prefix.<name>] section from a TOML file.
func extractTOMLSection(filePath, prefix, itemName string) ([]byte, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	header := fmt.Sprintf("[%s.%s]", prefix, itemName)

	var result []string
	capturing := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == header {
			capturing = true
			result = append(result, line)
			continue
		}
		if capturing {
			if strings.HasPrefix(trimmed, "[") {
				break
			}
			result = append(result, line)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("%q not found in %s", header, filePath)
	}
	return []byte(strings.Join(result, "\n") + "\n"), nil
}

// loadManifestItems reads the items array from a template's template.json manifest.
func loadManifestItems(templateDir string) []templateFileItem {
	data, err := os.ReadFile(filepath.Join(templateDir, "template.json"))
	if err != nil {
		return nil
	}
	var f templateFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil
	}
	return f.Items
}

// fileToTemplate converts the on-disk templateFile to a model.Template.
func fileToTemplate(f templateFile) (model.Template, error) {
	if f.Name == "" {
		return model.Template{}, errors.New("name is empty")
	}

	var assets []model.AssetType
	for _, a := range f.Assets {
		at, ok := model.ParseAssetType(a)
		if !ok {
			return model.Template{}, fmt.Errorf("unknown asset %q", a)
		}
		assets = append(assets, at)
	}

	var providers []model.Provider
	for _, p := range f.Providers {
		pv, ok := model.ParseProvider(p)
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

// copyDir recursively copies a directory tree from src to dst.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
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
