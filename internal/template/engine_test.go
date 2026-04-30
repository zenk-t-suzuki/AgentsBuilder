package template

import (
	"os"
	"path/filepath"
	"testing"

	"agentsbuilder/internal/model"
)

func TestListTemplates(t *testing.T) {
	templates := ListTemplates()
	if len(templates) == 0 {
		t.Fatal("expected at least one predefined template")
	}
	for _, tmpl := range templates {
		if tmpl.Name == "" {
			t.Error("template name should not be empty")
		}
		if len(tmpl.Assets) == 0 {
			t.Errorf("template %q has no assets", tmpl.Name)
		}
		if len(tmpl.Providers) == 0 {
			t.Errorf("template %q has no providers", tmpl.Name)
		}
	}
}

func TestValidateTemplate(t *testing.T) {
	tests := []struct {
		name    string
		tmpl    model.Template
		wantErr bool
	}{
		{
			name:    "valid template",
			tmpl:    model.Template{Name: "test", Assets: []model.AssetType{model.Skills}, Providers: []model.Provider{model.ClaudeCode}},
			wantErr: false,
		},
		{
			name:    "empty name",
			tmpl:    model.Template{Name: "", Assets: []model.AssetType{model.Skills}, Providers: []model.Provider{model.ClaudeCode}},
			wantErr: true,
		},
		{
			name:    "no assets",
			tmpl:    model.Template{Name: "test", Assets: nil, Providers: []model.Provider{model.ClaudeCode}},
			wantErr: true,
		},
		{
			name:    "no providers",
			tmpl:    model.Template{Name: "test", Assets: []model.AssetType{model.Skills}, Providers: nil},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTemplate(tt.tmpl)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTemplate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestApplyTemplate_ProjectScope(t *testing.T) {
	dir := t.TempDir()

	tmpl := model.Template{
		Name:      "test-claude",
		Assets:    []model.AssetType{model.Skills, model.Agents},
		Providers: []model.Provider{model.ClaudeCode},
	}

	if err := ApplyTemplate(tmpl, dir, model.Project); err != nil {
		t.Fatalf("ApplyTemplate: %v", err)
	}

	// Check that directories were created.
	expectDirs := []string{
		filepath.Join(dir, ".claude", "commands"),
		filepath.Join(dir, ".claude", "agents"),
	}
	for _, d := range expectDirs {
		info, err := os.Stat(d)
		if err != nil {
			t.Errorf("expected directory %s to exist: %v", d, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("expected %s to be a directory", d)
		}
	}
}

func TestApplyTemplate_GlobalScope(t *testing.T) {
	dir := t.TempDir()

	tmpl := model.Template{
		Name:      "test-codex",
		Assets:    []model.AssetType{model.Skills, model.MCP},
		Providers: []model.Provider{model.Codex},
	}

	if err := ApplyTemplate(tmpl, dir, model.Global); err != nil {
		t.Fatalf("ApplyTemplate: %v", err)
	}

	// Codex Skills live under .agents/skills (not .codex/skills) per
	// codex-rs/core-skills/src/loader.rs. MCP is global config.toml.
	expectDirs := []string{
		filepath.Join(dir, ".agents", "skills"),
		filepath.Join(dir, ".codex"),
	}
	for _, d := range expectDirs {
		info, err := os.Stat(d)
		if err != nil {
			t.Errorf("expected directory %s to exist: %v", d, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("expected %s to be a directory", d)
		}
	}
}

func TestApplyTemplate_Universal(t *testing.T) {
	dir := t.TempDir()

	tmpl := model.Template{
		Name:      "universal",
		Assets:    []model.AssetType{model.Skills, model.Agents, model.MCP},
		Providers: []model.Provider{model.ClaudeCode, model.Codex},
	}

	if err := ApplyTemplate(tmpl, dir, model.Project); err != nil {
		t.Fatalf("ApplyTemplate: %v", err)
	}

	// Claude Code dirs are project-scoped; Codex Project only has Skills
	// (.agents/skills) — Agents and MCP do not exist for Codex / Project.
	expectDirs := []string{
		filepath.Join(dir, ".claude", "commands"),
		filepath.Join(dir, ".claude", "agents"),
		filepath.Join(dir, ".claude"),
		filepath.Join(dir, ".agents", "skills"),
	}
	for _, d := range expectDirs {
		if _, err := os.Stat(d); err != nil {
			t.Errorf("expected directory %s to exist: %v", d, err)
		}
	}

	// And these *must not* exist — Codex has no agents/ dir and no
	// project-level config.toml.
	notExpect := []string{
		filepath.Join(dir, ".codex", "agents"),
		filepath.Join(dir, ".codex", "skills"),
		filepath.Join(dir, ".codex"),
	}
	for _, d := range notExpect {
		if _, err := os.Stat(d); err == nil {
			t.Errorf("did not expect directory %s to exist", d)
		}
	}
}

func TestApplyTemplate_InvalidTemplate(t *testing.T) {
	dir := t.TempDir()
	tmpl := model.Template{Name: ""} // invalid
	if err := ApplyTemplate(tmpl, dir, model.Project); err == nil {
		t.Error("expected error for invalid template")
	}
}
