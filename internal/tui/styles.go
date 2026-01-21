package tui

import "github.com/charmbracelet/lipgloss"

type Colors struct {
	Primary       lipgloss.Color
	PrimaryDark   lipgloss.Color
	TextOnPrimary lipgloss.Color
	Text          lipgloss.Color
	TextMuted     lipgloss.Color
	Surface       lipgloss.Color
	Error         lipgloss.Color
	CommitHash    lipgloss.Color
	GitAdded      lipgloss.Color
	GitRemoved    lipgloss.Color
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
	Namespace lipgloss.Style
	Help      lipgloss.Style
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
	Picker PickerStyles
	Commit CommitStyles
	Tree   TreeStyles
	Footer FooterStyles
	Common CommonStyles
}

var Styles = NewStyles()

func NewStyles() *StyleConfig {
	colors := Colors{
		Primary:       lipgloss.Color("62"),
		PrimaryDark:   lipgloss.Color("60"),
		TextOnPrimary: lipgloss.Color("230"),
		Text:          lipgloss.Color("255"),
		TextMuted:     lipgloss.Color("243"),
		Surface:       lipgloss.Color("236"),
		Error:         lipgloss.Color("9"),
		CommitHash:    lipgloss.Color("109"),
		GitAdded:      lipgloss.Color("71"),
		GitRemoved:    lipgloss.Color("203"),
	}

	titleStyle := lipgloss.NewStyle().Foreground(colors.Text)
	metaStyle := lipgloss.NewStyle().Foreground(colors.TextMuted)

	repoNormal := RepoItemStyles{
		Base:  lipgloss.NewStyle().PaddingLeft(2).Faint(true),
		Title: titleStyle,
	}

	repoCursor := RepoItemStyles{
		Base:  lipgloss.NewStyle().PaddingLeft(2),
		Title: titleStyle,
	}

	repoActive := RepoItemStyles{
		Base: lipgloss.NewStyle().
			Border(lipgloss.ThickBorder(), false, false, false, true).
			BorderForeground(colors.PrimaryDark).
			PaddingLeft(1),
		Title: titleStyle.Bold(true),
	}

	editingStyle := lipgloss.NewStyle().
		Background(colors.Surface).
		Foreground(colors.Text).
		Bold(true)

	return &StyleConfig{
		Colors: colors,

		Folder: FolderStyles{
			Selected: lipgloss.NewStyle().
				Background(colors.PrimaryDark).
				Foreground(colors.TextOnPrimary).
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
				Foreground(colors.TextMuted),
			TabBorderActive: lipgloss.NewStyle().
				Foreground(colors.PrimaryDark),
		},

		Dialog: DialogStyles{
			Box: lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colors.Primary).
				Padding(1, 2).
				Background(colors.Surface),
			Button: lipgloss.NewStyle().
				Padding(0, 2).
				Background(colors.Surface).
				Foreground(colors.Text).
				Faint(true),
			ButtonFocused: lipgloss.NewStyle().
				Padding(0, 2).
				Background(colors.Primary).
				Foreground(colors.TextOnPrimary).
				Bold(true),
			Hint: lipgloss.NewStyle().
				Foreground(colors.TextMuted).
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

		Picker: PickerStyles{
			Selected: lipgloss.NewStyle().Reverse(true),
		},

		Commit: CommitStyles{
			Hash: lipgloss.NewStyle().
				Foreground(colors.CommitHash),
			Stat: lipgloss.NewStyle().
				Foreground(colors.TextMuted).
				Faint(true),
			StatAdded: lipgloss.NewStyle().
				Foreground(colors.GitAdded).
				Faint(true),
			StatRemoved: lipgloss.NewStyle().
				Foreground(colors.GitRemoved).
				Faint(true),
		},

		Tree: TreeStyles{
			Enumerator: lipgloss.NewStyle().
				Foreground(colors.TextMuted).
				Padding(0, 1),
			Dir: lipgloss.NewStyle().
				Foreground(colors.Text).
				Bold(true),
		},

		Footer: FooterStyles{
			Namespace: lipgloss.NewStyle().
				Background(colors.Primary).
				Foreground(colors.TextOnPrimary).
				Bold(true).
				Padding(0, 1),
			Help: lipgloss.NewStyle().
				Background(colors.Surface).
				Foreground(colors.TextMuted),
		},

		Common: CommonStyles{
			Header: lipgloss.NewStyle().
				Bold(true).
				Foreground(colors.TextOnPrimary).
				Background(colors.Primary),
			MetaText: metaStyle,
			Error: lipgloss.NewStyle().
				Foreground(colors.Error),
		},
	}
}
