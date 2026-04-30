package scanner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"agentsbuilder/internal/assetdef"
	"agentsbuilder/internal/model"
)

// scanItems populates the Items field of an asset based on its definition.
func scanItems(asset *model.Asset, def *assetdef.AssetDef) {
	if !asset.Exists {
		return
	}
	switch def.Storage {
	case assetdef.DirListing:
		switch asset.Type {
		case model.Agents:
			asset.Items = scanAgentItems(asset.FilePath, asset.Provider)
		case model.Skills:
			asset.Items = scanSkillItems(asset.FilePath)
		case model.Plugins:
			if asset.Provider == model.Codex {
				asset.Items = scanPluginItemsCodex(asset.FilePath)
			}
		}
	case assetdef.EmbeddedJSON:
		if def.Key != nil {
			asset.Items = scanEmbeddedJSON(asset.FilePath, def.Key.JSONKey)
		}
	case assetdef.EmbeddedTOML:
		if def.Key != nil {
			asset.Items = scanEmbeddedTOML(asset.FilePath, def.Key.TOMLPrefix)
		}
	// SingleFile: no sub-items to scan
	}
}

// scanEmbeddedJSON extracts item names from a top-level JSON object key.
// Works for mcpServers, enabledPlugins, hooks, etc.
func scanEmbeddedJSON(filePath, jsonKey string) []model.AssetItem {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	section, ok := raw[jsonKey]
	if !ok {
		return nil
	}

	// Try as map[string]* — the value type varies but keys are item names.
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(section, &entries); err != nil {
		return nil
	}

	var items []model.AssetItem
	for name, val := range entries {
		// For enabledPlugins, skip disabled entries (value == false).
		if jsonKey == "enabledPlugins" {
			var enabled bool
			if json.Unmarshal(val, &enabled) == nil && !enabled {
				continue
			}
		}
		displayName := name
		// For enabledPlugins, strip the @marketplace suffix.
		if jsonKey == "enabledPlugins" {
			if at := strings.LastIndex(name, "@"); at >= 0 {
				displayName = name[:at]
			}
		}
		items = append(items, model.AssetItem{
			Name:     displayName,
			FilePath: filePath,
		})
	}
	return items
}

// scanEmbeddedTOML extracts item names from TOML section headers matching
// [prefix.<name>]. For example, prefix "mcp_servers" matches [mcp_servers.foo].
func scanEmbeddedTOML(filePath, prefix string) []model.AssetItem {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}

	headerPrefix := "[" + prefix + "."
	var items []model.AssetItem
	seen := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, headerPrefix) && strings.HasSuffix(line, "]") {
			name := strings.TrimPrefix(line, headerPrefix)
			name = strings.TrimSuffix(name, "]")
			if name != "" && !seen[name] {
				seen[name] = true
				items = append(items, model.AssetItem{
					Name:     name,
					FilePath: filePath,
				})
			}
		}
	}
	return items
}

// scanAgentItems reads agent definitions from a directory.
// Claude Code agents: .md files with YAML frontmatter (name, description).
// Codex agents: .toml files with name and description fields.
func scanAgentItems(dir string, provider model.Provider) []model.AssetItem {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var items []model.AssetItem
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		fullPath := filepath.Join(dir, name)

		var itemName, desc string
		switch provider {
		case model.ClaudeCode:
			if !strings.HasSuffix(name, ".md") {
				continue
			}
			data, err := os.ReadFile(fullPath)
			if err != nil {
				continue
			}
			itemName, desc = parseFrontmatter(string(data))
			if itemName == "" {
				itemName = strings.TrimSuffix(name, ".md")
			}
		case model.Codex:
			if !strings.HasSuffix(name, ".toml") {
				continue
			}
			data, err := os.ReadFile(fullPath)
			if err != nil {
				continue
			}
			itemName, desc = parseTomlNameDesc(string(data))
			if itemName == "" {
				itemName = strings.TrimSuffix(name, ".toml")
			}
		default:
			continue
		}

		items = append(items, model.AssetItem{
			Name:        itemName,
			Description: desc,
			FilePath:    fullPath,
		})
	}
	return items
}

// scanSkillItems reads skill entries from a directory.
// Each skill is either:
//   - A subdirectory containing a SKILL.md with frontmatter (name, description)
//   - A .md file whose name is the skill name
func scanSkillItems(dir string) []model.AssetItem {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var items []model.AssetItem
	for _, entry := range entries {
		fullPath := filepath.Join(dir, entry.Name())
		var itemName, desc string

		if entry.IsDir() {
			// Skill package dir: look for SKILL.md with frontmatter
			skillMD := filepath.Join(fullPath, "SKILL.md")
			data, err := os.ReadFile(skillMD)
			if err == nil {
				itemName, desc = parseFrontmatter(string(data))
			}
			if itemName == "" {
				itemName = entry.Name()
			}
		} else {
			name := entry.Name()
			if !strings.HasSuffix(name, ".md") {
				continue
			}
			data, err := os.ReadFile(fullPath)
			if err == nil {
				itemName, desc = parseFrontmatter(string(data))
			}
			if itemName == "" {
				itemName = strings.TrimSuffix(name, ".md")
			}
		}

		items = append(items, model.AssetItem{
			Name:        itemName,
			Description: desc,
			FilePath:    fullPath,
		})
	}
	return items
}

// scanPluginItemsCodex scans installed Codex plugins from the plugin cache directory.
// Each subdirectory is a plugin; metadata comes from .codex-plugin/plugin.json.
func scanPluginItemsCodex(pluginsDir string) []model.AssetItem {
	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		return nil
	}

	var items []model.AssetItem
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pluginJSON := filepath.Join(pluginsDir, entry.Name(), ".codex-plugin", "plugin.json")
		data, err := os.ReadFile(pluginJSON)
		if err != nil {
			// Fall back to directory name if plugin.json is missing.
			items = append(items, model.AssetItem{
				Name:     entry.Name(),
				FilePath: filepath.Join(pluginsDir, entry.Name()),
			})
			continue
		}

		var meta struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal(data, &meta); err != nil || meta.Name == "" {
			meta.Name = entry.Name()
		}
		items = append(items, model.AssetItem{
			Name:        meta.Name,
			Description: meta.Description,
			FilePath:    filepath.Join(pluginsDir, entry.Name()),
		})
	}
	return items
}

// parseFrontmatter extracts name and description from YAML frontmatter in a markdown file.
// The frontmatter must start at the very beginning of the content with "---".
func parseFrontmatter(content string) (name, description string) {
	if !strings.HasPrefix(content, "---") {
		return "", ""
	}
	// Skip the opening "---" and optional newline
	rest := strings.TrimPrefix(content, "---")
	rest = strings.TrimLeft(rest, "\r\n")

	// Find the closing "---"
	endIdx := strings.Index(rest, "\n---")
	if endIdx == -1 {
		return "", ""
	}
	yamlBlock := rest[:endIdx]

	for _, line := range strings.Split(yamlBlock, "\n") {
		line = strings.TrimRight(line, "\r")
		if after, ok := strings.CutPrefix(line, "name:"); ok {
			val := strings.TrimSpace(after)
			// Skip YAML block scalars (multi-line values: >-, >, |, |-…)
			if val != "" && val[0] != '>' && val[0] != '|' {
				name = val
			}
		}
		if after, ok := strings.CutPrefix(line, "description:"); ok {
			val := strings.TrimSpace(after)
			// Skip YAML block scalars (multi-line values: >-, >, |, |-…)
			if val != "" && val[0] != '>' && val[0] != '|' {
				description = val
			}
		}
	}
	return
}

// parseTomlNameDesc extracts name and description from a simple TOML file.
// Handles quoted and unquoted string values.
func parseTomlNameDesc(content string) (name, description string) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimRight(line, "\r")
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "name"); ok {
			after = strings.TrimSpace(after)
			if strings.HasPrefix(after, "=") {
				val := strings.TrimSpace(after[1:])
				name = strings.Trim(val, `"'`)
			}
		}
		if after, ok := strings.CutPrefix(line, "description"); ok {
			after = strings.TrimSpace(after)
			if strings.HasPrefix(after, "=") {
				val := strings.TrimSpace(after[1:])
				description = strings.Trim(val, `"'`)
			}
		}
	}
	return
}
