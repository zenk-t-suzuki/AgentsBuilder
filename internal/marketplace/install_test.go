package marketplace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentsbuilder/internal/model"
)

func TestMergeJSONFile_NewTarget(t *testing.T) {
	dir := t.TempDir()
	snippet := filepath.Join(dir, "snippet.json")
	target := filepath.Join(dir, "out.json")

	os.WriteFile(snippet, []byte(`{"hooks":{"PreToolUse":[{"matcher":"X"}]}}`), 0o644)

	if err := mergeJSONFile(snippet, target); err != nil {
		t.Fatalf("merge: %v", err)
	}

	data, _ := os.ReadFile(target)
	var got map[string]json.RawMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := got["hooks"]; !ok {
		t.Errorf("hooks key missing in output: %s", data)
	}
}

func TestMergeJSONFile_ObjectMerge(t *testing.T) {
	dir := t.TempDir()
	snippet := filepath.Join(dir, "snippet.json")
	target := filepath.Join(dir, "out.json")

	os.WriteFile(target, []byte(`{"mcpServers":{"a":{"command":"x"}}}`), 0o644)
	os.WriteFile(snippet, []byte(`{"mcpServers":{"b":{"command":"y"}}}`), 0o644)

	if err := mergeJSONFile(snippet, target); err != nil {
		t.Fatalf("merge: %v", err)
	}
	data, _ := os.ReadFile(target)
	if !strings.Contains(string(data), `"a"`) || !strings.Contains(string(data), `"b"`) {
		t.Errorf("expected merged object with a and b, got %s", data)
	}
}

func TestMergeJSONIntoTOML(t *testing.T) {
	dir := t.TempDir()
	snippet := filepath.Join(dir, "mcp.json")
	target := filepath.Join(dir, "config.toml")

	os.WriteFile(snippet, []byte(`{
		"mcpServers": {
			"my-server": {
				"command": "npx",
				"args": ["-y", "some-mcp"],
				"env": {"API_KEY": "secret"}
			}
		}
	}`), 0o644)

	if err := mergeJSONIntoTOML(snippet, target); err != nil {
		t.Fatalf("merge: %v", err)
	}

	out, _ := os.ReadFile(target)
	got := string(out)
	for _, want := range []string{
		`[mcp_servers.my-server]`,
		`command = "npx"`,
		`args = ["-y", "some-mcp"]`,
		`[mcp_servers.my-server.env]`,
		`API_KEY = "secret"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in output:\n%s", want, got)
		}
	}
}

func TestMergeJSONIntoTOML_DeduplicatesSection(t *testing.T) {
	dir := t.TempDir()
	snippet := filepath.Join(dir, "mcp.json")
	target := filepath.Join(dir, "config.toml")

	os.WriteFile(target, []byte("[mcp_servers.existing]\ncommand = \"old\"\n"), 0o644)
	os.WriteFile(snippet, []byte(`{"mcpServers":{"existing":{"command":"new"}}}`), 0o644)

	if err := mergeJSONIntoTOML(snippet, target); err != nil {
		t.Fatalf("merge: %v", err)
	}

	out, _ := os.ReadFile(target)
	if strings.Count(string(out), "[mcp_servers.existing]") != 1 {
		t.Errorf("expected single section, got:\n%s", out)
	}
	if !strings.Contains(string(out), `command = "old"`) {
		t.Errorf("existing section was clobbered")
	}
}

func TestInstallPlugin_Skills(t *testing.T) {
	dir := t.TempDir()

	// Plugin layout
	pluginDir := filepath.Join(dir, "plugin")
	os.MkdirAll(filepath.Join(pluginDir, "skills", "my-skill"), 0o755)
	os.WriteFile(filepath.Join(pluginDir, "skills", "my-skill", "SKILL.md"),
		[]byte("---\nname: my-skill\n---\nbody"), 0o644)

	p := Plugin{
		Name: "test",
		Dir:  pluginDir,
		Skills: []string{filepath.Join(pluginDir, "skills", "my-skill")},
	}
	target := InstallTarget{
		Provider: model.ClaudeCode,
		Scope:    model.Project,
		BasePath: filepath.Join(dir, "project"),
	}

	summaries, err := InstallPlugin(p, []InstallTarget{target})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if summaries[0].Skills != 1 {
		t.Errorf("expected 1 skill installed, got %d", summaries[0].Skills)
	}
	want := filepath.Join(dir, "project", ".claude", "commands", "my-skill", "SKILL.md")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("skill not at expected path %s: %v", want, err)
	}
}
