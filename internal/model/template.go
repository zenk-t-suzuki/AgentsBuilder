package model

// Template defines a built-in directory scaffold that a user can apply to a
// project to create the standard asset directory structure for a given
// provider/asset combination. Templates are predefined only — user-defined
// templates have been replaced by the Claude Code-compatible marketplace
// plugin system (see internal/marketplace).
type Template struct {
	Name        string
	Description string
	Assets      []AssetType
	Providers   []Provider
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
			Assets:      []AssetType{Skills, Agents, MCP, Hooks, AgentsMD, ClaudeMD},
			Providers:   []Provider{ClaudeCode},
		},
		{
			Name:        "full-codex",
			Description: "Full Codex setup with all asset types",
			Assets:      []AssetType{Skills, Agents, MCP, Hooks, AgentsMD},
			Providers:   []Provider{Codex},
		},
		{
			Name:        "universal",
			Description: "All asset types for both Claude Code and Codex",
			Assets:      []AssetType{Skills, Agents, MCP, Hooks, AgentsMD, ClaudeMD},
			Providers:   []Provider{ClaudeCode, Codex},
		},
	}
}
