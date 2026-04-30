package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"agentsbuilder/internal/config"
	"agentsbuilder/internal/model"
)

// templateFile mirrors template.templateFile for JSON parsing.
type templateFile struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Assets      []string `json:"assets"`
	Providers   []string `json:"providers"`
}

// NormalizeURL cleans up a user-provided URL so that common formats work.
//   - Browser URL:  https://github.com/org/repo      → https://github.com/org/repo.git
//   - Clone URL:    https://github.com/org/repo.git   → unchanged
//   - SSH URL:      git@github.com:org/repo.git       → unchanged
//   - Trailing slash removed
func NormalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimRight(raw, "/")

	// SSH URLs — leave as-is
	if strings.Contains(raw, "@") && !strings.Contains(raw, "://") {
		return raw
	}

	// HTTPS GitHub/GitLab browser URLs: add .git if missing
	if strings.Contains(raw, "://") && !strings.HasSuffix(raw, ".git") {
		// Only for known hosts that support this convention
		for _, host := range []string{"github.com", "gitlab.com", "bitbucket.org"} {
			if strings.Contains(raw, host) {
				raw += ".git"
				break
			}
		}
	}

	return raw
}

// Sync clones or pulls a registry repository into the local cache.
// Empty repositories are handled gracefully via git init + remote add.
// Returns the local cache directory path.
func Sync(reg model.RegistryInfo) (string, error) {
	cacheDir, err := registryCachePath(reg.Name)
	if err != nil {
		return "", err
	}

	if isGitRepo(cacheDir) {
		// Already cloned — pull if there are remote commits.
		if hasRemoteCommits(cacheDir) {
			cmd := exec.Command("git", "-C", cacheDir, "pull", "--ff-only", "-q")
			if out, err := cmd.CombinedOutput(); err != nil {
				return "", friendlyError("pull", string(out), err)
			}
		}
		return cacheDir, nil
	}

	if err := os.MkdirAll(filepath.Dir(cacheDir), 0o755); err != nil {
		return "", fmt.Errorf("キャッシュディレクトリの作成に失敗しました: %w", err)
	}

	// Try clone first.
	cmd := exec.Command("git", "clone", "--depth=1", "-q", reg.URL, cacheDir)
	out, cloneErr := cmd.CombinedOutput()
	if cloneErr != nil {
		outStr := strings.ToLower(string(out))
		if strings.Contains(outStr, "empty repository") ||
			strings.Contains(outStr, "warning: you appear to have cloned an empty") {
			// Empty repo — git clone creates a .git dir but no branch.
			// This is fine for us: the cache dir exists and is a valid git repo.
			// We can push to it later.
			return cacheDir, nil
		}
		_ = os.RemoveAll(cacheDir)
		return "", friendlyError("clone", string(out), cloneErr)
	}

	return cacheDir, nil
}

// PublishTemplate copies a local template directory into the registry cache
// and pushes it to the remote. Works even on empty (no-commit) repositories.
func PublishTemplate(reg model.RegistryInfo, tmplName string, srcDir string) error {
	cacheDir, err := registryCachePath(reg.Name)
	if err != nil {
		return err
	}

	if !isGitRepo(cacheDir) {
		// Ensure cache exists first.
		if _, syncErr := Sync(reg); syncErr != nil {
			return syncErr
		}
	}

	// Copy template directory into cache.
	destDir := filepath.Join(cacheDir, tmplName)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("テンプレートディレクトリの作成に失敗しました: %w", err)
	}
	if err := copyDirContents(srcDir, destDir); err != nil {
		return fmt.Errorf("テンプレートのコピーに失敗しました: %w", err)
	}

	// Stage all changes.
	addCmd := exec.Command("git", "-C", cacheDir, "add", "-A")
	if out, err := addCmd.CombinedOutput(); err != nil {
		return friendlyError("add", string(out), err)
	}

	// Check if there's anything to commit.
	statusCmd := exec.Command("git", "-C", cacheDir, "diff", "--cached", "--quiet")
	if statusCmd.Run() == nil {
		// Nothing to commit — template already exists with same content.
		return nil
	}

	// Commit.
	msg := fmt.Sprintf("Add template: %s", tmplName)
	commitCmd := exec.Command("git", "-C", cacheDir, "commit", "-m", msg, "-q")
	if out, err := commitCmd.CombinedOutput(); err != nil {
		return friendlyError("commit", string(out), err)
	}

	// Push. For repos that were cloned empty (no branch yet), we need to
	// explicitly push to the default branch.
	branch := currentBranch(cacheDir)
	if branch == "" {
		branch = "main"
	}
	pushCmd := exec.Command("git", "-C", cacheDir, "push", "-u", "origin", branch, "-q")
	if out, err := pushCmd.CombinedOutput(); err != nil {
		return friendlyPushError(string(out), err)
	}

	return nil
}

// ListLocalTemplates returns the names and paths of user-defined templates
// available locally in ~/.agentsbuilder/templates/ (excluding predefined/default).
func ListLocalTemplates() []LocalTemplate {
	dir, err := config.TemplatesDir()
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var result []LocalTemplate
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "*default" {
			continue
		}
		tmplPath := filepath.Join(dir, entry.Name(), "template.json")
		if _, err := os.Stat(tmplPath); err != nil {
			continue
		}
		result = append(result, LocalTemplate{
			Name: entry.Name(),
			Dir:  filepath.Join(dir, entry.Name()),
		})
	}
	return result
}

// LocalTemplate holds info about a locally saved user template.
type LocalTemplate struct {
	Name string
	Dir  string
}

// SyncResult holds the outcome of a registry sync operation.
type SyncResult struct {
	TemplateCount int // number of templates found after sync
}

// SyncWithResult clones/pulls and returns the sync result with template count.
func SyncWithResult(reg model.RegistryInfo) (*SyncResult, error) {
	_, err := Sync(reg)
	if err != nil {
		return nil, err
	}
	templates := LoadTemplates(reg)
	return &SyncResult{TemplateCount: len(templates)}, nil
}

// SyncAll synchronises all registered registries and returns per-registry errors.
func SyncAll(registries []model.RegistryInfo) map[string]error {
	errs := make(map[string]error)
	for _, reg := range registries {
		if _, err := Sync(reg); err != nil {
			errs[reg.Name] = err
		}
	}
	return errs
}

// LoadTemplates reads all valid templates from a single registry's cache directory.
func LoadTemplates(reg model.RegistryInfo) []model.Template {
	cacheDir, err := registryCachePath(reg.Name)
	if err != nil {
		return nil
	}
	return scanTemplatesIn(cacheDir, reg.Name)
}

// LoadAllTemplates loads templates from all registered registries.
func LoadAllTemplates(registries []model.RegistryInfo) []model.Template {
	var all []model.Template
	for _, reg := range registries {
		all = append(all, LoadTemplates(reg)...)
	}
	return all
}

// TemplateCount returns the number of templates available in a registry's cache.
func TemplateCount(reg model.RegistryInfo) int {
	return len(LoadTemplates(reg))
}

// IsCached returns true if the registry has been cloned locally.
func IsCached(name string) bool {
	dir, err := registryCachePath(name)
	if err != nil {
		return false
	}
	return isGitRepo(dir)
}

// RemoveCache deletes the cached clone for a registry.
func RemoveCache(name string) error {
	cacheDir, err := registryCachePath(name)
	if err != nil {
		return err
	}
	return os.RemoveAll(cacheDir)
}

// registryCachePath returns the cache directory for a named registry.
func registryCachePath(name string) (string, error) {
	base, err := config.RegistryCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, name), nil
}

// isGitRepo checks whether dir is a valid git repository.
func isGitRepo(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info.IsDir()
}

// hasRemoteCommits checks if the repo has any commits on the current branch.
func hasRemoteCommits(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "HEAD")
	return cmd.Run() == nil
}

// currentBranch returns the current branch name, or "" if on a detached HEAD
// or if no commits exist yet.
func currentBranch(dir string) string {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(string(out))
	if branch == "HEAD" {
		return ""
	}
	return branch
}

// copyDirContents recursively copies all files from src to dst.
func copyDirContents(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

// copyFile copies a single file from src to dst.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// friendlyPushError converts git push errors to user-friendly messages.
func friendlyPushError(output string, err error) error {
	out := strings.ToLower(output)
	switch {
	case strings.Contains(out, "permission denied") ||
		strings.Contains(out, "403") ||
		strings.Contains(out, "authentication"):
		return fmt.Errorf("プッシュ権限がありません。リポジトリへの書き込みアクセス権を確認してください")
	case strings.Contains(out, "rejected") ||
		strings.Contains(out, "non-fast-forward"):
		return fmt.Errorf("リモートに新しい変更があります。先に同期(s)してから再度アップロードしてください")
	case strings.Contains(out, "could not resolve host"):
		return fmt.Errorf("ネットワークに接続できません。インターネット接続を確認してください")
	}
	return friendlyError("push", output, err)
}

// friendlyError converts raw git output into a user-friendly Japanese error message.
func friendlyError(op string, output string, err error) error {
	out := strings.ToLower(output)

	switch {
	case strings.Contains(out, "empty repository"):
		return fmt.Errorf("リポジトリが空です。アップロード(p)でテンプレートを追加できます")
	case strings.Contains(out, "repository not found") ||
		strings.Contains(out, "not found"):
		return fmt.Errorf("リポジトリが見つかりません。URLを確認してください。" +
			"プライベートリポジトリの場合はアクセス権があるか確認してください")
	case strings.Contains(out, "could not resolve host"):
		return fmt.Errorf("ネットワークに接続できません。インターネット接続を確認してください")
	case strings.Contains(out, "authentication") ||
		strings.Contains(out, "permission denied") ||
		strings.Contains(out, "publickey"):
		return fmt.Errorf("認証に失敗しました。リポジトリへのアクセス権を確認してください。" +
			"SSH鍵の設定が必要な場合があります")
	case strings.Contains(out, "already exists and is not an empty directory"):
		return fmt.Errorf("キャッシュが壊れています。削除してから再度同期してください")
	case strings.Contains(out, "timeout") || strings.Contains(out, "timed out"):
		return fmt.Errorf("接続がタイムアウトしました。ネットワーク状況を確認してください")
	}

	// Fallback: include the raw output but with a prefix
	trimmed := strings.TrimSpace(output)
	if trimmed != "" {
		return fmt.Errorf("git %s に失敗しました: %s", op, trimmed)
	}
	return fmt.Errorf("git %s に失敗しました: %w", op, err)
}

// scanTemplatesIn walks a directory looking for subdirectories containing template.json.
func scanTemplatesIn(dir string, registryName string) []model.Template {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var templates []model.Template
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		tmplDir := filepath.Join(dir, entry.Name())
		tmplPath := filepath.Join(tmplDir, "template.json")
		data, err := os.ReadFile(tmplPath)
		if err != nil {
			continue
		}
		var f templateFile
		if err := json.Unmarshal(data, &f); err != nil {
			continue
		}
		tmpl, err := convertTemplate(f)
		if err != nil {
			continue
		}
		tmpl.RegistryName = registryName
		tmpl.TemplateDir = tmplDir
		templates = append(templates, tmpl)
	}
	return templates
}

// convertTemplate converts the on-disk JSON to a model.Template.
func convertTemplate(f templateFile) (model.Template, error) {
	if f.Name == "" {
		return model.Template{}, fmt.Errorf("name is empty")
	}

	var assets []model.AssetType
	for _, a := range f.Assets {
		at, ok := model.ParseAssetType(a)
		if !ok {
			continue
		}
		assets = append(assets, at)
	}

	var providers []model.Provider
	for _, p := range f.Providers {
		pv, ok := model.ParseProvider(p)
		if !ok {
			continue
		}
		providers = append(providers, pv)
	}

	return model.Template{
		Name:        f.Name,
		Description: f.Description,
		Assets:      assets,
		Providers:   providers,
	}, nil
}
