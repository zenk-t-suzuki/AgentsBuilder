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

	// Hooks → merge each JSON file into the provider's settings.json.
	// Codex has no hooks support; skip with a warning.
	if len(p.HookFiles) > 0 {
		def, ok := assetdef.Lookup(t.Provider, t.Scope, model.Hooks)
		if !ok {
			s.Skipped = append(s.Skipped,
				fmt.Sprintf("hooks not supported on %s", t.Provider.String()))
		} else {
			target := filepath.Join(t.BasePath, def.PrimaryPath)
			for _, h := range p.HookFiles {
				if err := mergeJSONFile(h, target); err != nil {
					return s, fmt.Errorf("merging hooks %s: %w", filepath.Base(h), err)
				}
				s.Hooks++
			}
		}
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
// appends it to target. The expected snippet shape is:
//
//	{ "mcpServers": { "<name>": { "command": "...", "args": [...], "env": {...} } } }
//
// Each server becomes a `[mcp_servers.<name>]` block with sub-table `[mcp_servers.<name>.env]`
// when env is present. Unknown fields are emitted with best-effort string/number/bool
// rendering and any failures fall back to a TOML comment.
func mergeJSONIntoTOML(snippetPath, target string) error {
	data, err := os.ReadFile(snippetPath)
	if err != nil {
		return err
	}
	var doc map[string]map[string]map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parsing snippet: %w", err)
	}

	servers, ok := doc["mcpServers"]
	if !ok {
		return fmt.Errorf("snippet missing top-level mcpServers key")
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	// If target exists, read it so we can skip duplicate sections.
	existing := ""
	if b, err := os.ReadFile(target); err == nil {
		existing = string(b)
	}

	f, err := os.OpenFile(target, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	for name, server := range servers {
		header := fmt.Sprintf("[mcp_servers.%s]", name)
		if strings.Contains(existing, header) {
			continue
		}
		var b strings.Builder
		if existing != "" || b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(header)
		b.WriteString("\n")

		// Render scalars first, env at the end as a sub-table.
		var envRaw json.RawMessage
		for k, v := range server {
			if k == "env" {
				envRaw = v
				continue
			}
			b.WriteString(k)
			b.WriteString(" = ")
			b.WriteString(tomlValue(v))
			b.WriteString("\n")
		}
		if len(envRaw) > 0 {
			var env map[string]string
			if err := json.Unmarshal(envRaw, &env); err == nil && len(env) > 0 {
				b.WriteString("\n")
				b.WriteString(fmt.Sprintf("[mcp_servers.%s.env]\n", name))
				for k, v := range env {
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
