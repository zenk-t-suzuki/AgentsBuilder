package tui

import (
	"strings"
	"testing"

	"agentsbuilder/internal/model"
)

func TestBuildLines_NoDescriptionInList(t *testing.T) {
	m := NewMainAreaModel()
	m.Width = 80
	m.Height = 20
	m.Focused = true
	m.Assets = []model.Asset{
		{
			Type:     model.Skills,
			Provider: model.Codex,
			Scope:    model.Global,
			FilePath: "/root/.agents/skills",
			Exists:   true,
			Items: []model.AssetItem{
				{Name: "find-skills", Description: "Helps users discover skills", FilePath: "/root/.agents/skills/find-skills"},
				{Name: "other-skill", Description: "", FilePath: "/root/.agents/skills/other-skill"},
			},
		},
	}

	lines := m.buildLines()

	for _, l := range lines {
		if strings.Contains(l.text, "Helps users discover skills") {
			t.Error("description text should not appear in list lines")
		}
	}

	// find-skills and other-skill should each have a selectable line
	itemNames := 0
	for _, l := range lines {
		if l.assetIdx >= 0 {
			itemNames++
		}
	}
	if itemNames != 2 {
		t.Errorf("expected 2 selectable item lines, got %d", itemNames)
	}
}

func TestBuildLines_ProviderLabelFitsWidth(t *testing.T) {
	m := NewMainAreaModel()
	m.Width = 80
	m.Height = 20
	m.Focused = false
	m.Assets = []model.Asset{
		{
			Type:     model.Agents,
			Provider: model.ClaudeCode,
			Scope:    model.Global,
			FilePath: "/root/.claude/agents",
			Exists:   true,
			Items: []model.AssetItem{
				{Name: "my-agent", Description: "An agent", FilePath: "/root/.claude/agents/my-agent.md"},
			},
		},
	}

	lines := m.buildLines()

	for _, l := range lines {
		if l.assetIdx < 0 {
			continue
		}
		// Strip ANSI escape codes and count runes (not bytes) for visual width.
		// "●" is 1 rune = 1 terminal column but 3 UTF-8 bytes, so rune count is correct here.
		visual := stripANSI(l.text)
		runeWidth := len([]rune(visual))
		if runeWidth > m.Width {
			t.Errorf("line rune-width %d exceeds container width %d: %q", runeWidth, m.Width, visual)
		}
	}
}

// TestBuildLines_MissingAssetHidden verifies that a missing asset with no items
// does not appear as a selectable row (only a dim "—" placeholder is shown).
func TestBuildLines_MissingAssetHidden(t *testing.T) {
	m := NewMainAreaModel()
	m.Width = 80
	m.Height = 20
	m.Focused = false
	m.Assets = []model.Asset{
		{
			Type:     model.Skills,
			Provider: model.ClaudeCode,
			Scope:    model.Global,
			FilePath: "/root/.claude/commands",
			Exists:   false,
			Items:    nil,
		},
	}

	lines := m.buildLines()

	// Path must not appear as a selectable row
	for _, l := range lines {
		if l.assetIdx >= 0 && strings.Contains(stripANSI(l.text), "/root/.claude/commands") {
			t.Error("missing asset path should not appear as a selectable row")
		}
	}

	// "—" placeholder should be shown
	found := false
	for _, l := range lines {
		if strings.Contains(stripANSI(l.text), "—") {
			found = true
		}
	}
	if !found {
		t.Error("expected '—' placeholder for section with no visible assets")
	}
}

// TestBuildLines_ExistingEmptyDirShown verifies that a directory that exists
// but has no items still shows its path as a selectable row.
func TestBuildLines_ExistingEmptyDirShown(t *testing.T) {
	m := NewMainAreaModel()
	m.Width = 80
	m.Height = 20
	m.Focused = false
	m.Assets = []model.Asset{
		{
			Type:     model.Agents,
			Provider: model.ClaudeCode,
			Scope:    model.Global,
			FilePath: "/root/.claude/agents",
			Exists:   true, // exists but empty
			Items:    nil,
		},
	}

	lines := m.buildLines()
	found := false
	for _, l := range lines {
		if l.assetIdx >= 0 && strings.Contains(stripANSI(l.text), "/root/.claude/agents") {
			found = true
		}
	}
	if !found {
		t.Error("existing empty directory should show its path as a selectable row")
	}
}

// stripANSI removes ANSI escape sequences for visual width measurement in tests.
func stripANSI(s string) string {
	var out strings.Builder
	inEscape := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inEscape = true
		case inEscape && r == 'm':
			inEscape = false
		case !inEscape:
			out.WriteRune(r)
		}
	}
	return out.String()
}
