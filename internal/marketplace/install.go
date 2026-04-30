package marketplace

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"agentsbuilder/internal/assetdef"
	"agentsbuilder/internal/model"
)

// InstallTarget specifies one (provider, scope) destination for installation.
// BasePath is the filesystem root the install paths are joined against —
// project root for Project scope, the user's home directory for Global scope.
type InstallTarget struct {
	Provider model.Provider
	Scope    model.Scope
	BasePath string
}

// InstallSummary reports per-target what was copied or merged. Skipped lists
// human-readable warnings (e.g. "hooks not supported on Codex"). Errors are
// returned separately and abort the install.
type InstallSummary struct {
	Target    InstallTarget
	Skills    int
	Commands  int
	Agents    int
	Hooks     int
	Mcp       int
	Skipped   []string
}

// InstallPlugin copies the plugin's components into each target. It returns
// per-target summaries. The first error encountered aborts the operation —
// partial state may remain on disk.
func InstallPlugin(p Plugin, targets []InstallTarget) ([]InstallSummary, error) {
	if p.Dir == "" {
		return nil, fmt.Errorf("plugin %q is unresolved: %s", p.Name, p.UnresolvedReason)
	}

	summaries := make([]InstallSummary, 0, len(targets))
	for _, t := range targets {
		s, err := installTo(p, t)
		if err != nil {
			return summaries, err
		}
		summaries = append(summaries, s)
	}
	return summaries, nil
}

func installTo(p Plugin, t InstallTarget) (InstallSummary, error) {
	s := InstallSummary{Target: t}

	// Skills → DestDir(provider, scope, Skills). Each plugin skill is a
	// directory; we copy it into the skills root with the same name.
	if len(p.Skills) > 0 {
		dst := assetdef.DestDir(t.Provider, model.Skills)
		if dst == "" {
			s.Skipped = append(s.Skipped, fmt.Sprintf("skills not supported on %s", t.Provider.String()))
		} else {
			full := filepath.Join(t.BasePath, dst)
			for _, sk := range p.Skills {
				name := filepath.Base(sk)
				if err := copyDir(sk, filepath.Join(full, name)); err != nil {
					return s, fmt.Errorf("copying skill %s: %w", name, err)
				}
				s.Skills++
			}
		}
	}

	// Commands → same destination as Skills (Claude Code uses .claude/commands
	// for both flat command markdowns and skill directories).
	if len(p.Commands) > 0 {
		dst := assetdef.DestDir(t.Provider, model.Skills)
		if dst == "" {
			s.Skipped = append(s.Skipped, fmt.Sprintf("commands not supported on %s", t.Provider.String()))
		} else {
			full := filepath.Join(t.BasePath, dst)
			if err := os.MkdirAll(full, 0o755); err != nil {
				return s, err
			}
			for _, c := range p.Commands {
				if err := copyFile(c, filepath.Join(full, filepath.Base(c))); err != nil {
					return s, fmt.Errorf("copying command %s: %w", filepath.Base(c), err)
				}
				s.Commands++
			}
		}
	}

	// Agents → DestDir(provider, scope, Agents). Each agent is a single .md.
	if len(p.Agents) > 0 {
		dst := assetdef.DestDir(t.Provider, model.Agents)
		if dst == "" {
			s.Skipped = append(s.Skipped, fmt.Sprintf("agents not supported on %s", t.Provider.String()))
		} else {
			full := filepath.Join(t.BasePath, dst)
			if err := os.MkdirAll(full, 0o755); err != nil {
				return s, err
			}
			for _, a := range p.Agents {
				if err := copyFile(a, filepath.Join(full, filepath.Base(a))); err != nil {
					return s, fmt.Errorf("copying agent %s: %w", filepath.Base(a), err)
				}
				s.Agents++
			}
		}
	}

	// Hooks → format-detect each snippet, install only into the provider whose
	// schema matches. Claude Code's hooks (PreToolUse/PostToolUse/etc.) and
	// Codex's hooks (after_agent/after_tool_use) are not interchangeable, so
	// snippets authored for one are silently skipped for the other.
	for _, h := range p.HookFiles {
		format, err := detectHooksFormat(h)
		if err != nil {
			s.Skipped = append(s.Skipped,
				fmt.Sprintf("hooks %s: 解析失敗 (%v)", filepath.Base(h), err))
			continue
		}

		want := providerHookFormat(t.Provider)
		if !formatMatches(format, want) {
			s.Skipped = append(s.Skipped,
				fmt.Sprintf("hooks %s は %s 形式のため %s には適用しません",
					filepath.Base(h), formatLabel(format), t.Provider.String()))
			continue
		}

		def, ok := assetdef.Lookup(t.Provider, t.Scope, model.Hooks)
		if !ok {
			s.Skipped = append(s.Skipped,
				fmt.Sprintf("hooks not supported at %s/%s",
					t.Provider.String(), scopeLabel(t.Scope)))
			continue
		}
		target := filepath.Join(t.BasePath, def.PrimaryPath)
		switch def.Storage {
		case assetdef.EmbeddedJSON:
			if err := mergeJSONFile(h, target); err != nil {
				return s, fmt.Errorf("merging hooks %s: %w", filepath.Base(h), err)
			}
		case assetdef.EmbeddedTOML:
			if err := mergeJSONHooksIntoTOML(h, target); err != nil {
				return s, fmt.Errorf("merging hooks %s into TOML: %w", filepath.Base(h), err)
			}
		default:
			return s, fmt.Errorf("unexpected hooks storage kind for %s", t.Provider.String())
		}
		s.Hooks++
	}

	// MCP → merge into the provider's MCP config. For Codex the JSON snippet
	// is converted to TOML before merging.
	if len(p.McpFiles) > 0 {
		def, ok := assetdef.Lookup(t.Provider, t.Scope, model.MCP)
		if !ok {
			s.Skipped = append(s.Skipped,
				fmt.Sprintf("MCP not supported on %s", t.Provider.String()))
		} else {
			target := filepath.Join(t.BasePath, def.PrimaryPath)
			for _, mp := range p.McpFiles {
				switch def.Storage {
				case assetdef.EmbeddedJSON:
					if err := mergeJSONFile(mp, target); err != nil {
						return s, fmt.Errorf("merging MCP %s: %w", filepath.Base(mp), err)
					}
				case assetdef.EmbeddedTOML:
					if err := mergeJSONIntoTOML(mp, target); err != nil {
						return s, fmt.Errorf("merging MCP %s into TOML: %w", filepath.Base(mp), err)
					}
				default:
					return s, fmt.Errorf("unexpected MCP storage kind for %s", t.Provider.String())
				}
				s.Mcp++
			}
		}
	}

	return s, nil
}

// mergeJSONFile reads snippetPath and merges its top-level keys into target.
// For object-valued keys, entries are merged in (snippet wins on conflict);
// for other types, the snippet value replaces the existing one.
func mergeJSONFile(snippetPath, target string) error {
	snippetData, err := os.ReadFile(snippetPath)
	if err != nil {
		return err
	}
	var snippet map[string]json.RawMessage
	if err := json.Unmarshal(snippetData, &snippet); err != nil {
		return fmt.Errorf("parsing snippet: %w", err)
	}

	var current map[string]json.RawMessage
	if data, err := os.ReadFile(target); err == nil {
		if err := json.Unmarshal(data, &current); err != nil {
			return fmt.Errorf("parsing target %s: %w", target, err)
		}
	}
	if current == nil {
		current = make(map[string]json.RawMessage)
	}

	for key, value := range snippet {
		existing, ok := current[key]
		if !ok {
			current[key] = value
			continue
		}
		var existingMap, snippetMap map[string]json.RawMessage
		if json.Unmarshal(existing, &existingMap) == nil &&
			json.Unmarshal(value, &snippetMap) == nil {
			for k, v := range snippetMap {
				existingMap[k] = v
			}
			merged, err := json.Marshal(existingMap)
			if err != nil {
				return err
			}
			current[key] = merged
		} else {
			current[key] = value
		}
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(target, append(out, '\n'), 0o644)
}

// copyDir recursively copies src into dst, creating directories as needed.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

// copyFile copies src to dst, creating parent dirs.
func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// mergeJSONIntoTOML converts a Claude-Code-style MCP JSON snippet to TOML and
// appends it to target. Expected snippet shape:
//
//	{ "mcpServers": { "<name>": { "command": "...", "args": [...], "env": {...} } } }
//
// HTTP transport entries (snippet contains `url` instead of `command`) are
// emitted with their fields written through verbatim: Codex's HTTP fields are
// `url`, `bearer_token_env_var`, `http_headers`, but Claude's are `url`,
// `bearer_token`, `headers`. Per the project's design decision, we do *not*
// translate field names — Codex may not accept the resulting entries, but the
// install does not silently drop them.
func mergeJSONIntoTOML(snippetPath, target string) error {
	return mergeJSONNamedTableIntoTOML(snippetPath, target, "mcpServers", "mcp_servers", "env")
}

// mergeJSONHooksIntoTOML converts a Codex-style hooks JSON snippet to TOML.
// Expected snippet shape mirrors Codex's [hooks.<name>] table:
//
//	{ "hooks": { "<name>": { "event": "after_tool_use", "command": "..." } } }
//
// or the equivalent flat top-level form (no enclosing "hooks" key).
func mergeJSONHooksIntoTOML(snippetPath, target string) error {
	return mergeJSONNamedTableIntoTOML(snippetPath, target, "hooks", "hooks", "")
}

// mergeJSONNamedTableIntoTOML reads a JSON snippet shaped like
// `{ <topKey>: { <name>: { ...fields... } } }` and appends each named entry
// to target as a `[<table>.<name>]` TOML section. When subtableField is
// non-empty, the named field's value (expected to be a string-keyed object)
// becomes a `[<table>.<name>.<subtableField>]` sub-table.
//
// If the snippet's top level *is* the named map (i.e. <topKey> is absent and
// the root looks like `{ <name>: {...} }`), that shape is also accepted.
//
// Existing sections of the same name in target are left untouched and skipped.
func mergeJSONNamedTableIntoTOML(snippetPath, target, topKey, table, subtableField string) error {
	data, err := os.ReadFile(snippetPath)
	if err != nil {
		return err
	}
	entries, err := extractNamedMap(data, topKey)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	existing := ""
	if b, err := os.ReadFile(target); err == nil {
		existing = string(b)
	}
	f, err := os.OpenFile(target, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	for name, entry := range entries {
		header := fmt.Sprintf("[%s.%s]", table, name)
		if strings.Contains(existing, header) {
			continue
		}
		var b strings.Builder
		b.WriteString("\n")
		b.WriteString(header)
		b.WriteString("\n")

		var subRaw json.RawMessage
		for k, v := range entry {
			if subtableField != "" && k == subtableField {
				subRaw = v
				continue
			}
			b.WriteString(k)
			b.WriteString(" = ")
			b.WriteString(tomlValue(v))
			b.WriteString("\n")
		}
		if len(subRaw) > 0 {
			var sub map[string]string
			if err := json.Unmarshal(subRaw, &sub); err == nil && len(sub) > 0 {
				b.WriteString("\n")
				b.WriteString(fmt.Sprintf("[%s.%s.%s]\n", table, name, subtableField))
				for k, v := range sub {
					b.WriteString(k)
					b.WriteString(" = ")
					b.WriteString(tomlString(v))
					b.WriteString("\n")
				}
			}
		}
		if _, err := f.WriteString(b.String()); err != nil {
			return err
		}
	}
	return nil
}

// extractNamedMap unmarshals data and returns the named-map shape used by
// mergeJSONNamedTableIntoTOML. It accepts both the wrapped form
// `{ topKey: { <name>: {...} } }` and the bare form `{ <name>: {...} }`.
func extractNamedMap(data []byte, topKey string) (map[string]map[string]json.RawMessage, error) {
	// Try wrapped form first.
	var wrapped map[string]map[string]map[string]json.RawMessage
	if err := json.Unmarshal(data, &wrapped); err == nil {
		if entries, ok := wrapped[topKey]; ok {
			return entries, nil
		}
	}
	// Fall back to bare form.
	var bare map[string]map[string]json.RawMessage
	if err := json.Unmarshal(data, &bare); err != nil {
		return nil, fmt.Errorf("parsing snippet: %w", err)
	}
	return bare, nil
}

// ─── Hook-format detection ────────────────────────────────────────────────────

// HooksFormat classifies a hooks JSON snippet by which provider's schema it
// matches. The same snippet can declare entries in both formats — that case
// returns `HooksFormatBoth` and the install layer treats it as installable to
// either provider.
type HooksFormat int

const (
	// HooksFormatUnknown means no recognisable Claude/Codex event names were
	// found and the snippet should be skipped on every provider.
	HooksFormatUnknown HooksFormat = iota
	// HooksFormatClaude matches Claude Code event names (PreToolUse, etc.).
	HooksFormatClaude
	// HooksFormatCodex matches Codex event names (after_agent, after_tool_use).
	HooksFormatCodex
	// HooksFormatBoth is set when both schemas' marker keys appear.
	HooksFormatBoth
)

var (
	claudeHookEvents = map[string]struct{}{
		"PreToolUse":        {},
		"PostToolUse":       {},
		"UserPromptSubmit":  {},
		"SessionStart":      {},
		"Stop":              {},
		"SubagentStop":      {},
		"Notification":      {},
		"PreCompact":        {},
		"PermissionRequest": {},
	}
	codexHookEvents = map[string]struct{}{
		"after_agent":    {},
		"after_tool_use": {},
	}
)

// detectHooksFormat reads a JSON snippet and classifies its schema by the
// presence of Claude or Codex hook event names anywhere in the document.
//
// Claude Code expresses event names as map *keys* (e.g. `{"PreToolUse": [...]}`)
// while Codex expresses them as string *values* of an `event` field
// (e.g. `{"my_hook": {"event": "after_tool_use", ...}}`). Detection therefore
// walks both keys and leaf string values.
func detectHooksFormat(snippetPath string) (HooksFormat, error) {
	data, err := os.ReadFile(snippetPath)
	if err != nil {
		return HooksFormatUnknown, err
	}
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		return HooksFormatUnknown, fmt.Errorf("parsing hooks snippet: %w", err)
	}
	var hasClaude, hasCodex bool
	walkJSON(doc, func(k string, isKey bool) {
		if isKey {
			if _, ok := claudeHookEvents[k]; ok {
				hasClaude = true
			}
			if _, ok := codexHookEvents[k]; ok {
				hasCodex = true
			}
			return
		}
		// String value: only Codex events are checked here. Claude events
		// would never appear as values in a well-formed snippet.
		if _, ok := codexHookEvents[k]; ok {
			hasCodex = true
		}
	})
	switch {
	case hasClaude && hasCodex:
		return HooksFormatBoth, nil
	case hasClaude:
		return HooksFormatClaude, nil
	case hasCodex:
		return HooksFormatCodex, nil
	}
	return HooksFormatUnknown, nil
}

// walkJSON recursively visits every map key (with isKey=true) and every leaf
// string value (with isKey=false).
func walkJSON(v any, visit func(s string, isKey bool)) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			visit(k, true)
			walkJSON(child, visit)
		}
	case []any:
		for _, child := range t {
			walkJSON(child, visit)
		}
	case string:
		visit(t, false)
	}
}

// providerHookFormat returns the hook format expected by a provider.
func providerHookFormat(p model.Provider) HooksFormat {
	if p == model.Codex {
		return HooksFormatCodex
	}
	return HooksFormatClaude
}

// formatMatches reports whether `got` is compatible with `want`. Snippets
// containing both schemas (`HooksFormatBoth`) match every concrete request.
func formatMatches(got, want HooksFormat) bool {
	if got == HooksFormatBoth {
		return true
	}
	return got == want
}

// formatLabel returns a short Japanese label for user-facing messages.
func formatLabel(f HooksFormat) string {
	switch f {
	case HooksFormatClaude:
		return "Claude Code"
	case HooksFormatCodex:
		return "Codex"
	case HooksFormatBoth:
		return "両対応"
	}
	return "形式不明"
}

// scopeLabel returns a short label for an install scope.
func scopeLabel(s model.Scope) string {
	if s == model.Project {
		return "Project"
	}
	return "Global"
}

// tomlValue renders a JSON value as a TOML literal. Strings are quoted, arrays
// of strings become TOML inline arrays, and other JSON shapes fall back to
// JSON-encoded strings (machine-readable but not idiomatic TOML).
func tomlValue(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return tomlString(s)
	}

	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		parts := make([]string, len(arr))
		for i, v := range arr {
			parts[i] = tomlString(v)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}

	// Numbers, booleans, nulls — TOML accepts them as-is in most cases.
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "true" || trimmed == "false" {
		return trimmed
	}
	if _, err := json.Number(trimmed).Int64(); err == nil {
		return trimmed
	}
	if _, err := json.Number(trimmed).Float64(); err == nil {
		return trimmed
	}
	// Fallback: store the raw JSON inside a TOML string.
	return tomlString(string(raw))
}

// tomlString quotes s as a TOML basic string, escaping " and \.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
