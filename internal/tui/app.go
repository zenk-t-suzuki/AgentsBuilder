package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agentsbuilder/internal/model"
	"agentsbuilder/internal/scanner"
	"agentsbuilder/internal/template"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Pane identifies which UI pane is currently focused.
type Pane int

const (
	SidebarPane Pane = iota
	MainPane
)

// AppModel is the root Bubble Tea model for the application.
type AppModel struct {
	Sidebar     SidebarModel
	MainArea    MainAreaModel
	DetailPanel DetailModel

	ActivePane    Pane
	ActiveScope   model.Scope
	ActiveProject *model.ProjectInfo

	Projects []model.ProjectInfo
	Assets   []model.Asset
	Diffs    []model.DiffResult

	// Main panel operation mode
	ActiveMainMode MainMode
	AppTabFocused  bool // true when keyboard cursor is in the outer mode tab bar
	TemplateUI     TemplateUIModel

	// Project picker modal
	ProjectPickerMode bool
	ProjectPicker     ProjectPickerModel

	// Delete confirmation modal
	DeleteConfirmMode bool
	DeleteConfirmName string

	Width  int
	Height int
	Keys   KeyMap
}

// NewAppModel creates a new root application model.
func NewAppModel(projects []model.ProjectInfo) AppModel {
	m := AppModel{
		ActivePane:  SidebarPane,
		ActiveScope: model.Global,
		Projects:    projects,
		Keys:        DefaultKeyMap(),
	}
	m.Sidebar = NewSidebarModel(projects)
	m.MainArea = NewMainAreaModel()
	m.DetailPanel = NewDetailModel()
	m.TemplateUI = NewTemplateUIModel()
	return m
}

func (m AppModel) Init() tea.Cmd {
	return func() tea.Msg { return RefreshMsg{} }
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		if m.Height < 20 {
			m.Height = 20
		}
		m.updateLayout()
		return m, nil

	case tea.KeyMsg:
		// q always quits unless a modal is open
		if key.Matches(msg, m.Keys.Quit) && !m.ProjectPickerMode && !m.DeleteConfirmMode {
			return m, tea.Quit
		}

		// Delete confirmation captures all input when open
		if m.DeleteConfirmMode {
			switch msg.String() {
			case "y", "Y", "enter":
				name := m.DeleteConfirmName
				m.DeleteConfirmMode = false
				m.DeleteConfirmName = ""
				return m, func() tea.Msg { return ProjectRemovedMsg{Name: name} }
			case "n", "N", "esc":
				m.DeleteConfirmMode = false
				m.DeleteConfirmName = ""
			}
			return m, nil
		}

		// Project picker captures all input when open
		if m.ProjectPickerMode {
			var cmd tea.Cmd
			m.ProjectPicker, cmd = m.ProjectPicker.Update(msg)
			return m, cmd
		}

		// Tab toggles focus between sidebar and mode tab bar
		if key.Matches(msg, m.Keys.SwitchPane) {
			if m.ActivePane == SidebarPane {
				m.focusModeTabs()
			} else {
				m.focusSidebar()
			}
			return m, nil
		}

		// Mode tab bar focused: ←/→ cycle modes, ↓ enters content
		if m.AppTabFocused {
			switch {
			case key.Matches(msg, m.Keys.Left):
				if m.ActiveMainMode > 0 {
					m.ActiveMainMode--
				} else {
					m.focusSidebar()
				}
			case key.Matches(msg, m.Keys.Right):
				if int(m.ActiveMainMode) < len(allMainModes)-1 {
					m.ActiveMainMode++
				}
			case key.Matches(msg, m.Keys.Down):
				if m.ActiveMainMode == ModeBrowse {
					m.focusBrowseTabs()
				} else {
					m.focusList()
				}
			}
			return m, nil
		}

		// Inner Browse tab bar focused: ↑ lifts to mode tabs, ←/→ switch tabs
		if m.ActiveMainMode == ModeBrowse && m.MainArea.TabFocused {
			switch {
			case key.Matches(msg, m.Keys.Up):
				m.focusModeTabs()
			case key.Matches(msg, m.Keys.Left):
				if m.MainArea.ActiveBrowseTab > 0 {
					var cmd tea.Cmd
					m.MainArea, cmd = m.MainArea.SwitchBrowseTab(-1)
					return m, cmd
				}
				m.focusSidebar()
			case key.Matches(msg, m.Keys.Right):
				var cmd tea.Cmd
				m.MainArea, cmd = m.MainArea.SwitchBrowseTab(1)
				return m, cmd
			}
			return m, nil
		}

		// Sidebar pane: Right → mode tab bar, otherwise delegate to Sidebar
		if m.ActivePane == SidebarPane {
			if key.Matches(msg, m.Keys.Right) {
				m.focusModeTabs()
				return m, nil
			}
			var cmd tea.Cmd
			m.Sidebar, cmd = m.Sidebar.Update(msg)
			return m, cmd
		}

		// Main pane (list/content area): Left → sidebar
		if key.Matches(msg, m.Keys.Left) {
			m.focusSidebar()
			return m, nil
		}

		// Mode switching (1-3) is always available in main pane
		switch msg.String() {
		case "1":
			m.ActiveMainMode = ModeBrowse
			return m, nil
		case "2":
			m.switchToTemplate()
			return m, nil
		case "3":
			m.ActiveMainMode = ModeMarketplace
			return m, nil
		}

		// Template mode: ↑ at the top of the selector lifts to mode tab bar
		if m.ActiveMainMode == ModeTemplate {
			if key.Matches(msg, m.Keys.Up) && m.TemplateUI.Step == StepSelectTemplate && m.TemplateUI.Cursor == 0 {
				m.focusModeTabs()
				return m, nil
			}
			var cmd tea.Cmd
			m.TemplateUI, cmd = m.TemplateUI.Update(msg)
			return m, cmd
		}

		// Marketplace mode: ↑ lifts to mode tab bar
		if m.ActiveMainMode == ModeMarketplace {
			if key.Matches(msg, m.Keys.Up) {
				m.focusModeTabs()
			}
			return m, nil
		}

		// Browse mode: `t` shortcut, detail scroll, pass rest to MainArea
		if key.Matches(msg, m.Keys.Template) {
			m.switchToTemplate()
			return m, nil
		}
		if key.Matches(msg, m.Keys.DetailScrollDown) {
			m.DetailPanel.ScrollDown(1)
			return m, nil
		}
		if key.Matches(msg, m.Keys.DetailScrollUp) {
			m.DetailPanel.ScrollUp(1)
			return m, nil
		}
		var cmd tea.Cmd
		m.MainArea, cmd = m.MainArea.Update(msg)
		return m, cmd

	case ConfirmDeleteProjectMsg:
		m.DeleteConfirmMode = true
		m.DeleteConfirmName = msg.Name
		return m, nil

	case OpenProjectPickerMsg:
		cwd, _ := os.Getwd()
		m.ProjectPicker = NewProjectPickerModel(cwd)
		m.pickerDimensions()
		m.ProjectPickerMode = true
		return m, nil

	case ProjectPickerConfirmMsg:
		m.ProjectPickerMode = false
		name := filepath.Base(msg.Path)
		proj := model.ProjectInfo{Name: name, Path: msg.Path}
		m.Projects = append(m.Projects, proj)
		m.Sidebar.Projects = m.Projects
		return m, func() tea.Msg { return ProjectAddedMsg{Project: proj} }

	case ProjectPickerCancelMsg:
		m.ProjectPickerMode = false
		return m, nil

	case ScopeSelectedMsg:
		m.ActiveScope = msg.Scope
		m.ActiveProject = msg.Project
		m.MainArea.SelectedAssetIndex = 0
		m.MainArea.GlobalAssets = nil
		m.DetailPanel.Asset = nil
		m.DetailPanel.Item = nil
		m.DetailPanel.Diff = nil
		return m, func() tea.Msg { return RefreshMsg{} }

	case AssetSelectedMsg:
		m.DetailPanel.Asset = msg.Asset
		m.DetailPanel.Item = msg.Item
		m.DetailPanel.ScrollOffset = 0
		if msg.Asset != nil {
			m.DetailPanel.Diff = m.findDiff(msg.Asset.Type, msg.Asset.Provider)
		} else {
			m.DetailPanel.Diff = nil
		}
		return m, nil

	case ProjectAddedMsg:
		// Already handled in ProjectPickerConfirmMsg; kept for external callers.
		return m, nil

	case ProjectRemovedMsg:
		removedSidebarIdx := -1
		for i, p := range m.Projects {
			if p.Name == msg.Name {
				removedSidebarIdx = i + 1 // 0=Global, 1..n=projects
				m.Projects = append(m.Projects[:i], m.Projects[i+1:]...)
				break
			}
		}
		m.Sidebar.Projects = m.Projects
		if m.Sidebar.Cursor > len(m.Projects) {
			m.Sidebar.Cursor = len(m.Projects)
		}
		// Keep ActiveIndex consistent after removal
		if removedSidebarIdx >= 0 {
			switch {
			case m.Sidebar.ActiveIndex == removedSidebarIdx:
				m.Sidebar.ActiveIndex = 0 // fall back to Global
			case m.Sidebar.ActiveIndex > removedSidebarIdx:
				m.Sidebar.ActiveIndex--
			}
		}
		if m.ActiveProject != nil && m.ActiveProject.Name == msg.Name {
			m.ActiveScope = model.Global
			m.ActiveProject = nil
		}
		return m, func() tea.Msg { return RefreshMsg{} }

	case ExitTemplateModeMsg:
		m.ActiveMainMode = ModeBrowse
		return m, func() tea.Msg { return RefreshMsg{} }

	case TemplateAppliedMsg:
		m.ActiveMainMode = ModeBrowse
		targetPath := ""
		if m.ActiveScope == model.Global {
			if home, err := os.UserHomeDir(); err == nil {
				targetPath = home
			}
		} else if m.ActiveProject != nil {
			targetPath = m.ActiveProject.Path
		}
		if targetPath != "" {
			_ = template.ApplyTemplate(msg.Template, targetPath, m.ActiveScope)
		}
		return m, func() tea.Msg { return RefreshMsg{} }

	case RefreshMsg:
		globalAssets := scanner.ScanAllGlobal()
		m.Assets = globalAssets

		if m.ActiveScope == model.Project && m.ActiveProject != nil {
			projectAssets := scanner.ScanAllProject(m.ActiveProject.Path)
			m.Assets = projectAssets
			m.Diffs = scanner.ComputeDiffs(globalAssets, projectAssets)
			m.MainArea.GlobalAssets = globalAssets
		} else {
			m.Diffs = nil
			m.MainArea.GlobalAssets = nil
		}

		m.MainArea.Assets = m.Assets
		m.MainArea.Diffs = m.Diffs
		m.MainArea.SelectedAssetIndex = 0
		m.DetailPanel.Asset = nil
		m.DetailPanel.Item = nil
		m.DetailPanel.Diff = nil

		if len(m.Assets) > 0 {
			asset := m.Assets[0]
			m.DetailPanel.Asset = &asset
			if len(asset.Items) > 0 {
				item := asset.Items[0]
				m.DetailPanel.Item = &item
			}
			m.DetailPanel.Diff = m.findDiff(asset.Type, asset.Provider)
		}
		return m, nil
	}

	return m, nil
}

func (m AppModel) View() string {
	if m.Width == 0 || m.Height == 0 {
		return "Loading..."
	}

	// Delete confirmation modal
	if m.DeleteConfirmMode {
		content := fmt.Sprintf(
			"%s\n\n  Delete project \"%s\"?\n  This only removes the registration,\n  not the files on disk.\n\n  %s   %s",
			TitleStyle.Render("Confirm Delete"),
			m.DeleteConfirmName,
			SelectedStyle.Render("[y] Yes, delete"),
			NormalStyle.Render("[n] Cancel"),
		)
		modal := ActiveBorderStyle.Width(50).Padding(1, 2).Render(content)
		return lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, modal)
	}

	// Project picker modal: render centered over blank background
	if m.ProjectPickerMode {
		modal := ActiveBorderStyle.
			Width(m.ProjectPicker.Width).
			Height(m.ProjectPicker.Height).
			Render(m.ProjectPicker.View())
		return lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, modal)
	}

	sidebarWidth := m.sidebarWidth()
	mainWidth := m.mainAreaWidth()
	contentHeight := m.Height - 2

	// Explicitly pad sidebar content to the inner height so its border always
	// aligns with the main panel's border, regardless of content length.
	innerH := contentHeight - 2
	sidebarContent := padToHeight(m.Sidebar.View(), innerH)
	sidebarBox := m.sidebarBorder().
		Width(sidebarWidth - 2).
		Height(innerH).
		Render(sidebarContent)

	var mainContent string
	mainInnerW := mainWidth - 2
	switch m.ActiveMainMode {
	case ModeBrowse:
		// Tab bar lives inside the list section so its right edge aligns with
		// the right-aligned provider labels. The bottom border is extended to
		// m.MainArea.Width so the separator spans the full list section width.
		tabBar := m.renderModeTabs(m.MainArea.Width)
		listSection := lipgloss.NewStyle().
			Width(m.MainArea.Width).
			Height(contentHeight - 2). // full main-box inner height
			Render(tabBar + "\n" + m.MainArea.View())
		detailBox := lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(MutedColor).
			Margin(0, 0, 0, 1).
			Padding(0, 0, 0, 1).
			Width(m.DetailPanel.Width).
			Height(m.DetailPanel.Height).
			Render(m.DetailPanel.View())
		mainContent = lipgloss.JoinHorizontal(lipgloss.Top, listSection, detailBox)
	case ModeTemplate:
		mainContent = m.renderModeTabs(mainInnerW) + "\n" + m.TemplateUI.View()
	case ModeMarketplace:
		mainContent = m.renderModeTabs(mainInnerW) + "\n" + m.renderMarketplace()
	}

	mainBox := m.mainBorder().
		Width(mainWidth - 2).
		Height(contentHeight - 2).
		Render(mainContent)

	layout := lipgloss.JoinHorizontal(lipgloss.Top, sidebarBox, mainBox)
	help := HelpStyle.Render(HelpText())
	return layout + "\n" + help
}

func (m AppModel) sidebarWidth() int {
	w := m.Width / 4
	if w < 20 {
		w = 20
	}
	if w > 40 {
		w = 40
	}
	return w
}

func (m AppModel) mainAreaWidth() int  { return m.Width - m.sidebarWidth() }
func (m AppModel) mainAreaHeight() int { return m.Height - 2 }

func (m AppModel) sidebarBorder() lipgloss.Style {
	if m.ActivePane == SidebarPane {
		return ActiveBorderStyle
	}
	return InactiveBorderStyle
}

func (m AppModel) mainBorder() lipgloss.Style {
	if m.ActivePane == MainPane {
		return ActiveBorderStyle
	}
	return InactiveBorderStyle
}

func (m *AppModel) pickerDimensions() {
	w := m.Width * 2 / 3
	if w < 50 {
		w = 50
	}
	h := m.Height * 2 / 3
	if h < 20 {
		h = 20
	}
	m.ProjectPicker.Width = w
	m.ProjectPicker.Height = h
}

func (m *AppModel) updateLayout() {
	// Main box inner height = Height - 4.
	// In Browse mode the tab bar (2 lines) + "\n" (1 line) sit above the list,
	// all inside the left section → list height = (Height-4) - 3 = Height - 7.
	contentH := m.Height - 7
	if contentH < 4 {
		contentH = 4
	}

	// Horizontal split: list (55%) on the left, detail (45%) on the right.
	// Inner main-box content width = mainAreaWidth() - 2 (border = 1 each side).
	totalW := m.mainAreaWidth() - 2
	listW := totalW * 70 / 100
	if listW < 20 {
		listW = 20
	}
	detailBoxW := totalW - listW
	if detailBoxW < 15 {
		detailBoxW = 15
	}

	m.Sidebar.Width = m.sidebarWidth() - 4
	m.Sidebar.Height = m.Height - 4 // inner height of sidebar box
	m.MainArea.Width = listW
	m.MainArea.Height = contentH
	m.DetailPanel.Width = detailBoxW - 3 // inner width (left margin:1 + left border:1 + left padding:1)
	detailH := m.Height - 4              // no top/bottom border
	if detailH < 3 {
		detailH = 3
	}
	m.DetailPanel.Height = detailH
	m.TemplateUI.Width = m.mainAreaWidth() - 4
	m.TemplateUI.Height = m.Height - 4
	m.pickerDimensions()
}

// renderMarketplace renders the Marketplace mode panel.
// skillsmp.com has no public API (all endpoints return 403), so we display
// the URL for the user to visit in a browser.
func (m AppModel) renderMarketplace() string {
	var b strings.Builder
	b.WriteString(TitleStyle.Render("Marketplace"))
	b.WriteString("\n\n")
	b.WriteString(NormalStyle.Render("  Skills Marketplace — skillsmp.com"))
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  No public API is available for in-TUI browsing."))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Visit the site directly in your browser:"))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Foreground(PrimaryColor).Bold(true).Render("  https://skillsmp.com/"))
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  Browse and discover community-contributed skills and agents for"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Claude Code and Codex at the URL above."))
	return b.String()
}

// switchToTemplate initialises and activates the template mode.
func (m *AppModel) switchToTemplate() {
	m.ActiveMainMode = ModeTemplate
	m.TemplateUI = NewTemplateUIModel()
	m.TemplateUI.Width = m.mainAreaWidth()
	m.TemplateUI.Height = m.mainAreaHeight()
}

// focusSidebar moves keyboard focus to the Scopes sidebar.
func (m *AppModel) focusSidebar() {
	m.AppTabFocused = false
	m.MainArea.TabFocused = false
	m.ActivePane = SidebarPane
	m.Sidebar.Focused = true
	m.MainArea.Focused = false
}

// focusModeTabs moves keyboard focus to the outer mode tab bar.
func (m *AppModel) focusModeTabs() {
	m.AppTabFocused = true
	m.MainArea.TabFocused = false
	m.ActivePane = MainPane
	m.Sidebar.Focused = false
	m.MainArea.Focused = false
}

// focusBrowseTabs moves keyboard focus to the inner Browse tab bar.
func (m *AppModel) focusBrowseTabs() {
	m.AppTabFocused = false
	m.MainArea.TabFocused = true
	m.ActivePane = MainPane
	m.Sidebar.Focused = false
	m.MainArea.Focused = true
}

// focusList moves keyboard focus to the main content list.
func (m *AppModel) focusList() {
	m.AppTabFocused = false
	m.MainArea.TabFocused = false
	m.ActivePane = MainPane
	m.Sidebar.Focused = false
	m.MainArea.Focused = true
}

// renderModeTabs renders the horizontal tab bar.
// fillWidth extends the bottom separator line to the given width so it aligns
// with right-aligned content below. Pass 0 to skip the filler.
func (m AppModel) renderModeTabs(fillWidth int) string {
	rendered := make([]string, len(allMainModes))
	for i, mode := range allMainModes {
		label := fmt.Sprintf("[%d] %s", i+1, mode.Label())
		if mode == m.ActiveMainMode {
			if m.AppTabFocused {
				rendered[i] = FocusedTabStyle.Render(label)
			} else {
				rendered[i] = ActiveTabStyle.Render(label)
			}
		} else {
			rendered[i] = InactiveTabStyle.Render(label)
		}
	}
	tabs := lipgloss.JoinHorizontal(lipgloss.Bottom, rendered...)

	// Extend the bottom separator line to fillWidth using a borderless filler
	// so the horizontal rule spans the full container width.
	if fillWidth > 0 {
		remaining := fillWidth - lipgloss.Width(tabs)
		if remaining > 0 {
			filler := lipgloss.NewStyle().
				Border(lipgloss.NormalBorder(), false, false, true, false).
				BorderForeground(MutedColor).
				Width(remaining).
				Render("")
			tabs = lipgloss.JoinHorizontal(lipgloss.Bottom, tabs, filler)
		}
	}
	return tabs
}

func (m AppModel) findDiff(assetType model.AssetType, provider model.Provider) *model.DiffResult {
	for _, d := range m.Diffs {
		if d.AssetType == assetType && d.Provider == provider {
			return &d
		}
	}
	return nil
}

// padToHeight ensures content is exactly h lines tall by appending empty lines.
// This guarantees the sidebar border matches the main panel border height.
func padToHeight(content string, h int) string {
	lines := strings.Split(content, "\n")
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines[:h], "\n")
}
