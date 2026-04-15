package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	githubOwner = "zenk-t-suzuki"
	githubRepo  = "AgentsBuilder"
	apiURL      = "https://api.github.com/repos/" + githubOwner + "/" + githubRepo + "/releases/latest"
)

// Release holds the fields from GitHub's release API that we care about.
type Release struct {
	TagName string `json:"tag_name"`
}

// CheckLatest fetches the latest release from GitHub.
// Returns (nil, nil) when the check cannot be completed (e.g. network unavailable).
func CheckLatest() (*Release, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("parsing release JSON: %w", err)
	}
	if rel.TagName == "" {
		return nil, fmt.Errorf("release tag is empty")
	}
	return &rel, nil
}

// IsNewer reports whether latest is strictly newer than current.
// Both strings should be in the form "vMAJOR.MINOR.PATCH".
// Non-numeric or missing segments are treated as 0.
func IsNewer(latest, current string) bool {
	l := parseSemver(latest)
	c := parseSemver(current)
	for i := range l {
		if l[i] > c[i] {
			return true
		}
		if l[i] < c[i] {
			return false
		}
	}
	return false
}

func parseSemver(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	var out [3]int
	for i, p := range parts {
		if i >= 3 {
			break
		}
		n, _ := strconv.Atoi(p)
		out[i] = n
	}
	return out
}

// AssetURL returns the download URL for the current platform's binary.
// Only linux/amd64 and linux/arm64 are supported (matching install.sh).
func AssetURL(tag string) (string, error) {
	arch := runtime.GOARCH
	switch arch {
	case "amd64", "arm64":
	default:
		return "", fmt.Errorf("unsupported architecture: %s", arch)
	}
	return fmt.Sprintf(
		"https://github.com/%s/%s/releases/download/%s/agentsbuilder-linux-%s",
		githubOwner, githubRepo, tag, arch,
	), nil
}

// DownloadAndReplace downloads the binary from url and atomically replaces
// the currently-running executable.  Returns an error if the download or the
// replacement fails (e.g. insufficient permissions).
func DownloadAndReplace(url string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine executable path: %w", err)
	}

	// Download to a sibling temp file so os.Rename stays on the same filesystem.
	tmpPath := exePath + ".update"
	if err := downloadFile(url, tmpPath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	if err := os.Chmod(tmpPath, 0o755); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("chmod failed: %w", err)
	}

	if err := os.Rename(tmpPath, exePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace failed (try running with sudo): %w", err)
	}

	return nil
}

func downloadFile(url, destPath string) error {
	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}
