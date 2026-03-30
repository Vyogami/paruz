package models

type Package struct {
	Name      string
	Version   string
	Desc      string
	Repo      string
	Installed bool
}

// FilterValue implements the list.Item interface for Charmbracelet bubbles list
func (p Package) FilterValue() string {
	return p.Name
}

// Title implements list.DefaultItem interface
func (p Package) Title() string {
	if p.Installed {
		return p.Name + " [installed]"
	}
	return p.Name
}

// Description implements list.DefaultItem interface
func (p Package) Description() string {
	return p.Version + " - " + p.Desc
}
