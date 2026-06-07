package backend

import (
	"bufio"
	"compress/gzip"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/vyogami/paruz/internal/models"
)

var (
	packageCache []string
	installedSet map[string]struct{}
	cacheMutex   sync.RWMutex
	cacheReady   bool
	isRefreshing bool
	waiters      []chan struct{}
)

// loadInstalledSet refreshes the set of locally installed package names via
// `pacman -Qq`. It is used to flag cached search results as installed.
func loadInstalledSet() {
	out, err := exec.Command("pacman", "-Qq").Output()
	if err != nil {
		return
	}
	set := make(map[string]struct{})
	for _, line := range strings.Split(string(out), "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			set[name] = struct{}{}
		}
	}
	cacheMutex.Lock()
	installedSet = set
	cacheMutex.Unlock()
}

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
	loadInstalledSet()
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
		loadInstalledSet()

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
		candidates := packageCache
		cacheMutex.RUnlock()

		terms := strings.Fields(query)
		return rankMatches(terms, candidates), nil
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

// rankMatches keeps candidates that fuzzily match every search term (each term
// must appear as a case-insensitive subsequence of the package name), then
// orders them so package-name search feels intuitive: exact matches first, then
// prefix matches, then substring matches, then remaining fuzzy matches. Within
// each bucket results are ordered by edit distance to the query and then
// alphabetically, so a query like "neo" surfaces "neon"/"neoss" above scattered
// matches such as "nexus-oss". Results are capped to 150 for UI performance.
func rankMatches(terms []string, candidates []string) []models.Package {
	if len(terms) == 0 {
		return nil
	}
	queryFlat := strings.ToLower(strings.Join(terms, ""))

	type scored struct {
		name   string
		bucket int
		dist   int
	}

	var matches []scored
	for _, name := range candidates {
		if name == "" {
			continue
		}
		matchesAll := true
		for _, term := range terms {
			if !fuzzy.MatchFold(term, name) {
				matchesAll = false
				break
			}
		}
		if !matchesAll {
			continue
		}

		nl := strings.ToLower(name)
		bucket := 3 // fuzzy-only
		switch {
		case nl == queryFlat:
			bucket = 0 // exact
		case strings.HasPrefix(nl, queryFlat):
			bucket = 1 // prefix
		case strings.Contains(nl, queryFlat):
			bucket = 2 // substring
		}

		matches = append(matches, scored{
			name:   name,
			bucket: bucket,
			dist:   fuzzy.LevenshteinDistance(queryFlat, nl),
		})
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].bucket != matches[j].bucket {
			return matches[i].bucket < matches[j].bucket
		}
		if matches[i].dist != matches[j].dist {
			return matches[i].dist < matches[j].dist
		}
		return matches[i].name < matches[j].name
	})

	var pkgs []models.Package
	cacheMutex.RLock()
	installed := installedSet
	cacheMutex.RUnlock()
	for i, m := range matches {
		if i >= 150 {
			break
		}
		_, isInstalled := installed[m.name]
		pkgs = append(pkgs, models.Package{
			Name:      m.name,
			Installed: isInstalled,
		})
	}
	return pkgs
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

// GetPackagesInfo runs `<aurHelper> -Si <pkg1> <pkg2> ...` in a single call and
// returns each package's info block keyed by name. It is used to prefetch
// details for a page of search results so scrolling is instant. Packages that
// aren't found are simply omitted; a partial result is still returned even if
// the helper exits non-zero.
func GetPackagesInfo(names []string, aurHelper string) (map[string]string, error) {
	if len(names) == 0 {
		return map[string]string{}, nil
	}
	args := append([]string{"-Si"}, names...)
	out, _ := exec.Command(aurHelper, args...).Output()
	if len(out) == 0 {
		return map[string]string{}, nil
	}
	return splitInfoBlocks(string(out)), nil
}

// splitInfoBlocks splits the concatenated `-Si` output into per-package blocks,
// keyed by each block's "Name" field. Records are separated by a blank line.
func splitInfoBlocks(out string) map[string]string {
	res := make(map[string]string)
	var cur []string
	flush := func() {
		if len(cur) == 0 {
			return
		}
		if name := infoBlockName(cur); name != "" {
			res[name] = strings.Join(cur, "\n") + "\n"
		}
		cur = nil
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		cur = append(cur, line)
	}
	flush()
	return res
}

// infoBlockName extracts the value of the "Name" field from a `-Si` block.
func infoBlockName(lines []string) string {
	for _, l := range lines {
		idx := strings.Index(l, ":")
		if idx < 0 {
			continue
		}
		if strings.TrimSpace(l[:idx]) == "Name" {
			return strings.TrimSpace(l[idx+1:])
		}
	}
	return ""
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
