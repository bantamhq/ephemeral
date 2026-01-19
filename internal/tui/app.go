package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"

	"ephemeral/internal/client"
)

type Model struct {
	client    *client.Client
	namespace string
	server    string

	repos    []client.Repo
	cursor   int
	loading  bool
	err      error

	spinner spinner.Model
	width   int
	height  int
	keys    KeyMap
}

type reposLoadedMsg struct {
	repos []client.Repo
}

type errMsg struct {
	err error
}

func NewModel(c *client.Client, namespace, server string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return Model{
		client:    c,
		namespace: namespace,
		server:    server,
		loading:   true,
		spinner:   s,
		keys:      DefaultKeyMap,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.loadRepos())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case reposLoadedMsg:
		m.loading = false
		m.repos = msg.repos
		return m, nil

	case errMsg:
		m.loading = false
		m.err = msg.err
		return m, nil

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit

	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.repos)-1 {
			m.cursor++
		}
		return m, nil
	}

	return m, nil
}

func (m Model) loadRepos() tea.Cmd {
	return func() tea.Msg {
		repos, _, err := m.client.ListRepos("", 0)
		if err != nil {
			return errMsg{err}
		}
		return reposLoadedMsg{repos}
	}
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	sections := []string{
		m.headerView(),
	}

	if m.loading {
		sections = append(sections, m.loadingView())
	} else if m.err != nil {
		sections = append(sections, m.errorView())
	} else {
		sections = append(sections, m.repoListView())
	}

	sections = append(sections, m.footerView())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) headerView() string {
	title := fmt.Sprintf("ephemeral · %s · %s", m.namespace, m.server)

	var repoCount string
	if !m.loading {
		repoCount = fmt.Sprintf("%d repos", len(m.repos))
	}

	leftWidth := lipgloss.Width(title)
	rightWidth := lipgloss.Width(repoCount)
	padding := m.width - leftWidth - rightWidth - 2

	if padding < 1 {
		padding = 1
	}

	header := title + strings.Repeat(" ", padding) + repoCount

	return StyleHeader.Width(m.width).Render(header)
}

func (m Model) loadingView() string {
	return fmt.Sprintf("\n  %s Loading repositories...\n", m.spinner.View())
}

func (m Model) errorView() string {
	return StyleError.Render(fmt.Sprintf("\n  Error: %v\n", m.err))
}

func (m Model) repoListView() string {
	if len(m.repos) == 0 {
		return StyleSubtle.Render("\n  No repositories found\n")
	}

	var b strings.Builder
	b.WriteString("\n")

	for i, repo := range m.repos {
		cursor := "  "
		if i == m.cursor {
			cursor = StyleCursor.Render("> ")
		}

		name := StyleRepoName.Render(repo.Name)

		var lastPush string
		if repo.LastPushAt != nil {
			lastPush = humanize.Time(*repo.LastPushAt)
		} else {
			lastPush = "never pushed"
		}

		size := humanize.Bytes(uint64(repo.SizeBytes))

		meta := StyleRepoMeta.Render(fmt.Sprintf("  %s  %s", lastPush, size))

		line := cursor + name + meta

		if i == m.cursor {
			line = StyleSelected.Width(m.width).Render(line)
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}

func (m Model) footerView() string {
	help := m.keys.ShortHelp()
	return StyleFooter.Width(m.width).Render("\n" + help)
}

func Run(c *client.Client, namespace, server string) error {
	p := tea.NewProgram(
		NewModel(c, namespace, server),
		tea.WithAltScreen(),
	)

	_, err := p.Run()
	return err
}
