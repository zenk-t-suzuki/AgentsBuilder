package tui

import (
	"fmt"
	"strings"

	"agentsbuilder/internal/assetdef"
	"agentsbuilder/internal/model"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
)

// BrowseTabDef describes an inner tab within Browse mode.
type BrowseTabDef struct {
	Label string             // display label
	Key   string             // shortcut key (empty = no shortcut; "h" conflicts with pane-switch)
	All   bool               // true = show all asset types
	Types []model.AssetType  // asset types matched by this tab (used when All=false)
}

// allBrowseTabs returns the ordered list of inner tabs for Browse mode.
// AGENTS.md and CLAUDE.md are combined into a single "Docs" tab (both here and
// in the All tab where they share a "Docs" section header).
// "h" is reserved for the pane-switch shortcut so Hooks has no direct key.
func allBrowseTabs() []BrowseTabDef {
	return []BrowseTabDef{
		{Label: "All", Key: "a", All: true},
		{Label: "Skills", Key: "s", Types: []model.AssetType{model.Skills}},
		{Label: "Custom Agents", Key: "c", Types: []model.AssetType{model.Agents}},
		{Label: "MCP", Key: "m", Types: []model.AssetType{model.MCP}},
		{Label: "Plugins", Key: "p", Types: []model.AssetType{model.Plugins}},
		{Label: "Hooks", Key: "", Types: []model.AssetType{model.Hooks}}, // 'h' conflicts with focus-sidebar
		{Label: "Docs", Key: "d", Types: []model.AssetType{model.AgentsMD, model.ClaudeMD}},
	}
}

// sectionLabel returns the section header label for an asset type.
// AgentsMD and ClaudeMD are both rendered under "Docs" so they share
// one section header in both the All tab and the Docs tab.
func sectionLabel(t model.AssetType) string {
	switch t {
	case model.AgentsMD, model.ClaudeMD:
		return "Docs"
	default:
		return t.String()
	}
}

// browseTabLabel returns the tab label with the shortcut key wrapped in brackets.
// e.g. label="All", key="a" → "[A]ll"
func browseTabLabel(tab BrowseTabDef) string {
	if tab.Key == "" {
		return tab.Label
	}
	upper := strings.ToUpper(tab.Key)
	idx := strings.Index(strings.ToUpper(tab.Label), upper)
	if idx < 0 {
		return tab.Label
	}
	return tab.Label[:idx] + "[" + tab.Label[idx:idx+1] + "]" + tab.Label[idx+1:]
}

// MainAreaModel displays assets grouped by type for the selected scope.
type MainAreaModel struct {
	Assets             []model.Asset
	GlobalAssets       []model.Asset  // non-nil when in project scope: shows inherited global config
	Diffs              []model.DiffResult
	Focused            bool
	TabFocused         bool // true when keyboard cursor is in the inner Browse tab bar
	Width              int
	Height             int // visible rows for content (set by updateLayout)
	SelectedAssetIndex int // index into the flat selectables list
	ActiveBrowseTab    int // index into allBrowseTabs()

	keys KeyMap
}

// NewMainAreaModel creates a new main area model.
func NewMainAreaModel() MainAreaModel {
	return MainAreaModel{
		keys: DefaultKeyMap(),
	}
}

// selectableItem maps a selectable list index to an asset and optional item.
// When global is true, assetIdx refers to GlobalAssets instead of Assets.
type selectableItem struct {
	assetIdx int
	itemIdx  int  // -1 means the asset directory/file itself
	global   bool // true when item comes from GlobalAssets
}

// isVisible returns true if an asset should appear in the list.
// Embedded assets (MCP, Plugins in JSON, etc.) are only shown when they have
// actual items — an empty settings.json with no MCP servers is not meaningful.
// Directory and single-file assets are shown whenever they exist on disk.
func isVisible(a model.Asset) bool {
	if def, ok := assetdef.Lookup(a.Provider, a.Scope, a.Type); ok && def.IsEmbedded() {
		return len(a.Items) > 0
	}
	return a.Exists || len(a.Items) > 0
}

// matchesBrowseTab returns true if the asset matches the currently active inner tab.
func (m MainAreaModel) matchesBrowseTab(a model.Asset) bool {
	tabs := allBrowseTabs()
	if m.ActiveBrowseTab >= len(tabs) || tabs[m.ActiveBrowseTab].All {
		return true
	}
	for _, t := range tabs[m.ActiveBrowseTab].Types {
		if a.Type == t {
			return true
		}
	}
	return false
}

// hasAnyAssets returns true if any project-scope asset is visible regardless of the active tab.
func (m MainAreaModel) hasAnyAssets() bool {
	for _, a := range m.Assets {
		if isVisible(a) {
			return true
		}
	}
	return false
}

// computeSelectables returns the flat list of selectable entries,
// filtered by the active Browse tab. Missing assets with no items are excluded.
// Global assets (when present) are appended after project assets.
func (m MainAreaModel) computeSelectables() []selectableItem {
	var sel []selectableItem
	appendAssets := func(assets []model.Asset, global bool) {
		for ai, asset := range assets {
			if !isVisible(asset) {
				continue
			}
			if !m.matchesBrowseTab(asset) {
				continue
			}
			if len(asset.Items) > 0 {
				for ii := range asset.Items {
					sel = append(sel, selectableItem{assetIdx: ai, itemIdx: ii, global: global})
				}
			} else {
				sel = append(sel, selectableItem{assetIdx: ai, itemIdx: -1, global: global})
			}
		}
	}
	appendAssets(m.Assets, false)
	appendAssets(m.GlobalAssets, true)
	return sel
}

func (m MainAreaModel) Update(msg tea.Msg) (MainAreaModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		tabs := allBrowseTabs()

		// Sequential tab navigation (,/.) — works from both tab bar and list.
		switch {
		case key.Matches(msg, m.keys.BrowseTabLeft):
			if m.ActiveBrowseTab > 0 {
				m.ActiveBrowseTab--
				m.SelectedAssetIndex = 0
			}
			return m, m.emitAssetSelected()
		case key.Matches(msg, m.keys.BrowseTabRight):
			if m.ActiveBrowseTab < len(tabs)-1 {
				m.ActiveBrowseTab++
				m.SelectedAssetIndex = 0
			}
			return m, m.emitAssetSelected()
		}

		// Initial key shortcuts — work from both tab bar and list.
		for i, tab := range tabs {
			if tab.Key != "" && msg.String() == tab.Key {
				m.ActiveBrowseTab = i
				m.SelectedAssetIndex = 0
				return m, m.emitAssetSelected()
			}
		}

		// Tab bar has focus: ↓ returns to list, other keys are consumed.
		if m.TabFocused {
			if key.Matches(msg, m.keys.Down) {
				m.TabFocused = false
				return m, m.emitAssetSelected()
			}
			return m, nil
		}

		// List navigation.
		sel := m.computeSelectables()
		switch {
		case key.Matches(msg, m.keys.Up):
			if m.SelectedAssetIndex == 0 {
				// At the top of the list: move focus up to the tab bar.
				m.TabFocused = true
				return m, nil
			}
			m.SelectedAssetIndex--
			return m, m.emitAssetSelected()
		case key.Matches(msg, m.keys.Down):
			if m.SelectedAssetIndex < len(sel)-1 {
				m.SelectedAssetIndex++
			}
			return m, m.emitAssetSelected()
		case key.Matches(msg, m.keys.Select):
			return m, m.emitAssetSelected()
		}
	}
	return m, nil
}

// AtTop reports whether the asset list cursor is at the very first item.
// Used by navAtEdge to detect the list→BrowseTabs/ModeTabs boundary.
func (m MainAreaModel) AtTop() bool {
	return m.SelectedAssetIndex == 0
}

// SwitchBrowseTab moves the active inner Browse tab by delta (-1 = left, +1 = right),
// resets the list cursor, and returns a selection-emit command.
func (m MainAreaModel) SwitchBrowseTab(delta int) (MainAreaModel, tea.Cmd) {
	tabs := allBrowseTabs()
	newTab := m.ActiveBrowseTab + delta
	if newTab < 0 {
		newTab = 0
	}
	if newTab >= len(tabs) {
		newTab = len(tabs) - 1
	}
	m.ActiveBrowseTab = newTab
	m.SelectedAssetIndex = 0
	return m, m.emitAssetSelected()
}

func (m MainAreaModel) emitAssetSelected() tea.Cmd {
	sel := m.computeSelectables()
	if len(sel) == 0 || m.SelectedAssetIndex >= len(sel) {
		return nil
	}
	s := sel[m.SelectedAssetIndex]
	assets := m.Assets
	if s.global {
		assets = m.GlobalAssets
	}
	asset := assets[s.assetIdx]
	var item *model.AssetItem
	if s.itemIdx >= 0 {
		i := asset.Items[s.itemIdx]
		item = &i
	}
	return func() tea.Msg { return AssetSelectedMsg{Asset: &asset, Item: item} }
}

// renderBrowseTabs renders the inner tab bar for Browse mode,
// showing each tab with its shortcut key highlighted in brackets.
// When the tab bar has focus (TabFocused && Focused), the active tab is
// rendered with an inverted/filled style to indicate cursor position.
func (m MainAreaModel) renderBrowseTabs() string {
	tabs := allBrowseTabs()
	rendered := make([]string, len(tabs))
	for i, tab := range tabs {
		label := browseTabLabel(tab)
		if i == m.ActiveBrowseTab {
			if m.Focused && m.TabFocused {
				rendered[i] = FocusedTabStyle.Render(label)
			} else {
				rendered[i] = ActiveTabStyle.Render(label)
			}
		} else {
			rendered[i] = InactiveTabStyle.Render(label)
		}
	}
	row := lipgloss.JoinHorizontal(lipgloss.Bottom, rendered...)
	// Extend the bottom border line to span the full width.
	remaining := m.Width - lipgloss.Width(row)
	if remaining > 0 {
		filler := lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(MutedColor).
			Width(remaining).
			Render("")
		row = lipgloss.JoinHorizontal(lipgloss.Bottom, row, filler)
	}
	return row
}

// providerLabel renders the provider name right-aligned within the row,
// styled in MutedColor. Use providerLabelPlain for selected rows so the
// outer SelectedStyle can apply its color without being overridden.
func (m MainAreaModel) providerLabel(provider model.Provider, usedWidth int) string {
	spacing, label := m.providerLabelParts(provider, usedWidth)
	return spacing + lipgloss.NewStyle().Foreground(MutedColor).Render(label)
}

// providerLabelPlain returns the same layout as providerLabel but with no
// colour applied — used for selected rows so SelectedStyle colours the whole
// line uniformly (inner MutedColor ANSI codes would otherwise override it).
func (m MainAreaModel) providerLabelPlain(provider model.Provider, usedWidth int) string {
	spacing, label := m.providerLabelParts(provider, usedWidth)
	return spacing + label
}

// renderSelectedRow renders a selected row in bright white + bold.
func (m MainAreaModel) renderSelectedRow(content string) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Render(content)
}

func (m MainAreaModel) providerLabelParts(provider model.Provider, usedWidth int) (spacing, label string) {
	const prefixLen = 2 // "  " or "> " added at render time
	label = provider.String()
	remaining := m.Width - prefixLen - usedWidth - len(label)
	if remaining < 1 {
		remaining = 1
	}
	spacing = strings.Repeat(" ", remaining)
	return
}

// assetLine holds a rendered line and the selectable index it corresponds to (-1 = header/placeholder).
type assetLine struct {
	text     string
	assetIdx int // selectable index, or -1 for non-selectable rows
}

// buildLines renders filtered assets into a flat list of lines, grouping by AssetType.
//
// Assets not matching the active Browse tab are skipped entirely.
// If every visible asset under a section type is hidden, a dim "—" placeholder is shown.
// When GlobalAssets is set, they are rendered below a "── Global ──" separator.
func (m MainAreaModel) buildLines() []assetLine {
	var lines []assetLine

	sel := m.computeSelectables()

	// Build lookup: (assetIdx, itemIdx, global) -> selectable index
	type lineKey struct {
		ai, ii int
		global bool
	}
	selIdxMap := make(map[lineKey]int, len(sel))
	for i, s := range sel {
		selIdxMap[lineKey{s.assetIdx, s.itemIdx, s.global}] = i
	}

	renderAssets := func(assets []model.Asset, global bool) {
		currentSection := ""

		// Compute which sections have at least one visible, tab-matching asset.
		sectionHasVisible := make(map[string]bool)
		for _, asset := range assets {
			if isVisible(asset) && m.matchesBrowseTab(asset) {
				sectionHasVisible[sectionLabel(asset.Type)] = true
			}
		}

		for ai, asset := range assets {
			if !m.matchesBrowseTab(asset) {
				continue
			}
			sec := sectionLabel(asset.Type)
			if sec != currentSection {
				currentSection = sec
				lines = append(lines, assetLine{text: "", assetIdx: -1})
				lines = append(lines, assetLine{
					text:     SectionHeaderStyle.Render(sec),
					assetIdx: -1,
				})
				if !sectionHasVisible[currentSection] {
					lines = append(lines, assetLine{
						text:     DimStyle.Render("  —"),
						assetIdx: -1,
					})
				}
			}

			if !isVisible(asset) {
				continue
			}

			if len(asset.Items) > 0 {
				for ii, item := range asset.Items {
					dispIdx := selIdxMap[lineKey{ai, ii, global}]
					isSelected := dispIdx == m.SelectedAssetIndex && m.Focused && !m.TabFocused

					var rendered string
					visibleLeft := fmt.Sprintf("    . %s", item.Name)
					if isSelected {
						line := fmt.Sprintf("    ● %s", item.Name) +
							m.providerLabelPlain(asset.Provider, len(visibleLeft))
						rendered = m.renderSelectedRow("> " + line)
					} else {
						line := fmt.Sprintf("    %s %s", ActiveIndicator, item.Name) +
							m.providerLabel(asset.Provider, len(visibleLeft))
						rendered = NormalStyle.Render("  " + line)
					}
					lines = append(lines, assetLine{text: rendered, assetIdx: dispIdx})
				}
			} else {
				dispIdx := selIdxMap[lineKey{ai, -1, global}]
				isSelected := dispIdx == m.SelectedAssetIndex && m.Focused && !m.TabFocused

				diffMarkStyled := "  "
				diffMarkPlain := "  "
				if !global {
					for _, d := range m.Diffs {
						if d.AssetType == asset.Type && d.Provider == asset.Provider && d.HasDiff {
							diffMarkStyled = DiffIndicator + " "
							diffMarkPlain = "△ "
							break
						}
					}
				}

				var rendered string
				visibleLeft := fmt.Sprintf("    . %s%s", diffMarkPlain, asset.FilePath)
				if isSelected {
					line := fmt.Sprintf("    ● %s%s", diffMarkPlain, asset.FilePath) +
						m.providerLabelPlain(asset.Provider, len(visibleLeft))
					rendered = m.renderSelectedRow("> " + line)
				} else {
					line := fmt.Sprintf("    %s %s%s", ActiveIndicator, diffMarkStyled, asset.FilePath) +
						m.providerLabel(asset.Provider, len(visibleLeft))
					rendered = NormalStyle.Render("  " + line)
				}
				lines = append(lines, assetLine{text: rendered, assetIdx: dispIdx})
			}
		}
	}

	renderAssets(m.Assets, false)

	if len(m.GlobalAssets) > 0 {
		sepWidth := m.Width - 4
		if sepWidth < 4 {
			sepWidth = 4
		}
		sep := strings.Repeat("─", sepWidth)
		lines = append(lines, assetLine{text: "", assetIdx: -1})
		lines = append(lines, assetLine{
			text:     DimStyle.Render("  " + sep),
			assetIdx: -1,
		})
		lines = append(lines, assetLine{
			text:     DimStyle.Render("  Global"),
			assetIdx: -1,
		})
		renderAssets(m.GlobalAssets, true)
	}

	return lines
}

func (m MainAreaModel) View() string {
	innerTabBar := m.renderBrowseTabs()

	sel := m.computeSelectables()
	if len(sel) == 0 {
		tabs := allBrowseTabs()
		var msg string
		switch {
		case m.GlobalAssets != nil && !m.hasAnyAssets():
			// Project scope, no project assets at all — suggest template.
			msg = "\n  No assets found for this project.\n  Press 2 (or t) to create directory structure from a template."
		case m.GlobalAssets == nil && len(m.Assets) == 0:
			// Global scope, no assets.
			msg = "\n  No assets found.\n  Select a scope from the sidebar to browse."
		default:
			// Current tab has no matching assets, but other tabs may have some.
			msg = fmt.Sprintf("\n  No %s assets found.", tabs[m.ActiveBrowseTab].Label)
		}
		return innerTabBar + "\n" + DimStyle.Render(msg)
	}

	lines := m.buildLines()

	// Find which line the cursor sits on.
	cursorLine := 0
	for li, l := range lines {
		if l.assetIdx == m.SelectedAssetIndex {
			cursorLine = li
			break
		}
	}

	// Subtract the inner tab bar (2 lines) + its trailing newline (1 line).
	visible := m.Height - 3
	if visible <= 0 {
		visible = 4
	}

	// Scroll so cursor stays centred in the visible window.
	offset := cursorLine - visible/2
	if offset < 0 {
		offset = 0
	}
	if offset+visible > len(lines) {
		offset = len(lines) - visible
		if offset < 0 {
			offset = 0
		}
	}
	end := offset + visible
	if end > len(lines) {
		end = len(lines)
	}

	// Scroll indicators appear only when a selectable item is out of view.
	firstSelLine, lastSelLine := -1, -1
	for li, l := range lines {
		if l.assetIdx >= 0 {
			if firstSelLine == -1 {
				firstSelLine = li
			}
			lastSelLine = li
		}
	}

	var b strings.Builder
	visLines := lines[offset:end]
	for i, l := range visLines {
		switch {
		case i == 0 && firstSelLine >= 0 && firstSelLine < offset:
			b.WriteString(DimStyle.Render("  ↑") + "\n")
		case i == len(visLines)-1 && lastSelLine >= end:
			b.WriteString(DimStyle.Render("  ↓") + "\n")
		default:
			b.WriteString(l.text + "\n")
		}
	}

	return innerTabBar + "\n" + b.String()
}
