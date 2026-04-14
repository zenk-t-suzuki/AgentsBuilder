package tui

// MainMode identifies the active operation mode in the main panel.
type MainMode int

const (
	ModeBrowse      MainMode = iota // 1: browse assets
	ModeTemplate                    // 2: apply a template
	ModeMarketplace                 // 3: marketplace
)

var allMainModes = []MainMode{ModeBrowse, ModeTemplate, ModeMarketplace}

// Label returns the display label for the mode.
func (m MainMode) Label() string {
	switch m {
	case ModeBrowse:
		return "Browse"
	case ModeTemplate:
		return "Template"
	case ModeMarketplace:
		return "Marketplace"
	default:
		return ""
	}
}
