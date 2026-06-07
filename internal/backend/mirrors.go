package backend

import (
	"fmt"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

// GetMirrorUpdateCmd returns a tea.Cmd that executes the mirror update process
// based on the configured helper (reflector or rate-mirrors).
func GetMirrorUpdateCmd(helper string) tea.Cmd {
	var cmdStr string

	switch helper {
	case "reflector":
		// reflector: find 20 fastest https mirrors and save to mirrorlist, then sync pacman
		cmdStr = "sudo reflector --verbose --latest 20 --protocol https --sort rate --save /etc/pacman.d/mirrorlist && sudo pacman -Sy"
	case "rate-mirrors":
		// rate-mirrors: fetch and test mirrors, pipe to tee with sudo, then sync pacman
		cmdStr = "rate-mirrors arch | sudo tee /etc/pacman.d/mirrorlist && sudo pacman -Sy"
	default:
		return func() tea.Msg {
			return fmt.Errorf("unknown mirror helper: %s", helper)
		}
	}

	// Wrap in a shell to allow piping and compound commands
	fullCmd := fmt.Sprintf("%s; echo '\n[Press any key to return to paruz...]'; read -n 1 -s -r", cmdStr)
	cmd := exec.Command("sh", "-c", fullCmd)

	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		// This message will be handled by the AppModel Update method
		return MirrorUpdateFinishedMsg{Err: err}
	})
}

type MirrorUpdateFinishedMsg struct {
	Err error
}
