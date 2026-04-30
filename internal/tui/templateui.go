package tui

import (
	"fmt"
	"strings"

	"agentsbuilder/internal/config"
	"agentsbuilder/internal/model"
	"agentsbuilder/internal/registry"
	tmplpkg "agentsbuilder/internal/template"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// TemplateStep tracks the current step in template creation.
type TemplateStep int

const (
	StepSelectTemplate TemplateStep = iota
	StepConfigure
	StepConfirm
)

// TemplateUIModel manages the template creation flow.
type TemplateUIModel struct {
	Step          TemplateStep
	Templates     []model.Template
	Cursor        int
	Width         int
	Height        int
	TemplatesPath string // path to ~/.agentsbuilder/templates/ for display

	// Configuration step
	SelectedTemplate *model.Template
	AssetChecks      []bool
	ProviderChecks   []bool
	ConfigCursor     int // index in the combined list

	keys KeyMap
}

// NewTemplateUIModel creates a new template UI model.
// It loads built-in predefined templates first, then appends any user-defined
// templates discovered in ~/.agentsbuilder/templates/, and finally appends
// templates from registered Git registries.
func NewTemplateUIModel(registries []model.RegistryInfo) TemplateUIModel {
	predefined := model.PredefinedTemplates()
	user := tmplpkg.LoadUserTemplates()
	remote := registry.LoadAllTemplates(registries)

	templates := make([]model.Template, 0, len(predefined)+len(user)+len(remote))
	templates = append(templates, predefined...)
	templates = append(templates, user...)
	templates = append(templates, remote...)

	templatesPath, _ := config.TemplatesDir()

	return TemplateUIModel{
		Step:          StepSelectTemplate,
		Templates:     templates,
		TemplatesPath: templatesPath,
		keys:          DefaultKeyMap(),
	}
}

func (m TemplateUIModel) Update(msg tea.Msg) (TemplateUIModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if key.Matches(msg, m.keys.Back) {
			switch m.Step {
			case StepSelectTemplate:
				return m, func() tea.Msg { return ExitTemplateModeMsg{} }
			case StepConfigure:
				m.Step = StepSelectTemplate
				return m, nil
			case StepConfirm:
				m.Step = StepConfigure
				return m, nil
			}
		}

		switch m.Step {
		case StepSelectTemplate:
			return m.updateSelectTemplate(msg)
		case StepConfigure:
			return m.updateConfigure(msg)
		case StepConfirm:
			return m.updateConfirm(msg)
		}
	}
	return m, nil
}

func (m TemplateUIModel) updateSelectTemplate(msg tea.KeyMsg) (TemplateUIModel, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Up):
		if m.Cursor > 0 {
			m.Cursor--
		}
	case key.Matches(msg, m.keys.Down):
		if m.Cursor < len(m.Templates)-1 {
			m.Cursor++
		}
	case key.Matches(msg, m.keys.Select):
		if m.Cursor < len(m.Templates) {
			tmpl := m.Templates[m.Cursor]
			m.SelectedTemplate = &tmpl
			m.initConfigure()
			m.Step = StepConfigure
		}
	}
	return m, nil
}

func (m *TemplateUIModel) initConfigure() {
	allAssets := model.AssetTypes()
	m.AssetChecks = make([]bool, len(allAssets))
	for i, a := range allAssets {
		for _, ta := range m.SelectedTemplate.Assets {
			if a == ta {
				m.AssetChecks[i] = true
				break
			}
		}
	}

	allProviders := model.Providers()
	m.ProviderChecks = make([]bool, len(allProviders))
	for i, p := range allProviders {
		for _, tp := range m.SelectedTemplate.Providers {
			if p == tp {
				m.ProviderChecks[i] = true
				break
			}
		}
	}
	m.ConfigCursor = 0
}

func (m TemplateUIModel) totalConfigItems() int {
	return len(m.AssetChecks) + len(m.ProviderChecks)
}

func (m TemplateUIModel) updateConfigure(msg tea.KeyMsg) (TemplateUIModel, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Up):
		if m.ConfigCursor > 0 {
			m.ConfigCursor--
		}
	case key.Matches(msg, m.keys.Down):
		if m.ConfigCursor < m.totalConfigItems()-1 {
			m.ConfigCursor++
		}
	case key.Matches(msg, m.keys.ToggleCheck):
		if m.ConfigCursor < len(m.AssetChecks) {
			m.AssetChecks[m.ConfigCursor] = !m.AssetChecks[m.ConfigCursor]
		} else {
			idx := m.ConfigCursor - len(m.AssetChecks)
			if idx < len(m.ProviderChecks) {
				m.ProviderChecks[idx] = !m.ProviderChecks[idx]
			}
		}
	case key.Matches(msg, m.keys.Select):
		m.Step = StepConfirm
	}
	return m, nil
}

func (m TemplateUIModel) updateConfirm(msg tea.KeyMsg) (TemplateUIModel, tea.Cmd) {
	if key.Matches(msg, m.keys.Select) {
		tmpl := m.buildTemplate()
		return m, func() tea.Msg { return TemplateAppliedMsg{Template: tmpl} }
	}
	return m, nil
}

func (m TemplateUIModel) buildTemplate() model.Template {
	var assets []model.AssetType
	allAssets := model.AssetTypes()
	for i, checked := range m.AssetChecks {
		if checked {
			assets = append(assets, allAssets[i])
		}
	}

	var providers []model.Provider
	allProviders := model.Providers()
	for i, checked := range m.ProviderChecks {
		if checked {
			providers = append(providers, allProviders[i])
		}
	}

	name := "custom"
	if m.SelectedTemplate != nil {
		name = m.SelectedTemplate.Name
	}

	return model.Template{
		Name:      name,
		Assets:    assets,
		Providers: providers,
	}
}

func (m TemplateUIModel) View() string {
	var b strings.Builder

	switch m.Step {
	case StepSelectTemplate:
		b.WriteString(TitleStyle.Render("Select Template"))
		b.WriteString("\n\n")
		for i, t := range m.Templates {
			marker := "  "
			if t.RegistryName != "" {
				marker = "◆ "
			} else if t.UserDefined {
				marker = "★ "
			}
			label := t.Name
			if t.RegistryName != "" {
				label = fmt.Sprintf("%s (%s)", t.Name, t.RegistryName)
			}
			if i == m.Cursor {
				b.WriteString(SelectedStyle.Render(fmt.Sprintf("> %s%s", marker, label)))
			} else {
				b.WriteString(NormalStyle.Render(fmt.Sprintf("  %s%s", marker, label)))
			}
			b.WriteString("\n")
			if t.Description != "" {
				b.WriteString(DimStyle.Render(fmt.Sprintf("    %s", t.Description)))
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  enter:select | esc:cancel"))
		if m.TemplatesPath != "" {
			b.WriteString("\n\n")
			b.WriteString(DimStyle.Render(fmt.Sprintf("  ★ user templates: %s", m.TemplatesPath)))
			b.WriteString("\n")
			b.WriteString(DimStyle.Render("  ◆ registry templates: [3] Registry タブで管理"))
		}

	case StepConfigure:
		b.WriteString(TitleStyle.Render("Configure Template"))
		b.WriteString("\n\n")

		b.WriteString(SectionHeaderStyle.Render("Assets"))
		b.WriteString("\n")
		allAssets := model.AssetTypes()
		for i, a := range allAssets {
			check := UncheckedStyle
			if m.AssetChecks[i] {
				check = CheckedStyle
			}
			line := fmt.Sprintf("  %s %s", check, a.String())
			if i == m.ConfigCursor {
				b.WriteString(SelectedStyle.Render("> " + line))
			} else {
				b.WriteString(NormalStyle.Render("  " + line))
			}
			b.WriteString("\n")
		}

		b.WriteString("\n")
		b.WriteString(SectionHeaderStyle.Render("Providers"))
		b.WriteString("\n")
		allProviders := model.Providers()
		for i, p := range allProviders {
			check := UncheckedStyle
			if m.ProviderChecks[i] {
				check = CheckedStyle
			}
			line := fmt.Sprintf("  %s %s", check, p.String())
			cfgIdx := len(m.AssetChecks) + i
			if cfgIdx == m.ConfigCursor {
				b.WriteString(SelectedStyle.Render("> " + line))
			} else {
				b.WriteString(NormalStyle.Render("  " + line))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  space:toggle | enter:confirm | esc:back"))

	case StepConfirm:
		b.WriteString(TitleStyle.Render("Confirm Template"))
		b.WriteString("\n\n")
		tmpl := m.buildTemplate()
		b.WriteString(fmt.Sprintf("  Template: %s\n", tmpl.Name))
		b.WriteString("  Assets:   ")
		for i, a := range tmpl.Assets {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(a.String())
		}
		b.WriteString("\n  Providers: ")
		for i, p := range tmpl.Providers {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(p.String())
		}
		b.WriteString("\n\n")
		b.WriteString(DimStyle.Render("  enter:apply | esc:back"))
	}

	return b.String()
}
