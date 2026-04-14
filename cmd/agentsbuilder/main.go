package main

import (
	"fmt"
	"os"

	"agentsbuilder/internal/config"
	"agentsbuilder/internal/template"
	"agentsbuilder/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Ensure ~/.agentsbuilder/templates/*default/template.json exists on first run.
	// Errors are non-fatal; built-in predefined templates always work.
	_ = template.EnsureDefaultTemplate()

	appModel := tui.NewAppModel(cfg.ListProjects())
	p := tea.NewProgram(appModel, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
