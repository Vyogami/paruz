package ui

import "github.com/charmbracelet/lipgloss"

type Theme struct {
	TitleBg   lipgloss.Color
	TitleFg   lipgloss.Color
	Border    lipgloss.Color
	InfoTitle lipgloss.Color
	InfoKey   lipgloss.Color
	Error     lipgloss.Color
	StatusBar lipgloss.Color
}

var Themes = map[string]Theme{
	"default": {
		TitleBg:   lipgloss.Color("#25A065"),
		TitleFg:   lipgloss.Color("#FFFDF5"),
		Border:    lipgloss.Color("62"),
		InfoTitle: lipgloss.Color("205"),
		InfoKey:   lipgloss.Color("86"),
		Error:     lipgloss.Color("196"),
		StatusBar: lipgloss.Color("241"),
	},
	"dracula": {
		TitleBg:   lipgloss.Color("#bd93f9"),
		TitleFg:   lipgloss.Color("#282a36"),
		Border:    lipgloss.Color("#6272a4"),
		InfoTitle: lipgloss.Color("#ff79c6"),
		InfoKey:   lipgloss.Color("#8be9fd"),
		Error:     lipgloss.Color("#ff5555"),
		StatusBar: lipgloss.Color("#f8f8f2"),
	},
	"nord": {
		TitleBg:   lipgloss.Color("#88C0D0"),
		TitleFg:   lipgloss.Color("#2E3440"),
		Border:    lipgloss.Color("#4C566A"),
		InfoTitle: lipgloss.Color("#B48EAD"),
		InfoKey:   lipgloss.Color("#8FBCBB"),
		Error:     lipgloss.Color("#BF616A"),
		StatusBar: lipgloss.Color("#D8DEE9"),
	},
	"ayu-dark": {
		TitleBg:   lipgloss.Color("#FFB454"),
		TitleFg:   lipgloss.Color("#0F1419"),
		Border:    lipgloss.Color("#3E4B59"),
		InfoTitle: lipgloss.Color("#E6B450"),
		InfoKey:   lipgloss.Color("#59C2FF"),
		Error:     lipgloss.Color("#F07178"),
		StatusBar: lipgloss.Color("#CBCCC6"),
	},
	}

	var (
	AppStyle       = lipgloss.NewStyle().Padding(1, 2)
	TitleStyle     = lipgloss.NewStyle().Padding(0, 1)
	PaneStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 1)
	ListPaneStyle  = PaneStyle.Copy().Width(50)
	DetailPaneStyle = PaneStyle.Copy().Padding(1, 2)
	InfoTitleStyle = lipgloss.NewStyle().Bold(true).MarginBottom(1)
	InfoKeyStyle   = lipgloss.NewStyle().Bold(true)
	ErrorStyle     = lipgloss.NewStyle().Bold(true)
	StatusBarStyle = lipgloss.NewStyle().MarginTop(1)
	)

	func ApplyTheme(themeName string) {
	theme, ok := Themes[themeName]
	if !ok {
		theme = Themes["ayu-dark"]
	}

	TitleStyle = TitleStyle.Background(theme.TitleBg).Foreground(theme.TitleFg)
	PaneStyle = PaneStyle.BorderForeground(theme.Border)
	ListPaneStyle = ListPaneStyle.BorderForeground(theme.Border)
	DetailPaneStyle = DetailPaneStyle.BorderForeground(theme.Border)
	InfoTitleStyle = InfoTitleStyle.Foreground(theme.InfoTitle)
	InfoKeyStyle = InfoKeyStyle.Foreground(theme.InfoKey)
	ErrorStyle = ErrorStyle.Foreground(theme.Error)
	StatusBarStyle = StatusBarStyle.Foreground(theme.StatusBar)
}
