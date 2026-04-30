package marketplace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Manifest mirrors the Claude Code `marketplace.json` schema. Only fields we
// currently use are typed strongly; anything else is preserved via the
// json.RawMessage `source` so unknown plugin source kinds don't break loading.
//
// See: https://code.claude.com/docs/en/plugin-marketplaces#marketplace-schema
type Manifest struct {
	Schema      string        `json:"$schema,omitempty"`
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Version     string        `json:"version,omitempty"`
	Owner       Owner         `json:"owner"`
	Metadata    *Metadata     `json:"metadata,omitempty"`
	Plugins     []PluginEntry `json:"plugins"`
}

// Owner identifies the marketplace maintainer (required by spec).
type Owner struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

// Metadata holds optional marketplace-level configuration.
type Metadata struct {
	PluginRoot string `json:"pluginRoot,omitempty"`
}

// PluginEntry is one entry in a marketplace's `plugins` array. The `source`
// field can be either a string (relative path) or an object describing a
// remote source — we store it raw and decode on demand via ParseSource.
type PluginEntry struct {
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

// Author identifies a plugin author.
type Author struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

// PluginSource describes the resolved source location of one plugin entry.
// Only one of the *Path / Repo / URL / Package fields is populated based on
// Kind. Fields are inferred from the marketplace.json `source` field.
type PluginSource struct {
	Kind PluginSourceKind

	// Local relative path within the marketplace repo (Kind == PluginLocalPath)
	RelPath string

	// GitHub source (Kind == PluginGitHub)
	Repo string // "owner/repo"
	Ref  string // optional branch/tag
	Sha  string // optional exact commit

	// URL source (Kind == PluginGitURL)
	URL string

	// git-subdir (Kind == PluginGitSubdir)
	SubdirPath string

	// npm (Kind == PluginNpm)
	Package  string
	Version  string
	Registry string
}

// PluginSourceKind classifies a plugin's source field.
type PluginSourceKind int

const (
	// PluginLocalPath is a relative path within the marketplace repo.
	PluginLocalPath PluginSourceKind = iota
	// PluginGitHub is `{source: "github", repo: "owner/repo", ...}`.
	PluginGitHub
	// PluginGitURL is `{source: "url", url: "...", ...}`.
	PluginGitURL
	// PluginGitSubdir is `{source: "git-subdir", url, path, ...}`.
	PluginGitSubdir
	// PluginNpm is `{source: "npm", package, ...}`.
	PluginNpm
)

// PluginManifest mirrors `<plugin>/.claude-plugin/plugin.json`.
type PluginManifest struct {
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	Version     string  `json:"version,omitempty"`
	Author      *Author `json:"author,omitempty"`
	Homepage    string  `json:"homepage,omitempty"`
	Repository  string  `json:"repository,omitempty"`
	License     string  `json:"license,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
}

// LoadManifest reads and validates a marketplace.json file. Returns a typed
// error for missing, unreadable, or schema-invalid files.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading marketplace manifest: %w", err)
	}
	return ParseManifest(data)
}

// ParseManifest parses raw marketplace.json bytes.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing marketplace manifest: %w", err)
	}
	if m.Name == "" {
		return nil, errors.New("marketplace manifest missing required field: name")
	}
	if m.Owner.Name == "" {
		return nil, errors.New("marketplace manifest missing required field: owner.name")
	}
	if m.Plugins == nil {
		// Empty array is OK; nil means the field was absent.
		return nil, errors.New("marketplace manifest missing required field: plugins")
	}
	return &m, nil
}

// FindManifest locates marketplace.json given a marketplace root directory.
// It returns the path to .claude-plugin/marketplace.json under root, or an
// error if not present.
func FindManifest(root string) (string, error) {
	p := filepath.Join(root, ".claude-plugin", "marketplace.json")
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("marketplace.json not found at %s: %w", p, err)
	}
	return p, nil
}

// LoadPluginManifest reads .claude-plugin/plugin.json from a plugin directory.
// Missing manifest is returned as nil with no error so callers can fall back
// to the marketplace entry's metadata.
func LoadPluginManifest(pluginDir string) (*PluginManifest, error) {
	p := filepath.Join(pluginDir, ".claude-plugin", "plugin.json")
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading plugin manifest %s: %w", p, err)
	}
	var pm PluginManifest
	if err := json.Unmarshal(data, &pm); err != nil {
		return nil, fmt.Errorf("parsing plugin manifest %s: %w", p, err)
	}
	return &pm, nil
}

// ResolvePluginSource decodes a plugin entry's `source` field into a typed
// PluginSource. The `source` field can be either a string (a relative path
// like "./plugins/foo") or an object describing a remote source.
func ResolvePluginSource(raw json.RawMessage) (PluginSource, error) {
	if len(raw) == 0 {
		return PluginSource{}, errors.New("plugin entry missing source")
	}

	// Try string form first.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		s = trimQuotes(s)
		return PluginSource{Kind: PluginLocalPath, RelPath: s}, nil
	}

	// Object form.
	var obj struct {
		Source   string `json:"source"`
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
	return PluginSource{}, fmt.Errorf("unknown plugin source kind: %q", obj.Source)
}

func trimQuotes(s string) string { return s }
