package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"ephemeral/internal/client"
)

type Model struct {
	client    *client.Client
	namespace string
	server    string

	tree     []*TreeNode
	flatTree []*TreeNode
	cursor   int
	loading  bool
	err      error

	spinner spinner.Model
	width   int
	height  int
	keys    KeyMap
}

type dataLoadedMsg struct {
	folders []client.Folder
	repos   []client.Repo
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
	return tea.Batch(m.spinner.Tick, m.loadData())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case dataLoadedMsg:
		m.loading = false
		m.tree = BuildTree(msg.folders, msg.repos)
		m.flatTree = FlattenTree(m.tree)
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
		if m.cursor < len(m.flatTree)-1 {
			m.cursor++
		}
		return m, nil

	case key.Matches(msg, m.keys.Select):
		if m.cursor < len(m.flatTree) {
			node := m.flatTree[m.cursor]
			if node.Kind == NodeFolder {
				node.Expanded = !node.Expanded
				m.flatTree = FlattenTree(m.tree)
				m.clampCursor()
			}
		}
		return m, nil
	}

	return m, nil
}

func (m *Model) clampCursor() {
	if m.cursor >= len(m.flatTree) {
		m.cursor = len(m.flatTree) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m Model) loadData() tea.Cmd {
	return func() tea.Msg {
		folders, _, err := m.client.ListFolders("", 0)
		if err != nil {
			return errMsg{err}
		}

		repos, _, err := m.client.ListRepos("", 0)
		if err != nil {
			return errMsg{err}
		}

		return dataLoadedMsg{folders: folders, repos: repos}
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
		sections = append(sections, m.treeView())
	}

	sections = append(sections, m.footerView())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) headerView() string {
	title := fmt.Sprintf("ephemeral · %s · %s", m.namespace, m.server)

	var stats string
	if !m.loading {
		repoCount := m.countRepos()
		stats = fmt.Sprintf("%d repos", repoCount)
	}

	leftWidth := lipgloss.Width(title)
	rightWidth := lipgloss.Width(stats)
	padding := m.width - leftWidth - rightWidth - 2

	if padding < 1 {
		padding = 1
	}

	header := title + strings.Repeat(" ", padding) + stats

	return StyleHeader.Width(m.width).Render(header)
}

func (m Model) countRepos() int {
	count := 0
	for _, node := range m.flatTree {
		if node.Kind == NodeRepo {
			count++
		}
	}
	return count
}

func (m Model) loadingView() string {
	return fmt.Sprintf("\n  %s Loading...\n", m.spinner.View())
}

func (m Model) errorView() string {
	return StyleError.Render(fmt.Sprintf("\n  Error: %v\n", m.err))
}

func (m Model) treeView() string {
	if len(m.flatTree) == 0 {
		return StyleSubtle.Render("\n  No repositories found\n")
	}

	var b strings.Builder
	b.WriteString("\n")

	for i, node := range m.flatTree {
		line := m.renderNode(node, i == m.cursor)
		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}

func (m Model) renderNode(node *TreeNode, selected bool) string {
	indent := strings.Repeat("  ", node.Depth)

	var icon, name string
	if node.Kind == NodeFolder {
		if node.Expanded {
			icon = StyleFolderIcon.Render("▼ ")
		} else {
			icon = StyleFolderIcon.Render("▶ ")
		}
		name = StyleFolderName.Render(node.Name + "/")
	} else {
		icon = StyleRepoIcon.Render("  ")
		name = StyleRepoName.Render(node.Name)
	}

	cursor := "  "
	if selected {
		cursor = StyleCursor.Render("> ")
	}

	line := cursor + indent + icon + name
	if selected {
		return StyleSelected.Width(m.width).Render(line)
	}
	return line
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
