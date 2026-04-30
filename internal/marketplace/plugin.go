package marketplace

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Plugin describes one installable plugin discovered inside a marketplace.
// Component file paths are absolute so callers can copy/merge directly.
type Plugin struct {
	// Identification
	Name        string
	DisplayName string
	Description string
	Version     string
	Author      string
	Homepage    string
	License     string

	// Marketplace this plugin belongs to (the catalog name from marketplace.json).
	Marketplace string

	// Dir is the local directory containing the plugin's files (with
	// .claude-plugin/plugin.json + skills/agents/hooks/mcp). Empty when the
	// plugin source could not be resolved (e.g. an unsupported source kind).
	Dir string

	// Resolution failure reason (non-empty when Dir is empty).
	UnresolvedReason string

	// Discovered component locations (absolute paths under Dir).
	Skills    []string // skill dirs (each contains SKILL.md)
	Commands  []string // *.md files
	Agents    []string // *.md files
	HookFiles []string // *.json files under hooks/ (or single configured file)
	McpFiles  []string // *.json files under mcp/ (or single configured file)
}

// LoadMarketplace reads marketplace.json from `manifestPath` (which can be
// either the manifest file itself or a directory containing
// .claude-plugin/marketplace.json) and resolves every plugin entry to a
// Plugin struct with discovered components.
//
// Plugin entries whose `source` references an external repository (github/url
// /git-subdir/npm) are cloned into a per-plugin sub-cache so the same install
// flow works regardless of source kind.
func LoadMarketplace(manifestPath string) (*Manifest, []Plugin, error) {
	root, manifest, err := readManifestAtAnyPath(manifestPath)
	if err != nil {
		return nil, nil, err
	}

	plugins := make([]Plugin, 0, len(manifest.Plugins))
	for _, e := range manifest.Plugins {
		p := Plugin{
			Name:        e.Name,
			DisplayName: e.Name,
			Description: e.Description,
			Version:     e.Version,
			Marketplace: manifest.Name,
			Homepage:    e.Homepage,
			License:     e.License,
		}
		if e.Author != nil {
			p.Author = e.Author.Name
		}

		dir, reason := resolvePluginDir(root, manifest.Metadata, e)
		if dir == "" {
			p.UnresolvedReason = reason
		} else {
			p.Dir = dir
			pm, _ := LoadPluginManifest(dir)
			discoverComponents(&p, pm)
			// Plugin manifest overrides metadata when present.
			if pm != nil {
				if pm.Description != "" {
					p.Description = pm.Description
				}
				if pm.Version != "" {
					p.Version = pm.Version
				}
				if pm.Author != nil && pm.Author.Name != "" {
					p.Author = pm.Author.Name
				}
				if pm.Homepage != "" {
					p.Homepage = pm.Homepage
				}
				if pm.License != "" {
					p.License = pm.License
				}
			}
		}
		plugins = append(plugins, p)
	}
	return manifest, plugins, nil
}

// readManifestAtAnyPath accepts either a directory (containing
// .claude-plugin/marketplace.json) or a direct path to marketplace.json, and
// returns the marketplace root directory plus the parsed manifest.
//
// For URL-based marketplaces (where path is a *.json file fetched into the
// cache), root is set to the file's parent directory; relative plugin paths
// will not resolve correctly there — that limitation is documented in the
// upstream spec.
func readManifestAtAnyPath(path string) (string, *Manifest, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", nil, err
	}
	var manifestPath, root string
	if info.IsDir() {
		manifestPath, err = FindManifest(path)
		if err != nil {
			return "", nil, err
		}
		root = path
	} else {
		manifestPath = path
		root = filepath.Dir(filepath.Dir(path)) // strip .claude-plugin/
	}

	m, err := LoadManifest(manifestPath)
	if err != nil {
		return "", nil, err
	}
	return root, m, nil
}

// resolvePluginDir returns the on-disk directory containing the plugin's
// files, cloning external sources into a per-plugin sub-cache when needed.
// On failure returns ("", reason).
func resolvePluginDir(marketplaceRoot string, metadata *Metadata, e PluginEntry) (string, string) {
	src, err := ResolvePluginSource(e.Source)
	if err != nil {
		return "", err.Error()
	}

	switch src.Kind {
	case PluginLocalPath:
		rel := src.RelPath
		// Apply metadata.pluginRoot prefix when the path doesn't start with "./".
		if metadata != nil && metadata.PluginRoot != "" && !strings.HasPrefix(rel, "./") && !strings.HasPrefix(rel, "../") && !filepath.IsAbs(rel) {
			rel = filepath.Join(metadata.PluginRoot, rel)
		}
		dir := filepath.Join(marketplaceRoot, filepath.FromSlash(rel))
		if _, err := os.Stat(dir); err != nil {
			return "", fmt.Sprintf("plugin path not found: %s", dir)
		}
		return dir, ""

	case PluginGitHub:
		url := "https://github.com/" + src.Repo + ".git"
		return clonePluginCache(e.Name, url, src.Ref, src.Sha)

	case PluginGitURL:
		return clonePluginCache(e.Name, src.URL, src.Ref, src.Sha)

	case PluginGitSubdir:
		dir, reason := clonePluginCache(e.Name, src.URL, src.Ref, src.Sha)
		if dir == "" {
			return "", reason
		}
		sub := filepath.Join(dir, filepath.FromSlash(src.SubdirPath))
		if _, err := os.Stat(sub); err != nil {
			return "", fmt.Sprintf("git-subdir path not found: %s", sub)
		}
		return sub, ""

	case PluginNpm:
		return "", "npm plugin sources are not yet supported"
	}
	return "", "unknown plugin source kind"
}

// clonePluginCache clones a plugin's source repo into the marketplace cache
// under "<cacheRoot>/_plugins/<sanitized-name>" so each external plugin has
// its own checkout. Re-clones are skipped when the directory already exists.
//
// `sha` pinning is supported by checkout-after-clone since `git clone --depth=1`
// to a specific commit requires a server-side option not all hosts honor.
func clonePluginCache(pluginName, url, ref, sha string) (string, string) {
	if cacheRoot == "" {
		return "", "marketplace cache root not configured"
	}
	safe := strings.NewReplacer("/", "_", "\\", "_", " ", "_").Replace(pluginName)
	dest := filepath.Join(cacheRoot, "_plugins", safe)

	if _, err := os.Stat(filepath.Join(dest, ".git")); err == nil {
		// Already cloned. Skip update — refresh happens on marketplace sync.
		return dest, ""
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err.Error()
	}

	args := []string{"clone", "-q"}
	if sha == "" {
		args = append(args, "--depth=1")
		if ref != "" {
			args = append(args, "--branch", ref)
		}
	}
	args = append(args, url, dest)

	if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		_ = os.RemoveAll(dest)
		return "", strings.TrimSpace(string(out))
	}

	if sha != "" {
		if out, err := exec.Command("git", "-C", dest, "checkout", "-q", sha).CombinedOutput(); err != nil {
			_ = os.RemoveAll(dest)
			return "", strings.TrimSpace(string(out))
		}
	}
	return dest, ""
}

// discoverComponents populates p.Skills/Commands/Agents/HookFiles/McpFiles by
// walking the plugin layout. When the plugin's own manifest declares custom
// paths (Codex's plugin.json `skills`, `mcpServers`, `apps`, `hooks` fields),
// those override the conventional `skills/`, `mcp/`, `hooks/` directories.
func discoverComponents(p *Plugin, pm *PluginManifest) {
	skillsDir := filepath.Join(p.Dir, "skills")
	mcpDir := filepath.Join(p.Dir, "mcp")
	hooksDir := filepath.Join(p.Dir, "hooks")

	if pm != nil {
		if pm.SkillsPath != "" {
			skillsDir = filepath.Join(p.Dir, filepath.FromSlash(pm.SkillsPath))
		}
		if pm.McpServersPath != "" {
			mcpDir = filepath.Join(p.Dir, filepath.FromSlash(pm.McpServersPath))
		}
	}

	p.Skills = collectSkillDirs(skillsDir)
	p.Commands = listFilesWithExt(filepath.Join(p.Dir, "commands"), ".md")
	p.Agents = append(listFilesWithExt(filepath.Join(p.Dir, "agents"), ".md"),
		listFilesWithExt(filepath.Join(p.Dir, "agents"), ".toml")...)
	p.McpFiles = collectJSONOrDir(mcpDir)
	if len(p.McpFiles) == 0 {
		p.McpFiles = collectJSONOrDir(filepath.Join(p.Dir, ".mcp.json"))
	}

	// Hooks: Codex's plugin.json may declare a string path to a single hooks
	// file (or, less commonly, an inline object/array — those are skipped here
	// and would need bespoke handling). Fall back to listing hooks/*.json.
	if pm != nil && len(pm.HooksField) > 0 {
		var hooksPath string
		if err := json.Unmarshal(pm.HooksField, &hooksPath); err == nil && hooksPath != "" {
			full := filepath.Join(p.Dir, filepath.FromSlash(hooksPath))
			if fi, err := os.Stat(full); err == nil && !fi.IsDir() {
				p.HookFiles = []string{full}
			}
		}
	}
	if p.HookFiles == nil {
		p.HookFiles = listFilesWithExt(hooksDir, ".json")
	}
}

// collectSkillDirs returns installable skill package directories. Codex plugin
// manifests can point `skills` at a directory that itself contains SKILL.md;
// Claude-style marketplaces usually put one skill package per child directory.
func collectSkillDirs(root string) []string {
	if _, err := os.Stat(filepath.Join(root, "SKILL.md")); err == nil {
		return []string{root}
	}
	return listSubdirsContaining(root, "SKILL.md")
}

// collectJSONOrDir returns the absolute paths of all .json files under root
// when root is a directory, or the path itself wrapped in a slice when root
// is a single .json file. Missing root returns nil.
func collectJSONOrDir(root string) []string {
	info, err := os.Stat(root)
	if err != nil {
		return nil
	}
	if !info.IsDir() {
		return []string{root}
	}
	return listFilesWithExt(root, ".json")
}

// listSubdirsContaining returns absolute paths to every direct subdirectory
// of root that contains a file named markerFile. Missing root is not an error.
func listSubdirsContaining(root, markerFile string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if _, err := os.Stat(filepath.Join(dir, markerFile)); err == nil {
			out = append(out, dir)
		}
	}
	return out
}

// listFilesWithExt returns absolute paths to every regular file directly under
// root whose extension matches ext (case-insensitive). Missing root is not an
// error. Files starting with '.' are skipped.
func listFilesWithExt(root, ext string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	want := strings.ToLower(ext)
	var out []string
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if strings.ToLower(filepath.Ext(e.Name())) == want {
			out = append(out, filepath.Join(root, e.Name()))
		}
	}
	return out
}
