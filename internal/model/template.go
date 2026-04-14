package model

// Template defines a reusable asset directory structure template.
type Template struct {
	Name        string
	Description string
	Assets      []AssetType
	Providers   []Provider
	UserDefined bool   // true for templates loaded from ~/.agentsbuilder/templates/
	TemplateDir string // absolute path to the template directory; non-empty for user-defined templates with files
}

// PredefinedTemplates returns the built-in templates available in MVP.
func PredefinedTemplates() []Template {
	return []Template{
		{
			Name:        "claude-code-basic",
			Description: "Basic Claude Code setup with Skills and CLAUDE.md",
			Assets:      []AssetType{Skills, ClaudeMD},
			Providers:   []Provider{ClaudeCode},
		},
		{
			Name:        "codex-basic",
			Description: "Basic Codex setup with Skills and AGENTS.md",
			Assets:      []AssetType{Skills, AgentsMD},
			Providers:   []Provider{Codex},
		},
		{
			Name:        "full-claude",
			Description: "Full Claude Code setup with all asset types",
			Assets:      []AssetType{Skills, Agents, MCP, AgentsMD, ClaudeMD},
			Providers:   []Provider{ClaudeCode},
		},
		{
			Name:        "full-codex",
			Description: "Full Codex setup with all asset types",
			Assets:      []AssetType{Skills, Agents, MCP, AgentsMD, ClaudeMD},
			Providers:   []Provider{Codex},
		},
		{
			Name:        "universal",
			Description: "All asset types for both Claude Code and Codex",
			Assets:      []AssetType{Skills, Agents, MCP, AgentsMD, ClaudeMD},
			Providers:   []Provider{ClaudeCode, Codex},
		},
	}
}
