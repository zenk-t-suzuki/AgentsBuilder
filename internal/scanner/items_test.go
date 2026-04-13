package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	cases := []struct {
		input   string
		name    string
		desc    string
	}{
		{
			input: "---\nname: my-agent\ndescription: Does stuff\n---\nBody",
			name:  "my-agent",
			desc:  "Does stuff",
		},
		{
			input: "---\nname: no-desc\n---\nBody",
			name:  "no-desc",
			desc:  "",
		},
		{
			input: "No frontmatter here",
			name:  "",
			desc:  "",
		},
	}
	for _, c := range cases {
		n, d := parseFrontmatter(c.input)
		if n != c.name {
			t.Errorf("name: got %q, want %q (input: %q)", n, c.name, c.input[:20])
		}
		if d != c.desc {
			t.Errorf("desc: got %q, want %q", d, c.desc)
		}
	}
}

func TestScanAgentItems_ClaudeCode(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "helper.md"), []byte("---\nname: my-helper\ndescription: A helpful agent\n---\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "plain.md"), []byte("no frontmatter"), 0o644)
	os.WriteFile(filepath.Join(dir, "skip.toml"), []byte("name = \"ignored\""), 0o644) // not .md

	items := scanAgentItems(dir, 0) // 0 = ClaudeCode
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	// my-helper has frontmatter
	found := map[string]string{}
	for _, it := range items {
		found[it.Name] = it.Description
	}
	if found["my-helper"] != "A helpful agent" {
		t.Errorf("expected description 'A helpful agent', got %q", found["my-helper"])
	}
	if _, ok := found["plain"]; !ok {
		t.Error("expected 'plain' (fallback to filename)")
	}
}

func TestScanMCPItemsClaude(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	os.WriteFile(settingsPath, []byte(`{"mcpServers":{"server-a":{},"server-b":{}}}`), 0o644)

	items := scanMCPItemsClaude(settingsPath)
	if len(items) != 2 {
		t.Fatalf("expected 2 MCP items, got %d", len(items))
	}
	names := map[string]bool{}
	for _, it := range items {
		names[it.Name] = true
	}
	if !names["server-a"] || !names["server-b"] {
		t.Errorf("expected server-a and server-b, got %v", names)
	}
}

func TestScanMCPItemsCodex(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	content := "[mcp_servers.my-server]\ncommand = \"npx\"\n[mcp_servers.other-server]\ncommand = \"uvx\"\n"
	os.WriteFile(configPath, []byte(content), 0o644)

	items := scanMCPItemsCodex(configPath)
	if len(items) != 2 {
		t.Fatalf("expected 2 Codex MCP items, got %d", len(items))
	}
	names := map[string]bool{}
	for _, it := range items {
		names[it.Name] = true
	}
	if !names["my-server"] || !names["other-server"] {
		t.Errorf("expected my-server and other-server, got %v", names)
	}
}

func TestParseFrontmatter_BlockScalar(t *testing.T) {
	// YAML >- block scalar description should be skipped (not parseable without full YAML parser)
	input := "---\nname: my-skill\ndescription: >-\n  Long multiline\n  description here\n---\n"
	n, d := parseFrontmatter(input)
	if n != "my-skill" {
		t.Errorf("name: got %q, want %q", n, "my-skill")
	}
	if d != "" {
		t.Errorf("description should be empty for block scalar, got %q", d)
	}
}
