package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"agentsbuilder/internal/model"
)

// AppConfig holds the persisted application configuration.
type AppConfig struct {
	Projects       []model.ProjectInfo     `json:"projects"`
	ActiveProject  string                  `json:"active_project"`
	ActiveProvider model.Provider          `json:"active_provider"`
	Marketplaces   []model.MarketplaceInfo `json:"marketplaces,omitempty"`
}

// configDir returns the path to the application config directory (~/.agentsbuilder/).
func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".agentsbuilder"), nil
}

// MarketplaceCacheDir returns the path to the marketplace cache directory
// (~/.agentsbuilder/cache/marketplaces/).
func MarketplaceCacheDir() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cache", "marketplaces"), nil
}

// ConfigPath returns the full path to the config file (~/.agentsbuilder/config.json).
func ConfigPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load reads the config from disk. If the file does not exist, it creates
// a default config and returns it.
func Load() (*AppConfig, error) {
	cfgPath, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg := defaultConfig()
			if saveErr := cfg.save(cfgPath); saveErr != nil {
				return nil, fmt.Errorf("creating default config: %w", saveErr)
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg AppConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

// Save persists the current config to disk.
func (c *AppConfig) Save() error {
	cfgPath, err := ConfigPath()
	if err != nil {
		return err
	}
	return c.save(cfgPath)
}

// save writes the config to the given path, creating the parent directory if needed.
func (c *AppConfig) save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

// AddProject registers a new project. Returns an error if the name is already taken.
func (c *AppConfig) AddProject(name, path string) error {
	if name == "" {
		return errors.New("project name cannot be empty")
	}
	if path == "" {
		return errors.New("project path cannot be empty")
	}
	for _, p := range c.Projects {
		if p.Name == name {
			return fmt.Errorf("project %q already exists", name)
		}
	}
	c.Projects = append(c.Projects, model.ProjectInfo{Name: name, Path: path})
	return c.Save()
}

// RemoveProject unregisters a project by name. Returns an error if not found.
// If the removed project was active, the active project is cleared.
func (c *AppConfig) RemoveProject(name string) error {
	idx := -1
	for i, p := range c.Projects {
		if p.Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("project %q not found", name)
	}
	c.Projects = append(c.Projects[:idx], c.Projects[idx+1:]...)
	if c.ActiveProject == name {
		c.ActiveProject = ""
	}
	return c.Save()
}

// ListProjects returns all registered projects.
func (c *AppConfig) ListProjects() []model.ProjectInfo {
	result := make([]model.ProjectInfo, len(c.Projects))
	copy(result, c.Projects)
	return result
}

// SetActiveProject sets the active project by name. The name must match
// a registered project, or be empty to select Global scope.
func (c *AppConfig) SetActiveProject(name string) error {
	if name == "" {
		c.ActiveProject = ""
		return c.Save()
	}
	for _, p := range c.Projects {
		if p.Name == name {
			c.ActiveProject = name
			return c.Save()
		}
	}
	return fmt.Errorf("project %q not found", name)
}

// SetActiveProvider switches the active provider.
func (c *AppConfig) SetActiveProvider(provider model.Provider) error {
	c.ActiveProvider = provider
	return c.Save()
}

// GetActiveProject returns the currently active project, or nil if Global is selected.
func (c *AppConfig) GetActiveProject() *model.ProjectInfo {
	for _, p := range c.Projects {
		if p.Name == c.ActiveProject {
			info := p
			return &info
		}
	}
	return nil
}

// AddMarketplace registers a new plugin marketplace. The name must be unique
// and matches the marketplace.json `name` field — callers should determine it
// from a Sync call before persisting.
func (c *AppConfig) AddMarketplace(name, source string) error {
	if name == "" {
		return errors.New("marketplace name cannot be empty")
	}
	if source == "" {
		return errors.New("marketplace source cannot be empty")
	}
	for _, m := range c.Marketplaces {
		if m.Name == name {
			return fmt.Errorf("marketplace %q already exists", name)
		}
	}
	c.Marketplaces = append(c.Marketplaces, model.MarketplaceInfo{Name: name, Source: source})
	return c.Save()
}

// RemoveMarketplace unregisters a marketplace by name.
func (c *AppConfig) RemoveMarketplace(name string) error {
	idx := -1
	for i, m := range c.Marketplaces {
		if m.Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("marketplace %q not found", name)
	}
	c.Marketplaces = append(c.Marketplaces[:idx], c.Marketplaces[idx+1:]...)
	return c.Save()
}

// ListMarketplaces returns all registered marketplaces.
func (c *AppConfig) ListMarketplaces() []model.MarketplaceInfo {
	result := make([]model.MarketplaceInfo, len(c.Marketplaces))
	copy(result, c.Marketplaces)
	return result
}

func defaultConfig() *AppConfig {
	return &AppConfig{
		Projects:       []model.ProjectInfo{},
		ActiveProject:  "",
		ActiveProvider: model.ClaudeCode,
	}
}
