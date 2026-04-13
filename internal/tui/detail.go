package tui

import (
	"fmt"
	"strings"

	"agentsbuilder/internal/model"
)

// DetailModel shows details of the currently selected asset or item.
type DetailModel struct {
	Asset        *model.Asset
	Item         *model.AssetItem // nil when a directory-level asset is selected
	Diff         *model.DiffResult
	Width        int
	Height       int
	ScrollOffset int
}

// NewDetailModel creates a new detail model.
func NewDetailModel() DetailModel {
	return DetailModel{}
}

// ScrollDown moves the scroll offset down by delta lines (clamped to content size).
func (m *DetailModel) ScrollDown(delta int) {
	m.ScrollOffset += delta
}

// ScrollUp moves the scroll offset up by delta lines (clamped in View).
func (m *DetailModel) ScrollUp(delta int) {
	m.ScrollOffset -= delta
	if m.ScrollOffset < 0 {
		m.ScrollOffset = 0
	}
}

func (m DetailModel) View() string {
	if m.Asset == nil {
		return m.applyScroll(DimStyle.Render("\n  Select an asset to view details."))
	}

	var b strings.Builder
	a := m.Asset

	b.WriteString(TitleStyle.Render("Detail"))
	b.WriteString("\n\n")

	row := func(label, value string) {
		b.WriteString(fmt.Sprintf("  %s %s\n",
			LabelStyle.Render(label+":"),
			ValueStyle.Render(value),
		))
	}

	if m.Item != nil {
		// Item-level detail (individual agent, skill, or MCP server)
		row("Name", m.Item.Name)
		if m.Item.Description != "" {
			row("Description", m.Item.Description)
		}
		row("File", m.Item.FilePath)
		row("Type", a.Type.String())
		row("Provider", a.Provider.String())
		row("Scope", a.Scope.String())
	} else {
		// Asset-level detail (directory or config file)
		row("Type", a.Type.String())
		row("Provider", a.Provider.String())
		row("Scope", a.Scope.String())
		row("Path", a.FilePath)

		if a.Exists {
			row("Status", "Exists")
		} else {
			row("Status", "Missing")
		}

		if a.Active {
			row("Active", "Yes (effective)")
		} else {
			row("Active", "No (overridden)")
		}

		if m.Diff != nil && m.Diff.HasDiff {
			b.WriteString("\n")
			b.WriteString(SectionHeaderStyle.Render("Diff / Priority"))
			b.WriteString("\n")
			row("Global", m.Diff.GlobalPath)
			row("Project", m.Diff.ProjectPath)
			row("Priority", m.Diff.Priority.String()+" takes precedence")

			if m.Diff.GlobalExists && m.Diff.ProjectExists {
				b.WriteString(fmt.Sprintf("  %s Both scopes have this asset\n", DiffIndicator))
			}
		}
	}

	return m.applyScroll(b.String())
}

// applyScroll windows the content to the visible height and adds ↑/↓ indicators.
func (m DetailModel) applyScroll(content string) string {
	if m.Height <= 0 {
		return content
	}

	lines := strings.Split(content, "\n")
	total := len(lines)

	if total <= m.Height {
		return content
	}

	// Clamp scroll offset.
	maxOff := total - m.Height
	off := m.ScrollOffset
	if off > maxOff {
		off = maxOff
	}
	if off < 0 {
		off = 0
	}

	visible := make([]string, m.Height)
	copy(visible, lines[off:off+m.Height])

	// Replace first/last lines with scroll indicators when there is overflow.
	if off > 0 {
		visible[0] = DimStyle.Render("  ↑")
	}
	if off < maxOff {
		visible[m.Height-1] = DimStyle.Render("  ↓")
	}

	return strings.Join(visible, "\n")
}

// clipLines truncates content to at most maxLines lines. 0 means no limit.
func clipLines(content string, maxLines int) string {
	if maxLines <= 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return strings.Join(lines, "\n")
}
