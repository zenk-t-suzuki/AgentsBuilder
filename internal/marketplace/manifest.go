package marketplace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ManifestFormat identifies the schema of a marketplace.json — Claude Code's
// or OpenAI Codex's. The two are distinguished by required fields:
// Claude requires `owner.name`; Codex does not.
type ManifestFormat int

const (
	// FormatClaude is `.claude-plugin/marketplace.json` per
	// https://code.claude.com/docs/en/plugin-marketplaces
	FormatClaude ManifestFormat = iota
	// FormatCodex is `.agents/plugins/marketplace.json` per the
	// codex-rs/core-plugins/src/marketplace.rs deserialiser.
	FormatCodex
)

// Manifest is the unified internal representation of a marketplace catalog.
// It carries the strict superset of Claude Code's and Codex's schemas, with
// `Format` tracking which on-disk schema produced it. Codex-only fields
// (policy/category) are preserved on each PluginEntry; Claude-only fields
// (owner) likewise.
type Manifest struct {
	Format      ManifestFormat
	Schema      string        // Claude `$schema`
	Name        string        // both: `name` (kebab-case)
	Description string        // Claude `description`, Codex unused
	Version     string        // Claude `version`, Codex unused
	Owner       Owner         // Claude `owner` (Name required); Codex sets empty
	DisplayName string        // Codex `interface.display_name`
	Metadata    *Metadata     // Claude `metadata.pluginRoot`
	Plugins     []PluginEntry // both: `plugins`
}

// Owner identifies the marketplace maintainer (required by the Claude spec).
type Owner struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

// Metadata holds optional marketplace-level configuration (Claude only).
type Metadata struct {
	PluginRoot string `json:"pluginRoot,omitempty"`
}

// PluginEntry is one plugin entry in the marketplace. Fields cover both
// Claude Code and Codex schemas — fields absent from the input format are
// left at zero value.
type PluginEntry struct {
	Name   string          // both
	Source json.RawMessage // both: string or object — decoded later via ResolvePluginSource

	// Claude metadata (optional in spec)
	Description string
	Version     string
	Author      *Author
	Homepage    string
	Repository  string
	License     string
	Keywords    []string

	// Both
	Category string
	Tags     []string
	Strict   *bool

	// Codex policy (omitted on Claude)
	Policy *CodexPluginPolicy
}

// Author identifies a plugin author (Claude only).
type Author struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

// CodexPluginPolicy mirrors Codex's `RawMarketplaceManifestPluginPolicy`.
// AgentsBuilder uses `Installation` to grey out plugins marked NOT_AVAILABLE.
type CodexPluginPolicy struct {
	Installation   string   `json:"installation"`
	Authentication string   `json:"authentication"`
	Products       []string `json:"products,omitempty"`
}

// ─── Raw on-disk shapes ───────────────────────────────────────────────────────

type claudeManifest struct {
	Schema      string            `json:"$schema,omitempty"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Version     string            `json:"version,omitempty"`
	Owner       Owner             `json:"owner"`
	Metadata    *Metadata         `json:"metadata,omitempty"`
	Plugins     []claudePluginRaw `json:"plugins"`
}

type claudePluginRaw struct {
	Name        string          `json:"name"`
	Source      json.RawMessage `json:"source"`
	Description string          `json:"description,omitempty"`
	Version     string          `json:"version,omitempty"`
	Author      *Author         `json:"author,omitempty"`
	Homepage    string          `json:"homepage,omitempty"`
	Repository  string          `json:"repository,omitempty"`
	License     string          `json:"license,omitempty"`
	Keywords    []string        `json:"keywords,omitempty"`
	Category    string          `json:"category,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
	Strict      *bool           `json:"strict,omitempty"`
}

type codexManifest struct {
	Name      string                  `json:"name"`
	Interface *codexManifestInterface `json:"interface,omitempty"`
	Plugins   []codexPluginRaw        `json:"plugins"`
}

type codexManifestInterface struct {
	DisplayName string `json:"display_name,omitempty"`
}

type codexPluginRaw struct {
	Name     string             `json:"name"`
	Source   json.RawMessage    `json:"source"`
	Policy   *CodexPluginPolicy `json:"policy,omitempty"`
	Category string             `json:"category,omitempty"`
}

// ─── Loading ──────────────────────────────────────────────────────────────────

// LoadManifest reads and validates a marketplace.json file at path. The schema
// is auto-detected: if `owner.name` is set, it's parsed as Claude's schema;
// otherwise it's parsed as Codex's.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading marketplace manifest: %w", err)
	}
	return ParseManifest(data)
}

// ParseManifest parses raw marketplace.json bytes, auto-detecting format.
func ParseManifest(data []byte) (*Manifest, error) {
	// Probe for `owner` to discriminate Claude vs Codex.
	var probe struct {
		Owner *Owner `json:"owner"`
	}
	_ = json.Unmarshal(data, &probe) // ignore error; fall through to format checks

	if probe.Owner != nil && probe.Owner.Name != "" {
		return parseClaudeManifest(data)
	}
	// No owner.name → assume Codex form. If that also fails to validate,
	// surface an error that mentions both schemas.
	m, err := parseCodexManifest(data)
	if err != nil {
		return nil, fmt.Errorf("manifest is neither Claude (.claude-plugin/marketplace.json with owner.name) "+
			"nor Codex (.agents/plugins/marketplace.json with policy entries) format: %w", err)
	}
	return m, nil
}

func parseClaudeManifest(data []byte) (*Manifest, error) {
	var raw claudeManifest
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing Claude marketplace manifest: %w", err)
	}
	if raw.Name == "" {
		return nil, errors.New("Claude marketplace manifest missing required field: name")
	}
	if raw.Owner.Name == "" {
		return nil, errors.New("Claude marketplace manifest missing required field: owner.name")
	}
	if raw.Plugins == nil {
		return nil, errors.New("Claude marketplace manifest missing required field: plugins")
	}
	plugins := make([]PluginEntry, 0, len(raw.Plugins))
	for _, p := range raw.Plugins {
		plugins = append(plugins, PluginEntry{
			Name:        p.Name,
			Source:      p.Source,
			Description: p.Description,
			Version:     p.Version,
			Author:      p.Author,
			Homepage:    p.Homepage,
			Repository:  p.Repository,
			License:     p.License,
			Keywords:    p.Keywords,
			Category:    p.Category,
			Tags:        p.Tags,
			Strict:      p.Strict,
		})
	}
	return &Manifest{
		Format:      FormatClaude,
		Schema:      raw.Schema,
		Name:        raw.Name,
		Description: raw.Description,
		Version:     raw.Version,
		Owner:       raw.Owner,
		Metadata:    raw.Metadata,
		Plugins:     plugins,
	}, nil
}

func parseCodexManifest(data []byte) (*Manifest, error) {
	var raw codexManifest
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing Codex marketplace manifest: %w", err)
	}
	if raw.Name == "" {
		return nil, errors.New("Codex marketplace manifest missing required field: name")
	}
	if raw.Plugins == nil {
		return nil, errors.New("Codex marketplace manifest missing required field: plugins")
	}
	plugins := make([]PluginEntry, 0, len(raw.Plugins))
	for _, p := range raw.Plugins {
		plugins = append(plugins, PluginEntry{
			Name:     p.Name,
			Source:   p.Source,
			Category: p.Category,
			Policy:   p.Policy,
		})
	}
	m := &Manifest{
		Format:  FormatCodex,
		Name:    raw.Name,
		Plugins: plugins,
	}
	if raw.Interface != nil {
		m.DisplayName = raw.Interface.DisplayName
	}
	return m, nil
}

// FindManifest locates a marketplace.json given a marketplace root directory.
// It returns the path to the first file that exists, in priority order:
//
//  1. <root>/.claude-plugin/marketplace.json (Claude Code form)
//  2. <root>/.agents/plugins/marketplace.json (Codex form)
//
// If neither exists it returns an error naming both candidates.
func FindManifest(root string) (string, error) {
	candidates := []string{
		filepath.Join(root, ".claude-plugin", "marketplace.json"),
		filepath.Join(root, ".agents", "plugins", "marketplace.json"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("marketplace.json not found at any of: %v", candidates)
}

// PluginManifest mirrors a plugin's manifest. Codex's `<plugin>/plugin.json`
// can declare component paths (skills, mcpServers, apps, hooks); Claude's
// `<plugin>/.claude-plugin/plugin.json` is metadata-only.
type PluginManifest struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Version     string   `json:"version,omitempty"`
	Author      *Author  `json:"author,omitempty"`
	Homepage    string   `json:"homepage,omitempty"`
	Repository  string   `json:"repository,omitempty"`
	License     string   `json:"license,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`

	// Codex plugin.json fields. When set, the loader uses these paths instead
	// of the conventional <plugin>/skills, <plugin>/mcp, <plugin>/hooks dirs.
	SkillsPath     string          `json:"skills,omitempty"`
	McpServersPath string          `json:"mcpServers,omitempty"`
	AppsPath       string          `json:"apps,omitempty"`
	HooksField     json.RawMessage `json:"hooks,omitempty"`
}

// LoadPluginManifest reads a plugin manifest. It tries Claude's path first
// (`<plugin>/.claude-plugin/plugin.json`) and falls back to Codex's current
// (`<plugin>/.codex-plugin/plugin.json`) and legacy (`<plugin>/plugin.json`)
// paths. Missing manifest is returned as nil with no error.
func LoadPluginManifest(pluginDir string) (*PluginManifest, error) {
	candidates := []string{
		filepath.Join(pluginDir, ".claude-plugin", "plugin.json"),
		filepath.Join(pluginDir, ".codex-plugin", "plugin.json"),
		filepath.Join(pluginDir, "plugin.json"),
	}
	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("reading plugin manifest %s: %w", p, err)
		}
		var pm PluginManifest
		if err := json.Unmarshal(data, &pm); err != nil {
			return nil, fmt.Errorf("parsing plugin manifest %s: %w", p, err)
		}
		return &pm, nil
	}
	return nil, nil
}

// PluginSource describes the resolved source location of one plugin entry.
// Only one of the *Path / Repo / URL / Package fields is populated based on
// Kind. Fields are inferred from the marketplace.json `source` field of
// either Claude or Codex schema.
type PluginSource struct {
	Kind PluginSourceKind

	// Local relative path within the marketplace repo (Kind == PluginLocalPath)
	RelPath string

	// GitHub source (Kind == PluginGitHub) — Claude only
	Repo string // "owner/repo"
	Ref  string // optional branch/tag
	Sha  string // optional exact commit

	// URL source (Kind == PluginGitURL)
	URL string

	// git-subdir (Kind == PluginGitSubdir)
	SubdirPath string

	// npm (Kind == PluginNpm) — Claude only
	Package  string
	Version  string
	Registry string
}

// PluginSourceKind classifies a plugin's source field.
type PluginSourceKind int

const (
	// PluginLocalPath is a relative path within the marketplace repo.
	PluginLocalPath PluginSourceKind = iota
	// PluginGitHub is `{source: "github", repo: "owner/repo", ...}` (Claude).
	PluginGitHub
	// PluginGitURL is `{source: "url", url: "...", ...}` (Claude) or
	// `{url: "...", ...}` without explicit `source` (Codex).
	PluginGitURL
	// PluginGitSubdir is `{source: "git-subdir", url, path, ...}` (Claude) or
	// `{url, path, ...}` without explicit `source` (Codex).
	PluginGitSubdir
	// PluginNpm is `{source: "npm", package, ...}` (Claude only).
	PluginNpm
)

// ResolvePluginSource decodes a plugin entry's `source` field into a typed
// PluginSource. Both Claude's tagged-object form (`source: "github"`) and
// Codex's shape-based forms (path-only or url+path) are supported.
func ResolvePluginSource(raw json.RawMessage) (PluginSource, error) {
	if len(raw) == 0 {
		return PluginSource{}, errors.New("plugin entry missing source")
	}

	// String form (both schemas): a relative path like "./plugins/foo".
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return PluginSource{Kind: PluginLocalPath, RelPath: s}, nil
	}

	// Object form: probe for fields used by either schema.
	var obj struct {
		Source   string `json:"source"` // Claude's tag
		Repo     string `json:"repo"`
		URL      string `json:"url"`
		Path     string `json:"path"`
		Ref      string `json:"ref"`
		Sha      string `json:"sha"`
		Package  string `json:"package"`
		Version  string `json:"version"`
		Registry string `json:"registry"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return PluginSource{}, fmt.Errorf("plugin source: %w", err)
	}

	// Claude-tagged variants take precedence when `source` is set.
	switch obj.Source {
	case "github":
		if obj.Repo == "" {
			return PluginSource{}, errors.New(`github source missing "repo"`)
		}
		return PluginSource{Kind: PluginGitHub, Repo: obj.Repo, Ref: obj.Ref, Sha: obj.Sha}, nil
	case "url":
		if obj.URL == "" {
			return PluginSource{}, errors.New(`url source missing "url"`)
		}
		return PluginSource{Kind: PluginGitURL, URL: obj.URL, Ref: obj.Ref, Sha: obj.Sha}, nil
	case "git-subdir":
		if obj.URL == "" || obj.Path == "" {
			return PluginSource{}, errors.New(`git-subdir source requires "url" and "path"`)
		}
		return PluginSource{Kind: PluginGitSubdir, URL: obj.URL, SubdirPath: obj.Path, Ref: obj.Ref, Sha: obj.Sha}, nil
	case "npm":
		if obj.Package == "" {
			return PluginSource{}, errors.New(`npm source missing "package"`)
		}
		return PluginSource{Kind: PluginNpm, Package: obj.Package, Version: obj.Version, Registry: obj.Registry}, nil
	}

	// Codex shape-based variants (no `source` tag).
	switch {
	case obj.URL != "" && obj.Path != "":
		return PluginSource{Kind: PluginGitSubdir, URL: obj.URL, SubdirPath: obj.Path, Ref: obj.Ref, Sha: obj.Sha}, nil
	case obj.URL != "":
		return PluginSource{Kind: PluginGitURL, URL: obj.URL, Ref: obj.Ref, Sha: obj.Sha}, nil
	case obj.Path != "":
		return PluginSource{Kind: PluginLocalPath, RelPath: obj.Path}, nil
	}

	return PluginSource{}, fmt.Errorf("unknown plugin source: %s", string(raw))
}
