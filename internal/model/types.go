package model

// Provider represents a supported AI coding tool.
type Provider int

const (
	ClaudeCode Provider = iota
	Codex
)

func (p Provider) String() string {
	switch p {
	case ClaudeCode:
		return "Claude Code"
	case Codex:
		return "Codex"
	default:
		return "Unknown"
	}
}

// Providers returns all supported providers.
func Providers() []Provider {
	return []Provider{ClaudeCode, Codex}
}

// Scope represents the configuration scope level.
type Scope int

const (
	Global Scope = iota
	Project
)

func (s Scope) String() string {
	switch s {
	case Global:
		return "Global"
	case Project:
		return "Project"
	default:
		return "Unknown"
	}
}

// AssetType represents a category of managed configuration asset.
type AssetType int

const (
	Skills   AssetType = iota // Commands/skills directories
	Agents                    // Subagent definition directories
	MCP                       // MCP server config (.claude.json / config.toml)
	Plugins                   // Plugin bundles (settings.json – Claude Code; .tmp/plugins – Codex)
	Hooks                     // Event hooks (settings.json – Claude Code only)
	AgentsMD                  // AGENTS.md instruction file
	ClaudeMD                  // CLAUDE.md memory file (Claude Code only)
)

func (a AssetType) String() string {
	switch a {
	case Skills:
		return "Skills"
	case Agents:
		return "Custom Agents"
	case MCP:
		return "MCP"
	case Plugins:
		return "Plugins"
	case Hooks:
		return "Hooks"
	case AgentsMD:
		return "AGENTS.md"
	case ClaudeMD:
		return "CLAUDE.md"
	default:
		return "Unknown"
	}
}

// AssetTypes returns all managed asset types.
func AssetTypes() []AssetType {
	return []AssetType{Skills, Agents, MCP, Plugins, Hooks, AgentsMD, ClaudeMD}
}

// ParseAssetType converts a string to an AssetType.
// Accepted values include both display names ("Custom Agents") and config
// names ("Agents", "CustomAgents").
func ParseAssetType(s string) (AssetType, bool) {
	at, ok := assetTypeNames[s]
	return at, ok
}

var assetTypeNames = map[string]AssetType{
	"Skills":        Skills,
	"Agents":        Agents,
	"CustomAgents":  Agents,
	"Custom Agents": Agents,
	"MCP":           MCP,
	"Plugins":       Plugins,
	"Hooks":         Hooks,
	"AgentsMD":      AgentsMD,
	"AGENTS.md":     AgentsMD,
	"ClaudeMD":      ClaudeMD,
	"CLAUDE.md":     ClaudeMD,
}

// ParseProvider converts a string to a Provider.
func ParseProvider(s string) (Provider, bool) {
	pv, ok := providerNames[s]
	return pv, ok
}

var providerNames = map[string]Provider{
	"ClaudeCode":  ClaudeCode,
	"Claude Code": ClaudeCode,
	"Codex":       Codex,
}

// AssetItem represents an individual named entry within an asset directory or file.
// For Agents/Skills, each item is a single file. For MCP, each item is a server entry.
type AssetItem struct {
	Name        string
	Description string
	FilePath    string
}

// Asset represents a single configuration asset on the filesystem.
type Asset struct {
	Type     AssetType
	Provider Provider
	Scope    Scope
	FilePath string
	Exists   bool
	Active   bool
	Items    []AssetItem // individual named entries (agents, skills, MCP servers)
}

// ProjectInfo holds registered project metadata.
type ProjectInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// DiffResult captures the diff/priority relationship between global and project assets.
type DiffResult struct {
	AssetType     AssetType
	Provider      Provider // which provider this diff belongs to
	GlobalPath    string
	ProjectPath   string
	GlobalExists  bool
	ProjectExists bool
	Priority      Scope // which scope takes precedence
	HasDiff       bool  // true when both exist (potential override)
}
