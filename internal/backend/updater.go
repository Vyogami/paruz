package backend

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	githubReleaseURL = "https://api.github.com/repos/vyogami/paruz/releases/latest"
	httpTimeout      = 10 * time.Second
	downloadTimeout  = 120 * time.Second
	maxDownloadSize  = 50 * 1024 * 1024 // 50 MB sanity limit
)

// GitHubRelease represents a GitHub release API response.
type GitHubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []GitHubAsset `json:"assets"`
	HTMLURL string        `json:"html_url"`
}

// GitHubAsset represents an asset attached to a GitHub release.
type GitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// UpdateInfo holds metadata about an available update.
type UpdateInfo struct {
	CurrentVersion string
	LatestVersion  string
	DownloadURL    string
	ReleaseURL     string
	PackageManaged bool // true if binary is managed by pacman
}

// UpdateCheckMsg is sent after a background update check completes.
type UpdateCheckMsg struct {
	Info *UpdateInfo
	Err  error
}

// UpdateDownloadedMsg is sent after the update download+install completes.
type UpdateDownloadedMsg struct {
	Err error
}

// CheckForUpdate returns a tea.Cmd that checks GitHub for a newer release.
func CheckForUpdate(currentVersion string) tea.Cmd {
	return func() tea.Msg {
		info, err := checkLatestRelease(currentVersion)
		return UpdateCheckMsg{Info: info, Err: err}
	}
}

func checkLatestRelease(currentVersion string) (*UpdateInfo, error) {
	client := &http.Client{Timeout: httpTimeout}
	req, err := http.NewRequest("GET", githubReleaseURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "paruz-updater/"+currentVersion)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	if release.TagName == "" {
		return nil, nil
	}

	if !isNewer(release.TagName, currentVersion) {
		return nil, nil
	}

	// Check if the current binary is managed by pacman
	pkgManaged := isPackageManaged()

	assetName := getAssetName()
	if assetName == "" && !pkgManaged {
		return nil, fmt.Errorf("unsupported architecture: %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	return &UpdateInfo{
		CurrentVersion: currentVersion,
		LatestVersion:  release.TagName,
		DownloadURL:    downloadURL,
		ReleaseURL:     release.HTMLURL,
		PackageManaged: pkgManaged,
	}, nil
}

// getAssetName maps runtime.GOOS/GOARCH to the GoReleaser archive name.
func getAssetName() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	switch runtime.GOARCH {
	case "amd64":
		return "paruz_linux_x86_64.tar.gz"
	case "arm64":
		return "paruz_linux_arm64.tar.gz"
	case "386":
		return "paruz_linux_i386.tar.gz"
	case "arm":
		return "paruz_linux_v7.tar.gz"
	default:
		return ""
	}
}

// isPackageManaged checks if the running binary is managed by pacman.
func isPackageManaged() bool {
	execPath, err := os.Executable()
	if err != nil {
		return false
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return false
	}
	// pacman -Qo returns 0 if the file belongs to a package
	cmd := exec.Command("pacman", "-Qo", execPath)
	return cmd.Run() == nil
}

// isNewer returns true if latest is a newer version than current.
// Both should be semver-like (optionally prefixed with 'v').
func isNewer(latest, current string) bool {
	lMajor, lMinor, lPatch, lPre := parseVersion(latest)
	cMajor, cMinor, cPatch, cPre := parseVersion(current)

	if lMajor != cMajor {
		return lMajor > cMajor
	}
	if lMinor != cMinor {
		return lMinor > cMinor
	}
	if lPatch != cPatch {
		return lPatch > cPatch
	}
	// If numeric parts are equal: release (no pre-release) > pre-release
	if cPre != "" && lPre == "" {
		return true
	}
	return false
}

// parseVersion parses "v1.2.3-alpha" into (1, 2, 3, "alpha").
func parseVersion(v string) (major, minor, patch int, pre string) {
	v = strings.TrimPrefix(v, "v")
	// Split pre-release: "1.2.3-alpha" -> "1.2.3", "alpha"
	if idx := strings.Index(v, "-"); idx != -1 {
		pre = v[idx+1:]
		v = v[:idx]
	}
	parts := strings.Split(v, ".")
	if len(parts) >= 1 {
		major, _ = strconv.Atoi(parts[0])
	}
	if len(parts) >= 2 {
		minor, _ = strconv.Atoi(parts[1])
	}
	if len(parts) >= 3 {
		patch, _ = strconv.Atoi(parts[2])
	}
	return
}

// DownloadAndInstallUpdate returns a tea.Cmd that downloads and installs the update.
func DownloadAndInstallUpdate(info *UpdateInfo) tea.Cmd {
	return func() tea.Msg {
		err := downloadAndReplace(info.DownloadURL)
		return UpdateDownloadedMsg{Err: err}
	}
}

func downloadAndReplace(downloadURL string) error {
	if downloadURL == "" {
		return fmt.Errorf("no download URL available")
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	// Check the target is writable before downloading
	fi, err := os.Stat(execPath)
	if err != nil {
		return fmt.Errorf("failed to stat current binary: %w", err)
	}
	originalMode := fi.Mode()

	// Download the tar.gz archive
	client := &http.Client{Timeout: downloadTimeout}
	resp, err := client.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	// Enforce size limit
	body := io.LimitReader(resp.Body, maxDownloadSize)

	// Extract the paruz binary from the tar.gz
	gz, err := gzip.NewReader(body)
	if err != nil {
		return fmt.Errorf("failed to decompress: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var binaryData []byte
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read archive: %w", err)
		}
		// Safety: only accept regular files named exactly "paruz"
		baseName := filepath.Base(header.Name)
		if baseName != "paruz" {
			continue
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		// Reject path traversal
		if strings.Contains(header.Name, "..") {
			continue
		}
		binaryData, err = io.ReadAll(tr)
		if err != nil {
			return fmt.Errorf("failed to read binary from archive: %w", err)
		}
		break
	}

	if binaryData == nil {
		return fmt.Errorf("paruz binary not found in archive")
	}

	// Write to a temp file in the same directory (required for atomic rename)
	tmpPath := execPath + ".update-tmp"
	if err := os.WriteFile(tmpPath, binaryData, originalMode.Perm()); err != nil {
		return fmt.Errorf("failed to write new binary: %w", err)
	}

	// Atomic rename replaces the old binary
	if err := os.Rename(tmpPath, execPath); err != nil {
		os.Remove(tmpPath) // cleanup on failure
		return fmt.Errorf("failed to replace binary: %w", err)
	}

	return nil
}
