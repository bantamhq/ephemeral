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

	StyleRepoMeta = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	StyleCursor = lipgloss.NewStyle().
			Foreground(lipgloss.Color("12"))
)
