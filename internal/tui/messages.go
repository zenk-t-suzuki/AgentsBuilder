package tui

import (
	"agentsbuilder/internal/marketplace"
	"agentsbuilder/internal/model"
)

// ScopeSelectedMsg is sent when the user selects a scope in the sidebar.
type ScopeSelectedMsg struct {
	Scope   model.Scope
	Project *model.ProjectInfo // nil when scope is Global
}

// AssetSelectedMsg is sent when the user selects an asset or item in the main area.
type AssetSelectedMsg struct {
	Asset *model.Asset     // nil to clear selection
	Item  *model.AssetItem // nil when selecting at the asset/directory level
}

// ProjectAddedMsg is sent when a new project is registered.
type ProjectAddedMsg struct {
	Project model.ProjectInfo
}

// ProjectRemovedMsg is sent when a project is removed.
type ProjectRemovedMsg struct {
	Name string
}

// ConfirmDeleteProjectMsg is sent by the sidebar to request a delete confirmation.
type ConfirmDeleteProjectMsg struct {
	Name string
}

// OpenProjectPickerMsg is sent by the sidebar to open the directory picker modal.
type OpenProjectPickerMsg struct{}

// ProjectPickerConfirmMsg is sent when the user confirms a directory.
type ProjectPickerConfirmMsg struct {
	Path string
}

// ProjectPickerCancelMsg is sent when the user cancels the picker.
type ProjectPickerCancelMsg struct{}

// TemplateAppliedMsg is sent after a template has been applied.
type TemplateAppliedMsg struct {
	Template model.Template
}

// RefreshMsg triggers a full re-scan and UI refresh.
type RefreshMsg struct{}

// ExitTemplateModeMsg returns the Template tab to the Browse tab.
type ExitTemplateModeMsg struct{}

// ---------------------------------------------------------------------------
// Marketplace messages
// ---------------------------------------------------------------------------

// MarketplaceAddedMsg is sent when the user submits a new source. The handler
// must parse the source, sync it, read the manifest, and persist the resulting
// MarketplaceInfo.
type MarketplaceAddedMsg struct {
	Source string
}

// MarketplaceRemovedMsg requests removal of a marketplace by name (and cache).
type MarketplaceRemovedMsg struct {
	Name string
}

// MarketplaceSyncMsg requests a sync. Empty Name means sync all.
type MarketplaceSyncMsg struct {
	Name string
}

// MarketplaceSyncDoneMsg is sent after a sync completes.
type MarketplaceSyncDoneMsg struct {
	Errors map[string]error
}

// MarketplaceOpenMsg requests loading the plugin list for a marketplace and
// switching the UI to the plugin browse mode.
type MarketplaceOpenMsg struct {
	Name string
}

// MarketplaceLoadDoneMsg delivers the loaded plugin list for the named
// marketplace, or an error.
type MarketplaceLoadDoneMsg struct {
	Name    string
	Plugins []marketplace.Plugin
	Err     error
}

// MarketplaceInstallMsg requests installation of a plugin into the chosen
// targets. The handler resolves base paths and invokes marketplace.InstallPlugin.
type MarketplaceInstallMsg struct {
	Plugin  marketplace.Plugin
	Targets []installTargetOption
}

// MarketplaceInstallDoneMsg reports the install outcome.
type MarketplaceInstallDoneMsg struct {
	PluginName string
	Summaries  []marketplace.InstallSummary
	Err        error
}
