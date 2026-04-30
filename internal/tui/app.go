package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agentsbuilder/internal/config"
	"agentsbuilder/internal/marketplace"
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
	navLeft navDir = iota
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

	// Sub-models for the Template and Marketplace tabs
	TemplateUI    TemplateUIModel
	MarketplaceUI MarketplaceUIModel

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
func NewAppModel(projects []model.ProjectInfo, marketplaces []model.MarketplaceInfo) AppModel {
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
	m.MarketplaceUI = NewMarketplaceUIModel(marketplaces, nil)
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
		// ctrl+c always quits; q quits unless a modal or text-input is open.
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		textInputActive := m.ActiveMainMode == ModeMarketplace && m.MarketplaceUI.Mode == MpModeAdd
		if key.Matches(msg, m.Keys.Quit) && !m.ProjectPickerMode && !m.DeleteConfirmMode && !textInputActive {
			return m, tea.Quit
		}

		// Delete confirmation captures all input when open.
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

		// Project picker captures all input when open.
		if m.ProjectPickerMode {
			var cmd tea.Cmd
			m.ProjectPicker, cmd = m.ProjectPicker.Update(msg)
			return m, cmd
		}

		// Marketplace tab: source-input, plugin browse, install dialog, or
		// delete confirmation each capture all input.
		if m.ActiveMainMode == ModeMarketplace && m.marketplaceCapturesInput() {
			var cmd tea.Cmd
			m.MarketplaceUI, cmd = m.MarketplaceUI.Update(msg)
			return m, cmd
		}

		// Tab toggles focus between sidebar and main pane.
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

		// Directional navigation (data-driven: edge-aware neighbor jumping).
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

		// Non-directional keys: delegate to sidebar when sidebar is focused.
		if m.ActivePane == SidebarPane {
			var cmd tea.Cmd
			m.Sidebar, cmd = m.Sidebar.Update(msg)
			return m, cmd
		}

		// Mode switching (1-3).
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

		// Mode-specific non-directional key handling.
		if m.ActiveMainMode == ModeTemplate {
			var cmd tea.Cmd
			m.TemplateUI, cmd = m.TemplateUI.Update(msg)
			return m, cmd
		}
		if m.ActiveMainMode == ModeMarketplace {
			var cmd tea.Cmd
			m.MarketplaceUI, cmd = m.MarketplaceUI.Update(msg)
			return m, cmd
		}

		// Browse mode: delegate remaining keys to MainArea.
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
		if cfg, err := config.Load(); err == nil {
			if err := cfg.AddProject(name, msg.Path); err == nil {
				m.Projects = cfg.ListProjects()
				m.Sidebar.Projects = m.Projects
			}
		}
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
		m.MarketplaceUI.SetActiveProject(msg.Project)
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
				break
			}
		}
		if cfg, err := config.Load(); err == nil {
			if err := cfg.RemoveProject(msg.Name); err == nil {
				m.Projects = cfg.ListProjects()
			}
		}
		m.Sidebar.Projects = m.Projects
		if m.Sidebar.Cursor > len(m.Projects) {
			m.Sidebar.Cursor = len(m.Projects)
		}
		if removedSidebarIdx >= 0 {
			switch {
			case m.Sidebar.ActiveIndex == removedSidebarIdx:
				m.Sidebar.ActiveIndex = 0
			case m.Sidebar.ActiveIndex > removedSidebarIdx:
				m.Sidebar.ActiveIndex--
			}
		}
		if m.ActiveProject != nil && m.ActiveProject.Name == msg.Name {
			m.ActiveScope = model.Global
			m.ActiveProject = nil
			m.MarketplaceUI.SetActiveProject(nil)
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

	// ── Marketplace messages ──────────────────────────────────────────────────

	case MarketplaceAddedMsg:
		return m.handleMarketplaceAdded(msg)

	case MarketplaceRemovedMsg:
		return m.handleMarketplaceRemoved(msg)

	case MarketplaceSyncMsg:
		return m.handleMarketplaceSync(msg)

	case MarketplaceSyncDoneMsg:
		m.MarketplaceUI.Syncing = false
		if len(msg.Errors) > 0 {
			m.MarketplaceUI.SyncErrors = msg.Errors
			m.MarketplaceUI.SyncOK = false
		} else {
			m.MarketplaceUI.SyncErrors = nil
			m.MarketplaceUI.SyncOK = true
		}
		return m, nil

	case marketplaceAddCompletedMsg:
		m.MarketplaceUI.Marketplaces = msg.Marketplaces
		m.MarketplaceUI.Syncing = false
		m.MarketplaceUI.SyncOK = true
		return m, nil

	case MarketplaceOpenMsg:
		return m.handleMarketplaceOpen(msg)

	case MarketplaceLoadDoneMsg:
		if msg.Err != nil {
			m.MarketplaceUI.LoadErr = msg.Err.Error()
			m.MarketplaceUI.Mode = MpModePlugins
			m.MarketplaceUI.Plugins = nil
			return m, nil
		}
		// Set the selected marketplace pointer.
		for i, mp := range m.MarketplaceUI.Marketplaces {
			if mp.Name == msg.Name {
				selected := m.MarketplaceUI.Marketplaces[i]
				m.MarketplaceUI.SelectedMarketplace = &selected
				break
			}
		}
		m.MarketplaceUI.Plugins = msg.Plugins
		m.MarketplaceUI.PluginCursor = 0
		m.MarketplaceUI.LoadErr = ""
		m.MarketplaceUI.Mode = MpModePlugins
		return m, nil

	case MarketplaceInstallMsg:
		return m.handleMarketplaceInstall(msg)

	case MarketplaceInstallDoneMsg:
		m.MarketplaceUI.Installing = false
		if msg.Err != nil {
			m.MarketplaceUI.InstallErr = msg.Err.Error()
			return m, nil
		}
		m.MarketplaceUI.InstallErr = ""
		m.MarketplaceUI.InstallOK = fmt.Sprintf("%q をインストールしました（%s）",
			msg.PluginName, summariesLabel(msg.Summaries))
		m.MarketplaceUI.Mode = MpModeList
		m.MarketplaceUI.SelectedPlugin = nil
		m.MarketplaceUI.Targets = nil
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

// marketplaceCapturesInput returns true when the marketplace tab is in a
// sub-mode that needs to consume every keypress (text input, focused dialog,
// or modal confirmation).
func (m AppModel) marketplaceCapturesInput() bool {
	if m.MarketplaceUI.DeleteConfirm {
		return true
	}
	switch m.MarketplaceUI.Mode {
	case MpModeAdd, MpModePlugins, MpModeInstall:
		return true
	}
	return false
}

// handleMarketplaceAdded parses the source, syncs it, reads the manifest, and
// persists the resulting MarketplaceInfo (using the manifest `name`).
func (m AppModel) handleMarketplaceAdded(msg MarketplaceAddedMsg) (tea.Model, tea.Cmd) {
	source := msg.Source
	m.MarketplaceUI.Syncing = true
	m.MarketplaceUI.SyncErrors = nil
	m.MarketplaceUI.SyncOK = false

	return m, func() tea.Msg {
		src, err := marketplace.ParseSource(source)
		if err != nil {
			return MarketplaceSyncDoneMsg{Errors: map[string]error{source: err}}
		}
		manifestPath, err := marketplace.Sync(src)
		if err != nil {
			return MarketplaceSyncDoneMsg{Errors: map[string]error{source: err}}
		}
		manifest, _, err := marketplace.LoadMarketplace(manifestPath)
		if err != nil {
			return MarketplaceSyncDoneMsg{Errors: map[string]error{source: err}}
		}
		// Persist, return the (now populated) marketplaces list inline by
		// triggering a re-load via a synthetic added-then-sync round trip.
		cfg, cfgErr := config.Load()
		if cfgErr != nil {
			return MarketplaceSyncDoneMsg{Errors: map[string]error{source: cfgErr}}
		}
		if err := cfg.AddMarketplace(manifest.Name, source); err != nil {
			return MarketplaceSyncDoneMsg{Errors: map[string]error{manifest.Name: err}}
		}
		return marketplaceAddCompletedMsg{
			Marketplaces: cfg.ListMarketplaces(),
		}
	}
}

// marketplaceAddCompletedMsg is delivered once the new marketplace is on disk
// in config.json. The handler pushes the updated list into the UI model.
type marketplaceAddCompletedMsg struct {
	Marketplaces []model.MarketplaceInfo
}

func (m AppModel) handleMarketplaceRemoved(msg MarketplaceRemovedMsg) (tea.Model, tea.Cmd) {
	cfg, err := config.Load()
	if err != nil {
		return m, nil
	}
	// Find the source so we can remove the cache via marketplace.RemoveCache.
	var src marketplace.Source
	for _, mp := range cfg.Marketplaces {
		if mp.Name == msg.Name {
			if parsed, err := marketplace.ParseSource(mp.Source); err == nil {
				src = parsed
			}
			break
		}
	}
	if rmErr := cfg.RemoveMarketplace(msg.Name); rmErr == nil {
		_ = marketplace.RemoveCache(src)
		m.MarketplaceUI.Marketplaces = cfg.ListMarketplaces()
		if m.MarketplaceUI.Cursor >= len(m.MarketplaceUI.Marketplaces) && m.MarketplaceUI.Cursor > 0 {
			m.MarketplaceUI.Cursor--
		}
	}
	return m, nil
}

func (m AppModel) handleMarketplaceSync(msg MarketplaceSyncMsg) (tea.Model, tea.Cmd) {
	m.MarketplaceUI.Syncing = true
	m.MarketplaceUI.SyncErrors = nil
	mps := m.MarketplaceUI.Marketplaces
	target := msg.Name
	return m, func() tea.Msg {
		errs := make(map[string]error)
		for _, mp := range mps {
			if target != "" && mp.Name != target {
				continue
			}
			src, err := marketplace.ParseSource(mp.Source)
			if err != nil {
				errs[mp.Name] = err
				continue
			}
			if _, err := marketplace.Sync(src); err != nil {
				errs[mp.Name] = err
			}
		}
		return MarketplaceSyncDoneMsg{Errors: errs}
	}
}

func (m AppModel) handleMarketplaceOpen(msg MarketplaceOpenMsg) (tea.Model, tea.Cmd) {
	var info *model.MarketplaceInfo
	for i, mp := range m.MarketplaceUI.Marketplaces {
		if mp.Name == msg.Name {
			selected := m.MarketplaceUI.Marketplaces[i]
			info = &selected
			break
		}
	}
	if info == nil {
		return m, nil
	}
	infoCopy := *info
	return m, func() tea.Msg {
		src, err := marketplace.ParseSource(infoCopy.Source)
		if err != nil {
			return MarketplaceLoadDoneMsg{Name: infoCopy.Name, Err: err}
		}
		manifestPath, err := marketplace.Sync(src)
		if err != nil {
			return MarketplaceLoadDoneMsg{Name: infoCopy.Name, Err: err}
		}
		_, plugins, err := marketplace.LoadMarketplace(manifestPath)
		if err != nil {
			return MarketplaceLoadDoneMsg{Name: infoCopy.Name, Err: err}
		}
		return MarketplaceLoadDoneMsg{Name: infoCopy.Name, Plugins: plugins}
	}
}

func (m AppModel) handleMarketplaceInstall(msg MarketplaceInstallMsg) (tea.Model, tea.Cmd) {
	plugin := msg.Plugin
	chosen := msg.Targets
	activeProject := m.ActiveProject

	return m, func() tea.Msg {
		home, err := os.UserHomeDir()
		if err != nil {
			return MarketplaceInstallDoneMsg{PluginName: plugin.Name, Err: err}
		}

		var targets []marketplace.InstallTarget
		for _, opt := range chosen {
			base := home
			if opt.Scope == model.Project {
				if activeProject == nil {
					continue
				}
				base = activeProject.Path
			}
			targets = append(targets, marketplace.InstallTarget{
				Provider: opt.Provider,
				Scope:    opt.Scope,
				BasePath: base,
			})
		}
		if len(targets) == 0 {
			return MarketplaceInstallDoneMsg{
				PluginName: plugin.Name,
				Err:        fmt.Errorf("インストール先が選択されていません"),
			}
		}

		summaries, err := marketplace.InstallPlugin(plugin, targets)
		return MarketplaceInstallDoneMsg{
			PluginName: plugin.Name,
			Summaries:  summaries,
			Err:        err,
		}
	}
}

// summariesLabel renders a one-line digest for the install banner.
func summariesLabel(ss []marketplace.InstallSummary) string {
	if len(ss) == 0 {
		return "no targets"
	}
	parts := make([]string, 0, len(ss))
	for _, s := range ss {
		parts = append(parts, fmt.Sprintf("%s/%s", s.Target.Provider.String(), scopeShort(s.Target.Scope)))
	}
	return strings.Join(parts, ", ")
}

func scopeShort(s model.Scope) string {
	if s == model.Project {
		return "Project"
	}
	return "Global"
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
		tabBar := m.renderModeTabs(m.MainArea.Width)
		listSection := lipgloss.NewStyle().
			Width(m.MainArea.Width).
			Height(contentHeight - 2).
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
		mainContent = m.renderModeTabs(mainInnerW) + "\n" + m.MarketplaceUI.View()
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
	contentH := m.Height - 7
	if contentH < 4 {
		contentH = 4
	}
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
	m.Sidebar.Height = m.Height - 4
	m.MainArea.Width = listW
	m.MainArea.Height = contentH
	m.DetailPanel.Width = detailBoxW - 3
	detailH := m.Height - 4
	if detailH < 3 {
		detailH = 3
	}
	m.DetailPanel.Height = detailH
	m.TemplateUI.Width = m.mainAreaWidth() - 4
	m.TemplateUI.Height = m.Height - 4
	m.MarketplaceUI.Width = m.mainAreaWidth() - 4
	m.MarketplaceUI.Height = m.Height - 4
	m.pickerDimensions()
}

func (m *AppModel) switchToTemplate() {
	m.ActiveMainMode = ModeTemplate
	m.TemplateUI = NewTemplateUIModel()
	m.TemplateUI.Width = m.mainAreaWidth()
	m.TemplateUI.Height = m.mainAreaHeight()
}

// ── Focus management ──────────────────────────────────────────────────────────

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

func (m *AppModel) focusModeTabs() {
	m.AppTabFocused = true
	m.MainArea.TabFocused = false
	m.ActivePane = MainPane
	m.Sidebar.Focused = false
	m.MainArea.Focused = false
}

func (m *AppModel) focusBrowseTabs() {
	m.AppTabFocused = false
	m.MainArea.TabFocused = true
	m.ActivePane = MainPane
	m.Sidebar.Focused = false
	m.MainArea.Focused = true
}

func (m *AppModel) focusList() {
	m.AppTabFocused = false
	m.MainArea.TabFocused = false
	m.ActivePane = MainPane
	m.Sidebar.Focused = false
	m.MainArea.Focused = true
}

// ── Data-driven navigation ────────────────────────────────────────────────────

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

func (m AppModel) navAtEdge(dir navDir) bool {
	switch m.currentElem() {
	case elemSidebar:
		return dir == navLeft || dir == navRight
	case elemModeTabs:
		switch dir {
		case navUp:
			return true
		case navDown:
			return true
		case navLeft:
			return m.ActiveMainMode == 0
		case navRight:
			return false
		}
	case elemBrowseTabs:
		switch dir {
		case navUp, navDown:
			return true
		case navLeft:
			return m.MainArea.ActiveBrowseTab == 0
		case navRight:
			return false
		}
	case elemList:
		switch dir {
		case navLeft, navRight:
			return true
		case navUp:
			switch m.ActiveMainMode {
			case ModeMarketplace:
				return m.MarketplaceUI.Mode == MpModeList && m.MarketplaceUI.Cursor == 0
			case ModeTemplate:
				return m.TemplateUI.Step == StepSelectTemplate && m.TemplateUI.Cursor == 0
			default:
				return m.MainArea.AtTop()
			}
		case navDown:
			return false
		}
	}
	return false
}

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
		if m.ActiveMainMode == ModeMarketplace {
			var cmd tea.Cmd
			m.MarketplaceUI, cmd = m.MarketplaceUI.Update(msg)
			return m, cmd
		}
		var cmd tea.Cmd
		m.MainArea, cmd = m.MainArea.Update(msg)
		return m, cmd
	}
	return m, nil
}

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

func (m *AppModel) handleCtrlNav(dir navDir) (tea.Model, tea.Cmd) {
	neighbor := m.navNeighbor(dir)
	if neighbor != elemNone {
		m.focusTo(neighbor)
	}
	return m, nil
}

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

func padToHeight(content string, h int) string {
	lines := strings.Split(content, "\n")
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines[:h], "\n")
}
