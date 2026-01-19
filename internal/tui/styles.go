package tui

import "github.com/charmbracelet/lipgloss"

var (
	StyleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12"))

	StyleSubtle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	StyleSelected = lipgloss.NewStyle().
			Background(lipgloss.Color("236"))

	StyleHeader = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1).
			Background(lipgloss.Color("62")).
			Foreground(lipgloss.Color("230"))

	StyleFooter = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Padding(0, 1)

	StyleError = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9"))

	StyleRepoName = lipgloss.NewStyle().
			Bold(true)

	StyleRepoIcon = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	StyleFolderName = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12"))

	StyleFolderIcon = lipgloss.NewStyle().
			Foreground(lipgloss.Color("12"))

	StyleCursor = lipgloss.NewStyle().
			Foreground(lipgloss.Color("12"))

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
			Foreground(lipgloss.Color("8")).
			Italic(true)

	StylePickerSelected = lipgloss.NewStyle().
				Foreground(lipgloss.Color("12")).
				Bold(true)

	StylePublicBadge = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10"))

	StyleStatusMsg = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")).
			Italic(true)
)
