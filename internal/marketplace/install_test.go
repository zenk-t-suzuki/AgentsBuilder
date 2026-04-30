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
		Name:   "test",
		Dir:    pluginDir,
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

func TestInstallPlugin_CodexRootSkillAndEnable(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plugin")
	os.MkdirAll(filepath.Join(pluginDir, "skills"), 0o755)
	os.WriteFile(filepath.Join(pluginDir, "skills", "SKILL.md"), []byte("---\nname: root\n---\n"), 0o644)

	p := Plugin{
		Name:        "codex-plugin",
		Marketplace: "market",
		Dir:         pluginDir,
		Skills:      []string{filepath.Join(pluginDir, "skills")},
	}
	target := InstallTarget{
		Provider: model.Codex,
		Scope:    model.Global,
		BasePath: filepath.Join(dir, "home"),
	}

	summaries, err := InstallPlugin(p, []InstallTarget{target})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if summaries[0].Skills != 1 {
		t.Errorf("expected 1 skill installed, got %d", summaries[0].Skills)
	}
	wantSkill := filepath.Join(dir, "home", ".agents", "skills", "codex-plugin", "SKILL.md")
	if _, err := os.Stat(wantSkill); err != nil {
		t.Errorf("root skill not at expected path %s: %v", wantSkill, err)
	}
	config, err := os.ReadFile(filepath.Join(dir, "home", ".codex", "config.toml"))
	if err != nil {
		t.Fatalf("codex config.toml missing: %v", err)
	}
	if !strings.Contains(string(config), `[plugins."codex-plugin@market"]`) || !strings.Contains(string(config), "enabled = true") {
		t.Errorf("Codex plugin was not enabled:\n%s", config)
	}
}

func TestDetectHooksFormat(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name    string
		content string
		want    HooksFormat
	}{
		{
			name:    "claude wrapped",
			content: `{"hooks":{"PreToolUse":[{"matcher":"Edit"}]}}`,
			want:    HooksFormatClaude,
		},
		{
			name:    "claude top-level",
			content: `{"PostToolUse":[{"matcher":"Bash"}]}`,
			want:    HooksFormatClaude,
		},
		{
			name:    "codex wrapped",
			content: `{"hooks":{"my_hook":{"event":"after_tool_use","command":"./run.sh"}}}`,
			want:    HooksFormatCodex,
		},
		{
			name:    "codex bare",
			content: `{"my_hook":{"after_agent":{"command":"x"}}}`,
			want:    HooksFormatCodex,
		},
		{
			name:    "both formats",
			content: `{"hooks":{"PreToolUse":[],"my_hook":{"event":"after_tool_use"}}}`,
			want:    HooksFormatBoth,
		},
		{
			name:    "neither",
			content: `{"unrelated":"value"}`,
			want:    HooksFormatUnknown,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(dir, c.name+".json")
			os.WriteFile(path, []byte(c.content), 0o644)
			got, err := detectHooksFormat(path)
			if err != nil {
				t.Fatalf("detect: %v", err)
			}
			if got != c.want {
				t.Errorf("detectHooksFormat(%s) = %v, want %v", c.name, got, c.want)
			}
		})
	}
}

func TestMergeJSONHooksIntoTOML(t *testing.T) {
	dir := t.TempDir()
	snippet := filepath.Join(dir, "hooks.json")
	target := filepath.Join(dir, "config.toml")

	os.WriteFile(snippet, []byte(`{
		"hooks": {
			"linter": { "event": "after_tool_use", "command": "./run.sh" },
			"notify": { "event": "after_agent" }
		}
	}`), 0o644)

	if err := mergeJSONHooksIntoTOML(snippet, target); err != nil {
		t.Fatalf("merge: %v", err)
	}
	out, _ := os.ReadFile(target)
	got := string(out)
	for _, want := range []string{
		`[hooks.linter]`,
		`event = "after_tool_use"`,
		`command = "./run.sh"`,
		`[hooks.notify]`,
		`event = "after_agent"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in output:\n%s", want, got)
		}
	}
}

func TestInstallPlugin_HooksRoutedByFormat(t *testing.T) {
	// A plugin ships two hooks files — one Claude-format, one Codex-format.
	// Installing to both providers should route each snippet to exactly one.
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plugin")
	hooksDir := filepath.Join(pluginDir, "hooks")
	os.MkdirAll(hooksDir, 0o755)
	claudeFile := filepath.Join(hooksDir, "claude.json")
	codexFile := filepath.Join(hooksDir, "codex.json")
	os.WriteFile(claudeFile, []byte(`{"hooks":{"PreToolUse":[{"matcher":"Edit"}]}}`), 0o644)
	os.WriteFile(codexFile, []byte(`{"hooks":{"my_hook":{"event":"after_tool_use","command":"./x"}}}`), 0o644)

	p := Plugin{
		Name:      "test-hooks",
		Dir:       pluginDir,
		HookFiles: []string{claudeFile, codexFile},
	}
	claudeBase := filepath.Join(dir, "claude-base")
	codexBase := filepath.Join(dir, "codex-base")
	targets := []InstallTarget{
		{Provider: model.ClaudeCode, Scope: model.Global, BasePath: claudeBase},
		{Provider: model.Codex, Scope: model.Global, BasePath: codexBase},
	}

	summaries, err := InstallPlugin(p, targets)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}

	// Each provider should install exactly 1 hook file (the one matching its format).
	for _, s := range summaries {
		if s.Hooks != 1 {
			t.Errorf("%v: expected 1 hook installed, got %d (skipped: %v)", s.Target.Provider, s.Hooks, s.Skipped)
		}
	}

	// Claude side should have a settings.json with the hooks key.
	claudeSettings, err := os.ReadFile(filepath.Join(claudeBase, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("claude settings.json missing: %v", err)
	}
	if !strings.Contains(string(claudeSettings), "PreToolUse") {
		t.Errorf("Claude settings.json should contain PreToolUse: %s", claudeSettings)
	}

	// Codex side should have config.toml with [hooks.my_hook].
	codexToml, err := os.ReadFile(filepath.Join(codexBase, ".codex", "config.toml"))
	if err != nil {
		t.Fatalf("codex config.toml missing: %v", err)
	}
	if !strings.Contains(string(codexToml), "[hooks.my_hook]") {
		t.Errorf("Codex config.toml should contain [hooks.my_hook]: %s", codexToml)
	}
}
