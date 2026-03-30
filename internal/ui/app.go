package ui

import (
	"fmt"
	"os/exec"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vyogami/paruz/internal/backend"
	"github.com/vyogami/paruz/internal/models"
)

type AppModel struct {
	list        list.Model
	detailView  viewport.Model
	searchInput textinput.Model
	searching   bool

	// Data
	packages    []models.Package
	selectedPkg *models.Package
	pkgInfo     string
	errorMsg    string

	// Window dimensions
	width  int
	height int
}

func InitialModel() *AppModel {
	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.Title = "paruz (installed)"
	l.SetShowStatusBar(false)
	l.DisableQuitKeybindings()

	dv := viewport.New(0, 0)

	ti := textinput.New()
	ti.Placeholder = "Search AUR & Repos..."
	ti.CharLimit = 156
	ti.Width = 40

	m := &AppModel{
		list:        l,
		detailView:  dv,
		searchInput: ti,
	}
	return m
}

func (m *AppModel) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		textinput.Blink,
		m.fetchPackages(""), // initial fetch
	)
}

// Commands
type packagesFetchedMsg struct {
	packages []models.Package
	err      error
}

func (m *AppModel) fetchPackages(query string) tea.Cmd {
	return func() tea.Msg {
		pkgs, err := backend.SearchPackages(query)
		return packagesFetchedMsg{packages: pkgs, err: err}
	}
}

type pkgInfoFetchedMsg struct {
	info string
	err  error
}

func (m *AppModel) fetchPkgInfo(pkgName string) tea.Cmd {
	return func() tea.Msg {
		info, err := backend.GetPackageInfo(pkgName)
		return pkgInfoFetchedMsg{info: info, err: err}
	}
}

type execFinishedMsg struct {
	err error
}

func (m *AppModel) runInstallCmd(pkgName string) tea.Cmd {
	// Wrap in sh -c to allow pausing before returning to TUI
	cmdStr := fmt.Sprintf("paru -S %s; echo '\n[Press any key to return to paruz...]'; read -n 1 -s -r", pkgName)
	cmd := exec.Command("sh", "-c", cmdStr)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return execFinishedMsg{err: err}
	})
}

func (m *AppModel) runMirrorUpdate() tea.Cmd {
	cmdStr := "sudo rate-mirrors arch | sudo tee /etc/pacman.d/mirrorlist; echo '\n[Press any key to return to paruz...]'; read -n 1 -s -r"
	cmd := exec.Command("sh", "-c", cmdStr)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return execFinishedMsg{err: err}
	})
}

func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

		if m.searching {
			switch msg.String() {
			case "enter":
				m.searching = false
				m.searchInput.Blur()
				title := m.searchInput.Value()
				if title == "" {
					title = "installed"
				}
				m.list.Title = "paruz (" + title + ")"
				return m, m.fetchPackages(m.searchInput.Value())
			case "esc":
				m.searching = false
				m.searchInput.Blur()
				return m, nil
			}
			m.searchInput, cmd = m.searchInput.Update(msg)
			return m, cmd
		} else if m.list.FilterState() != list.Filtering {
			switch msg.String() {
			case "enter":
				// Install selected
				if i, ok := m.list.SelectedItem().(models.Package); ok {
					return m, m.runInstallCmd(i.Name)
				}
			case "u":
				// Update Mirrors
				return m, m.runMirrorUpdate()
			case "s":
				m.searching = true
				m.searchInput.Focus()
				return m, nil
			case "q":
				return m, tea.Quit
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateSizes()

	case packagesFetchedMsg:
		if msg.err != nil {
			m.errorMsg = msg.err.Error()
		} else {
			items := make([]list.Item, len(msg.packages))
			for i, p := range msg.packages {
				items[i] = p
			}
			m.list.SetItems(items)
			if len(msg.packages) > 0 {
				m.selectedPkg = &msg.packages[0]
				cmds = append(cmds, m.fetchPkgInfo(m.selectedPkg.Name))
			} else {
				m.pkgInfo = "No packages found."
				m.detailView.SetContent(m.pkgInfo)
			}
		}

	case pkgInfoFetchedMsg:
		if msg.err != nil {
			m.pkgInfo = ErrorStyle.Render(msg.err.Error())
		} else {
			m.pkgInfo = msg.info
		}
		m.detailView.SetContent(m.pkgInfo)

	case execFinishedMsg:
		if msg.err != nil {
			m.errorMsg = fmt.Sprintf("Execution failed: %v", msg.err)
		} else {
			m.errorMsg = ""
		}
		// Refresh UI dimensions since terminal might have been resized during exec
		m.updateSizes()
		// Redraw full alt screen might be needed, but bubbles handles it.
		// If needed, can send a window size msg or clear screen.

	case error:
		m.errorMsg = msg.Error()
	}

	// Update list if not typing in our remote search
	if !m.searching {
		prevIndex := m.list.Index()
		m.list, cmd = m.list.Update(msg)
		cmds = append(cmds, cmd)

		// Check if selection changed
		if m.list.Index() != prevIndex && len(m.list.Items()) > 0 {
			if i, ok := m.list.SelectedItem().(models.Package); ok {
				m.selectedPkg = &i
				cmds = append(cmds, m.fetchPkgInfo(i.Name))
			}
		}
	}

	if m.searching {
		m.searchInput, cmd = m.searchInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *AppModel) getSearchBar() string {
	if m.searching {
		return lipgloss.NewStyle().MarginBottom(1).Render(m.searchInput.View())
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("241")).MarginBottom(1).Render("Press 's' to search AUR & Repos, '/' to filter local list.")
}

func (m *AppModel) updateSizes() {
	appH, appV := AppStyle.GetFrameSize()
	listH, listV := ListPaneStyle.GetFrameSize()
	detailH, detailV := DetailPaneStyle.GetFrameSize()

	searchBarHeight := lipgloss.Height(m.getSearchBar())

	panesHeight := m.height - appV - 2 - searchBarHeight
	if panesHeight < 0 {
		panesHeight = 0
	}

	listWidth := 45
	listInnerHeight := panesHeight - listV
	if listInnerHeight < 0 {
		listInnerHeight = 0
	}
	m.list.SetSize(listWidth, listInnerHeight)

	detailInnerWidth := m.width - appH - (listWidth + listH) - detailH - 2
	if detailInnerWidth < 0 {
		detailInnerWidth = 0
	}
	detailInnerHeight := panesHeight - detailV
	if detailInnerHeight < 0 {
		detailInnerHeight = 0
	}
	m.detailView.Width = detailInnerWidth
	m.detailView.Height = detailInnerHeight
}

func (m *AppModel) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	listPane := ListPaneStyle.Render(m.list.View())

	detailContent := m.detailView.View()
	if m.detailView.Height > 0 {
		detailContent = DetailPaneStyle.Render(detailContent)
	}

	searchBar := m.getSearchBar()
	mainView := lipgloss.JoinVertical(lipgloss.Left,
		searchBar,
		lipgloss.JoinHorizontal(lipgloss.Top, listPane, detailContent),
	)

	statusText := "Ready"
	if m.searching {
		statusText = "Typing Search Query..."
	}
	statusBar := StatusBarStyle.Render(fmt.Sprintf("Status: %s | [Enter] Install  [u] Update Mirrors  [q] Quit", statusText))
	if m.errorMsg != "" {
		statusBar = StatusBarStyle.Render("Error: " + m.errorMsg)
	}

	return AppStyle.Render(lipgloss.JoinVertical(lipgloss.Left, mainView, statusBar))
}

var programRef *tea.Program

func SetProgramRef(p *tea.Program) {
	programRef = p
}
