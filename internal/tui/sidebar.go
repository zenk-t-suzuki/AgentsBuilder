package tui

import (
	"strings"

	"agentsbuilder/internal/model"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// SidebarModel is the sidebar TUI component.
type SidebarModel struct {
	Projects    []model.ProjectInfo
	Cursor      int // navigation position: 0=Global, 1..n=projects
	ActiveIndex int // currently active scope (displayed in main area): same numbering
	Focused     bool
	Width       int
	Height      int

	keys KeyMap
}

// NewSidebarModel creates a new sidebar model.
func NewSidebarModel(projects []model.ProjectInfo) SidebarModel {
	return SidebarModel{
		Projects:    projects,
		Cursor:      0,
		ActiveIndex: 0, // Global is active by default
		Focused:     true,
		keys:        DefaultKeyMap(),
	}
}

func (m SidebarModel) Update(msg tea.Msg) (SidebarModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Up):
			if m.Cursor > 0 {
				m.Cursor--
				return m, m.selectCurrent()
			}
		case key.Matches(msg, m.keys.Down):
			if m.Cursor < len(m.Projects) {
				m.Cursor++
				return m, m.selectCurrent()
			}
		case key.Matches(msg, m.keys.AddProject):
			return m, func() tea.Msg { return OpenProjectPickerMsg{} }
		case key.Matches(msg, m.keys.DeleteProject):
			if m.Cursor > 0 {
				idx := m.Cursor - 1
				if idx < len(m.Projects) {
					name := m.Projects[idx].Name
					return m, func() tea.Msg {
						return ConfirmDeleteProjectMsg{Name: name}
					}
				}
			}
		}
	}
	return m, nil
}

func (m SidebarModel) View() string {
	var b strings.Builder

	b.WriteString(TitleStyle.Render("Scopes"))
	b.WriteString("\n\n")

	b.WriteString(m.renderItem(0, "Global"))
	b.WriteString("\n")

	for i, p := range m.Projects {
		b.WriteString(m.renderItem(i+1, p.Name))
		b.WriteString("\n")
	}

	if len(m.Projects) == 0 {
		b.WriteString(DimStyle.Render("  (no projects)"))
		b.WriteString("\n")
	}

	if m.Focused {
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  a:add | d:delete"))
	}

	return clipLines(b.String(), m.Height)
}

// selectCurrent emits a ScopeSelectedMsg for the current cursor position
// and updates ActiveIndex.
func (m *SidebarModel) selectCurrent() tea.Cmd {
	m.ActiveIndex = m.Cursor
	if m.Cursor == 0 {
		return func() tea.Msg { return ScopeSelectedMsg{Scope: model.Global} }
	}
	idx := m.Cursor - 1
	if idx < len(m.Projects) {
		p := m.Projects[idx]
		return func() tea.Msg { return ScopeSelectedMsg{Scope: model.Project, Project: &p} }
	}
	return nil
}

// renderItem renders a single sidebar scope item.
// Active scope: "▶ name" in green.
// Inactive scope: "  ● name" with a dim bullet — same symbol as the main area list.
func (m SidebarModel) renderItem(index int, name string) string {
	if m.ActiveIndex == index {
		return ActiveScopeStyle.Render("▶ ● " + name)
	}
	return "  " + DimStyle.Render("●") + NormalStyle.Render(" "+name)
}
