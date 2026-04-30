package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agentsbuilder/internal/model"
	"agentsbuilder/internal/scanner"
	tmplpkg "agentsbuilder/internal/template"

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

// focusElem identifies which UI element currently holds keyboard focus.
type focusElem int

const (
	elemNone       focusElem = iota
	elemSidebar              // sidebar (scope/project list)
	elemModeTabs             // outer mode tab bar (Browse/Template/Marketplace)
	elemBrowseTabs           // inner Browse tab bar (All/Skills/Agents/…)
	elemList                 // main content list / template UI / marketplace
)

// navDir identifies a navigation direction.
type navDir int

const (
	navLeft  navDir = iota
	navRight
	navUp
	navDown
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
	AppTabFocused  bool      // true when keyboard cursor is in the outer mode tab bar
	prevMainElem   focusElem // remembered main-pane element for sidebar return
	TemplateUI     TemplateUIModel

	// Template creation wizard
	TemplateCreating    bool
	TmplStep            TemplateCreateStep
	TmplSelItems        []TmplItem
	TmplNameBuf         []rune
	TmplCancelModal     bool
	TmplReviewCursor    int
	TmplSaveErr         string

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
		// ctrl+c always quits; q quits unless a modal or name-input is open
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		nameInputActive := m.TemplateCreating && m.TmplStep == TmplStepName
		if key.Matches(msg, m.Keys.Quit) && !m.ProjectPickerMode && !m.DeleteConfirmMode && !nameInputActive {
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

		// Template creation: cancel confirmation modal
		if m.TmplCancelModal {
			switch msg.String() {
			case "y", "Y", "enter":
				m.stopTmplCreate()
			case "n", "N", "esc":
				m.TmplCancelModal = false
			}
			return m, nil
		}

		// Template creation: review step — capture navigation
		if m.TemplateCreating && m.TmplStep == TmplStepReview {
			return m.updateTmplReview(msg)
		}

		// Template creation: name-input step — capture all keys as text
		if m.TemplateCreating && m.TmplStep == TmplStepName {
			return m.updateTmplName(msg)
		}

		// Template creation browse phase: ESC shows cancel modal
		if m.TemplateCreating && key.Matches(msg, m.Keys.Back) {
			m.TmplCancelModal = true
			return m, nil
		}

		// Tab toggles focus between sidebar and main pane.
		// When leaving the sidebar, restore the previously focused main element.
		if key.Matches(msg, m.Keys.SwitchPane) {
			if m.ActivePane == SidebarPane {
				if m.prevMainElem != elemNone {
					m.focusTo(m.prevMainElem)
				} else {
					m.focusModeTabs()
				}
			} else {
				m.focusSidebar()
			}
			return m, nil
		}

		// 'n': start template creation wizard (works regardless of focused element)
		if !m.TemplateCreating && m.ActivePane == MainPane && m.ActiveMainMode == ModeBrowse {
			if key.Matches(msg, m.Keys.CreateTemplate) {
				m.startTmplCreate()
				return m, nil
			}
		}

		// Directional navigation (data-driven: edge-aware neighbor jumping)
		switch {
		case key.Matches(msg, m.Keys.Up):
			return m.handleNav(navUp, msg)
		case key.Matches(msg, m.Keys.Down):
			return m.handleNav(navDown, msg)
		case key.Matches(msg, m.Keys.Left):
			return m.handleNav(navLeft, msg)
		case key.Matches(msg, m.Keys.Right):
			return m.handleNav(navRight, msg)
		case key.Matches(msg, m.Keys.CtrlUp):
			return m.handleCtrlNav(navUp)
		case key.Matches(msg, m.Keys.CtrlDown):
			return m.handleCtrlNav(navDown)
		case key.Matches(msg, m.Keys.CtrlLeft):
			return m.handleCtrlNav(navLeft)
		case key.Matches(msg, m.Keys.CtrlRight):
			return m.handleCtrlNav(navRight)
		}

		// Non-directional keys: delegate to sidebar when sidebar is focused
		if m.ActivePane == SidebarPane {
			var cmd tea.Cmd
			m.Sidebar, cmd = m.Sidebar.Update(msg)
			return m, cmd
		}

		// Mode switching (1-3) — main pane only
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

		// Template creation browse phase: Space toggles, Enter advances to review
		if m.TemplateCreating {
			switch {
			case key.Matches(msg, m.Keys.ToggleCheck):
				m.toggleTmplItem()
				return m, nil
			case key.Matches(msg, m.Keys.Select):
				if len(m.TmplSelItems) > 0 {
					m.TmplStep = TmplStepReview
					m.TmplReviewCursor = 0
				}
				return m, nil
			}
		} else if key.Matches(msg, m.Keys.Template) {
			// `t` shortcut: jump to template mode (disabled during template creation)
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

		// Mode-specific non-directional key handling
		if m.ActiveMainMode == ModeTemplate {
			var cmd tea.Cmd
			m.TemplateUI, cmd = m.TemplateUI.Update(msg)
			return m, cmd
		}
		if m.ActiveMainMode == ModeMarketplace {
			return m, nil
		}

		// Browse mode: delegate remaining keys to MainArea
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
			_ = tmplpkg.ApplyTemplate(msg.Template, targetPath, m.ActiveScope)
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

	// Template creation cancel confirmation modal
	if m.TmplCancelModal {
		content := fmt.Sprintf(
			"%s\n\n  Cancel template creation?\n  All %d selected item(s) will be discarded.\n\n  %s   %s",
			TemplateCreateBannerStyle.Render(" ◈ TEMPLATE CREATION "),
			len(m.TmplSelItems),
			SelectedStyle.Render("[y] Yes, cancel"),
			NormalStyle.Render("[n] Keep going"),
		)
		modal := ActiveBorderStyle.Width(52).Padding(1, 2).Render(content)
		return lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, modal)
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
		if m.TemplateCreating && m.TmplStep != TmplStepBrowse {
			// Wizard review / name steps: full-width, no detail panel
			var wizardContent string
			switch m.TmplStep {
			case TmplStepReview:
				wizardContent = m.renderTmplReview()
			case TmplStepName:
				wizardContent = m.renderTmplName()
			}
			mainContent = m.renderModeTabs(mainInnerW) + "\n" + wizardContent
		} else {
			// Normal Browse: list + detail panel.
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
		}
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

	// Help bar — context-aware in template creation mode.
	var helpText string
	if m.TemplateCreating {
		switch m.TmplStep {
		case TmplStepBrowse:
			helpText = "space:select/deselect · enter:review (need ≥1) · esc:cancel creation"
		case TmplStepReview:
			helpText = "↑↓:navigate · space:deselect · enter:name template · esc:back"
		case TmplStepName:
			helpText = "type name · enter:save · esc:back"
		}
	} else {
		helpText = HelpText()
	}
	help := HelpStyle.Render(helpText)

	if m.TemplateCreating {
		count := len(m.TmplSelItems)
		bannerText := fmt.Sprintf(" ◈ TEMPLATE CREATION · %d selected ", count)
		banner := TemplateCreateBannerStyle.Render(bannerText)
		gap := m.Width - lipgloss.Width(help) - lipgloss.Width(banner)
		if gap < 1 {
			gap = 1
		}
		help = help + strings.Repeat(" ", gap) + banner
	}

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
	if m.TemplateCreating {
		// Use a bright red border to signal template creation mode.
		return lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#C92A2A"))
	}
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

// ── Template creation helpers ─────────────────────────────────────────────────

// startTmplCreate enters template creation mode and switches to Browse.
func (m *AppModel) startTmplCreate() {
	m.TemplateCreating = true
	m.TmplStep = TmplStepBrowse
	m.TmplSelItems = nil
	m.TmplNameBuf = nil
	m.TmplCancelModal = false
	m.TmplReviewCursor = 0
	m.TmplSaveErr = ""
	m.ActiveMainMode = ModeBrowse
	m.focusList()
	m.MainArea.TemplateCreating = true
	m.MainArea.TemplateSelPaths = nil
}

// stopTmplCreate resets all template creation state.
func (m *AppModel) stopTmplCreate() {
	m.TemplateCreating = false
	m.TmplStep = TmplStepBrowse
	m.TmplSelItems = nil
	m.TmplNameBuf = nil
	m.TmplCancelModal = false
	m.TmplReviewCursor = 0
	m.TmplSaveErr = ""
	m.MainArea.TemplateCreating = false
	m.MainArea.TemplateSelPaths = nil
}

// tmplScopeLabel returns a display label for the current active scope.
func (m AppModel) tmplScopeLabel() string {
	if m.ActiveProject != nil {
		return m.ActiveProject.Name
	}
	return "Global"
}

// tmplSelPaths builds a set of selected keys for the MainArea renderer.
// For embedded items the key includes the item name; for regular items it's the source path.
func (m AppModel) tmplSelPaths() map[string]bool {
	if len(m.TmplSelItems) == 0 {
		return nil
	}
	out := make(map[string]bool, len(m.TmplSelItems))
	for _, item := range m.TmplSelItems {
		out[item.SelectKey()] = true
	}
	return out
}

// toggleTmplItem toggles the item currently under the cursor in the Browse list.
// It relies on the DetailPanel to know which asset/item is focused.
func (m *AppModel) toggleTmplItem() {
	asset := m.DetailPanel.Asset
	item := m.DetailPanel.Item
	if asset == nil {
		return
	}
	ti := MakeTmplItem(*asset, item, m.tmplScopeLabel())
	if ti == nil {
		return
	}
	for i, existing := range m.TmplSelItems {
		if existing.SelectKey() == ti.SelectKey() {
			// Already selected → deselect
			m.TmplSelItems = append(m.TmplSelItems[:i], m.TmplSelItems[i+1:]...)
			m.MainArea.TemplateSelPaths = m.tmplSelPaths()
			return
		}
	}
	// Not yet selected → add
	m.TmplSelItems = append(m.TmplSelItems, *ti)
	m.MainArea.TemplateSelPaths = m.tmplSelPaths()
}

// updateTmplReview handles key input during the review step.
func (m AppModel) updateTmplReview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.Keys.Back):
		m.TmplStep = TmplStepBrowse
	case key.Matches(msg, m.Keys.Up):
		if m.TmplReviewCursor > 0 {
			m.TmplReviewCursor--
		}
	case key.Matches(msg, m.Keys.Down):
		if m.TmplReviewCursor < len(m.TmplSelItems)-1 {
			m.TmplReviewCursor++
		}
	case key.Matches(msg, m.Keys.ToggleCheck):
		if m.TmplReviewCursor < len(m.TmplSelItems) {
			m.TmplSelItems = append(
				m.TmplSelItems[:m.TmplReviewCursor],
				m.TmplSelItems[m.TmplReviewCursor+1:]...,
			)
			if m.TmplReviewCursor >= len(m.TmplSelItems) && m.TmplReviewCursor > 0 {
				m.TmplReviewCursor--
			}
			m.MainArea.TemplateSelPaths = m.tmplSelPaths()
			if len(m.TmplSelItems) == 0 {
				m.TmplStep = TmplStepBrowse
			}
		}
	case key.Matches(msg, m.Keys.Select):
		if len(m.TmplSelItems) > 0 {
			m.TmplStep = TmplStepName
			m.TmplSaveErr = ""
		}
	}
	return m, nil
}

// updateTmplName handles key input during the name-input step.
func (m AppModel) updateTmplName(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.Keys.Back):
		m.TmplStep = TmplStepReview
		m.TmplSaveErr = ""
	case key.Matches(msg, m.Keys.Select):
		name := strings.TrimSpace(string(m.TmplNameBuf))
		if name == "" {
			m.TmplSaveErr = "Name cannot be empty."
			break
		}
		refs := make([]tmplpkg.FileRef, 0, len(m.TmplSelItems))
		for _, it := range m.TmplSelItems {
			refs = append(refs, tmplpkg.FileRef{
				SrcPath:    it.SrcPath,
				DestRelDir: it.DestRelDir,
				Filename:   it.Filename,
				AssetType:  it.AssetType,
				Provider:   it.Provider,
				ItemName:   it.ItemName,
				Embedded:   it.Embedded,
			})
		}
		if err := tmplpkg.SaveUserTemplate(name, refs); err != nil {
			m.TmplSaveErr = err.Error()
			break
		}
		m.stopTmplCreate()
		// Reload template UI so the new template appears immediately.
		m.TemplateUI = NewTemplateUIModel()
		return m, func() tea.Msg { return RefreshMsg{} }
	default:
		m.TmplSaveErr = "" // clear error on any key press
		switch msg.String() {
		case "backspace":
			if len(m.TmplNameBuf) > 0 {
				m.TmplNameBuf = m.TmplNameBuf[:len(m.TmplNameBuf)-1]
			}
		default:
			if msg.Type == tea.KeyRunes {
				m.TmplNameBuf = append(m.TmplNameBuf, msg.Runes...)
			}
		}
	}
	return m, nil
}

// renderTmplReview renders the review step content.
func (m AppModel) renderTmplReview() string {
	var b strings.Builder
	b.WriteString(TitleStyle.Render(fmt.Sprintf("Review Selected Items (%d)", len(m.TmplSelItems))))
	b.WriteString("\n\n")
	for i, item := range m.TmplSelItems {
		var line string
		if i == m.TmplReviewCursor {
			line = SelectedStyle.Render(fmt.Sprintf("> [x] %s", item.Label))
		} else {
			line = NormalStyle.Render(fmt.Sprintf("  [x] %s", item.Label))
		}
		b.WriteString(line + "\n")
	}
	if len(m.TmplSelItems) == 0 {
		b.WriteString(DimStyle.Render("  (no items selected)") + "\n")
	}
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  space:deselect · enter:next → name · esc:back"))
	return b.String()
}

// renderTmplName renders the name-input step content.
func (m AppModel) renderTmplName() string {
	var b strings.Builder
	b.WriteString(TitleStyle.Render("Name Your Template"))
	b.WriteString("\n\n")

	name := string(m.TmplNameBuf)
	nameBox := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(WarningColor).
		Padding(0, 1).
		Width(40).
		Render(name + "▌")
	b.WriteString("  ")
	b.WriteString(nameBox)
	b.WriteString("\n")

	if m.TmplSaveErr != "" {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(ErrorColor).Render("  ✗ " + m.TmplSaveErr))
	}

	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(fmt.Sprintf("  %d item(s) will be saved:", len(m.TmplSelItems))))
	b.WriteString("\n")
	for _, item := range m.TmplSelItems {
		b.WriteString(DimStyle.Render("    · " + item.Label) + "\n")
	}
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  enter:save · esc:back"))
	return b.String()
}

// focusSidebar moves keyboard focus to the Scopes sidebar.
// The caller's current main-pane element is remembered so that
// leaving the sidebar restores focus to the same place.
func (m *AppModel) focusSidebar() {
	if m.ActivePane == MainPane {
		m.prevMainElem = m.currentElem()
	}
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

// ── Data-driven navigation ────────────────────────────────────────────────────

// currentElem returns which UI element currently holds keyboard focus.
func (m AppModel) currentElem() focusElem {
	if m.ActivePane == SidebarPane {
		return elemSidebar
	}
	if m.AppTabFocused {
		return elemModeTabs
	}
	if m.ActiveMainMode == ModeBrowse && m.MainArea.TabFocused {
		return elemBrowseTabs
	}
	return elemList
}

// navNeighbor returns the element adjacent to the current element in direction dir.
// Returns elemNone when no neighbor exists.
func (m AppModel) navNeighbor(dir navDir) focusElem {
	switch m.currentElem() {
	case elemSidebar:
		if dir == navRight {
			if m.prevMainElem != elemNone {
				return m.prevMainElem
			}
			return elemModeTabs
		}
	case elemModeTabs:
		switch dir {
		case navLeft:
			return elemSidebar
		case navDown:
			if m.ActiveMainMode == ModeBrowse {
				return elemBrowseTabs
			}
			return elemList
		}
	case elemBrowseTabs:
		switch dir {
		case navUp:
			return elemModeTabs
		case navDown:
			return elemList
		case navLeft:
			return elemSidebar
		}
	case elemList:
		switch dir {
		case navLeft:
			return elemSidebar
		case navUp:
			if m.ActiveMainMode == ModeBrowse {
				return elemBrowseTabs
			}
			return elemModeTabs
		}
	}
	return elemNone
}

// navAtEdge reports whether the current element is at the boundary in direction dir,
// meaning navigation should jump to the neighbor rather than act locally.
func (m AppModel) navAtEdge(dir navDir) bool {
	switch m.currentElem() {
	case elemSidebar:
		// Left/Right always jump (or no-op if no neighbor); Up/Down handled locally.
		return dir == navLeft || dir == navRight
	case elemModeTabs:
		switch dir {
		case navUp:
			return true // no element above mode tabs
		case navDown:
			return true // always jump down to content
		case navLeft:
			return m.ActiveMainMode == 0
		case navRight:
			return false // local: clamp at last mode
		}
	case elemBrowseTabs:
		switch dir {
		case navUp, navDown:
			return true // always jump to neighbor
		case navLeft:
			return m.MainArea.ActiveBrowseTab == 0
		case navRight:
			return false // local: SwitchBrowseTab handles clamping
		}
	case elemList:
		switch dir {
		case navLeft, navRight:
			return true // left→Sidebar; right→nothing (clamp)
		case navUp:
			switch m.ActiveMainMode {
			case ModeMarketplace:
				return true
			case ModeTemplate:
				return m.TemplateUI.Step == StepSelectTemplate && m.TemplateUI.Cursor == 0
			default: // ModeBrowse
				return m.MainArea.AtTop()
			}
		case navDown:
			return false // local: component handles clamping
		}
	}
	return false
}

// focusTo switches keyboard focus to the specified element.
func (m *AppModel) focusTo(elem focusElem) {
	switch elem {
	case elemSidebar:
		m.focusSidebar()
	case elemModeTabs:
		m.focusModeTabs()
	case elemBrowseTabs:
		m.focusBrowseTabs()
	case elemList:
		m.focusList()
	}
}

// navLocal performs the intra-element navigation action for the current element.
// Called by handleNav when the element is NOT at an edge.
func (m *AppModel) navLocal(dir navDir, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.currentElem() {
	case elemSidebar:
		var cmd tea.Cmd
		m.Sidebar, cmd = m.Sidebar.Update(msg)
		return m, cmd
	case elemModeTabs:
		switch dir {
		case navLeft:
			if m.ActiveMainMode > 0 {
				m.ActiveMainMode--
			}
		case navRight:
			if int(m.ActiveMainMode) < len(allMainModes)-1 {
				m.ActiveMainMode++
			}
		}
		return m, nil
	case elemBrowseTabs:
		var cmd tea.Cmd
		switch dir {
		case navLeft:
			m.MainArea, cmd = m.MainArea.SwitchBrowseTab(-1)
		case navRight:
			m.MainArea, cmd = m.MainArea.SwitchBrowseTab(1)
		}
		return m, cmd
	case elemList:
		if m.ActiveMainMode == ModeTemplate {
			var cmd tea.Cmd
			m.TemplateUI, cmd = m.TemplateUI.Update(msg)
			return m, cmd
		}
		var cmd tea.Cmd
		m.MainArea, cmd = m.MainArea.Update(msg)
		return m, cmd
	}
	return m, nil
}

// handleNav handles a directional key press with edge-aware neighbor jumping.
// At edge → jump to neighbor (if any); otherwise → local intra-element action.
func (m *AppModel) handleNav(dir navDir, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.navAtEdge(dir) {
		neighbor := m.navNeighbor(dir)
		if neighbor != elemNone {
			m.focusTo(neighbor)
		}
		return m, nil
	}
	return m.navLocal(dir, msg)
}

// handleCtrlNav handles Ctrl+Arrow: always jump to the neighbor element,
// skipping any local intra-element action regardless of edge state.
func (m *AppModel) handleCtrlNav(dir navDir) (tea.Model, tea.Cmd) {
	neighbor := m.navNeighbor(dir)
	if neighbor != elemNone {
		m.focusTo(neighbor)
	}
	return m, nil
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
