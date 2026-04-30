package marketplace

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CacheRoot is overridden in tests via SetCacheRoot.
var cacheRoot string

// SetCacheRoot overrides the cache directory. Production code calls
// config.MarketplaceCacheDir() and passes the result here.
func SetCacheRoot(dir string) { cacheRoot = dir }

// CacheRoot returns the configured cache root. Empty if SetCacheRoot has not
// been called — callers should check before invoking Sync.
func CacheRoot() string { return cacheRoot }

// Sync fetches the marketplace described by src into the local cache and
// returns the on-disk path containing `.claude-plugin/marketplace.json` (for
// dir/git/local-dir sources) or the manifest file directly (for *json sources).
//
// For git sources the cache is `<root>/<source.CacheKey>` and the repo is
// cloned shallowly with --branch when a ref is set. Subsequent calls run
// `git pull --ff-only` if the working tree exists.
//
// For local-dir sources, no copy is made — the path is returned as-is.
// For local-json sources, the path is returned as-is.
// For remote-json sources, the JSON is fetched into the cache.
func Sync(src Source) (string, error) {
	if cacheRoot == "" {
		return "", errors.New("marketplace cache root not configured")
	}

	switch src.Kind {
	case SourceLocalDir:
		abs, err := filepath.Abs(src.Path)
		if err != nil {
			return "", err
		}
		if _, err := FindManifest(abs); err != nil {
			return "", err
		}
		return abs, nil

	case SourceLocalJSON:
		abs, err := filepath.Abs(src.Path)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(abs); err != nil {
			return "", fmt.Errorf("marketplace.json not found at %s: %w", abs, err)
		}
		return abs, nil

	case SourceRemoteJSON:
		dest := filepath.Join(cacheRoot, src.CacheKey()+".json")
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return "", err
		}
		if err := fetchJSON(src.URL, dest); err != nil {
			return "", err
		}
		return dest, nil

	case SourceGit:
		dest := filepath.Join(cacheRoot, src.CacheKey())
		if err := syncGit(src, dest); err != nil {
			return "", err
		}
		if _, err := FindManifest(dest); err != nil {
			return "", err
		}
		return dest, nil
	}
	return "", fmt.Errorf("unsupported source kind: %v", src.Kind)
}

// RemoveCache deletes a marketplace's cached working tree (or fetched JSON)
// for the given source.
func RemoveCache(src Source) error {
	if cacheRoot == "" {
		return errors.New("marketplace cache root not configured")
	}
	target := filepath.Join(cacheRoot, src.CacheKey())
	_ = os.RemoveAll(target)
	_ = os.Remove(target + ".json")
	return nil
}

// syncGit clones the repository to dest if absent, otherwise pulls --ff-only.
// When src.Ref is set, the clone uses --branch and the pull is run on that
// branch (other branches are not fetched, matching Claude Code's behavior of
// pinning to a specific ref).
func syncGit(src Source, dest string) error {
	if isGitRepo(dest) {
		if !hasCommits(dest) {
			return nil
		}
		args := []string{"-C", dest, "pull", "--ff-only", "-q"}
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			return friendlyGitError("pull", string(out), err)
		}
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("creating cache parent: %w", err)
	}

	args := []string{"clone", "--depth=1", "-q"}
	if src.Ref != "" {
		args = append(args, "--branch", src.Ref)
	}
	args = append(args, src.URL, dest)
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.RemoveAll(dest)
		return friendlyGitError("clone", string(out), err)
	}
	return nil
}

// fetchJSON GETs url into dest, creating the parent directory and overwriting
// any existing file. A short timeout protects against hung remotes.
func fetchJSON(url, dest string) error {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("fetching %s: HTTP %d", url, resp.StatusCode)
	}

	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

func isGitRepo(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info.IsDir()
}

func hasCommits(dir string) bool {
	return exec.Command("git", "-C", dir, "rev-parse", "HEAD").Run() == nil
}

// friendlyGitError converts raw git output into a Japanese, user-facing error
// for common failure modes. Falls back to the raw output otherwise.
func friendlyGitError(op, output string, err error) error {
	low := strings.ToLower(output)
	switch {
	case strings.Contains(low, "repository not found"), strings.Contains(low, "not found"):
		return fmt.Errorf("リポジトリが見つかりません。URLとアクセス権を確認してください")
	case strings.Contains(low, "could not resolve host"):
		return fmt.Errorf("ネットワークに接続できません。インターネット接続を確認してください")
	case strings.Contains(low, "authentication"), strings.Contains(low, "permission denied"),
		strings.Contains(low, "publickey"):
		return fmt.Errorf("認証に失敗しました。SSH鍵またはアクセストークンを確認してください")
	case strings.Contains(low, "timeout"), strings.Contains(low, "timed out"):
		return fmt.Errorf("接続がタイムアウトしました。ネットワーク状況を確認してください")
	case strings.Contains(low, "remote branch"), strings.Contains(low, "couldn't find"):
		return fmt.Errorf("指定したブランチ/タグが見つかりません: %s", strings.TrimSpace(output))
	}
	if trimmed := strings.TrimSpace(output); trimmed != "" {
		return fmt.Errorf("git %s に失敗しました: %s", op, trimmed)
	}
	return fmt.Errorf("git %s に失敗しました: %w", op, err)
}
