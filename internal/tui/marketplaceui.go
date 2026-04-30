package tui

import (
	"fmt"
	"strings"

	"agentsbuilder/internal/marketplace"
	"agentsbuilder/internal/model"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// MarketplaceMode is the current sub-screen of the marketplace tab.
type MarketplaceMode int

const (
	MpModeList    MarketplaceMode = iota // browsing registered marketplaces
	MpModeAdd                            // typing a source to add
	MpModePlugins                        // browsing plugins in the selected marketplace
	MpModeInstall                        // install dialog (target picker)
)

// installTargetOption is one row of the install dialog's checkbox list.
type installTargetOption struct {
	Provider model.Provider
	Scope    model.Scope
	Label    string
	Disabled bool   // true when no project is active for Project-scope targets
	Selected bool
}

// MarketplaceUIModel is the model for the Marketplace tab.
type MarketplaceUIModel struct {
	Marketplaces []model.MarketplaceInfo
	Cursor       int

	Mode MarketplaceMode

	// Sync state
	Syncing    bool
	SyncErrors map[string]error
	SyncOK     bool

	// Add flow
	SourceBuf []rune
	AddErr    string

	// Delete confirmation
	DeleteConfirm bool
	DeleteName    string

	// Plugin browsing
	SelectedMarketplace *model.MarketplaceInfo
	Plugins             []marketplace.Plugin
	PluginCursor        int
	LoadErr             string

	// Install flow
	SelectedPlugin *marketplace.Plugin
	Targets        []installTargetOption
	TargetCursor   int
	InstallErr     string
	InstallOK      string
	Installing     bool

	// Active project (if any) for project-scope target eligibility
	ActiveProject *model.ProjectInfo

	Width  int
	Height int
	keys   KeyMap
}

// NewMarketplaceUIModel creates a marketplace UI bound to the given config
// state. ActiveProject is nil when the user is in Global scope.
func NewMarketplaceUIModel(mps []model.MarketplaceInfo, active *model.ProjectInfo) MarketplaceUIModel {
	return MarketplaceUIModel{
		Marketplaces:  mps,
		ActiveProject: active,
		keys:          DefaultKeyMap(),
	}
}

// SetActiveProject keeps the project context in sync for install eligibility.
// Called by the parent app when the active project changes.
func (m *MarketplaceUIModel) SetActiveProject(p *model.ProjectInfo) { m.ActiveProject = p }

// Update handles a key message and returns the updated model + any command.
func (m MarketplaceUIModel) Update(msg tea.Msg) (MarketplaceUIModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Delete confirmation captures all input.
		if m.DeleteConfirm {
			switch msg.String() {
			case "y", "Y", "enter":
				name := m.DeleteName
				m.DeleteConfirm = false
				m.DeleteName = ""
				return m, func() tea.Msg { return MarketplaceRemovedMsg{Name: name} }
			case "n", "N", "esc":
				m.DeleteConfirm = false
				m.DeleteName = ""
			}
			return m, nil
		}

		switch m.Mode {
		case MpModeAdd:
			return m.updateAdd(msg)
		case MpModePlugins:
			return m.updatePlugins(msg)
		case MpModeInstall:
			return m.updateInstall(msg)
		default:
			return m.updateList(msg)
		}
	}
	return m, nil
}

func (m MarketplaceUIModel) updateList(msg tea.KeyMsg) (MarketplaceUIModel, tea.Cmd) {
	// Clear transient banners on any key.
	m.SyncOK = false
	m.SyncErrors = nil
	m.InstallOK = ""
	m.InstallErr = ""

	switch {
	case key.Matches(msg, m.keys.Up):
		if m.Cursor > 0 {
			m.Cursor--
		}
	case key.Matches(msg, m.keys.Down):
		if m.Cursor < len(m.Marketplaces)-1 {
			m.Cursor++
		}
	case key.Matches(msg, m.keys.AddProject): // 'a'
		m.Mode = MpModeAdd
		m.SourceBuf = nil
		m.AddErr = ""
	case key.Matches(msg, m.keys.DeleteProject): // 'd'
		if m.Cursor < len(m.Marketplaces) {
			m.DeleteConfirm = true
			m.DeleteName = m.Marketplaces[m.Cursor].Name
		}
	case msg.String() == "s":
		if m.Cursor < len(m.Marketplaces) {
			return m, func() tea.Msg {
				return MarketplaceSyncMsg{Name: m.Marketplaces[m.Cursor].Name}
			}
		}
	case msg.String() == "r":
		if len(m.Marketplaces) > 0 {
			return m, func() tea.Msg { return MarketplaceSyncMsg{} }
		}
	case key.Matches(msg, m.keys.Select):
		if m.Cursor < len(m.Marketplaces) {
			mp := m.Marketplaces[m.Cursor]
			return m, func() tea.Msg { return MarketplaceOpenMsg{Name: mp.Name} }
		}
	}
	return m, nil
}

func (m MarketplaceUIModel) updateAdd(msg tea.KeyMsg) (MarketplaceUIModel, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back):
		m.Mode = MpModeList
		m.SourceBuf = nil
		m.AddErr = ""
	case key.Matches(msg, m.keys.Select):
		raw := strings.TrimSpace(string(m.SourceBuf))
		if raw == "" {
			m.AddErr = "ソースを入力してください"
			return m, nil
		}
		if _, err := marketplace.ParseSource(raw); err != nil {
			m.AddErr = err.Error()
			return m, nil
		}
		m.Mode = MpModeList
		m.SourceBuf = nil
		m.AddErr = ""
		return m, func() tea.Msg { return MarketplaceAddedMsg{Source: raw} }
	default:
		m.AddErr = ""
		switch msg.String() {
		case "backspace":
			if len(m.SourceBuf) > 0 {
				m.SourceBuf = m.SourceBuf[:len(m.SourceBuf)-1]
			}
		default:
			if msg.Type == tea.KeyRunes {
				m.SourceBuf = append(m.SourceBuf, msg.Runes...)
			}
		}
	}
	return m, nil
}

func (m MarketplaceUIModel) updatePlugins(msg tea.KeyMsg) (MarketplaceUIModel, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back):
		m.Mode = MpModeList
		m.SelectedMarketplace = nil
		m.Plugins = nil
		m.LoadErr = ""
	case key.Matches(msg, m.keys.Up):
		if m.PluginCursor > 0 {
			m.PluginCursor--
		}
	case key.Matches(msg, m.keys.Down):
		if m.PluginCursor < len(m.Plugins)-1 {
			m.PluginCursor++
		}
	case key.Matches(msg, m.keys.Select):
		if m.PluginCursor < len(m.Plugins) {
			plugin := m.Plugins[m.PluginCursor]
			if plugin.Dir == "" {
				m.LoadErr = fmt.Sprintf("プラグイン %q を解決できませんでした: %s", plugin.Name, plugin.UnresolvedReason)
				return m, nil
			}
			m.SelectedPlugin = &plugin
			m.Targets = m.buildTargets()
			m.TargetCursor = 0
			m.Mode = MpModeInstall
			m.InstallErr = ""
		}
	}
	return m, nil
}

func (m MarketplaceUIModel) buildTargets() []installTargetOption {
	hasProject := m.ActiveProject != nil
	return []installTargetOption{
		{Provider: model.ClaudeCode, Scope: model.Global, Label: "Claude Code · Global"},
		{Provider: model.ClaudeCode, Scope: model.Project,
			Label: "Claude Code · Project", Disabled: !hasProject},
		{Provider: model.Codex, Scope: model.Global, Label: "Codex · Global"},
		{Provider: model.Codex, Scope: model.Project,
			Label: "Codex · Project", Disabled: !hasProject},
	}
}

func (m MarketplaceUIModel) updateInstall(msg tea.KeyMsg) (MarketplaceUIModel, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back):
		m.Mode = MpModePlugins
		m.SelectedPlugin = nil
		m.Targets = nil
	case key.Matches(msg, m.keys.Up):
		if m.TargetCursor > 0 {
			m.TargetCursor--
		}
	case key.Matches(msg, m.keys.Down):
		if m.TargetCursor < len(m.Targets)-1 {
			m.TargetCursor++
		}
	case key.Matches(msg, m.keys.ToggleCheck):
		if m.TargetCursor < len(m.Targets) {
			t := &m.Targets[m.TargetCursor]
			if !t.Disabled {
				t.Selected = !t.Selected
			}
		}
	case key.Matches(msg, m.keys.Select):
		var picked []installTargetOption
		for _, t := range m.Targets {
			if t.Selected && !t.Disabled {
				picked = append(picked, t)
			}
		}
		if len(picked) == 0 {
			m.InstallErr = "インストール先を1つ以上選択してください"
			return m, nil
		}
		if m.SelectedPlugin == nil {
			return m, nil
		}
		plugin := *m.SelectedPlugin
		m.Installing = true
		m.InstallErr = ""
		return m, func() tea.Msg {
			return MarketplaceInstallMsg{Plugin: plugin, Targets: picked}
		}
	}
	return m, nil
}

// View renders the marketplace tab.
func (m MarketplaceUIModel) View() string {
	var b strings.Builder

	if m.DeleteConfirm {
		b.WriteString(TitleStyle.Render("Marketplace"))
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("  マーケットプレイス \"%s\" を削除しますか？\n", m.DeleteName))
		b.WriteString("  キャッシュも削除されます。\n\n")
		b.WriteString(fmt.Sprintf("  %s   %s\n",
			SelectedStyle.Render("[y] はい"),
			NormalStyle.Render("[n] いいえ"),
		))
		return b.String()
	}

	switch m.Mode {
	case MpModeAdd:
		return m.viewAdd()
	case MpModePlugins:
		return m.viewPlugins()
	case MpModeInstall:
		return m.viewInstall()
	}
	return m.viewList()
}

func (m MarketplaceUIModel) viewList() string {
	var b strings.Builder
	b.WriteString(TitleStyle.Render("Plugin Marketplaces"))
	b.WriteString("\n\n")

	if len(m.Marketplaces) == 0 {
		b.WriteString(m.emptyGuide())
		return b.String()
	}

	for i, mp := range m.Marketplaces {
		var status string
		if m.SyncErrors != nil {
			if _, ok := m.SyncErrors[mp.Name]; ok {
				status = lipgloss.NewStyle().Foreground(ErrorColor).Render(" [!]")
			}
		}
		row := fmt.Sprintf("%s%s", mp.Name, status)
		src := DimStyle.Render(fmt.Sprintf("    %s", mp.Source))

		if i == m.Cursor {
			b.WriteString(SelectedStyle.Render("> " + row))
		} else {
			b.WriteString(NormalStyle.Render("  " + row))
		}
		b.WriteString("\n")
		b.WriteString(src)
		b.WriteString("\n")

		if i == m.Cursor && m.SyncErrors != nil {
			if e, ok := m.SyncErrors[mp.Name]; ok {
				b.WriteString(lipgloss.NewStyle().Foreground(ErrorColor).
					Render(fmt.Sprintf("    %s", e.Error())))
				b.WriteString("\n")
			}
		}
	}

	if m.Syncing {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(SecondaryColor).Render("  同期中..."))
		b.WriteString("\n")
	}
	if m.SyncOK {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(SecondaryColor).
			Render("  同期完了。enter でプラグイン一覧を表示します。"))
		b.WriteString("\n")
	}
	if m.InstallOK != "" {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(SecondaryColor).Render("  " + m.InstallOK))
		b.WriteString("\n")
	}
	if m.InstallErr != "" {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(ErrorColor).Render("  " + m.InstallErr))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  enter:プラグイン一覧 | a:追加 | d:削除 | s:同期 | r:全同期"))
	return b.String()
}

func (m MarketplaceUIModel) emptyGuide() string {
	var b strings.Builder
	b.WriteString(SectionHeaderStyle.Render("Claude Code 互換のプラグインマーケットプレイス"))
	b.WriteString("\n\n")
	b.WriteString(NormalStyle.Render("  Git リポジトリやローカルディレクトリからプラグインを"))
	b.WriteString("\n")
	b.WriteString(NormalStyle.Render("  インストールできます。Claude Code の /plugin marketplace add"))
	b.WriteString("\n")
	b.WriteString(NormalStyle.Render("  と同じ書式が使えます。"))
	b.WriteString("\n\n")

	b.WriteString(SectionHeaderStyle.Render("対応する追加方法"))
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  • GitHub 短縮形:    anthropics/skills"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  • Git URL:           https://gitlab.com/team/plugins.git"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  • ref ピン留め:       owner/repo#v1.0.0"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  • ローカルパス:       ./my-marketplace"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  • marketplace.json:   https://example.com/marketplace.json"))
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  a:マーケットプレイス追加"))
	return b.String()
}

func (m MarketplaceUIModel) viewAdd() string {
	var b strings.Builder
	b.WriteString(TitleStyle.Render("Marketplace を追加"))
	b.WriteString("\n\n")
	b.WriteString("  Claude Code と同じ書式でソースを指定してください:\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(PrimaryColor).
		Padding(0, 1).
		Width(60).
		Render(string(m.SourceBuf) + "▌")
	b.WriteString("  ")
	b.WriteString(box)
	b.WriteString("\n")
	if m.AddErr != "" {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(ErrorColor).Render("  " + m.AddErr))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  例: anthropics/skills"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("      https://gitlab.com/team/plugins.git#v1.0.0"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("      ./my-marketplace"))
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  enter:登録（自動 sync） | esc:キャンセル"))
	return b.String()
}

func (m MarketplaceUIModel) viewPlugins() string {
	var b strings.Builder
	name := ""
	if m.SelectedMarketplace != nil {
		name = m.SelectedMarketplace.Name
	}
	b.WriteString(TitleStyle.Render(fmt.Sprintf("Plugins · %s", name)))
	b.WriteString("\n\n")

	if m.LoadErr != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(ErrorColor).Render("  " + m.LoadErr))
		b.WriteString("\n\n")
		b.WriteString(DimStyle.Render("  esc:戻る"))
		return b.String()
	}

	if len(m.Plugins) == 0 {
		b.WriteString(DimStyle.Render("  プラグインが見つかりません。先に [s] で sync してください。"))
		b.WriteString("\n\n")
		b.WriteString(DimStyle.Render("  esc:戻る"))
		return b.String()
	}

	for i, p := range m.Plugins {
		marker := "  "
		if p.UnresolvedReason != "" {
			marker = lipgloss.NewStyle().Foreground(WarningColor).Render("⚠ ")
		}
		head := fmt.Sprintf("%s%s", marker, p.Name)
		if p.Version != "" {
			head += DimStyle.Render(" v" + p.Version)
		}
		if i == m.PluginCursor {
			b.WriteString(SelectedStyle.Render("> " + head))
		} else {
			b.WriteString(NormalStyle.Render("  " + head))
		}
		b.WriteString("\n")
		if p.Description != "" {
			b.WriteString(DimStyle.Render(fmt.Sprintf("    %s", p.Description)))
			b.WriteString("\n")
		}
		if p.UnresolvedReason != "" {
			b.WriteString(lipgloss.NewStyle().Foreground(WarningColor).
				Render(fmt.Sprintf("    %s", p.UnresolvedReason)))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  enter:インストール | esc:戻る"))
	return b.String()
}

func (m MarketplaceUIModel) viewInstall() string {
	var b strings.Builder
	name := ""
	if m.SelectedPlugin != nil {
		name = m.SelectedPlugin.Name
	}
	b.WriteString(TitleStyle.Render(fmt.Sprintf("Install · %s", name)))
	b.WriteString("\n\n")

	if m.SelectedPlugin != nil {
		b.WriteString(DimStyle.Render(fmt.Sprintf("  含まれるコンポーネント:"))) // header
		b.WriteString("\n")
		b.WriteString(componentSummary(*m.SelectedPlugin))
		b.WriteString("\n\n")
	}

	b.WriteString(SectionHeaderStyle.Render("インストール先"))
	b.WriteString("\n\n")
	for i, t := range m.Targets {
		check := UncheckedStyle
		if t.Selected {
			check = CheckedStyle
		}
		label := t.Label
		if t.Disabled {
			label += " (プロジェクト未選択)"
		}
		line := fmt.Sprintf("  %s %s", check, label)
		switch {
		case t.Disabled:
			b.WriteString(DimStyle.Render("  " + line))
		case i == m.TargetCursor:
			b.WriteString(SelectedStyle.Render("> " + line))
		default:
			b.WriteString(NormalStyle.Render("  " + line))
		}
		b.WriteString("\n")
	}
	if m.Installing {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(SecondaryColor).Render("  インストール中..."))
		b.WriteString("\n")
	}
	if m.InstallErr != "" {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(ErrorColor).Render("  " + m.InstallErr))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  space:選択 | enter:インストール | esc:戻る"))
	return b.String()
}

// componentSummary renders a one-line list of the plugin's component counts.
func componentSummary(p marketplace.Plugin) string {
	var parts []string
	if n := len(p.Skills); n > 0 {
		parts = append(parts, fmt.Sprintf("Skills: %d", n))
	}
	if n := len(p.Commands); n > 0 {
		parts = append(parts, fmt.Sprintf("Commands: %d", n))
	}
	if n := len(p.Agents); n > 0 {
		parts = append(parts, fmt.Sprintf("Agents: %d", n))
	}
	if n := len(p.HookFiles); n > 0 {
		parts = append(parts, fmt.Sprintf("Hooks: %d", n))
	}
	if n := len(p.McpFiles); n > 0 {
		parts = append(parts, fmt.Sprintf("MCP: %d", n))
	}
	if len(parts) == 0 {
		return DimStyle.Render("    （コンポーネントが検出されませんでした）")
	}
	return DimStyle.Render("    " + strings.Join(parts, " · "))
}
