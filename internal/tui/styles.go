package tui

import "github.com/charmbracelet/lipgloss"

var (
	StyleSelected = lipgloss.NewStyle().
			Background(lipgloss.Color("62")).
			Foreground(lipgloss.Color("230")).
			Bold(true)

	StyleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("62"))

	StyleSubtle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

	StyleError = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9"))

	StyleStatusMsg = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")).
			Italic(true)

	StyleEditing = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("255")).
			Bold(true)

	StyleFooterNamespace = lipgloss.NewStyle().
				Background(lipgloss.Color("62")).
				Foreground(lipgloss.Color("230")).
				Bold(true).
				Padding(0, 1)

	StyleFooterHelp = lipgloss.NewStyle().
			Background(lipgloss.Color("237")).
			Foreground(lipgloss.Color("243")).
			PaddingLeft(1)

	StyleDialogBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 2).
			Background(lipgloss.Color("235"))

	StyleDialogButton = lipgloss.NewStyle().
				Padding(0, 2).
				Background(lipgloss.Color("240")).
				Foreground(lipgloss.Color("252"))

	StyleDialogButtonFocused = lipgloss.NewStyle().
					Padding(0, 2).
					Background(lipgloss.Color("62")).
					Foreground(lipgloss.Color("230")).
					Bold(true)

	StyleDialogHint = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Italic(true)

	StyleMetaText = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

)
