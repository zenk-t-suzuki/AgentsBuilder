package tui

// MainMode identifies the active operation mode in the main panel.
type MainMode int

const (
	ModeBrowse   MainMode = iota // 1: browse assets
	ModeEdit                     // 2: edit an asset
	ModeCreate                   // 3: create an asset
	ModeTemplate                 // 4: apply a template
)

var allMainModes = []MainMode{ModeBrowse, ModeEdit, ModeCreate, ModeTemplate}

// Label returns the display label for the mode.
func (m MainMode) Label() string {
	switch m {
	case ModeBrowse:
		return "Browse"
	case ModeEdit:
		return "Edit"
	case ModeCreate:
		return "Create"
	case ModeTemplate:
		return "Template"
	default:
		return ""
	}
}
