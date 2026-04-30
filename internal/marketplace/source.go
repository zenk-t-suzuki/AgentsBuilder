// Package marketplace implements Claude Code-compatible plugin marketplace
// support. A marketplace is a Git repository, local directory, or URL that
// hosts a `.claude-plugin/marketplace.json` catalog plus a set of plugin
// directories. AgentsBuilder uses these marketplaces to discover and install
// plugins into Claude Code or Codex providers.
//
// The schema this package implements is documented at:
//
//	https://code.claude.com/docs/en/plugin-marketplaces
//	https://code.claude.com/docs/en/plugins-reference
package marketplace

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// SourceKind classifies how a marketplace source must be fetched.
type SourceKind int

const (
	// SourceGit is a Git repository (any host) accessed via clone/pull.
	SourceGit SourceKind = iota
	// SourceLocalDir is a local directory that already contains
	// .claude-plugin/marketplace.json.
	SourceLocalDir
	// SourceLocalJSON is a direct path to a marketplace.json file.
	SourceLocalJSON
	// SourceRemoteJSON is an HTTP(S) URL that returns marketplace.json.
	SourceRemoteJSON
)

func (k SourceKind) String() string {
	switch k {
	case SourceGit:
		return "git"
	case SourceLocalDir:
		return "local-dir"
	case SourceLocalJSON:
		return "local-json"
	case SourceRemoteJSON:
		return "remote-json"
	}
	return "unknown"
}

// Source describes a parsed marketplace source. Raw is what the user typed,
// preserved so we can round-trip it through config.json.
type Source struct {
	Kind SourceKind
	Raw  string // original input, e.g. "anthropics/skills#main"
	URL  string // normalized URL (git URL or remote JSON URL); empty for local sources
	Path string // local filesystem path; empty for non-local sources
	Ref  string // git branch/tag, parsed from "#ref" suffix; empty for unpinned
}

// shorthand pattern: "owner/repo" without protocol or path separator, allowing
// only the limited character set used by GitHub for owners and repository names.
var shorthandRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9._-]+$`)

// ParseSource accepts every form documented for `/plugin marketplace add`:
//
//   - GitHub shorthand:   anthropics/skills            (with optional #ref)
//   - HTTPS git URL:      https://github.com/foo/bar   (with optional #ref, .git ok)
//   - SSH git URL:        git@github.com:foo/bar.git
//   - Local directory:    ./my-mp  or  /abs/my-mp      (must contain .claude-plugin/)
//   - Local JSON file:    ./path/to/marketplace.json
//   - Remote JSON URL:    https://example.com/marketplace.json
//
// The "#ref" suffix is only meaningful for Git sources and is preserved on
// Source.Ref. For non-Git sources, "#ref" is rejected as an error.
func ParseSource(raw string) (Source, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return Source{}, errors.New("source is empty")
	}

	// Split off "#ref" — but only the *last* "#" so URL fragments inside paths
	// (rare) survive. Refs cannot contain "#" themselves.
	var ref string
	if i := strings.LastIndex(s, "#"); i >= 0 && !isFileSchemeFragmentSafe(s, i) {
		ref = s[i+1:]
		s = s[:i]
	}

	src := Source{Raw: raw, Ref: ref}

	switch {
	case strings.HasPrefix(s, "git@"):
		// SSH URL — Git only, ref optional
		src.Kind = SourceGit
		src.URL = s
		return src, nil

	case strings.HasPrefix(s, "https://"), strings.HasPrefix(s, "http://"):
		if strings.HasSuffix(strings.ToLower(s), ".json") {
			if ref != "" {
				return Source{}, errors.New("ref pin (#) is not supported for marketplace.json URLs")
			}
			src.Kind = SourceRemoteJSON
			src.URL = s
			return src, nil
		}
		src.Kind = SourceGit
		src.URL = normalizeGitURL(s)
		return src, nil

	case strings.HasPrefix(s, "./"), strings.HasPrefix(s, "../"),
		strings.HasPrefix(s, "/"), strings.HasPrefix(s, "~"):
		if ref != "" {
			return Source{}, errors.New("ref pin (#) is not supported for local sources")
		}
		path := s
		if strings.HasPrefix(path, "~") {
			// Expand ~ at install time, not parse time — keep raw path here.
			// Callers can resolve via os.UserHomeDir.
		}
		if strings.HasSuffix(strings.ToLower(path), ".json") {
			src.Kind = SourceLocalJSON
		} else {
			src.Kind = SourceLocalDir
		}
		src.Path = path
		return src, nil

	case shorthandRe.MatchString(s):
		// owner/repo → GitHub HTTPS clone URL
		src.Kind = SourceGit
		src.URL = "https://github.com/" + s + ".git"
		return src, nil
	}

	return Source{}, fmt.Errorf("unrecognized source format: %q", raw)
}

// isFileSchemeFragmentSafe returns true when a "#" at position i is part of
// a Windows-style path or other context where it should not be treated as a
// ref delimiter. We currently only flag literal "C:\" / "C:/" prefixes — Linux
// is the supported platform so this is a placeholder for future safety.
func isFileSchemeFragmentSafe(_ string, _ int) bool { return false }

// normalizeGitURL adds ".git" to common Git host URLs that omit it, so both
// `https://github.com/foo/bar` and `https://github.com/foo/bar.git` work.
// URLs to other hosts are returned unchanged.
func normalizeGitURL(u string) string {
	if strings.HasSuffix(u, ".git") {
		return u
	}
	for _, host := range []string{"github.com/", "gitlab.com/", "bitbucket.org/"} {
		if strings.Contains(u, "://"+host) {
			return strings.TrimRight(u, "/") + ".git"
		}
	}
	return u
}

// CacheKey returns a stable, filesystem-safe identifier for caching this
// source. For Git sources this is host_owner_repo_ref; for local sources this
// is a hash of the absolute path. The cache key is *not* the marketplace
// name (which comes from marketplace.json `name`) — it only ensures that two
// distinct sources don't collide on disk before we've read the manifest.
func (s Source) CacheKey() string {
	switch s.Kind {
	case SourceGit:
		key := strings.NewReplacer("https://", "", "http://", "", "git@", "",
			":", "_", "/", "_").Replace(s.URL)
		key = strings.TrimSuffix(key, ".git")
		if s.Ref != "" {
			key += "@" + s.Ref
		}
		return key
	case SourceLocalDir, SourceLocalJSON:
		// Resolve to absolute path for stability across CWD changes. Errors
		// fall back to the raw path (still deterministic for the same input).
		if abs, err := filepath.Abs(s.Path); err == nil {
			return "local_" + strings.NewReplacer("/", "_", "\\", "_").Replace(abs)
		}
		return "local_" + strings.NewReplacer("/", "_", "\\", "_").Replace(s.Path)
	case SourceRemoteJSON:
		return strings.NewReplacer("https://", "", "http://", "",
			"/", "_", ":", "_", "?", "_").Replace(s.URL)
	}
	return s.Raw
}
