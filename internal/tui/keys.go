package tui

import "github.com/charmbracelet/bubbles/key"

const (
	columnFolders = 0
	columnRepos   = 1
	columnDetail  = 2
)

type KeyMap struct {
	Up            key.Binding
	Down          key.Binding
	Left          key.Binding
	Right         key.Binding
	Enter         key.Binding
	Tab           key.Binding
	Escape        key.Binding
	Quit          key.Binding
	NewFolder     key.Binding
	Rename        key.Binding
	Delete        key.Binding
	Clone         key.Binding
	CloneDir      key.Binding
	ManageFolders key.Binding
}

var DefaultKeyMap = KeyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("k/↑", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("j/↓", "down"),
	),
	Left: key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("h/←", "left column"),
	),
	Right: key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("l/→", "right column"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "switch tab"),
	),
	Escape: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "back"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	NewFolder: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "new folder"),
	),
	Rename: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "rename"),
	),
	Delete: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "delete"),
	),
	Clone: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "clone"),
	),
	CloneDir: key.NewBinding(
		key.WithKeys("C"),
		key.WithHelp("C", "clone to dir"),
	),
	ManageFolders: key.NewBinding(
		key.WithKeys("m"),
		key.WithHelp("m", "manage folders"),
	),
}

func (k KeyMap) ShortHelp(focusedColumn int) string {
	switch focusedColumn {
	case columnFolders:
		return "n new folder • r rename • d delete"
	case columnRepos:
		return "tab switch tab • enter details • c clone • m folders"
	case columnDetail:
		return "↑↓ scroll • tab switch tab • esc back"
	default:
		return ""
	}
}
