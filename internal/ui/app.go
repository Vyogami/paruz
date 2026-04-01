package ui

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vyogami/paruz/internal/backend"
	"github.com/vyogami/paruz/internal/config"
	"github.com/vyogami/paruz/internal/models"
)

type sessionState int

const (
	stateSearch sessionState = iota
	stateSettings
	stateConfirmSettings
	stateBootstrap
	stateConfirmBootstrap
	stateBuildingCache
)

type AppModel struct {
	state       sessionState
	list        list.Model
	delegate    list.DefaultDelegate
	detailView  viewport.Model
	searchInput textinput.Model
	spinner     spinner.Model
	searching   bool
	fetching    bool
	refreshingCache bool
	config      config.Config
	oldConfig   config.Config
	version     string

	// Bootstrap state
	missingDeps []backend.Dependency
	bootstrapIdx int
	bootstrapSelected map[int]bool

	// Settings State
	settingsIndex int
	settingsTotal int

	// Data
	packages    []models.Package
	selectedPkg *models.Package
	pkgInfo     string
	errorMsg    string

	// Window dimensions
	width  int
	height int
}

func InitialModel(version string) *AppModel {
	cfg := config.LoadConfig()
	if cfg.AURHelper == "" {
		cfg.AURHelper = "paru"
	}

	// Load custom themes
	MergeCustomThemes(config.LoadCustomThemes())

	d := list.NewDefaultDelegate()
	l := list.New([]list.Item{}, d, 0, 0)
	l.Title = "paruz (installed)"
	l.SetShowStatusBar(false)
	l.DisableQuitKeybindings()
	// Disable default filter since we use our custom fuzzy search
	l.SetFilteringEnabled(false)

	dv := viewport.New(0, 0)

	ti := textinput.New()
	ti.Placeholder = "Search AUR & Repos..."
	ti.CharLimit = 156
	ti.Width = 40

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	m := &AppModel{
		state:             stateSearch,
		list:              l,
		delegate:          d,
		detailView:        dv,
		searchInput:       ti,
		spinner:           s,
		config:            cfg,
		version:           version,
		settingsIndex:     0,
		settingsTotal:     3, // AUR Helper, Mirror Helper, Theme
		bootstrapSelected: make(map[int]bool),
	}

	// Check dependencies
	m.missingDeps = backend.GetMissingDependencies(cfg)
	if len(m.missingDeps) > 0 {
		m.state = stateBootstrap
		// Pre-select only essentials and recommended by default
		for i, dep := range m.missingDeps {
			if dep.Category == "System Essentials" || 
			   dep.Name == "paru" || 
			   dep.Name == "rate-mirrors" ||
			   dep.Name == cfg.AURHelper ||
			   dep.Name == cfg.MirrorHelper {
				m.bootstrapSelected[i] = true
			}
		}
	} else {
		backend.InitCacheLocal()
		if !backend.IsCacheReady() {
			m.state = stateBuildingCache
		}
	}

	m.updateTheme()
	return m
}

func (m *AppModel) updateTheme() {
	ApplyTheme(m.config.Theme)
	theme, ok := Themes[m.config.Theme]
	if !ok {
		theme = Themes["ayu-dark"]
	}

	// Update list styles
	m.list.Styles.Title = TitleStyle
	m.list.Styles.ActivePaginationDot = lipgloss.NewStyle().Foreground(theme.InfoKey)

	// Update delegate styles for selection
	m.delegate.Styles.SelectedTitle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(theme.TitleBg).
		Foreground(theme.TitleBg).
		Padding(0, 0, 0, 1)
	m.delegate.Styles.SelectedDesc = m.delegate.Styles.SelectedTitle.Copy().
		Foreground(lipgloss.Color("241"))
	m.list.SetDelegate(m.delegate)

	// Update text input styles
	m.searchInput.PromptStyle = lipgloss.NewStyle().Foreground(theme.InfoKey)
	m.searchInput.TextStyle = lipgloss.NewStyle().Foreground(theme.InfoKey)
}

func (m *AppModel) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds,
		tea.EnterAltScreen,
		textinput.Blink,
		m.fetchPackages(""), // initial fetch
		m.spinner.Tick,
	)
	
	if len(m.missingDeps) == 0 {
		m.refreshingCache = true
		cmds = append(cmds, backend.RefreshCache())
	}
	
	return tea.Batch(cmds...)
}

// Commands
type stopFetchingMsg struct{}

func stopFetchingCmd() tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(_ time.Time) tea.Msg {
		return stopFetchingMsg{}
	})
}

type packagesFetchedMsg struct {
	query    string
	packages []models.Package
	err      error
}

func (m *AppModel) fetchPackages(query string) tea.Cmd {
	return func() tea.Msg {
		pkgs, err := backend.SearchPackages(strings.TrimSpace(query), m.config.AURHelper)
		return packagesFetchedMsg{query: query, packages: pkgs, err: err}
	}
}

type pkgInfoFetchedMsg struct {
	info string
	err  error
}

func (m *AppModel) fetchPkgInfo(pkgName string) tea.Cmd {
	return func() tea.Msg {
		info, err := backend.GetPackageInfo(pkgName, m.config.AURHelper)
		return pkgInfoFetchedMsg{info: info, err: err}
	}
}

type execFinishedMsg struct {
	err error
}

func (m *AppModel) runInstallCmd(pkgName string) tea.Cmd {
	// Wrap in sh -c to allow pausing before returning to TUI
	cmdStr := fmt.Sprintf("%s -S %s; echo '\n[Press any key to return to paruz...]'; read -n 1 -s -r", m.config.AURHelper, pkgName)
	cmd := exec.Command("sh", "-c", cmdStr)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return execFinishedMsg{err: err}
	})
}

func (m *AppModel) runMirrorUpdate() tea.Cmd {
	return backend.GetMirrorUpdateCmd(m.config.MirrorHelper)
}

func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

		if m.state == stateBuildingCache {
			switch msg.String() {
			case "q", "esc":
				return m, tea.Quit
			}
			return m, nil
		}

		if m.state == stateBootstrap {
			switch msg.String() {
			case "j", "down":
				m.bootstrapIdx = (m.bootstrapIdx + 1) % len(m.missingDeps)
			case "k", "up":
				m.bootstrapIdx = (m.bootstrapIdx - 1 + len(m.missingDeps)) % len(m.missingDeps)
			case " ":
				m.bootstrapSelected[m.bootstrapIdx] = !m.bootstrapSelected[m.bootstrapIdx]
			case "enter":
				var selectedDeps []backend.Dependency
				for i, dep := range m.missingDeps {
					if m.bootstrapSelected[i] {
						selectedDeps = append(selectedDeps, dep)
					}
				}
				if len(selectedDeps) > 0 {
					m.state = stateConfirmBootstrap
				}
			case "q", "esc":
				// Start cache init and fetch packages even if skipped
				backend.InitCacheLocal()
				if !backend.IsCacheReady() {
					m.state = stateBuildingCache
				} else {
					m.state = stateSearch
				}
				m.refreshingCache = true
				return m, tea.Batch(m.fetchPackages(""), backend.RefreshCache())
			}
			return m, nil
		}

		if m.state == stateConfirmBootstrap {
			switch msg.String() {
			case "y", "Y":
				var selectedDeps []backend.Dependency
				for i, dep := range m.missingDeps {
					if m.bootstrapSelected[i] {
						selectedDeps = append(selectedDeps, dep)
					}
				}
				return m, backend.InstallBatchCmd(selectedDeps)
			case "n", "N", "esc", "q":
				m.state = stateBootstrap
			}
			return m, nil
		}

		if m.state == stateSettings {
			switch msg.String() {
			case "esc", "q":
				if m.config != m.oldConfig {
					m.state = stateConfirmSettings
				} else {
					m.state = stateSearch
				}
				return m, nil
			case "j", "down":
				m.settingsIndex = (m.settingsIndex + 1) % m.settingsTotal
			case "k", "up":
				m.settingsIndex = (m.settingsIndex - 1 + m.settingsTotal) % m.settingsTotal
			case " ", "enter", "l", "right":
				switch m.settingsIndex {
				case 0: // AUR Helper
					if m.config.AURHelper == "paru" {
						m.config.AURHelper = "yay"
					} else {
						m.config.AURHelper = "paru"
					}
				case 1: // Mirror Helper
					if m.config.MirrorHelper == "rate-mirrors" {
						m.config.MirrorHelper = "reflector"
					} else {
						m.config.MirrorHelper = "rate-mirrors"
					}
				case 2: // Theme
					var themesList []string
					for k := range Themes {
						themesList = append(themesList, k)
					}
					// Sort to have consistent order
					sort.Strings(themesList)

					currentIdx := 0
					for i, t := range themesList {
						if t == m.config.Theme {
							currentIdx = i
							break
						}
					}
					m.config.Theme = themesList[(currentIdx+1)%len(themesList)]
					m.updateTheme()
				}
				// Don't save yet, wait for exit confirmation
			}
			return m, nil
		}

		if m.state == stateConfirmSettings {
			switch msg.String() {
			case "y", "Y", "enter":
				config.SaveConfig(m.config)
				m.state = stateSearch
			case "n", "N", "esc", "q":
				m.config = m.oldConfig
				m.updateTheme()
				m.state = stateSearch
			}
			return m, nil
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
				return m, nil // Already fetching while typing
			case "esc":
				m.searching = false
				m.searchInput.Blur()
				return m, nil
			}

			// Handle input and fetch immediately if changed
			prevVal := m.searchInput.Value()
			m.searchInput, cmd = m.searchInput.Update(msg)

			var fetchCmd tea.Cmd
			if m.searchInput.Value() != prevVal {
				m.fetching = true
				fetchCmd = m.fetchPackages(m.searchInput.Value())
			}

			return m, tea.Batch(cmd, fetchCmd)
		} else {
			switch msg.String() {
			case "enter":
				// Install selected
				if i, ok := m.list.SelectedItem().(models.Package); ok {
					return m, m.runInstallCmd(i.Name)
				}
			case "u":
				// Update Mirrors
				return m, m.runMirrorUpdate()
			case "r":
				// Refresh Cache
				m.refreshingCache = true
				return m, backend.RefreshCache()
			case "/", "s":
				// Pressing / or s opens the search bar
				m.searching = true
				m.searchInput.Focus()
				return m, nil
			case ",":
				// Settings Screen
				m.oldConfig = m.config
				m.state = stateSettings
				return m, nil
			case "q":
				return m, tea.Quit
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateSizes()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case stopFetchingMsg:
		m.fetching = false
		return m, nil

	case packagesFetchedMsg:
		if msg.query != m.searchInput.Value() {
			return m, nil
		}

		cmds = append(cmds, stopFetchingCmd())

		if msg.err != nil {			m.errorMsg = msg.err.Error()
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

	case backend.BatchBootstrapFinishedMsg:
		if msg.Err != nil {
			m.errorMsg = fmt.Sprintf("Installation failed: %v", msg.Err)
		} else {
			// Update config if we installed a tool that wasn't previously selected
			// or if the selected one is now available.
			for _, dep := range msg.Deps {
				if (dep.Name == "paru" || dep.Name == "yay") && !backend.CheckDependency(m.config.AURHelper) {
					m.config.AURHelper = dep.Name
				}
				if (dep.Name == "reflector" || dep.Name == "rate-mirrors") && !backend.CheckDependency(m.config.MirrorHelper) {
					m.config.MirrorHelper = dep.Name
				}
			}
			config.SaveConfig(m.config)

			m.missingDeps = backend.GetMissingDependencies(m.config)
			if len(m.missingDeps) == 0 {
				backend.InitCacheLocal()
				if !backend.IsCacheReady() {
					m.state = stateBuildingCache
				} else {
					m.state = stateSearch
				}
				m.refreshingCache = true
				return m, tea.Batch(m.fetchPackages(""), backend.RefreshCache())
			}
			if m.bootstrapIdx >= len(m.missingDeps) {
				m.bootstrapIdx = 0
			}
		}
		m.updateSizes()

	case backend.MirrorUpdateFinishedMsg:
		if msg.Err != nil {
			m.errorMsg = fmt.Sprintf("Mirror update failed: %v", msg.Err)
		} else {
			m.errorMsg = ""
		}
		m.updateSizes()

	case execFinishedMsg:
		if msg.err != nil {
			m.errorMsg = fmt.Sprintf("Execution failed: %v", msg.err)
		} else {
			m.errorMsg = ""
		}
		// Refresh UI dimensions since terminal might have been resized during exec
		m.updateSizes()

	case error:
		m.errorMsg = msg.Error()

	case backend.CacheRefreshedMsg:
		m.refreshingCache = false
		m.errorMsg = ""
		if m.state == stateBuildingCache {
			m.state = stateSearch
			m.fetching = true
			cmds = append(cmds, m.fetchPackages(m.searchInput.Value()))
		} else if m.searchInput.Value() != "" {
			// If searching, trigger a new search to use the fresh cache
			m.fetching = true
			cmds = append(cmds, m.fetchPackages(m.searchInput.Value()))
		}
	}

	// Update list if not typing in our remote search and not in settings
	if m.state == stateSearch && !m.searching {
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

	return m, tea.Batch(cmds...)
}

func (m *AppModel) getSearchBar() string {
	theme := Themes[m.config.Theme]
	
	style := SearchStyle.Copy().
		Width(m.width).
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(theme.Border)
	
	var content string
	if m.searching {
		prefix := lipgloss.NewStyle().
			Background(theme.InfoKey).
			Foreground(theme.TitleFg).
			Bold(true).
			Padding(0, 1).
			Render(" SEARCH ")
		
		content = lipgloss.JoinHorizontal(lipgloss.Top, prefix, " ", m.searchInput.View())
		style = style.BorderForeground(theme.InfoKey)
	} else {
		prefix := lipgloss.NewStyle().
			Background(theme.Border).
			Foreground(theme.TitleFg).
			Padding(0, 1).
			Render(" PARUZ ")
			
		hint := lipgloss.NewStyle().Foreground(theme.StatusBar).Render(" Press [/] to start searching packages...")
		content = lipgloss.JoinHorizontal(lipgloss.Top, prefix, hint)
	}
	
	return style.Render(content)
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

	if m.state == stateBuildingCache {
		theme := Themes[m.config.Theme]
		keyColor := lipgloss.NewStyle().Foreground(theme.InfoKey)

		title := TitleStyle.Render(" Initializing Cache ")
		shortcuts := fmt.Sprintf("%s Quit", keyColor.Render("[q]"))
		content := fmt.Sprintf("%s Building the initial package cache...\nThis may take a few moments.\n\n%s", m.spinner.View(), shortcuts)
		pane := PaneStyle.Copy().
			Width(55).
			Height(5).
			Align(lipgloss.Center).
			Render(content)

		dialog := lipgloss.JoinVertical(lipgloss.Center, title, pane)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
	}

	if m.state == stateBootstrap {
		return m.bootstrapView()
	}

	if m.state == stateConfirmBootstrap {
		theme := Themes[m.config.Theme]
		keyColor := lipgloss.NewStyle().Foreground(theme.InfoKey)

		title := TitleStyle.Render(" Install Dependencies? ")
		var selected []string
		for i, dep := range m.missingDeps {
			if m.bootstrapSelected[i] {
				selected = append(selected, dep.Name)
			}
		}
		
		selectedText := strings.Join(selected, ", ")
		if len(selectedText) > 40 {
			selectedText = selectedText[:37] + "..."
		}
		
		shortcuts := fmt.Sprintf("%s Yes  %s No", keyColor.Render("[y]"), keyColor.Render("[n]"))
		content := fmt.Sprintf("Install %d selected items?\n(%s)\n\n%s", len(selected), selectedText, shortcuts)
		pane := PaneStyle.Copy().
			Width(50).
			Height(7).
			Align(lipgloss.Center).
			Render(content)
		
		dialog := lipgloss.JoinVertical(lipgloss.Center, title, pane)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
	}

	if m.state == stateSettings {
		return m.settingsView()
	}

	if m.state == stateConfirmSettings {
		theme := Themes[m.config.Theme]
		keyColor := lipgloss.NewStyle().Foreground(theme.InfoKey)

		title := TitleStyle.Render(" Save Changes? ")
		shortcuts := fmt.Sprintf("%s Yes  %s No", keyColor.Render("[y]"), keyColor.Render("[n]"))
		content := fmt.Sprintf("You have unsaved changes. Do you want to save them?\n\n%s", shortcuts)
		pane := PaneStyle.Copy().
			Width(40).
			Height(5).
			Align(lipgloss.Center).
			Render(content)

		dialog := lipgloss.JoinVertical(lipgloss.Center, title, pane)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
	}


	theme := Themes[m.config.Theme]
	listStyle := ListPaneStyle.Copy()
	detailStyle := DetailPaneStyle.Copy()

	if !m.searching && m.state == stateSearch {
		listStyle = listStyle.BorderForeground(theme.InfoKey)
	}

	listPane := listStyle.Render(m.list.View())

	detailContent := m.detailView.View()
	if m.detailView.Height > 0 {
		detailContent = detailStyle.Render(detailContent)
	}

	searchBar := m.getSearchBar()
	mainView := lipgloss.JoinVertical(lipgloss.Left,
		searchBar,
		lipgloss.JoinHorizontal(lipgloss.Top, listPane, detailContent),
	)

	statusColor := lipgloss.NewStyle().Foreground(theme.InfoKey).Bold(true)
	keyColor := lipgloss.NewStyle().Foreground(theme.InfoKey)
	
	statusLabel := statusColor.Render("Status:")
	tickView := lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✓") + " "

	statusText := fmt.Sprintf("Ready %s", tickView)
	if m.searching {
		spinnerView := tickView
		if m.fetching {
			spinnerView = m.spinner.View()
		}
		statusText = fmt.Sprintf("Typing Search Query %s", spinnerView)
	} else if m.refreshingCache {
		statusText = fmt.Sprintf("Refreshing Cache %s", m.spinner.View())
	}

	shortcuts := ""
	if m.searching {
		shortcuts = fmt.Sprintf("%s finish • %s cancel",
			keyColor.Render("[enter]"),
			keyColor.Render("[esc]"),
		)
	} else {
		shortcuts = fmt.Sprintf("%s install • %s update mirrors • %s refresh cache • %s settings • %s quit",
			keyColor.Render("[enter]"),
			keyColor.Render("[u]"),
			keyColor.Render("[r]"),
			keyColor.Render("[,]"),
			keyColor.Render("[q]"),
		)
	}

	statusPart := fmt.Sprintf("%s %s", statusLabel, statusText)
	spacerWidth := m.width - lipgloss.Width(statusPart) - lipgloss.Width(shortcuts)
	if spacerWidth < 1 {
		spacerWidth = 1
	}
	spacer := strings.Repeat(" ", spacerWidth)

	statusBar := StatusBarStyle.Render(statusPart + spacer + shortcuts)
	if m.errorMsg != "" {
		statusBar = StatusBarStyle.Render("Error: " + m.errorMsg)
	}

	return AppStyle.Render(lipgloss.JoinVertical(lipgloss.Left, mainView, statusBar))
}

func (m *AppModel) bootstrapView() string {
	theme := Themes[m.config.Theme]
	title := TitleStyle.Render(" Dependencies Setup ")
	
	var content string
	lastCategory := ""
	
	for i, dep := range m.missingDeps {
		if dep.Category != lastCategory {
			if content != "" {
				content += "\n"
			}
			content += lipgloss.NewStyle().Foreground(theme.InfoKey).Bold(true).Underline(true).Render(dep.Category) + "\n"
			lastCategory = dep.Category
		}

		cursor := "  "
		checked := lipgloss.NewStyle().Foreground(theme.Error).Render("✘")
		if m.bootstrapSelected[i] {
			checked = lipgloss.NewStyle().Foreground(theme.TitleBg).Render("✔")
		}
		
		name := dep.Name
		if i == m.bootstrapIdx {
			cursor = lipgloss.NewStyle().Foreground(theme.TitleBg).Render("> ")
			name = lipgloss.NewStyle().Foreground(theme.InfoTitle).Bold(true).Render(name)
		}
		
		content += fmt.Sprintf("%s%s %s - %s\n", cursor, checked, name, dep.Description)
	}

	content += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("Press [space] to toggle, [enter] to install.")

	// Use a more compact width for a dialog feel
	dialogWidth := 60
	if m.width-10 < dialogWidth {
		dialogWidth = m.width - 10
	}
	if dialogWidth < 0 {
		dialogWidth = 0
	}

	keyColor := lipgloss.NewStyle().Foreground(theme.InfoKey)
	footerText := fmt.Sprintf("%s Navigate | %s Toggle | %s Install | %s Skip",
		keyColor.Render("[j/k]"),
		keyColor.Render("[space]"),
		keyColor.Render("[enter]"),
		keyColor.Render("[q]"),
	)
	
	pane := PaneStyle.Copy().Width(dialogWidth).Render(content)
	footer := StatusBarStyle.Render(footerText)
	
	dialog := lipgloss.JoinVertical(lipgloss.Center, title, pane, footer)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
}

func (m *AppModel) settingsView() string {
	theme := Themes[m.config.Theme]
	title := TitleStyle.Render(" Settings ")
	
	options := []string{
		fmt.Sprintf("AUR Helper:    %s", m.config.AURHelper),
		fmt.Sprintf("Mirror Helper: %s", m.config.MirrorHelper),
		fmt.Sprintf("Theme:         %s", m.config.Theme),
	}

	var content string
	for i, opt := range options {
		cursor := "  "
		if i == m.settingsIndex {
			cursor = lipgloss.NewStyle().Foreground(theme.TitleBg).Render("> ")
			opt = lipgloss.NewStyle().Foreground(theme.InfoTitle).Bold(true).Render(opt)
		}
		content += fmt.Sprintf("%s %s\n", cursor, opt)
	}

	dialogWidth := 60
	if m.width-10 < dialogWidth {
		dialogWidth = m.width - 10
	}
	if dialogWidth < 0 {
		dialogWidth = 0
	}

	keyColor := lipgloss.NewStyle().Foreground(theme.InfoKey)
	footerText := fmt.Sprintf("%s Navigate | %s Toggle | %s Back",
		keyColor.Render("[j/k]"),
		keyColor.Render("[space]"),
		keyColor.Render("[esc]"),
	)

	pane := PaneStyle.Copy().Width(dialogWidth).Render(content)
	footer := StatusBarStyle.Render(footerText)
	
	dialog := lipgloss.JoinVertical(lipgloss.Center, title, pane, footer)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
}

var programRef *tea.Program

func SetProgramRef(p *tea.Program) {
	programRef = p
}
