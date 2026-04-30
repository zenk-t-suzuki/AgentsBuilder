// Package assetdef is the single authoritative source for managed asset
// definitions — which providers, scopes, filesystem paths, and storage
// formats the application knows about. Every consumer (scanner, template
// engine, TUI) derives its path knowledge from this package.
package assetdef

import (
	"path/filepath"

	"agentsbuilder/internal/model"
)

// StorageKind classifies how an asset's individual items are stored.
type StorageKind int

const (
	// DirListing means items are separate files/directories within a directory.
	DirListing StorageKind = iota
	// EmbeddedJSON means items are entries inside a JSON config file.
	EmbeddedJSON
	// EmbeddedTOML means items are sections inside a TOML config file.
	EmbeddedTOML
	// SingleFile means the asset is one standalone file with no sub-items.
	SingleFile
	// CodexAgentRoles means Codex agent roles discovered from both
	// .codex/config.toml [agents.roles.<name>] and .codex/agents/*.toml.
	CodexAgentRoles
)

// ConfigKey describes how to extract items from an embedded config file.
type ConfigKey struct {
	JSONKey    string // top-level JSON object key, e.g. "mcpServers"
	TOMLPrefix string // TOML section prefix, e.g. "mcp_servers"
}

// AssetDef is the single authoritative definition for one managed asset
// at a specific (Provider, Scope) combination.
type AssetDef struct {
	Type     model.AssetType
	Provider model.Provider
	Scope    model.Scope
	Storage  StorageKind

	// ScanPaths lists filesystem paths relative to the scope root to check,
	// in priority order. All existing paths are merged during scanning.
	ScanPaths []string

	// PrimaryPath is the canonical path used when creating new assets
	// (template apply, directory creation). Usually ScanPaths[0].
	PrimaryPath string

	// Key describes how to extract/merge individual items for embedded
	// assets. Nil for DirListing and SingleFile.
	Key *ConfigKey
}

// IsFile returns true when the asset is a file (not a directory listing).
func (d AssetDef) IsFile() bool {
	return d.Storage != DirListing && d.Storage != CodexAgentRoles
}

// IsEmbedded returns true when individual items live inside a shared config file.
func (d AssetDef) IsEmbedded() bool {
	return d.Storage == EmbeddedJSON || d.Storage == EmbeddedTOML || d.Storage == CodexAgentRoles
}

// ParentDir returns the directory portion of PrimaryPath, suitable for
// os.MkdirAll. For DirListing assets it returns PrimaryPath itself.
func (d AssetDef) ParentDir() string {
	if d.Storage == DirListing || d.Storage == CodexAgentRoles {
		return d.PrimaryPath
	}
	dir := filepath.Dir(d.PrimaryPath)
	if dir == "." {
		return ""
	}
	return dir
}

// ---------------------------------------------------------------------------
// Master definition table
// ---------------------------------------------------------------------------

var allDefs = []AssetDef{
	// ── Claude Code / Global ──
	{model.Skills, model.ClaudeCode, model.Global, DirListing,
		[]string{".claude/commands", ".claude/skills"}, ".claude/commands", nil},
	{model.Agents, model.ClaudeCode, model.Global, DirListing,
		[]string{".claude/agents"}, ".claude/agents", nil},
	{model.MCP, model.ClaudeCode, model.Global, EmbeddedJSON,
		[]string{".claude.json", ".claude/settings.json"}, ".claude.json",
		&ConfigKey{JSONKey: "mcpServers"}},
	{model.Plugins, model.ClaudeCode, model.Global, EmbeddedJSON,
		[]string{".claude/settings.json"}, ".claude/settings.json",
		&ConfigKey{JSONKey: "enabledPlugins"}},
	{model.Hooks, model.ClaudeCode, model.Global, EmbeddedJSON,
		[]string{".claude/settings.json"}, ".claude/settings.json",
		&ConfigKey{JSONKey: "hooks"}},
	{model.AgentsMD, model.ClaudeCode, model.Global, SingleFile,
		[]string{".claude/AGENTS.md"}, ".claude/AGENTS.md", nil},
	{model.ClaudeMD, model.ClaudeCode, model.Global, SingleFile,
		[]string{".claude/CLAUDE.md"}, ".claude/CLAUDE.md", nil},

	// ── Claude Code / Project ──
	{model.Skills, model.ClaudeCode, model.Project, DirListing,
		[]string{".claude/commands", ".claude/skills"}, ".claude/commands", nil},
	{model.Agents, model.ClaudeCode, model.Project, DirListing,
		[]string{".claude/agents"}, ".claude/agents", nil},
	{model.MCP, model.ClaudeCode, model.Project, EmbeddedJSON,
		[]string{".mcp.json", ".claude/settings.json"}, ".mcp.json",
		&ConfigKey{JSONKey: "mcpServers"}},
	{model.Hooks, model.ClaudeCode, model.Project, EmbeddedJSON,
		[]string{".claude/settings.json"}, ".claude/settings.json",
		&ConfigKey{JSONKey: "hooks"}},
	{model.AgentsMD, model.ClaudeCode, model.Project, SingleFile,
		[]string{"AGENTS.md"}, "AGENTS.md", nil},
	{model.ClaudeMD, model.ClaudeCode, model.Project, SingleFile,
		[]string{"CLAUDE.md"}, "CLAUDE.md", nil},

	// ── Codex / Global ──
	// Paths verified against openai/codex Rust source (2026-04 snapshot):
	//   - codex-rs/core-skills/src/loader.rs   ($CODEX_HOME/skills, $HOME/.agents/skills)
	//   - codex-rs/config/src/config_toml.rs   ([mcp_servers], [hooks], [plugins], etc.)
	//   - codex-rs/core/src/config/agent_roles.rs ([agents.roles], .codex/agents)
	//   - codex-rs/core/src/agents_md.rs       (AGENTS.override.md > AGENTS.md priority)
	{model.Skills, model.Codex, model.Global, DirListing,
		[]string{".agents/skills", ".codex/skills"}, ".agents/skills", nil},
	{model.Agents, model.Codex, model.Global, CodexAgentRoles,
		[]string{".codex/agents", ".codex/config.toml"}, ".codex/agents", nil},
	{model.MCP, model.Codex, model.Global, EmbeddedTOML,
		[]string{".codex/config.toml"}, ".codex/config.toml",
		&ConfigKey{TOMLPrefix: "mcp_servers"}},
	{model.Hooks, model.Codex, model.Global, EmbeddedTOML,
		[]string{".codex/config.toml"}, ".codex/config.toml",
		&ConfigKey{TOMLPrefix: "hooks"}},
	{model.Plugins, model.Codex, model.Global, EmbeddedTOML,
		[]string{".codex/config.toml"}, ".codex/config.toml",
		&ConfigKey{TOMLPrefix: "plugins"}},
	{model.AgentsMD, model.Codex, model.Global, SingleFile,
		[]string{".codex/AGENTS.override.md", ".codex/AGENTS.md"}, ".codex/AGENTS.md", nil},

	// ── Codex / Project ──
	// Codex loads project-local config layers from .codex/config.toml between
	// the project root and cwd when the project is trusted. Plugins remain
	// user-config only; project [plugins] entries are ignored by Codex.
	{model.Skills, model.Codex, model.Project, DirListing,
		[]string{".agents/skills"}, ".agents/skills", nil},
	{model.Agents, model.Codex, model.Project, CodexAgentRoles,
		[]string{".codex/agents", ".codex/config.toml"}, ".codex/agents", nil},
	{model.MCP, model.Codex, model.Project, EmbeddedTOML,
		[]string{".codex/config.toml"}, ".codex/config.toml",
		&ConfigKey{TOMLPrefix: "mcp_servers"}},
	{model.Hooks, model.Codex, model.Project, EmbeddedTOML,
		[]string{".codex/config.toml"}, ".codex/config.toml",
		&ConfigKey{TOMLPrefix: "hooks"}},
	{model.AgentsMD, model.Codex, model.Project, SingleFile,
		[]string{"AGENTS.override.md", "AGENTS.md"}, "AGENTS.md", nil},
}

// defIndex is a lookup map keyed by (Provider, Scope, AssetType).
type defKey struct {
	p model.Provider
	s model.Scope
	t model.AssetType
}

var defIndex map[defKey]AssetDef

func init() {
	defIndex = make(map[defKey]AssetDef, len(allDefs))
	for _, d := range allDefs {
		defIndex[defKey{d.Provider, d.Scope, d.Type}] = d
	}
}

// All returns every AssetDef in the system.
func All() []AssetDef {
	out := make([]AssetDef, len(allDefs))
	copy(out, allDefs)
	return out
}

// Lookup returns the definition for a specific (Provider, Scope, AssetType).
func Lookup(provider model.Provider, scope model.Scope, assetType model.AssetType) (AssetDef, bool) {
	d, ok := defIndex[defKey{provider, scope, assetType}]
	return d, ok
}

// ForProviderScope returns all defs for a (Provider, Scope) pair,
// in the same order as the master table.
func ForProviderScope(provider model.Provider, scope model.Scope) []AssetDef {
	var out []AssetDef
	for _, d := range allDefs {
		if d.Provider == provider && d.Scope == scope {
			out = append(out, d)
		}
	}
	return out
}

// LookupAny returns the definition for a (Provider, AssetType) pair,
// trying Project scope first, then falling back to Global.
// This is useful in scope-agnostic contexts like template creation
// where the storage kind is the same regardless of scope.
func LookupAny(provider model.Provider, assetType model.AssetType) (AssetDef, bool) {
	if d, ok := Lookup(provider, model.Project, assetType); ok {
		return d, true
	}
	return Lookup(provider, model.Global, assetType)
}

// IsEmbedded is a convenience function that returns true when the given
// (Provider, AssetType) stores items inside a shared config file.
// Uses LookupAny (Project→Global fallback).
func IsEmbedded(provider model.Provider, assetType model.AssetType) bool {
	d, ok := LookupAny(provider, assetType)
	return ok && d.IsEmbedded()
}

// DestDir returns the relative destination directory for an asset type
// and provider when applying to a project. For DirListing assets this
// is the PrimaryPath; for embedded/single-file assets this is the
// parent directory. Returns "" if the definition is not found.
func DestDir(provider model.Provider, assetType model.AssetType) string {
	def, ok := Lookup(provider, model.Project, assetType)
	if !ok {
		return ""
	}
	if def.Storage == DirListing {
		return def.PrimaryPath
	}
	return def.ParentDir()
}
