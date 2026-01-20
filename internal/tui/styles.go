package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorPrimary       = lipgloss.Color("62")
	colorPrimaryDark   = lipgloss.Color("60")
	colorTextOnPrimary = lipgloss.Color("230")
	colorText          = lipgloss.Color("255")
	colorTextMuted     = lipgloss.Color("243")
	colorSurface       = lipgloss.Color("236")
	colorWarning       = lipgloss.Color("11")
	colorError         = lipgloss.Color("9")
)

var (
	StyleFolderSelected = lipgloss.NewStyle().
				Background(colorPrimaryDark).
				Foreground(colorTextOnPrimary).
				Bold(true)

	StyleRepoSelected = lipgloss.NewStyle().
				Border(lipgloss.ThickBorder(), false, false, false, true).
				BorderForeground(colorPrimaryDark).
				PaddingLeft(1)

	StyleEditing = lipgloss.NewStyle().
			Background(colorSurface).
			Foreground(colorText).
			Bold(true)

	StyleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorTextOnPrimary).
			Background(colorPrimary)

	StyleRepoTitle = lipgloss.NewStyle().
			Foreground(colorText)

	StyleMetaText = lipgloss.NewStyle().
			Foreground(colorTextMuted)

	StyleError = lipgloss.NewStyle().
			Foreground(colorError)

	StyleStatusMsg = lipgloss.NewStyle().
			Foreground(colorWarning).
			Italic(true)

	StyleFooterNamespace = lipgloss.NewStyle().
				Background(colorPrimary).
				Foreground(colorTextOnPrimary).
				Bold(true).
				Padding(0, 1)

	StyleFooterHelp = lipgloss.NewStyle().
			Background(colorSurface).
			Foreground(colorTextMuted).
			PaddingLeft(1)

	StyleDialogBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(1, 2).
			Background(colorSurface)

	StyleDialogButton = lipgloss.NewStyle().
				Padding(0, 2).
				Background(colorSurface).
				Foreground(colorText).
				Faint(true)

	StyleDialogButtonFocused = lipgloss.NewStyle().
					Padding(0, 2).
					Background(colorPrimary).
					Foreground(colorTextOnPrimary).
					Bold(true)

	StyleDialogHint = lipgloss.NewStyle().
			Foreground(colorTextMuted).
			Italic(true)

	StylePickerSelected = lipgloss.NewStyle().
				Reverse(true)
)
