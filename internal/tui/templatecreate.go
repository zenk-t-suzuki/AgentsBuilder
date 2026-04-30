package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"agentsbuilder/internal/assetdef"
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
	ItemName   string // for embedded items: the specific entry name (e.g. MCP server name)
	Embedded   bool   // true when this item is embedded in a shared config file
}

// SelectKey returns a unique key for this item, used for dedup and selection tracking.
// For embedded items sharing a config file, the key includes the item name.
func (t TmplItem) SelectKey() string {
	if t.Embedded {
		return t.SrcPath + "\t" + t.ItemName
	}
	return t.SrcPath
}

// isEmbeddedItem returns true for asset types where individual items are
// stored inside a shared configuration file rather than as separate files.
func isEmbeddedItem(assetType model.AssetType, provider model.Provider) bool {
	return assetdef.IsEmbedded(provider, assetType)
}

// TmplSelectKey computes the selection key for an item in the browse view.
// This must produce the same key as TmplItem.SelectKey() for proper highlight sync.
func TmplSelectKey(assetType model.AssetType, provider model.Provider, filePath, itemName string) string {
	if isEmbeddedItem(assetType, provider) && itemName != "" {
		return filePath + "\t" + itemName
	}
	return filePath
}

// sanitizeFilename replaces characters that are unsafe in filenames.
func sanitizeFilename(name string) string {
	r := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", " ", "_", "@", "_at_",
	)
	return r.Replace(name)
}

// embeddedFilename generates a synthetic filename for an embedded item's extracted config.
func embeddedFilename(assetType model.AssetType, provider model.Provider, itemName string) string {
	safe := sanitizeFilename(itemName)
	switch assetType {
	case model.MCP:
		if provider == model.Codex {
			return fmt.Sprintf("mcp_%s.toml", safe)
		}
		return fmt.Sprintf("mcp_%s.json", safe)
	case model.Plugins:
		return fmt.Sprintf("plugin_%s.json", safe)
	case model.Hooks:
		return fmt.Sprintf("hook_%s.json", safe)
	}
	return fmt.Sprintf("item_%s.json", safe)
}

// tmplDestRelDir returns the relative destination directory for an asset type
// and provider when the template is applied to a project root.
func tmplDestRelDir(assetType model.AssetType, provider model.Provider) string {
	return assetdef.DestDir(provider, assetType)
}

// MakeTmplItem constructs a TmplItem from an asset and optional sub-item.
// scopeLabel is used in the display label ("Global" or project name).
// Returns nil when the source path cannot be determined.
func MakeTmplItem(asset model.Asset, item *model.AssetItem, scopeLabel string) *TmplItem {
	// For embedded items (MCP servers, plugins in shared config files),
	// create a synthetic file entry so each item can be selected individually.
	if item != nil && isEmbeddedItem(asset.Type, asset.Provider) {
		if item.FilePath == "" || item.Name == "" {
			return nil
		}
		filename := embeddedFilename(asset.Type, asset.Provider, item.Name)
		label := fmt.Sprintf("%s / %s  [%s · %s]",
			asset.Type.String(), item.Name, asset.Provider.String(), scopeLabel)

		return &TmplItem{
			SrcPath:    item.FilePath,
			DestRelDir: "__merge__",
			Filename:   filename,
			Label:      label,
			AssetType:  asset.Type,
			Provider:   asset.Provider,
			ItemName:   item.Name,
			Embedded:   true,
		}
	}

	// Non-embedded items: use the actual file/directory path.
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
