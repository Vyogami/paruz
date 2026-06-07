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
	Category    string
}

var RequiredDependencies = []Dependency{
	{Name: "base-devel", Command: "make", Description: "Needed for building AUR packages", IsAUR: false, Category: "System Essentials"},
	{Name: "git", Command: "git", Description: "Needed to clone AUR repositories", IsAUR: false, Category: "System Essentials"},
}

var HelperDependencies = []Dependency{
	{Name: "paru", Command: "paru", Description: "AUR Helper (Recommended)", IsAUR: true, Category: "AUR Helpers"},
	{Name: "yay", Command: "yay", Description: "AUR Helper (Alternative)", IsAUR: true, Category: "AUR Helpers"},
	{Name: "rate-mirrors", Command: "rate-mirrors", Description: "Fast mirror ranking tool (Recommended)", IsAUR: true, Category: "Mirror Helpers"},
	{Name: "reflector", Command: "reflector", Description: "Mirror management tool (Alternative)", IsAUR: false, Category: "Mirror Helpers"},
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

	// If no AUR helper is installed, offer both
	if !CheckDependency("paru") && !CheckDependency("yay") {
		missing = append(missing, HelperDependencies[0]) // paru
		missing = append(missing, HelperDependencies[1]) // yay
	} else if cfg.AURHelper != "" && !CheckDependency(cfg.AURHelper) {
		// If specifically configured one is missing, offer it
		missing = append(missing, Dependency{
			Name:        cfg.AURHelper,
			Command:     cfg.AURHelper,
			Description: "Configured AUR Helper",
			IsAUR:       true,
			Category:    "AUR Helpers",
		})
	}

	// If no Mirror helper is installed, offer both
	if !CheckDependency("reflector") && !CheckDependency("rate-mirrors") {
		missing = append(missing, HelperDependencies[2]) // reflector
		missing = append(missing, HelperDependencies[3]) // rate-mirrors
	} else if cfg.MirrorHelper != "" && !CheckDependency(cfg.MirrorHelper) {
		// If specifically configured one is missing, offer it
		isAUR := cfg.MirrorHelper == "rate-mirrors"
		missing = append(missing, Dependency{
			Name:        cfg.MirrorHelper,
			Command:     cfg.MirrorHelper,
			Description: "Configured Mirror Helper",
			IsAUR:       isAUR,
			Category:    "Mirror Helpers",
		})
	}

	return missing
}

func InstallBatchCmd(deps []Dependency) tea.Cmd {
	if len(deps) == 0 {
		return nil
	}

	var fullCmdStr string
	for i, dep := range deps {
		var cmdStr string
		if !dep.IsAUR {
			cmdStr = fmt.Sprintf("sudo pacman -S --needed --noconfirm %s", dep.Name)
		} else {
			if dep.Name == "paru" || dep.Name == "yay" {
				cmdStr = fmt.Sprintf("sudo pacman -S --needed --noconfirm base-devel git && "+
					"rm -rf /tmp/paruz-build-%s && "+
					"git clone https://aur.archlinux.org/%s.git /tmp/paruz-build-%s && "+
					"cd /tmp/paruz-build-%s && makepkg -si --noconfirm", dep.Name, dep.Name, dep.Name, dep.Name)
			} else {
				helper := "paru"
				if !CheckDependency("paru") && CheckDependency("yay") {
					helper = "yay"
				}
				// If we are installing the helper in this same batch, we need to be careful.
				// For simplicity, we assume helpers are installed first or already exist.
				cmdStr = fmt.Sprintf("%s -S --needed --noconfirm %s", helper, dep.Name)
			}
		}

		if i == 0 {
			fullCmdStr = fmt.Sprintf("echo 'Installing %s...'; %s", dep.Name, cmdStr)
		} else {
			fullCmdStr = fmt.Sprintf("%s && echo '\nInstalling %s...'; %s", fullCmdStr, dep.Name, cmdStr)
		}
	}

	finalCmd := fmt.Sprintf("%s; echo '\n[Batch installation finished. Press any key to continue...]'; read -n 1 -s -r", fullCmdStr)
	c := exec.Command("sh", "-c", finalCmd)

	return tea.ExecProcess(c, func(err error) tea.Msg {
		return BatchBootstrapFinishedMsg{Deps: deps, Err: err}
	})
}

type BatchBootstrapFinishedMsg struct {
	Deps []Dependency
	Err  error
}
