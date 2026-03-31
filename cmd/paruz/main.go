package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vyogami/paruz/internal/ui"
)

var version = "v1.1.0-alpha"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "v", "-v", "--version", "version":
			fmt.Printf("paruz %s\n", version)
			return
		}
	}

	m := ui.InitialModel(version)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	ui.SetProgramRef(p)

	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v\n", err)
		os.Exit(1)
	}
}
