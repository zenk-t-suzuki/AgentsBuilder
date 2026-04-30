package tui

import (
	"fmt"
	"strings"

	"agentsbuilder/internal/model"
	"agentsbuilder/internal/registry"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// RegistryInputMode tracks the current sub-mode of the registry panel.
type RegistryInputMode int

const (
	RegistryBrowse      RegistryInputMode = iota // browsing registry list
	RegistryInputURL                             // typing URL to add
	RegistryPickPublish                          // picking a local template to upload
)

// RegistryUIModel manages the registry management panel.
type RegistryUIModel struct {
	Registries []model.RegistryInfo
	Cursor     int
	Width      int
	Height     int

	InputMode RegistryInputMode
	URLBuf    []rune
	InputErr  string

	Syncing    bool
	SyncErrors map[string]error
	SyncOK     bool

	// Publish (upload) state
	Publishing     bool
	PublishErr     string
	PublishOK      string // success message
	LocalTemplates []registry.LocalTemplate
	PublishCursor  int

	// Delete confirmation
	DeleteConfirm bool
	DeleteName    string

	keys KeyMap
}

// NewRegistryUIModel creates a new registry UI model.
func NewRegistryUIModel(registries []model.RegistryInfo) RegistryUIModel {
	return RegistryUIModel{
		Registries: registries,
		keys:       DefaultKeyMap(),
	}
}

func (m RegistryUIModel) Update(msg tea.Msg) (RegistryUIModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Delete confirmation captures all input
		if m.DeleteConfirm {
			switch msg.String() {
			case "y", "Y", "enter":
				name := m.DeleteName
				m.DeleteConfirm = false
				m.DeleteName = ""
				return m, func() tea.Msg { return RegistryRemovedMsg{Name: name} }
			case "n", "N", "esc":
				m.DeleteConfirm = false
				m.DeleteName = ""
			}
			return m, nil
		}

		switch m.InputMode {
		case RegistryInputURL:
			return m.updateURLInput(msg)
		case RegistryPickPublish:
			return m.updatePublishPick(msg)
		default:
			return m.updateBrowse(msg)
		}
	}
	return m, nil
}

func (m RegistryUIModel) updateBrowse(msg tea.KeyMsg) (RegistryUIModel, tea.Cmd) {
	// Clear transient status on any key
	m.SyncOK = false
	m.SyncErrors = nil
	m.PublishOK = ""
	m.PublishErr = ""

	switch {
	case key.Matches(msg, m.keys.Up):
		if m.Cursor > 0 {
			m.Cursor--
		}
	case key.Matches(msg, m.keys.Down):
		if m.Cursor < len(m.Registries)-1 {
			m.Cursor++
		}
	case key.Matches(msg, m.keys.AddProject):
		// 'a' — start adding a new registry
		m.InputMode = RegistryInputURL
		m.URLBuf = nil
		m.InputErr = ""
	case key.Matches(msg, m.keys.DeleteProject):
		// 'd' — delete selected registry
		if m.Cursor < len(m.Registries) {
			m.DeleteConfirm = true
			m.DeleteName = m.Registries[m.Cursor].Name
		}
	case msg.String() == "p":
		// 'p' — publish (upload) a local template to the selected registry
		if m.Cursor < len(m.Registries) {
			locals := registry.ListLocalTemplates()
			if len(locals) == 0 {
				m.PublishErr = "アップロードできるテンプレートがありません。先に [2] Template → n でテンプレートを作成してください"
				return m, nil
			}
			m.InputMode = RegistryPickPublish
			m.LocalTemplates = locals
			m.PublishCursor = 0
			m.PublishErr = ""
		}
	case msg.String() == "r":
		// 'r' — sync all registries
		if len(m.Registries) > 0 {
			return m, func() tea.Msg { return RegistrySyncMsg{} }
		}
	case msg.String() == "s":
		// 's' — sync selected registry
		if m.Cursor < len(m.Registries) {
			name := m.Registries[m.Cursor].Name
			return m, func() tea.Msg { return RegistrySyncMsg{Name: name} }
		}
	}
	return m, nil
}

func (m RegistryUIModel) updateURLInput(msg tea.KeyMsg) (RegistryUIModel, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back):
		m.InputMode = RegistryBrowse
		m.URLBuf = nil
		m.InputErr = ""
	case key.Matches(msg, m.keys.Select):
		raw := strings.TrimSpace(string(m.URLBuf))
		if raw == "" {
			m.InputErr = "URLを入力してください"
			return m, nil
		}
		url := registry.NormalizeURL(raw)
		name := registryNameFromURL(url)
		if name == "" {
			m.InputErr = "URLからリポジトリ名を取得できません"
			return m, nil
		}
		for _, r := range m.Registries {
			if r.Name == name {
				m.InputErr = fmt.Sprintf("%q は既に登録されています", name)
				return m, nil
			}
		}
		m.InputMode = RegistryBrowse
		m.URLBuf = nil
		m.InputErr = ""
		return m, func() tea.Msg { return RegistryAddedMsg{Name: name, URL: url} }
	default:
		m.InputErr = ""
		switch msg.String() {
		case "backspace":
			if len(m.URLBuf) > 0 {
				m.URLBuf = m.URLBuf[:len(m.URLBuf)-1]
			}
		default:
			if msg.Type == tea.KeyRunes {
				m.URLBuf = append(m.URLBuf, msg.Runes...)
			}
		}
	}
	return m, nil
}

func (m RegistryUIModel) updatePublishPick(msg tea.KeyMsg) (RegistryUIModel, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back):
		m.InputMode = RegistryBrowse
		m.LocalTemplates = nil
	case key.Matches(msg, m.keys.Up):
		if m.PublishCursor > 0 {
			m.PublishCursor--
		}
	case key.Matches(msg, m.keys.Down):
		if m.PublishCursor < len(m.LocalTemplates)-1 {
			m.PublishCursor++
		}
	case key.Matches(msg, m.keys.Select):
		if m.PublishCursor < len(m.LocalTemplates) {
			lt := m.LocalTemplates[m.PublishCursor]
			reg := m.Registries[m.Cursor]
			m.InputMode = RegistryBrowse
			m.LocalTemplates = nil
			m.Publishing = true
			m.PublishErr = ""
			return m, func() tea.Msg {
				return RegistryPublishMsg{
					RegistryName: reg.Name,
					TemplateName: lt.Name,
					TemplateDir:  lt.Dir,
				}
			}
		}
	}
	return m, nil
}

func (m RegistryUIModel) View() string {
	var b strings.Builder

	b.WriteString(TitleStyle.Render("Template Registry"))
	b.WriteString("\n\n")

	// Delete confirmation
	if m.DeleteConfirm {
		b.WriteString(fmt.Sprintf("  レジストリ \"%s\" を削除しますか？\n", m.DeleteName))
		b.WriteString("  ダウンロード済みのキャッシュも削除されます。\n\n")
		b.WriteString(fmt.Sprintf("  %s   %s\n",
			SelectedStyle.Render("[y] はい"),
			NormalStyle.Render("[n] いいえ"),
		))
		return b.String()
	}

	// URL input mode
	if m.InputMode == RegistryInputURL {
		b.WriteString(SectionHeaderStyle.Render("レジストリを追加"))
		b.WriteString("\n\n")
		b.WriteString("  GitリポジトリのURLを貼り付けてください:\n")
		url := string(m.URLBuf)
		nameBox := lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(PrimaryColor).
			Padding(0, 1).
			Width(60).
			Render(url + "▌")
		b.WriteString("  ")
		b.WriteString(nameBox)
		b.WriteString("\n")
		if m.InputErr != "" {
			b.WriteString("\n")
			b.WriteString(lipgloss.NewStyle().Foreground(ErrorColor).Render("  " + m.InputErr))
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  GitHubのURLをそのまま貼り付けてOKです:"))
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  例: https://github.com/your-org/agent-templates"))
		b.WriteString("\n\n")
		b.WriteString(DimStyle.Render("  enter:登録 | esc:キャンセル"))
		return b.String()
	}

	// Publish: pick local template
	if m.InputMode == RegistryPickPublish {
		regName := ""
		if m.Cursor < len(m.Registries) {
			regName = m.Registries[m.Cursor].Name
		}
		b.WriteString(SectionHeaderStyle.Render(fmt.Sprintf("アップロード先: %s", regName)))
		b.WriteString("\n\n")
		b.WriteString(NormalStyle.Render("  アップロードするテンプレートを選択してください:"))
		b.WriteString("\n\n")
		for i, lt := range m.LocalTemplates {
			if i == m.PublishCursor {
				b.WriteString(SelectedStyle.Render(fmt.Sprintf("> %s", lt.Name)))
			} else {
				b.WriteString(NormalStyle.Render(fmt.Sprintf("  %s", lt.Name)))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  enter:アップロード | esc:キャンセル"))
		return b.String()
	}

	// Empty state
	if len(m.Registries) == 0 {
		b.WriteString(m.renderEmptyGuide())
		return b.String()
	}

	// Registry list
	for i, reg := range m.Registries {
		cached := registry.IsCached(reg.Name)
		tmplCount := 0
		if cached {
			tmplCount = registry.TemplateCount(reg)
		}

		// Status indicator
		var status string
		if m.SyncErrors != nil {
			if _, hasErr := m.SyncErrors[reg.Name]; hasErr {
				status = lipgloss.NewStyle().Foreground(ErrorColor).Render(" [!]")
			}
		}
		if status == "" && cached {
			status = lipgloss.NewStyle().Foreground(SecondaryColor).
				Render(fmt.Sprintf(" (%d templates)", tmplCount))
		} else if status == "" {
			status = DimStyle.Render(" (未同期)")
		}

		content := fmt.Sprintf("%s%s", reg.Name, status)
		urlLine := DimStyle.Render(fmt.Sprintf("    %s", reg.URL))

		if i == m.Cursor {
			b.WriteString(SelectedStyle.Render("> " + content))
		} else {
			b.WriteString(NormalStyle.Render("  " + content))
		}
		b.WriteString("\n")
		b.WriteString(urlLine)
		b.WriteString("\n")

		// Show error detail for selected item
		if i == m.Cursor && m.SyncErrors != nil {
			if syncErr, ok := m.SyncErrors[reg.Name]; ok {
				b.WriteString(lipgloss.NewStyle().Foreground(ErrorColor).
					Render(fmt.Sprintf("    %s", syncErr)))
				b.WriteString("\n")
			}
		}
	}

	// Status messages
	if m.Syncing || m.Publishing {
		b.WriteString("\n")
		label := "同期中..."
		if m.Publishing {
			label = "アップロード中..."
		}
		b.WriteString(lipgloss.NewStyle().Foreground(SecondaryColor).Render("  " + label))
		b.WriteString("\n")
	}

	if m.SyncOK {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(SecondaryColor).
			Render("  同期が完了しました。[2] Template タブでテンプレートを使用できます。"))
		b.WriteString("\n")
	}

	if m.PublishOK != "" {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(SecondaryColor).Render("  " + m.PublishOK))
		b.WriteString("\n")
	}

	if m.PublishErr != "" {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(ErrorColor).Render("  " + m.PublishErr))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  a:追加 | d:削除 | p:アップロード | s:同期 | r:全て同期"))

	return b.String()
}

// renderEmptyGuide renders a setup guide when no registries are registered.
func (m RegistryUIModel) renderEmptyGuide() string {
	var b strings.Builder

	b.WriteString(SectionHeaderStyle.Render("チームでテンプレートを共有しよう"))
	b.WriteString("\n\n")

	b.WriteString(NormalStyle.Render("  Gitリポジトリにテンプレートを保存して、チーム全員で"))
	b.WriteString("\n")
	b.WriteString(NormalStyle.Render("  同じ設定を使えるようにできます。"))
	b.WriteString("\n\n")

	b.WriteString(SectionHeaderStyle.Render("使い方"))
	b.WriteString("\n\n")

	b.WriteString(NormalStyle.Render("  1. GitHubで新しいリポジトリを作成"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("     空のリポジトリでOK — 中身はここから追加できます"))
	b.WriteString("\n\n")

	b.WriteString(NormalStyle.Render("  2. ここでリポジトリURLを登録 (a キー)"))
	b.WriteString("\n\n")

	b.WriteString(NormalStyle.Render("  3. テンプレートをアップロード (p キー)"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("     ローカルで作成したテンプレートをリポジトリに追加"))
	b.WriteString("\n\n")

	b.WriteString(NormalStyle.Render("  4. チームメンバーは同じURLを登録して同期 (s キー)"))
	b.WriteString("\n\n")

	b.WriteString(DimStyle.Render("  a:レジストリを追加"))

	return b.String()
}

// registryNameFromURL extracts a short name from a Git URL.
func registryNameFromURL(url string) string {
	url = strings.TrimSuffix(url, ".git")
	url = strings.TrimRight(url, "/")

	// SSH format: git@host:org/repo
	if idx := strings.LastIndex(url, ":"); idx > 0 && !strings.Contains(url, "://") {
		url = url[idx+1:]
	}

	// HTTPS format: get last path component
	if idx := strings.LastIndex(url, "/"); idx >= 0 {
		url = url[idx+1:]
	}

	return url
}
