package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines the application key bindings.
type KeyMap struct {
	Quit             key.Binding
	SwitchPane       key.Binding
	Up               key.Binding
	Down             key.Binding
	Left             key.Binding
	Right            key.Binding
	CtrlUp           key.Binding // jump to upper element (skip local action)
	CtrlDown         key.Binding // jump to lower element (skip local action)
	CtrlLeft         key.Binding // jump to left element (skip local action)
	CtrlRight        key.Binding // jump to right element (skip local action)
	Select           key.Binding
	Back             key.Binding
	AddProject       key.Binding
	DeleteProject    key.Binding
	Template         key.Binding
	ToggleCheck      key.Binding
	DetailScrollUp   key.Binding // scroll detail panel up
	DetailScrollDown key.Binding // scroll detail panel down
	BrowseTabLeft    key.Binding // prev inner Browse tab
	BrowseTabRight   key.Binding // next inner Browse tab
	CreateTemplate   key.Binding // start template creation wizard
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		SwitchPane: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch pane"),
		),
		Up: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("↓/j", "down"),
		),
		Left: key.NewBinding(
			key.WithKeys("h", "left"),
			key.WithHelp("←/h", "focus sidebar"),
		),
		Right: key.NewBinding(
			key.WithKeys("l", "right"),
			key.WithHelp("→/l", "focus main"),
		),
		CtrlUp: key.NewBinding(
			key.WithKeys("ctrl+up"),
			key.WithHelp("Ctrl+↑", "jump up"),
		),
		CtrlDown: key.NewBinding(
			key.WithKeys("ctrl+down"),
			key.WithHelp("Ctrl+↓", "jump down"),
		),
		CtrlLeft: key.NewBinding(
			key.WithKeys("ctrl+left"),
			key.WithHelp("Ctrl+←", "jump left"),
		),
		CtrlRight: key.NewBinding(
			key.WithKeys("ctrl+right"),
			key.WithHelp("Ctrl+→", "jump right"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		AddProject: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "add project"),
		),
		DeleteProject: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "delete project"),
		),
		Template: key.NewBinding(
			key.WithKeys("t"),
			key.WithHelp("t", "template"),
		),
		ToggleCheck: key.NewBinding(
			key.WithKeys(" "),
			key.WithHelp("space", "toggle"),
		),
		DetailScrollUp: key.NewBinding(
			key.WithKeys("["),
			key.WithHelp("[", "scroll detail up"),
		),
		DetailScrollDown: key.NewBinding(
			key.WithKeys("]"),
			key.WithHelp("]", "scroll detail down"),
		),
		BrowseTabLeft: key.NewBinding(
			key.WithKeys(","),
			key.WithHelp(",", "prev tab"),
		),
		BrowseTabRight: key.NewBinding(
			key.WithKeys("."),
			key.WithHelp(".", "next tab"),
		),
		CreateTemplate: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "new template"),
		),
	}
}

// HelpText returns a short help string.
func HelpText() string {
	return "←/→:pane | ↑/↓:nav | Ctrl+←/→/↑/↓:jump | ,/.:tab | [/]:detail | 1-3:mode | n:tmpl | a:add | d:del | q:quit"
}
