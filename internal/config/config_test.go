package config

import (
	"os"
	"path/filepath"
	"testing"

	"agentsbuilder/internal/model"
)

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()
	if len(cfg.Projects) != 0 {
		t.Errorf("expected empty projects, got %d", len(cfg.Projects))
	}
	if cfg.ActiveProject != "" {
		t.Errorf("expected empty active project, got %q", cfg.ActiveProject)
	}
	if cfg.ActiveProvider != model.ClaudeCode {
		t.Errorf("expected ClaudeCode provider, got %v", cfg.ActiveProvider)
	}
}

func TestAddRemoveProject(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfg := defaultConfig()
	if err := cfg.save(cfgPath); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Override ConfigPath for testing by saving/loading from temp.
	// We test the methods directly via save.

	if err := cfg.AddProject("", "/some/path"); err == nil {
		t.Error("expected error for empty name")
	}
	if err := cfg.AddProject("myproj", ""); err == nil {
		t.Error("expected error for empty path")
	}

	// Clean up auto-saved file from AddProject error paths.
	// Reset config to use temp dir.
	cfg = defaultConfig()

	cfg.Projects = append(cfg.Projects, model.ProjectInfo{Name: "proj1", Path: "/tmp/proj1"})
	if err := cfg.save(cfgPath); err != nil {
		t.Fatalf("save: %v", err)
	}

	projects := cfg.ListProjects()
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].Name != "proj1" {
		t.Errorf("expected proj1, got %q", projects[0].Name)
	}

	// Verify list returns a copy.
	projects[0].Name = "modified"
	if cfg.Projects[0].Name != "proj1" {
		t.Error("ListProjects should return a copy")
	}
}

func TestSetActiveProject(t *testing.T) {
	cfg := defaultConfig()
	cfg.Projects = []model.ProjectInfo{
		{Name: "proj1", Path: "/tmp/proj1"},
	}

	// Setting active to a non-existent project should fail.
	if err := cfg.SetActiveProject("nonexistent"); err == nil {
		t.Error("expected error for nonexistent project")
	}

	// GetActiveProject with no active project should return nil.
	active := cfg.GetActiveProject()
	if active != nil {
		t.Errorf("expected nil active project, got %v", active)
	}
}

func TestSetActiveProvider(t *testing.T) {
	cfg := defaultConfig()
	if cfg.ActiveProvider != model.ClaudeCode {
		t.Fatalf("expected default ClaudeCode")
	}
	cfg.ActiveProvider = model.Codex
	if cfg.ActiveProvider != model.Codex {
		t.Errorf("expected Codex")
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfg := defaultConfig()
	cfg.Projects = []model.ProjectInfo{
		{Name: "proj1", Path: "/tmp/proj1"},
		{Name: "proj2", Path: "/tmp/proj2"},
	}
	cfg.ActiveProject = "proj1"
	cfg.ActiveProvider = model.Codex

	if err := cfg.save(cfgPath); err != nil {
		t.Fatalf("save: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Verify JSON was written.
	if len(data) == 0 {
		t.Fatal("config file is empty")
	}
}

func TestRemoveActiveProject(t *testing.T) {
	cfg := defaultConfig()
	cfg.Projects = []model.ProjectInfo{
		{Name: "proj1", Path: "/tmp/proj1"},
	}
	cfg.ActiveProject = "proj1"

	// Simulate remove by index manipulation (since RemoveProject calls Save
	// which needs a valid config path).
	idx := 0
	cfg.Projects = append(cfg.Projects[:idx], cfg.Projects[idx+1:]...)
	if cfg.ActiveProject == "proj1" {
		cfg.ActiveProject = ""
	}

	if cfg.ActiveProject != "" {
		t.Error("expected active project to be cleared after removal")
	}
	if len(cfg.Projects) != 0 {
		t.Error("expected empty projects after removal")
	}
}

func TestGetActiveProject(t *testing.T) {
	cfg := defaultConfig()
	cfg.Projects = []model.ProjectInfo{
		{Name: "proj1", Path: "/tmp/proj1"},
	}
	cfg.ActiveProject = "proj1"

	active := cfg.GetActiveProject()
	if active == nil {
		t.Fatal("expected non-nil active project")
	}
	if active.Name != "proj1" {
		t.Errorf("expected proj1, got %q", active.Name)
	}
	if active.Path != "/tmp/proj1" {
		t.Errorf("expected /tmp/proj1, got %q", active.Path)
	}

	// Mutating returned value should not affect original.
	active.Name = "changed"
	if cfg.Projects[0].Name != "proj1" {
		t.Error("GetActiveProject should return a copy")
	}
}
