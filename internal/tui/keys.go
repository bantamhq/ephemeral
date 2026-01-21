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
	Escape        key.Binding
	Quit          key.Binding
	Help          key.Binding
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
	Escape: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "back"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
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

type helpKeyMap struct {
	KeyMap
	hasSelectedRepo bool
}

func (h helpKeyMap) ShortHelp() []key.Binding {
	if h.hasSelectedRepo {
		return []key.Binding{h.Help, h.Clone, h.ManageFolders, h.Rename, h.Delete, h.Quit}
	}
	return []key.Binding{h.Help, h.NewFolder, h.Rename, h.Delete, h.Quit}
}

func (h helpKeyMap) FullHelp() [][]key.Binding {
	shortcuts := []key.Binding{h.Up, h.Down, h.Left, h.Right, h.Enter, h.Escape}
	editActions := []key.Binding{h.NewFolder, h.Rename, h.Delete}
	meta := []key.Binding{h.Help, h.Quit}

	if !h.hasSelectedRepo {
		return [][]key.Binding{shortcuts, editActions, meta}
	}

	repoActions := []key.Binding{h.Clone, h.CloneDir, h.ManageFolders}
	return [][]key.Binding{shortcuts, editActions, repoActions, meta}
}
