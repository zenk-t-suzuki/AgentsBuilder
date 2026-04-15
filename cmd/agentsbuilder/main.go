package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"agentsbuilder/internal/config"
	"agentsbuilder/internal/template"
	"agentsbuilder/internal/tui"
	"agentsbuilder/internal/updater"

	tea "github.com/charmbracelet/bubbletea"
)

// Version is set at build time via -ldflags "-X main.Version=vX.Y.Z".
// When built with "go build" directly (no ldflags), it remains "dev" and
// the update check is skipped.
var Version = "dev"

func main() {
	// Run the update check before starting the TUI.
	// Skipped for dev builds so that "go run ." / "go build" without ldflags
	// does not hit the network.
	if Version != "dev" {
		runUpdateCheck()
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "設定の読み込みに失敗しました: %v\n", err)
		os.Exit(1)
	}

	// Ensure ~/.agentsbuilder/templates/*default/template.json exists on first run.
	// Errors are non-fatal; built-in predefined templates always work.
	_ = template.EnsureDefaultTemplate()

	appModel := tui.NewAppModel(cfg.ListProjects())
	p := tea.NewProgram(appModel, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// runUpdateCheck checks GitHub for a newer release and, if one is found,
// prompts the user to update.  All errors are handled silently so that a
// network issue never prevents the TUI from starting.
func runUpdateCheck() {
	rel, err := updater.CheckLatest()
	if err != nil || rel == nil {
		// Network unavailable or API error — proceed silently.
		return
	}

	if !updater.IsNewer(rel.TagName, Version) {
		// Already up to date.
		return
	}

	fmt.Printf("新しいバージョンが見つかりました: %s (現在: %s)\n", rel.TagName, Version)
	fmt.Print("今すぐアップデートしますか? [y/N] ")

	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	if strings.ToLower(strings.TrimSpace(answer)) != "y" {
		fmt.Println("スキップしました。")
		return
	}

	url, err := updater.AssetURL(rel.TagName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "アップデートできません: %v\n", err)
		return
	}

	fmt.Printf("%s をダウンロード中...\n", rel.TagName)
	if err := updater.DownloadAndReplace(url); err != nil {
		fmt.Fprintf(os.Stderr, "アップデートに失敗しました: %v\n", err)
		return
	}

	fmt.Println("アップデートが完了しました。agentsbuilder を再起動してください。")
	os.Exit(0)
}
