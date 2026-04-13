package scanner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"agentsbuilder/internal/model"
)

// scanItems populates the Items field of an asset based on its type.
// Called after the asset's Exists field is determined.
func scanItems(asset *model.Asset) {
	if !asset.Exists {
		return
	}
	switch asset.Type {
	case model.Agents:
		asset.Items = scanAgentItems(asset.FilePath, asset.Provider)
	case model.Skills:
		asset.Items = scanSkillItems(asset.FilePath)
	case model.MCP:
		switch asset.Provider {
		case model.ClaudeCode:
			asset.Items = scanMCPItemsClaude(asset.FilePath)
		case model.Codex:
			asset.Items = scanMCPItemsCodex(asset.FilePath)
		}
	case model.Plugins:
		switch asset.Provider {
		case model.ClaudeCode:
			asset.Items = scanPluginItemsClaudeCode(asset.FilePath)
		case model.Codex:
			asset.Items = scanPluginItemsCodex(asset.FilePath)
		}
	}
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

// scanMCPItemsClaude extracts MCP server names from Claude Code's settings.json.
func scanMCPItemsClaude(settingsPath string) []model.AssetItem {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil
	}

	var settings struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil
	}

	var items []model.AssetItem
	for name := range settings.MCPServers {
		items = append(items, model.AssetItem{
			Name:     name,
			FilePath: settingsPath,
		})
	}
	return items
}

// scanMCPItemsCodex extracts MCP server names from Codex's config.toml.
// Looks for section headers of the form [mcp_servers.<name>].
func scanMCPItemsCodex(configPath string) []model.AssetItem {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}

	var items []model.AssetItem
	seen := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[mcp_servers.") && strings.HasSuffix(line, "]") {
			serverName := strings.TrimPrefix(line, "[mcp_servers.")
			serverName = strings.TrimSuffix(serverName, "]")
			if serverName != "" && !seen[serverName] {
				seen[serverName] = true
				items = append(items, model.AssetItem{
					Name:     serverName,
					FilePath: configPath,
				})
			}
		}
	}
	return items
}

// scanPluginItemsClaudeCode extracts enabled plugin names from Claude Code's settings.json.
// enabledPlugins is a map of "pluginName@marketplace" → bool.
func scanPluginItemsClaudeCode(settingsPath string) []model.AssetItem {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil
	}

	var settings struct {
		EnabledPlugins map[string]bool `json:"enabledPlugins"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil
	}

	var items []model.AssetItem
	for key, enabled := range settings.EnabledPlugins {
		if !enabled {
			continue
		}
		// Key format: "pluginName@marketplace" — use only the plugin name portion.
		name := key
		if at := strings.LastIndex(key, "@"); at >= 0 {
			name = key[:at]
		}
		items = append(items, model.AssetItem{
			Name:     name,
			FilePath: settingsPath,
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
