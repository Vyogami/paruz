package backend

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/mod/semver"
)

const (
	githubReleaseURL = "https://api.github.com/repos/vyogami/paruz/releases/latest"
	httpTimeout      = 10 * time.Second
	downloadTimeout  = 120 * time.Second
	maxArchiveSize   = 50 * 1024 * 1024  // 50 MB compressed sanity limit
	maxBinarySize    = 200 * 1024 * 1024 // 200 MB uncompressed sanity limit
	userAgent        = "paruz-updater"
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
	AssetName      string
	DownloadURL    string
	ChecksumURL    string
	ReleaseURL     string
	PackageManaged bool // true if binary is managed by pacman
}

// UpdateCheckMsg is sent after a background update check completes.
type UpdateCheckMsg struct {
	Info *UpdateInfo
	Err  error
}

// PreparedUpdate holds a downloaded, verified binary ready to be installed.
type PreparedUpdate struct {
	BinaryPath string      // verified new binary in a writable temp dir
	ExecPath   string      // destination path to replace
	Mode       os.FileMode // permission bits to apply
	NeedsRoot  bool        // true if the destination dir is not user-writable
	tempDir    string      // temp dir to clean up after install
}

// Cleanup removes the temporary download directory.
func (p *PreparedUpdate) Cleanup() {
	if p != nil && p.tempDir != "" {
		_ = os.RemoveAll(p.tempDir)
	}
}

// UpdateDownloadedMsg is sent after the update is downloaded and verified.
// The binary is staged but NOT yet installed (installation may require sudo).
type UpdateDownloadedMsg struct {
	Prepared *PreparedUpdate
	Err      error
}

// UpdateInstalledMsg is sent after the staged binary has been installed.
type UpdateInstalledMsg struct {
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
	// Development / unknown builds (e.g. plain `go build`) carry no real
	// version, so never offer to self-update them.
	if !semver.IsValid(canonicalVersion(currentVersion)) {
		return nil, nil
	}

	client := &http.Client{Timeout: httpTimeout}
	req, err := http.NewRequest(http.MethodGet, githubReleaseURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent+"/"+currentVersion)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
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

	// Check if the current binary is managed by pacman.
	pkgManaged := isPackageManaged()

	info := &UpdateInfo{
		CurrentVersion: currentVersion,
		LatestVersion:  release.TagName,
		ReleaseURL:     release.HTMLURL,
		PackageManaged: pkgManaged,
	}

	// Package-managed binaries are updated via pacman/paru, so we don't need
	// to resolve a downloadable asset for them.
	if pkgManaged {
		return info, nil
	}

	candidates := getAssetCandidates()
	if len(candidates) == 0 {
		return nil, fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	for _, asset := range release.Assets {
		if asset.Name == "checksums.txt" {
			info.ChecksumURL = asset.BrowserDownloadURL
		}
	}

	for _, name := range candidates {
		for _, asset := range release.Assets {
			if asset.Name == name {
				info.AssetName = asset.Name
				info.DownloadURL = asset.BrowserDownloadURL
				break
			}
		}
		if info.DownloadURL != "" {
			break
		}
	}

	if info.DownloadURL == "" {
		return nil, fmt.Errorf("release %s has no asset for %s/%s", release.TagName, runtime.GOOS, runtime.GOARCH)
	}

	return info, nil
}

// getAssetCandidates maps runtime.GOOS/GOARCH to the GoReleaser archive
// name(s), in order of preference. For ARM we can't read GOARM at runtime, so
// prefer the v6 build, which also runs on v7 hardware.
func getAssetCandidates() []string {
	if runtime.GOOS != "linux" {
		return nil
	}
	switch runtime.GOARCH {
	case "amd64":
		return []string{"paruz_linux_x86_64.tar.gz"}
	case "arm64":
		return []string{"paruz_linux_arm64.tar.gz"}
	case "386":
		return []string{"paruz_linux_i386.tar.gz"}
	case "arm":
		return []string{"paruz_linux_v6.tar.gz", "paruz_linux_v7.tar.gz"}
	default:
		return nil
	}
}

// isPackageManaged checks if the running binary is managed by pacman.
func isPackageManaged() bool {
	execPath, err := os.Executable()
	if err != nil {
		return false
	}
	resolved, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		resolved = execPath
	}
	// pacman -Qo returns 0 if the file belongs to a package. Check both the
	// symlink and the resolved target, since a package may own either.
	if exec.Command("pacman", "-Qo", resolved).Run() == nil {
		return true
	}
	if resolved != execPath && exec.Command("pacman", "-Qo", execPath).Run() == nil {
		return true
	}
	return false
}

// canonicalVersion ensures the version has the leading "v" that
// golang.org/x/mod/semver requires.
func canonicalVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return v
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v
}

// isNewer returns true if latest is a newer version than current.
func isNewer(latest, current string) bool {
	l := canonicalVersion(latest)
	c := canonicalVersion(current)
	if !semver.IsValid(l) || !semver.IsValid(c) {
		return false
	}
	return semver.Compare(l, c) > 0
}

// DownloadAndInstallUpdate returns a tea.Cmd that downloads and verifies the
// update into a writable temp dir. The binary is staged (not installed); the
// caller installs it via InstallUpdateCmd, which may require sudo.
func DownloadAndInstallUpdate(info *UpdateInfo) tea.Cmd {
	return func() tea.Msg {
		if info == nil {
			return UpdateDownloadedMsg{Err: fmt.Errorf("no update information available")}
		}
		prepared, err := downloadAndStage(info)
		return UpdateDownloadedMsg{Prepared: prepared, Err: err}
	}
}

// InstallUpdateCmd installs a previously staged binary over the running
// executable, escalating with sudo when the destination is not user-writable.
// It runs as an external process so sudo can prompt for a password in the TUI.
func InstallUpdateCmd(p *PreparedUpdate) tea.Cmd {
	mode := fmt.Sprintf("%o", p.Mode.Perm())
	// Copy the staged binary to a temp file in the destination directory (same
	// filesystem, so the following rename is atomic and never leaves a partial
	// binary), then rename it over the running executable. Paths are passed as
	// positional args ($1..$3) and never interpolated into the script, so there
	// is no shell-quoting or injection risk regardless of the install path.
	const script = `tmp="$3.update.$$"
install -m "$1" "$2" "$tmp" && mv -f "$tmp" "$3" || { rm -f "$tmp"; echo; echo '[Update failed — press any key to return to paruz...]'; read -n 1 -s -r; exit 1; }`
	args := []string{"sh", "-c", script, "sh", mode, p.BinaryPath, p.ExecPath}
	if p.NeedsRoot {
		args = append([]string{"sudo"}, args...)
	}
	cmd := exec.Command(args[0], args[1:]...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		p.Cleanup()
		return UpdateInstalledMsg{Err: err}
	})
}

func downloadAndStage(info *UpdateInfo) (*PreparedUpdate, error) {
	if info.DownloadURL == "" {
		return nil, fmt.Errorf("no download URL available")
	}

	execPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve executable path: %w", err)
	}
	destDir := filepath.Dir(execPath)

	fi, err := os.Stat(execPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat current binary: %w", err)
	}
	originalMode := fi.Mode().Perm()

	// Stage the download in a user-writable temp dir; the destination dir
	// (e.g. /usr/local/bin) may be root-owned, so we never write there directly.
	tempDir, err := os.MkdirTemp("", "paruz-update-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	// Fetch the expected checksum before downloading the archive.
	var expectedSum string
	if info.ChecksumURL != "" && info.AssetName != "" {
		expectedSum, err = fetchChecksum(info.ChecksumURL, info.AssetName)
		if err != nil {
			os.RemoveAll(tempDir)
			return nil, fmt.Errorf("failed to fetch checksum: %w", err)
		}
	}

	client := &http.Client{Timeout: downloadTimeout}

	// Download the archive, capping the compressed size and computing its
	// SHA-256 in the same pass.
	archive, archiveSum, err := downloadArchive(client, info.DownloadURL, tempDir)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, err
	}
	defer os.Remove(archive)

	if expectedSum != "" && !strings.EqualFold(archiveSum, expectedSum) {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("checksum mismatch: archive may be corrupt or tampered with")
	}

	// Extract the paruz binary into the temp dir.
	tmpBinary, err := extractBinary(archive, tempDir, originalMode)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, err
	}

	return &PreparedUpdate{
		BinaryPath: tmpBinary,
		ExecPath:   execPath,
		Mode:       originalMode,
		NeedsRoot:  !dirWritable(destDir),
		tempDir:    tempDir,
	}, nil
}

// dirWritable reports whether the current user can create files in dir.
func dirWritable(dir string) bool {
	probe, err := os.CreateTemp(dir, ".paruz-perm-*")
	if err != nil {
		return false
	}
	name := probe.Name()
	probe.Close()
	os.Remove(name)
	return true
}

// downloadArchive streams the archive to a temp file, enforcing the compressed
// size limit and returning the temp path plus the archive's hex SHA-256.
func downloadArchive(client *http.Client, url, destDir string) (string, string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp(destDir, ".paruz-archive-*")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	hasher := sha256.New()
	// Allow one extra byte so we can detect an oversized archive.
	limited := io.LimitReader(resp.Body, maxArchiveSize+1)
	n, err := io.Copy(tmp, io.TeeReader(limited, hasher))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmpPath)
		return "", "", fmt.Errorf("failed to write archive: %w", err)
	}
	if n > maxArchiveSize {
		os.Remove(tmpPath)
		return "", "", fmt.Errorf("archive exceeds %d byte limit", maxArchiveSize)
	}

	return tmpPath, hex.EncodeToString(hasher.Sum(nil)), nil
}

// extractBinary reads the paruz binary out of the tar.gz archive and writes it
// to a temp file (fsync'd) in destDir, returning the temp path.
func extractBinary(archivePath, destDir string, mode os.FileMode) (string, error) {
	af, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("failed to open archive: %w", err)
	}
	defer af.Close()

	gz, err := gzip.NewReader(af)
	if err != nil {
		return "", fmt.Errorf("failed to decompress: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to read archive: %w", err)
		}
		// Only accept a regular file named exactly "paruz".
		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != "paruz" {
			continue
		}
		if strings.Contains(header.Name, "..") {
			continue
		}
		if header.Size > maxBinarySize {
			return "", fmt.Errorf("binary exceeds %d byte limit", maxBinarySize)
		}

		out, err := os.CreateTemp(destDir, ".paruz-update-*")
		if err != nil {
			return "", fmt.Errorf("failed to create temp file: %w", err)
		}
		tmpPath := out.Name()

		// Cap extraction defensively in case the tar header lies about size.
		limited := io.LimitReader(tr, maxBinarySize+1)
		n, err := io.Copy(out, limited)
		if err == nil && n > maxBinarySize {
			err = fmt.Errorf("binary exceeds %d byte limit", maxBinarySize)
		}
		if err == nil {
			err = out.Chmod(mode)
		}
		if err == nil {
			err = out.Sync()
		}
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(tmpPath)
			return "", fmt.Errorf("failed to write new binary: %w", err)
		}
		return tmpPath, nil
	}

	return "", fmt.Errorf("paruz binary not found in archive")
}

// fetchChecksum downloads a GoReleaser checksums.txt and returns the hex
// SHA-256 recorded for assetName.
func fetchChecksum(url, assetName string) (string, error) {
	client := &http.Client{Timeout: httpTimeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checksums returned status %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(io.LimitReader(resp.Body, 1024*1024))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		if filepath.Base(fields[1]) == assetName {
			return fields[0], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("no checksum entry for %s", assetName)
}
