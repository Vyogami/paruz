package backend

import (
	"bufio"
	"compress/gzip"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sahilm/fuzzy"
	"github.com/vyogami/paruz/internal/models"
)

var (
	packageCache []string
	cacheMutex   sync.RWMutex
	cacheReady   bool
	isRefreshing bool
	waiters      []chan struct{}
)

func getCacheFile() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = "/tmp"
	}
	paruzDir := filepath.Join(cacheDir, "paruz")
	os.MkdirAll(paruzDir, 0755)
	return filepath.Join(paruzDir, "packages.txt")
}

func InitCacheLocal() {
	cacheFile := getCacheFile()
	data, err := os.ReadFile(cacheFile)
	if err == nil {
		cacheMutex.Lock()
		packageCache = strings.Split(string(data), "\n")
		cacheReady = true
		cacheMutex.Unlock()
	}
}

func IsCacheReady() bool {
	cacheMutex.RLock()
	defer cacheMutex.RUnlock()
	return cacheReady
}

// InitCache downloads the AUR package list and gets local packages to allow instant fuzzy search.
func InitCache() {
	doCacheInit(nil)
}

func doCacheInit(done chan struct{}) {
	cacheMutex.Lock()
	if done != nil {
		waiters = append(waiters, done)
	}

	if isRefreshing {
		cacheMutex.Unlock()
		return
	}

	isRefreshing = true
	// We DON'T set cacheReady = false or packageCache = nil here.
	// This allows SearchPackages to continue using the old cache until the new one is ready.
	cacheMutex.Unlock()

	go func() {
		// 1. Get Repo packages
		cmd := exec.Command("pacman", "-Slq")
		out, _ := cmd.Output()
		repos := strings.Split(string(out), "\n")

		// 2. Get AUR packages
		resp, err := http.Get("https://aur.archlinux.org/packages.gz")
		var aur []string
		if err == nil {
			defer resp.Body.Close()
			gz, err := gzip.NewReader(resp.Body)
			if err == nil {
				defer gz.Close()
				scanner := bufio.NewScanner(gz)
				for scanner.Scan() {
					aur = append(aur, scanner.Text())
				}
			}
		}

		var combined []string
		// Combine and filter empties
		for _, r := range repos {
			if r != "" {
				combined = append(combined, r)
			}
		}
		for _, a := range aur {
			if a != "" {
				combined = append(combined, a)
			}
		}

		cacheMutex.Lock()
		packageCache = combined
		cacheReady = true
		isRefreshing = false
		for _, w := range waiters {
			close(w)
		}
		waiters = nil
		cacheMutex.Unlock()

		// Write to disk cache
		os.WriteFile(getCacheFile(), []byte(strings.Join(combined, "\n")), 0644)
	}()
}

type CacheRefreshedMsg struct{}

func RefreshCache() tea.Cmd {
	return func() tea.Msg {
		done := make(chan struct{})
		doCacheInit(done)
		<-done
		return CacheRefreshedMsg{}
	}
}

// SearchPackages runs `<aurHelper> -Ss <query>` and parses the output into a list of Packages.
// If query is empty, we can fetch installed packages or just return an empty list.
func SearchPackages(query string, aurHelper string) ([]models.Package, error) {
	if query == "" {
		// If query is empty, let's just return a few basic packages or local ones.
		// For now, we'll run `<aurHelper> -Qs` to get installed packages.
		cmd := exec.Command(aurHelper, "-Qs")
		out, err := cmd.Output()
		if err != nil {
			return nil, err
		}
		return parseParuSearch(string(out)), nil
	}

	cacheMutex.RLock()
	ready := cacheReady
	cacheMutex.RUnlock()

	if ready {
		cacheMutex.RLock()
		defer cacheMutex.RUnlock()
		fuzzyQuery := strings.ReplaceAll(query, " ", "")
		matches := fuzzy.Find(fuzzyQuery, packageCache)
		var pkgs []models.Package
		for i, match := range matches {
			if i > 150 { // Limit to top 150 results for UI performance
				break
			}
			pkgs = append(pkgs, models.Package{
				Name: match.Str,
				Desc: "Press Enter to install, or scroll to view details.",
			})
		}
		return pkgs, nil
	}

	// Fallback if cache not ready
	args := []string{"-Ss"}
	args = append(args, strings.Fields(query)...)
	cmd := exec.Command(aurHelper, args...)
	out, err := cmd.Output()
	if err != nil {
		// returns non-zero if nothing is found.
		return []models.Package{}, nil
	}

	return parseParuSearch(string(out)), nil
}

// GetPackageInfo runs `<aurHelper> -Si <pkg>` and returns the raw string output.
func GetPackageInfo(pkgName string, aurHelper string) (string, error) {
	cmd := exec.Command(aurHelper, "-Si", pkgName)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// parseParuSearch parses the multi-line output of `paru -Ss` or `paru -Qs`.
func parseParuSearch(output string) []models.Package {
	var pkgs []models.Package
	lines := strings.Split(output, "\n")

	var currentPkg *models.Package

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		// First line of a package entry doesn't start with space
		if !strings.HasPrefix(line, " ") {
			// Format: repo/name version [installed] or (+votes popularity)
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				repoName := strings.Split(parts[0], "/")
				repo := "unknown"
				name := parts[0]
				if len(repoName) == 2 {
					repo = repoName[0]
					name = repoName[1]
				} else if len(repoName) == 1 && strings.HasPrefix(parts[0], "local/") {
					// local/pkg from paru -Qs
					repo = "local"
					name = strings.TrimPrefix(parts[0], "local/")
				}

				version := parts[1]
				installed := strings.Contains(line, "[installed]") || strings.Contains(line, "(Installed)") || repo == "local"

				currentPkg = &models.Package{
					Repo:      repo,
					Name:      name,
					Version:   version,
					Installed: installed,
				}
			}
		} else {
			// Description line, starts with spaces
			if currentPkg != nil {
				currentPkg.Desc = strings.TrimSpace(line)
				pkgs = append(pkgs, *currentPkg)
				currentPkg = nil // Reset for next package
			}
		}
	}

	return pkgs
}
