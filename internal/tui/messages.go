package tui

import "agentsbuilder/internal/model"

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

// EnterTemplateMode switches the main area to template creation UI.
type EnterTemplateModeMsg struct{}

// ExitTemplateMode returns from template creation to normal browsing.
type ExitTemplateModeMsg struct{}
