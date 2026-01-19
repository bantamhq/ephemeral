package tui

import "github.com/charmbracelet/bubbles/key"

type KeyMap struct {
	Up         key.Binding
	Down       key.Binding
	Select     key.Binding
	Quit       key.Binding
	NewFolder  key.Binding
	Rename     key.Binding
	Delete     key.Binding
	Visibility key.Binding
	Move       key.Binding
	Clone      key.Binding
	CloneDir   key.Binding
}

var DefaultKeyMap = KeyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("k/up", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("j/down", "down"),
	),
	Select: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
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
	Visibility: key.NewBinding(
		key.WithKeys("v"),
		key.WithHelp("v", "visibility"),
	),
	Move: key.NewBinding(
		key.WithKeys("m"),
		key.WithHelp("m", "move"),
	),
	Clone: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "clone"),
	),
	CloneDir: key.NewBinding(
		key.WithKeys("C"),
		key.WithHelp("C", "clone to dir"),
	),
}

func (k KeyMap) ShortHelp(nodeKind *NodeKind) string {
	base := "q quit  j/k navigate  n new folder"
	if nodeKind == nil {
		return base
	}

	switch *nodeKind {
	case NodeRepo:
		return base + "  r rename  d delete  v visibility  m move  c/C clone"
	case NodeFolder:
		return base + "  d delete"
	default:
		return base
	}
}
