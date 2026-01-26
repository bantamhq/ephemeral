package tui

import "github.com/charmbracelet/lipgloss"

type Colors struct {
	Primary        lipgloss.AdaptiveColor
	Subdued        lipgloss.AdaptiveColor
	PrimaryReverse lipgloss.AdaptiveColor
	Subtle         lipgloss.AdaptiveColor
	Success        lipgloss.AdaptiveColor
	Critical       lipgloss.AdaptiveColor
}

type FolderStyles struct {
	Selected lipgloss.Style
	Editing  lipgloss.Style
}

type RepoItemStyles struct {
	Base  lipgloss.Style
	Title lipgloss.Style
}

type RepoStyles struct {
	Normal  RepoItemStyles
	Cursor  RepoItemStyles
	Active  RepoItemStyles
	Editing lipgloss.Style
}

type DetailStyles struct {
	TabBorder       lipgloss.Style
	TabBorderActive lipgloss.Style
}

type DialogStyles struct {
	Box           lipgloss.Style
	Button        lipgloss.Style
	ButtonFocused lipgloss.Style
	Hint          lipgloss.Style
}

type HelpStyles struct {
	Box   lipgloss.Style
	Title lipgloss.Style
}

type PickerStyles struct {
	Selected lipgloss.Style
}

type CommitStyles struct {
	Hash        lipgloss.Style
	Stat        lipgloss.Style
	StatAdded   lipgloss.Style
	StatRemoved lipgloss.Style
}

type TreeStyles struct {
	Enumerator lipgloss.Style
	Dir        lipgloss.Style
}

type FooterStyles struct {
	Namespace     lipgloss.Style
	Help          lipgloss.Style
	StatusMessage lipgloss.Style
}

type ErrorStyles struct {
	Box   lipgloss.Style
	Title lipgloss.Style
}

type CommonStyles struct {
	Header   lipgloss.Style
	MetaText lipgloss.Style
	Error    lipgloss.Style
}

type StyleConfig struct {
	Colors Colors
	Folder FolderStyles
	Repo   RepoStyles
	Detail DetailStyles
	Dialog DialogStyles
	Help   HelpStyles
	Error  ErrorStyles
	Picker PickerStyles
	Commit CommitStyles
	Tree   TreeStyles
	Footer FooterStyles
	Common CommonStyles
}

var Styles = NewStyles()

// NewStyles creates a new StyleConfig with the default TUI styles.
func NewStyles() *StyleConfig {
	colors := Colors{
		Primary:        lipgloss.AdaptiveColor{Light: "240", Dark: "252"},
		Subdued:        lipgloss.AdaptiveColor{Light: "235", Dark: "249"},
		PrimaryReverse: lipgloss.AdaptiveColor{Light: "255", Dark: "235"},
		Subtle:         lipgloss.AdaptiveColor{Light: "243", Dark: "246"},
		Success:        lipgloss.AdaptiveColor{Light: "2", Dark: "2"},
		Critical:       lipgloss.AdaptiveColor{Light: "1", Dark: "1"},
	}

	repoNormal := RepoItemStyles{
		Base:  lipgloss.NewStyle().PaddingLeft(2).Faint(true),
		Title: lipgloss.NewStyle(),
	}

	repoCursor := RepoItemStyles{
		Base: lipgloss.NewStyle().
			Border(lipgloss.ThickBorder(), false, false, false, true).
			BorderForeground(colors.Subtle).
			PaddingLeft(1),
		Title: lipgloss.NewStyle(),
	}

	repoActive := RepoItemStyles{
		Base: lipgloss.NewStyle().
			Border(lipgloss.ThickBorder(), false, false, false, true).
			BorderForeground(colors.Subdued).
			PaddingLeft(1),
		Title: lipgloss.NewStyle().Bold(true),
	}

	editingStyle := lipgloss.NewStyle().
		Bold(true)

	return &StyleConfig{
		Colors: colors,

		Folder: FolderStyles{
			Selected: lipgloss.NewStyle().
				Background(colors.Subdued).
				Foreground(colors.PrimaryReverse).
				Bold(true),
			Editing: editingStyle,
		},

		Repo: RepoStyles{
			Normal:  repoNormal,
			Cursor:  repoCursor,
			Active:  repoActive,
			Editing: editingStyle,
		},

		Detail: DetailStyles{
			TabBorder: lipgloss.NewStyle().
				Faint(true),
			TabBorderActive: lipgloss.NewStyle().
				Foreground(colors.Subdued),
		},

		Dialog: DialogStyles{
			Box: lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colors.Primary).
				Padding(1, 2),
			Button: lipgloss.NewStyle().
				Padding(0, 2).
				Faint(true),
			ButtonFocused: lipgloss.NewStyle().
				Padding(0, 2).
				Background(colors.Primary).
				Foreground(colors.PrimaryReverse).
				Bold(true),
			Hint: lipgloss.NewStyle().
				Faint(true).
				Italic(true),
		},

		Help: HelpStyles{
			Box: lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colors.Primary).
				Padding(1, 2),
			Title: lipgloss.NewStyle().
				Foreground(colors.Primary).
				Bold(true),
		},

		Error: ErrorStyles{
			Box: lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colors.Critical).
				Padding(1, 2),
			Title: lipgloss.NewStyle().
				Foreground(colors.Critical).
				Bold(true),
		},

		Picker: PickerStyles{
			Selected: lipgloss.NewStyle().Reverse(true),
		},

		Commit: CommitStyles{
			Hash: lipgloss.NewStyle().
				Foreground(colors.Subtle),
			Stat: lipgloss.NewStyle().
				Faint(true),
			StatAdded: lipgloss.NewStyle().
				Foreground(colors.Success).
				Faint(true),
			StatRemoved: lipgloss.NewStyle().
				Foreground(colors.Critical).
				Faint(true),
		},

		Tree: TreeStyles{
			Enumerator: lipgloss.NewStyle().
				Faint(true).
				Padding(0, 1),
			Dir: lipgloss.NewStyle().
				Bold(true),
		},

		Footer: FooterStyles{
			Namespace: lipgloss.NewStyle().
				Background(colors.Primary).
				Foreground(colors.PrimaryReverse).
				Bold(true).
				Padding(0, 1),
			Help: lipgloss.NewStyle().
				Faint(true),
			StatusMessage: lipgloss.NewStyle().
				Faint(true),
		},

		Common: CommonStyles{
			Header: lipgloss.NewStyle().
				Bold(true).
				Foreground(colors.PrimaryReverse).
				Background(colors.Primary),
			MetaText: lipgloss.NewStyle().Faint(true),
			Error:    lipgloss.NewStyle().Foreground(colors.Critical),
		},
	}
}
