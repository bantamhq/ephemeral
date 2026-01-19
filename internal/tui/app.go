package tui

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rmhubbert/bubbletea-overlay"

	"ephemeral/internal/client"
)

type modalState int

const (
	modalNone modalState = iota
	modalCreateFolder
	modalRenameRepo
	modalDeleteRepo
	modalDeleteFolder
	modalToggleVisibility
	modalMoveRepo
	modalCloneDir
)

type Model struct {
	client    *client.Client
	namespace string
	server    string

	tree     []*TreeNode
	flatTree []*TreeNode
	folders  []client.Folder
	cursor   int
	loading  bool
	err      error

	modal        modalState
	dialog       DialogModel
	folderPicker FolderPickerModel
	actionTarget *TreeNode
	statusMsg    string

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
		if m.modal != modalNone {
			return m.handleModalKey(msg)
		}
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case dataLoadedMsg:
		m.loading = false
		m.folders = msg.folders
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

	case DialogSubmitMsg:
		return m.handleDialogSubmit(msg)

	case DialogCancelMsg:
		m.modal = modalNone
		m.actionTarget = nil
		return m, nil

	case FolderSelectedMsg:
		return m.handleFolderSelected(msg)

	case FolderPickerCancelMsg:
		m.modal = modalNone
		m.actionTarget = nil
		return m, nil

	case FolderCreatedMsg:
		m.statusMsg = "Folder created: " + msg.Folder.Name
		return m, m.loadData()

	case RepoUpdatedMsg:
		m.statusMsg = "Repo updated: " + msg.Repo.Name
		return m, m.loadData()

	case RepoDeletedMsg:
		m.statusMsg = "Repo deleted"
		return m, m.loadData()

	case FolderDeletedMsg:
		m.statusMsg = "Folder deleted"
		return m, m.loadData()

	case CloneStartedMsg:
		m.statusMsg = "Cloning " + msg.RepoName + "..."
		return m, nil

	case CloneCompletedMsg:
		m.statusMsg = "Cloned " + msg.RepoName + " to " + msg.Dir
		return m, nil

	case CloneFailedMsg:
		m.statusMsg = "Clone failed: " + msg.Err.Error()
		return m, nil

	case ActionErrorMsg:
		m.statusMsg = msg.Operation + " failed: " + msg.Err.Error()
		return m, nil
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.statusMsg = ""

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
			if node.Kind == NodeFolder || node.Kind == NodeRoot {
				node.Expanded = !node.Expanded
				m.flatTree = FlattenTree(m.tree)
				m.clampCursor()
			}
		}
		return m, nil

	case key.Matches(msg, m.keys.NewFolder):
		return m.openCreateFolder()

	case key.Matches(msg, m.keys.Rename):
		return m.openRename()

	case key.Matches(msg, m.keys.Delete):
		return m.openDelete()

	case key.Matches(msg, m.keys.Visibility):
		return m.openToggleVisibility()

	case key.Matches(msg, m.keys.Move):
		return m.openMove()

	case key.Matches(msg, m.keys.Clone):
		return m.executeClone("")

	case key.Matches(msg, m.keys.CloneDir):
		return m.openCloneDir()
	}

	return m, nil
}

func (m Model) handleModalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.modal {
	case modalMoveRepo:
		var cmd tea.Cmd
		m.folderPicker, cmd = m.folderPicker.Update(msg)
		return m, cmd
	default:
		var cmd tea.Cmd
		m.dialog, cmd = m.dialog.Update(msg)
		return m, cmd
	}
}

func (m Model) selectedNode() *TreeNode {
	if m.cursor >= 0 && m.cursor < len(m.flatTree) {
		return m.flatTree[m.cursor]
	}
	return nil
}

func (m Model) openCreateFolder() (tea.Model, tea.Cmd) {
	var parentID *string
	node := m.selectedNode()
	if node != nil {
		switch node.Kind {
		case NodeRoot:
		case NodeFolder:
			parentID = &node.ID
		case NodeRepo:
			if node.Repo != nil {
				parentID = node.Repo.FolderID
			}
		}
	}

	m.modal = modalCreateFolder
	m.dialog = NewNameInputDialog("New Folder", "Enter folder name:", "folder-name")
	m.actionTarget = &TreeNode{ParentID: parentID}
	return m, m.dialog.Init()
}

func (m Model) openRename() (tea.Model, tea.Cmd) {
	node := m.selectedNode()
	if node == nil || node.Kind != NodeRepo {
		return m, nil
	}

	m.modal = modalRenameRepo
	m.dialog = NewNameInputDialog("Rename Repo", "Enter new name:", node.Name)
	m.dialog.SetValue(node.Name)
	m.actionTarget = node
	return m, m.dialog.Init()
}

func (m Model) openDelete() (tea.Model, tea.Cmd) {
	node := m.selectedNode()
	if node == nil || node.Kind == NodeRoot {
		return m, nil
	}

	m.actionTarget = node
	if node.Kind == NodeRepo {
		m.modal = modalDeleteRepo
		m.dialog = NewConfirmDialog(
			"Delete Repo",
			fmt.Sprintf("Are you sure you want to delete '%s'?\nThis cannot be undone.", node.Name),
		)
	} else {
		m.modal = modalDeleteFolder
		m.dialog = NewConfirmDialog(
			"Delete Folder",
			fmt.Sprintf("Are you sure you want to delete folder '%s'?\nContents will be moved to root.", node.Name),
		)
	}
	return m, nil
}

func (m Model) openToggleVisibility() (tea.Model, tea.Cmd) {
	node := m.selectedNode()
	if node == nil || node.Kind != NodeRepo || node.Repo == nil {
		return m, nil
	}

	newVisibility := "public"
	if node.Repo.Public {
		newVisibility = "private"
	}

	m.modal = modalToggleVisibility
	m.dialog = NewConfirmDialog(
		"Change Visibility",
		fmt.Sprintf("Make '%s' %s?", node.Name, newVisibility),
	)
	m.actionTarget = node
	return m, nil
}

func (m Model) openMove() (tea.Model, tea.Cmd) {
	node := m.selectedNode()
	if node == nil || node.Kind != NodeRepo {
		return m, nil
	}

	m.modal = modalMoveRepo
	m.folderPicker = NewFolderPicker(
		fmt.Sprintf("Move '%s' to:", node.Name),
		m.folders,
		nil,
	)
	m.actionTarget = node
	return m, nil
}

func (m Model) openCloneDir() (tea.Model, tea.Cmd) {
	node := m.selectedNode()
	if node == nil || node.Kind != NodeRepo {
		return m, nil
	}

	cwd, _ := os.Getwd()
	m.modal = modalCloneDir
	m.dialog = NewInputDialog("Clone to Directory", "Enter path:", cwd)
	m.dialog.SetValue(cwd)
	m.actionTarget = node
	return m, m.dialog.Init()
}

func (m Model) handleDialogSubmit(msg DialogSubmitMsg) (tea.Model, tea.Cmd) {
	modal := m.modal
	target := m.actionTarget
	m.modal = modalNone
	m.actionTarget = nil

	switch modal {
	case modalCreateFolder:
		if msg.Value == "" {
			return m, nil
		}
		var parentID *string
		if target != nil {
			parentID = target.ParentID
		}
		return m, m.createFolder(msg.Value, parentID)

	case modalRenameRepo:
		if msg.Value == "" || target == nil {
			return m, nil
		}
		return m, m.renameRepo(target.ID, msg.Value)

	case modalDeleteRepo:
		if target == nil {
			return m, nil
		}
		return m, m.deleteRepo(target.ID)

	case modalDeleteFolder:
		if target == nil {
			return m, nil
		}
		return m, m.deleteFolder(target.ID)

	case modalToggleVisibility:
		if target == nil || target.Repo == nil {
			return m, nil
		}
		newPublic := !target.Repo.Public
		return m, m.updateRepoVisibility(target.ID, newPublic)

	case modalCloneDir:
		if msg.Value == "" || target == nil {
			return m, nil
		}
		return m.executeClone(msg.Value)
	}

	return m, nil
}

func (m Model) handleFolderSelected(msg FolderSelectedMsg) (tea.Model, tea.Cmd) {
	target := m.actionTarget
	m.modal = modalNone
	m.actionTarget = nil

	if target == nil {
		return m, nil
	}

	return m, m.moveRepo(target.ID, msg.FolderID)
}

func (m Model) createFolder(name string, parentID *string) tea.Cmd {
	return func() tea.Msg {
		folder, err := m.client.CreateFolder(name, parentID)
		if err != nil {
			return ActionErrorMsg{Operation: "create folder", Err: err}
		}
		return FolderCreatedMsg{Folder: *folder}
	}
}

func (m Model) renameRepo(id, name string) tea.Cmd {
	return func() tea.Msg {
		repo, err := m.client.UpdateRepo(id, &name, nil, nil)
		if err != nil {
			return ActionErrorMsg{Operation: "rename repo", Err: err}
		}
		return RepoUpdatedMsg{Repo: *repo}
	}
}

func (m Model) deleteRepo(id string) tea.Cmd {
	return func() tea.Msg {
		if err := m.client.DeleteRepo(id); err != nil {
			return ActionErrorMsg{Operation: "delete repo", Err: err}
		}
		return RepoDeletedMsg{ID: id}
	}
}

func (m Model) deleteFolder(id string) tea.Cmd {
	return func() tea.Msg {
		if err := m.client.DeleteFolder(id, true); err != nil {
			return ActionErrorMsg{Operation: "delete folder", Err: err}
		}
		return FolderDeletedMsg{ID: id}
	}
}

func (m Model) updateRepoVisibility(id string, public bool) tea.Cmd {
	return func() tea.Msg {
		repo, err := m.client.UpdateRepo(id, nil, &public, nil)
		if err != nil {
			return ActionErrorMsg{Operation: "update visibility", Err: err}
		}
		return RepoUpdatedMsg{Repo: *repo}
	}
}

func (m Model) moveRepo(id string, folderID *string) tea.Cmd {
	return func() tea.Msg {
		var fid string
		if folderID != nil {
			fid = *folderID
		}
		repo, err := m.client.UpdateRepo(id, nil, nil, &fid)
		if err != nil {
			return ActionErrorMsg{Operation: "move repo", Err: err}
		}
		return RepoUpdatedMsg{Repo: *repo}
	}
}

func (m Model) executeClone(targetDir string) (tea.Model, tea.Cmd) {
	node := m.selectedNode()
	if node == nil || node.Kind != NodeRepo {
		return m, nil
	}

	if targetDir == "" {
		var err error
		targetDir, err = os.Getwd()
		if err != nil {
			m.statusMsg = "Failed to get current directory: " + err.Error()
			return m, nil
		}
	}

	cloneURL := m.buildCloneURL(node.Name)
	repoName := node.Name

	return m, tea.Batch(
		func() tea.Msg {
			return CloneStartedMsg{RepoName: repoName, Dir: targetDir}
		},
		m.runClone(cloneURL, targetDir, repoName),
	)
}

func (m Model) buildCloneURL(repoName string) string {
	baseURL := m.client.BaseURL()
	token := m.client.Token()

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return baseURL + "/git/" + m.namespace + "/" + repoName + ".git"
	}

	parsed.User = url.UserPassword("x-token", token)
	parsed.Path = "/git/" + m.namespace + "/" + repoName + ".git"
	return parsed.String()
}

func (m Model) runClone(cloneURL, targetDir, repoName string) tea.Cmd {
	return func() tea.Msg {
		destPath := filepath.Join(targetDir, repoName)

		cmd := exec.Command("git", "clone", cloneURL, destPath)
		if err := cmd.Run(); err != nil {
			return CloneFailedMsg{RepoName: repoName, Err: err}
		}
		return CloneCompletedMsg{RepoName: repoName, Dir: destPath}
	}
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

	base := lipgloss.JoinVertical(lipgloss.Left, sections...)

	if m.modal != modalNone {
		return m.overlayModal(base)
	}

	return base
}

func (m Model) overlayModal(background string) string {
	var modalContent string
	if m.modal == modalMoveRepo {
		modalContent = m.folderPicker.View()
	} else {
		modalContent = m.dialog.View()
	}

	return overlay.Composite(modalContent, background, overlay.Center, overlay.Center, 0, 0)
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

	var icon, name, badge string
	switch node.Kind {
	case NodeRoot:
		if node.Expanded {
			icon = StyleRootIcon.Render("▼ ")
		} else {
			icon = StyleRootIcon.Render("▶ ")
		}
		name = StyleRootName.Render(node.Name)
	case NodeFolder:
		if node.Expanded {
			icon = StyleFolderIcon.Render("▼ ")
		} else {
			icon = StyleFolderIcon.Render("▶ ")
		}
		name = StyleFolderName.Render(node.Name)
	case NodeRepo:
		icon = StyleRepoIcon.Render("  ")
		name = StyleRepoName.Render(node.Name)
		if node.Repo != nil && node.Repo.Public {
			badge = StylePublicBadge.Render(" [public]")
		}
	}

	cursor := "  "
	if selected {
		cursor = StyleCursor.Render("> ")
	}

	line := cursor + indent + icon + name + badge
	if selected {
		return StyleSelected.Width(m.width).Render(line)
	}
	return line
}

func (m Model) footerView() string {
	var nodeKind *NodeKind
	if node := m.selectedNode(); node != nil {
		nodeKind = &node.Kind
	}

	help := m.keys.ShortHelp(nodeKind)
	footer := "\n" + help

	if m.statusMsg != "" {
		footer += "\n" + StyleStatusMsg.Render(m.statusMsg)
	}

	return StyleFooter.Width(m.width).Render(footer)
}

func Run(c *client.Client, namespace, server string) error {
	p := tea.NewProgram(
		NewModel(c, namespace, server),
		tea.WithAltScreen(),
	)

	_, err := p.Run()
	return err
}
