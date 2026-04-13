package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type pickerFocus int

const (
	pickerFocusInput pickerFocus = iota
	pickerFocusList
)

// ProjectPickerModel is a modal directory browser for selecting a project path.
type ProjectPickerModel struct {
	Input      textinput.Model
	CurrentDir string
	Entries    []string // ".." + non-hidden subdirectory names
	Cursor     int
	Focus      pickerFocus
	Width      int
	Height     int
	keys       KeyMap
}

// NewProjectPickerModel creates a picker starting at startDir (defaults to CWD).
func NewProjectPickerModel(startDir string) ProjectPickerModel {
	if startDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			startDir = home
		} else if cwd, err := os.Getwd(); err == nil {
			startDir = cwd
		} else {
			startDir = "/"
		}
	}
	startDir = filepath.Clean(startDir)

	inp := textinput.New()
	inp.Placeholder = "/path/to/project"
	inp.CharLimit = 512
	inp.SetValue(startDir)
	inp.Focus()
	inp.CursorEnd()

	m := ProjectPickerModel{
		Input: inp,
		Focus: pickerFocusInput,
		keys:  DefaultKeyMap(),
	}
	m.reloadEntries(startDir)
	return m
}

func (m *ProjectPickerModel) reloadEntries(dir string) {
	m.CurrentDir = dir
	m.Entries = listSubdirs(dir)
	if m.Cursor >= len(m.Entries) {
		m.Cursor = 0
	}
}

// listSubdirs returns ".." (unless at root) followed by non-hidden subdirectories.
func listSubdirs(dir string) []string {
	var result []string
	if filepath.Clean(dir) != "/" {
		result = append(result, "..")
	}
	infos, err := os.ReadDir(dir)
	if err != nil {
		return result
	}
	for _, info := range infos {
		if info.IsDir() && !strings.HasPrefix(info.Name(), ".") {
			result = append(result, info.Name())
		}
	}
	return result
}

func isDirPath(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func (m ProjectPickerModel) Update(msg tea.Msg) (ProjectPickerModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if key.Matches(msg, m.keys.Back) {
			return m, func() tea.Msg { return ProjectPickerCancelMsg{} }
		}
		if m.Focus == pickerFocusInput {
			return m.updateInput(msg)
		}
		return m.updateList(msg)
	}
	return m, nil
}

func (m ProjectPickerModel) updateInput(msg tea.KeyMsg) (ProjectPickerModel, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Down):
		if len(m.Entries) > 0 {
			m.Focus = pickerFocusList
			m.Cursor = 0
			m.Input.Blur()
		}
		return m, nil

	case key.Matches(msg, m.keys.Select):
		path := filepath.Clean(m.Input.Value())
		if isDirPath(path) {
			return m, func() tea.Msg { return ProjectPickerConfirmMsg{Path: path} }
		}
		return m, nil

	default:
		var cmd tea.Cmd
		m.Input, cmd = m.Input.Update(msg)
		candidate := filepath.Clean(m.Input.Value())
		if isDirPath(candidate) {
			m.reloadEntries(candidate)
		} else {
			parent := filepath.Dir(candidate)
			if isDirPath(parent) {
				m.reloadEntries(parent)
			}
		}
		return m, cmd
	}
}

func (m ProjectPickerModel) updateList(msg tea.KeyMsg) (ProjectPickerModel, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Up):
		if m.Cursor > 0 {
			m.Cursor--
		} else {
			// Return focus to input
			m.Focus = pickerFocusInput
			m.Input.Focus()
			m.Input.CursorEnd()
		}
	case key.Matches(msg, m.keys.Down):
		if m.Cursor < len(m.Entries)-1 {
			m.Cursor++
		}
	case key.Matches(msg, m.keys.Select):
		if m.Cursor < len(m.Entries) {
			entry := m.Entries[m.Cursor]
			var newDir string
			if entry == ".." {
				newDir = filepath.Dir(m.CurrentDir)
			} else {
				newDir = filepath.Join(m.CurrentDir, entry)
			}
			newDir = filepath.Clean(newDir)
			m.reloadEntries(newDir)
			m.Input.SetValue(newDir)
			m.Input.CursorEnd()
			// Return focus to input so Enter immediately confirms
			m.Focus = pickerFocusInput
			m.Input.Focus()
		}
	}
	return m, nil
}

func (m ProjectPickerModel) View() string {
	innerWidth := m.Width - 6
	if innerWidth < 20 {
		innerWidth = 20
	}
	visibleRows := m.Height - 12
	if visibleRows < 4 {
		visibleRows = 4
	}

	var b strings.Builder
	b.WriteString(TitleStyle.Render("Add Project"))
	b.WriteString("\n\n")

	// Path input with focus-sensitive border
	var inputBorder lipgloss.Style
	if m.Focus == pickerFocusInput {
		inputBorder = ActiveBorderStyle.Width(innerWidth)
	} else {
		inputBorder = InactiveBorderStyle.Width(innerWidth)
	}
	b.WriteString(inputBorder.Render(m.Input.View()))
	b.WriteString("\n\n")

	// Directory listing
	if len(m.Entries) == 0 {
		b.WriteString(DimStyle.Render("  (no subdirectories)"))
	} else {
		start := 0
		if m.Focus == pickerFocusList && m.Cursor >= visibleRows {
			start = m.Cursor - visibleRows + 1
		}
		end := start + visibleRows
		if end > len(m.Entries) {
			end = len(m.Entries)
		}

		for i := start; i < end; i++ {
			entry := m.Entries[i]
			display := entry + "/"
			if entry == ".." {
				display = "../"
			}

			if m.Focus == pickerFocusList && i == m.Cursor {
				b.WriteString(SelectedStyle.Render("> " + display))
			} else if entry == ".." {
				b.WriteString(DimStyle.Render("  " + display))
			} else {
				b.WriteString(NormalStyle.Render("  " + display))
			}
			b.WriteString("\n")
		}

		if len(m.Entries) > end {
			b.WriteString(DimStyle.Render(fmt.Sprintf("  … +%d more", len(m.Entries)-end)))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	if m.Focus == pickerFocusInput {
		b.WriteString(DimStyle.Render("  enter:confirm | ↓:browse subdirs | esc:cancel"))
	} else {
		b.WriteString(DimStyle.Render("  enter:open dir → back to input | ↑:back to input | esc:cancel"))
	}

	return b.String()
}
