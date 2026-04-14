package tui

import (
	"fmt"
	"path/filepath"

	"agentsbuilder/internal/model"
)

// TemplateCreateStep tracks the phase of the template-creation wizard.
type TemplateCreateStep int

const (
	TmplStepBrowse TemplateCreateStep = iota // selecting assets in the Browse view
	TmplStepReview                           // reviewing the selection
	TmplStepName                             // entering the template name
)

// TmplItem holds one asset file chosen during template creation.
type TmplItem struct {
	SrcPath    string          // absolute source file path
	DestRelDir string          // relative destination dir for project scope (e.g. ".claude/commands")
	Filename   string          // filename at destination
	Label      string          // human-readable display label
	AssetType  model.AssetType
	Provider   model.Provider
}

// tmplDestRelDir returns the relative destination directory for an asset type
// and provider when the template is applied to a project root.
func tmplDestRelDir(assetType model.AssetType, provider model.Provider) string {
	switch provider {
	case model.ClaudeCode:
		switch assetType {
		case model.Skills:
			return ".claude/commands"
		case model.Agents:
			return ".claude/agents"
		case model.MCP:
			return ".claude"
		default: // AgentsMD, ClaudeMD → project root
			return ""
		}
	case model.Codex:
		switch assetType {
		case model.Skills:
			return ".codex/skills"
		case model.Agents:
			return ".codex/agents"
		case model.MCP:
			return ".codex"
		case model.ClaudeMD:
			return ".codex"
		default: // AgentsMD → project root
			return ""
		}
	}
	return ""
}

// MakeTmplItem constructs a TmplItem from an asset and optional sub-item.
// scopeLabel is used in the display label ("Global" or project name).
// Returns nil when the source path cannot be determined.
func MakeTmplItem(asset model.Asset, item *model.AssetItem, scopeLabel string) *TmplItem {
	var srcPath, filename string
	if item != nil && item.FilePath != "" {
		srcPath = item.FilePath
		filename = filepath.Base(srcPath)
	} else if asset.FilePath != "" {
		srcPath = asset.FilePath
		filename = filepath.Base(srcPath)
	}
	if srcPath == "" || filename == "" || filename == "." {
		return nil
	}

	destRelDir := tmplDestRelDir(asset.Type, asset.Provider)
	label := fmt.Sprintf("%s / %s  [%s · %s]",
		asset.Type.String(), filename, asset.Provider.String(), scopeLabel)

	return &TmplItem{
		SrcPath:    srcPath,
		DestRelDir: destRelDir,
		Filename:   filename,
		Label:      label,
		AssetType:  asset.Type,
		Provider:   asset.Provider,
	}
}
