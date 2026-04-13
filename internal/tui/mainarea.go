package tui

import (
	"fmt"
	"strings"

	"agentsbuilder/internal/model"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
)

// MainAreaModel displays assets grouped by type for the selected scope.
type MainAreaModel struct {
	Assets             []model.Asset
	GlobalAssets       []model.Asset  // non-nil when in project scope: shows inherited global config
	Diffs              []model.DiffResult
	Focused            bool
	Width              int
	Height             int // visible rows for content (set by updateLayout)
	SelectedAssetIndex int // index into the flat selectables list

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
// Assets that are missing AND have no items are hidden — showing a missing
// directory path provides no useful information to the user.
func isVisible(a model.Asset) bool {
	return a.Exists || len(a.Items) > 0
}

// computeSelectables returns the flat list of selectable entries.
// Missing assets with no items are excluded.
// Global assets (when present) are appended after project assets.
func (m MainAreaModel) computeSelectables() []selectableItem {
	var sel []selectableItem
	appendAssets := func(assets []model.Asset, global bool) {
		for ai, asset := range assets {
			if !isVisible(asset) {
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
		sel := m.computeSelectables()
		switch {
		case key.Matches(msg, m.keys.Up):
			if m.SelectedAssetIndex > 0 {
				m.SelectedAssetIndex--
			}
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
// No background — selected items are distinguished by white text,
// normal items by the default grey (NormalStyle).
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

// buildLines renders all assets into a flat list of lines, grouping by AssetType.
//
// Visibility rules:
//   - Assets with items: show each item by name.
//   - Assets that exist but have no items: show the directory/file path.
//   - Missing assets with no items: hidden (not shown).
//
// If every asset under a section type is hidden, a dim "—" placeholder is shown.
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

	// renderAssets renders one slice of assets into lines.
	// global=true uses GlobalAssets as the source set for selectable lookup.
	renderAssets := func(assets []model.Asset, global bool) {
		currentType := model.AssetType(-1)

		typeHasVisible := make(map[model.AssetType]bool)
		for _, asset := range assets {
			if isVisible(asset) {
				typeHasVisible[asset.Type] = true
			}
		}

		for ai, asset := range assets {
			if asset.Type != currentType {
				currentType = asset.Type
				lines = append(lines, assetLine{text: "", assetIdx: -1})
				lines = append(lines, assetLine{
					text:     SectionHeaderStyle.Render(asset.Type.String()),
					assetIdx: -1,
				})
				if !typeHasVisible[currentType] {
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
					isSelected := dispIdx == m.SelectedAssetIndex && m.Focused

					visibleLeft := fmt.Sprintf("    . %s", item.Name)

					var rendered string
					if isSelected {
						// Use plain text only — inner ANSI resets (from ActiveIndicator)
						// cancel the outer white foreground, so we avoid them entirely.
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
				isSelected := dispIdx == m.SelectedAssetIndex && m.Focused

				// Build two variants of diffMark: styled (for normal rows) and
				// plain (for selected rows, to avoid ANSI resets).
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

				visibleLeft := fmt.Sprintf("    . %s%s", diffMarkPlain, asset.FilePath)

				var rendered string
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
		// Separator between project and global sections.
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
	if len(m.Assets) == 0 {
		return DimStyle.Render("\n  No assets found.\n  Select a scope from the sidebar to browse.\n  Press 4 (or t) to create from template.")
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

	// Scroll so cursor stays centred in the visible window.
	visible := m.Height
	if visible <= 0 {
		visible = 10
	}
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

	// Find the line range that actually contains selectables.
	// Scroll indicators should only appear when a selectable item is out of view,
	// not merely because non-selectable header/blank rows are off-screen.
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
			// A selectable item is above the visible window.
			b.WriteString(DimStyle.Render("  ↑") + "\n")
		case i == len(visLines)-1 && lastSelLine >= end:
			// A selectable item is below the visible window.
			b.WriteString(DimStyle.Render("  ↓") + "\n")
		default:
			b.WriteString(l.text + "\n")
		}
	}
	return b.String()
}
