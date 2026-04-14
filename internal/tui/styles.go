package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	PrimaryColor   = lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7B78F2"}
	SecondaryColor = lipgloss.AdaptiveColor{Light: "#04B575", Dark: "#04D98B"}
	MutedColor     = lipgloss.AdaptiveColor{Light: "#9B9B9B", Dark: "#626262"}
	WarningColor   = lipgloss.AdaptiveColor{Light: "#FF6600", Dark: "#FF9933"}
	ErrorColor     = lipgloss.AdaptiveColor{Light: "#FF0000", Dark: "#FF4444"}

	// Borders
	ActiveBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(PrimaryColor)

	InactiveBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(MutedColor)

	// Title
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(PrimaryColor).
			Padding(0, 1)

	// Selected item (cursor)
	SelectedStyle = lipgloss.NewStyle().
			Foreground(PrimaryColor).
			Bold(true)

	// Active scope in sidebar (persists even when sidebar is unfocused)
	ActiveScopeStyle = lipgloss.NewStyle().
				Foreground(SecondaryColor).
				Bold(true)

	// Normal item
	NormalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#333333", Dark: "#DDDDDD"})

	// Dimmed/inactive
	DimStyle = lipgloss.NewStyle().
			Foreground(MutedColor)

	// Status indicators
	ActiveIndicator   = lipgloss.NewStyle().Foreground(SecondaryColor).Render("●")
	InactiveIndicator = lipgloss.NewStyle().Foreground(MutedColor).Render("○")
	DiffIndicator     = lipgloss.NewStyle().Foreground(WarningColor).Render("△")

	// Tab styles
	ActiveTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(PrimaryColor).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(PrimaryColor).
			Padding(0, 2)

	// FocusedTabStyle is used for the active inner Browse tab when the tab bar
	// itself has keyboard focus (cursor moved up from the top of the list).
	FocusedTabStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#000000")).
				Background(PrimaryColor).
				Border(lipgloss.NormalBorder(), false, false, true, false).
				BorderForeground(PrimaryColor).
				Padding(0, 2)

	InactiveTabStyle = lipgloss.NewStyle().
				Foreground(MutedColor).
				Border(lipgloss.NormalBorder(), false, false, true, false).
				BorderForeground(MutedColor).
				Padding(0, 2)

	// Detail panel labels
	LabelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(PrimaryColor).
			Width(12)

	ValueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#333333", Dark: "#DDDDDD"})

	// Help bar
	HelpStyle = lipgloss.NewStyle().
			Foreground(MutedColor).
			Padding(0, 1)

	// Section header in asset list
	SectionHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(SecondaryColor).
				Padding(0, 0, 0, 1)

	// Checkbox
	CheckedStyle   = lipgloss.NewStyle().Foreground(SecondaryColor).Render("[x]")
	UncheckedStyle = lipgloss.NewStyle().Foreground(MutedColor).Render("[ ]")
)
