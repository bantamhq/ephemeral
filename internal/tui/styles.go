package tui

import "github.com/charmbracelet/lipgloss"

// Colors
var (
	colorPurple     = lipgloss.Color("62")
	colorPurpleDark = lipgloss.Color("60")
	colorCream      = lipgloss.Color("230")
	colorWhite      = lipgloss.Color("255")
	colorGray       = lipgloss.Color("243")
	colorGrayDark   = lipgloss.Color("237")
	colorGrayDarker = lipgloss.Color("236")
	colorGrayBg     = lipgloss.Color("235")
	colorGrayLight  = lipgloss.Color("240")
	colorGrayText   = lipgloss.Color("252")
	colorYellow     = lipgloss.Color("11")
	colorRed        = lipgloss.Color("9")
)

// Selection styles
var (
	StyleFolderSelected = lipgloss.NewStyle().
				Background(colorPurpleDark).
				Foreground(colorCream).
				Bold(true)

	StyleRepoSelected = lipgloss.NewStyle().
				Border(lipgloss.ThickBorder(), false, false, false, true).
				BorderForeground(colorPurpleDark).
				PaddingLeft(1)

	StyleEditing = lipgloss.NewStyle().
			Background(colorGrayDarker).
			Foreground(colorWhite).
			Bold(true)
)

// Text styles
var (
	StyleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorCream).
			Background(colorPurple)

	StyleRepoTitle = lipgloss.NewStyle().
			Foreground(colorWhite)

	StyleMetaText = lipgloss.NewStyle().
			Foreground(colorGray)

	StyleError = lipgloss.NewStyle().
			Foreground(colorRed)

	StyleStatusMsg = lipgloss.NewStyle().
			Foreground(colorYellow).
			Italic(true)
)

// Footer styles
var (
	StyleFooterNamespace = lipgloss.NewStyle().
				Background(colorPurple).
				Foreground(colorCream).
				Bold(true).
				Padding(0, 1)

	StyleFooterHelp = lipgloss.NewStyle().
			Background(colorGrayDark).
			Foreground(colorGray).
			PaddingLeft(1)
)

// Dialog styles
var (
	StyleDialogBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPurple).
			Padding(1, 2).
			Background(colorGrayBg)

	StyleDialogButton = lipgloss.NewStyle().
				Padding(0, 2).
				Background(colorGrayLight).
				Foreground(colorGrayText)

	StyleDialogButtonFocused = lipgloss.NewStyle().
					Padding(0, 2).
					Background(colorPurple).
					Foreground(colorCream).
					Bold(true)

	StyleDialogHint = lipgloss.NewStyle().
			Foreground(colorGray).
			Italic(true)

	StylePickerSelected = lipgloss.NewStyle().
				Reverse(true)
)
