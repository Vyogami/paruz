package backend

import (
	"fmt"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vyogami/paruz/internal/config"
)

type Dependency struct {
	Name        string
	Command     string
	Description string
	IsAUR       bool
}

var RequiredDependencies = []Dependency{
	{Name: "base-devel", Command: "make", Description: "Needed for building AUR packages", IsAUR: false},
	{Name: "git", Command: "git", Description: "Needed to clone AUR repositories", IsAUR: false},
}

var HelperDependencies = []Dependency{
	{Name: "paru", Command: "paru", Description: "AUR Helper (Recommended)", IsAUR: true},
	{Name: "yay", Command: "yay", Description: "AUR Helper (Alternative)", IsAUR: true},
	{Name: "reflector", Command: "reflector", Description: "Mirror management tool", IsAUR: false},
	{Name: "rate-mirrors", Command: "rate-mirrors", Description: "Fast mirror ranking tool", IsAUR: true},
}

func CheckDependency(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func GetMissingDependencies(cfg config.Config) []Dependency {
	var missing []Dependency
	for _, dep := range RequiredDependencies {
		if !CheckDependency(dep.Command) {
			missing = append(missing, dep)
		}
	}
	
	// Check specifically the configured AUR helper
	if cfg.AURHelper != "" && !CheckDependency(cfg.AURHelper) {
		description := "AUR Helper (Configured)"
		if cfg.AURHelper == "paru" {
			description = HelperDependencies[0].Description
		} else if cfg.AURHelper == "yay" {
			description = HelperDependencies[1].Description
		}
		missing = append(missing, Dependency{
			Name:        cfg.AURHelper,
			Command:     cfg.AURHelper,
			Description: description,
			IsAUR:       true,
		})
	}

	// Check the specifically configured Mirror helper
	if cfg.MirrorHelper != "" && !CheckDependency(cfg.MirrorHelper) {
		description := "Mirror Helper (Configured)"
		isAUR := false
		if cfg.MirrorHelper == "reflector" {
			description = "Mirror management tool"
			isAUR = false
		} else if cfg.MirrorHelper == "rate-mirrors" {
			description = "Fast mirror ranking tool"
			isAUR = true
		}
		
		missing = append(missing, Dependency{
			Name:        cfg.MirrorHelper,
			Command:     cfg.MirrorHelper,
			Description: description,
			IsAUR:       isAUR,
		})
	}

	return missing
}

func InstallDependencyCmd(dep Dependency) tea.Cmd {
	var cmdStr string
	if !dep.IsAUR {
		cmdStr = fmt.Sprintf("sudo pacman -S --needed --noconfirm %s", dep.Name)
	} else {
		// Manual AUR bootstrap for the first helper
		if dep.Name == "paru" || dep.Name == "yay" {
			cmdStr = fmt.Sprintf("sudo pacman -S --needed --noconfirm base-devel git && " +
				"rm -rf /tmp/paruz-build-%s && " +
				"git clone https://aur.archlinux.org/%s.git /tmp/paruz-build-%s && " +
				"cd /tmp/paruz-build-%s && makepkg -si --noconfirm", dep.Name, dep.Name, dep.Name, dep.Name)
		} else {
			// If we have a helper, use it. Otherwise, we can't easily install other AUR deps.
			// This part assumes we install the helper first.
			helper := "paru"
			if !CheckDependency("paru") && CheckDependency("yay") {
				helper = "yay"
			}
			cmdStr = fmt.Sprintf("%s -S --needed --noconfirm %s", helper, dep.Name)
		}
	}

	fullCmd := fmt.Sprintf("echo 'Installing %s...'; %s; echo '\n[Done. Press any key to continue...]'; read -n 1 -s -r", dep.Name, cmdStr)
	c := exec.Command("sh", "-c", fullCmd)
	
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return BootstrapFinishedMsg{Dependency: dep, Err: err}
	})
}

type BootstrapFinishedMsg struct {
	Dependency Dependency
	Err        error
}
