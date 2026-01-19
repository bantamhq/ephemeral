package tui

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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
	modalDeleteRepo
	modalDeleteFolder
	modalToggleVisibility
	modalCloneDir
)

const footerHeight = 1

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

	modal            modalState
	dialog           DialogModel
	actionTarget     *TreeNode
	movingRepo        *TreeNode
	preMoveExpanded   map[string]bool
	moveTargetID      string
	moveToRoot        bool
	recentlyMovedID   string
	expandAfterLoad   string
	editingNode       *TreeNode
	editText          string
	statusMsg         string

	spinner      spinner.Model
	scrollOffset int
	width        int
	height       int
	keys         KeyMap
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
		if m.preMoveExpanded != nil {
			m.restoreFolderStates(m.tree)
			if m.moveTargetID != "" {
				m.expandFolderAndAncestors(m.tree, m.moveTargetID)
			}
			m.preMoveExpanded = nil
		}
		if m.expandAfterLoad != "" {
			m.expandFolderAndAncestors(m.tree, m.expandAfterLoad)
			m.expandAfterLoad = ""
		}
		m.flatTree = FlattenTree(m.tree)
		if m.moveTargetID != "" || m.moveToRoot {
			m.selectFolderByID(m.moveTargetID)
			m.moveTargetID = ""
			m.moveToRoot = false
		} else if m.cursor == 0 && len(m.flatTree) > 1 {
			m.cursor = 1
		}
		m.syncViewportWithCursor()
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

	case FolderCreatedMsg:
		m.statusMsg = "Folder created: " + msg.Folder.Name
		return m, m.loadData()

	case FolderUpdatedMsg:
		m.statusMsg = "Folder updated: " + msg.Folder.Name
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
	m.recentlyMovedID = ""

	if m.editingNode != nil {
		return m.handleEditMode(msg)
	}

	if m.movingRepo != nil {
		return m.handleMoveMode(msg)
	}

	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit

	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
			m.syncViewportWithCursor()
		}
		return m, nil

	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.flatTree)-1 {
			m.cursor++
			m.syncViewportWithCursor()
		}
		return m, nil

	case key.Matches(msg, m.keys.Left):
		m.setFolderExpanded(false)
		return m, nil

	case key.Matches(msg, m.keys.Right):
		m.setFolderExpanded(true)
		return m, nil

	case key.Matches(msg, m.keys.NewFolder):
		return m.openCreateFolder()

	case key.Matches(msg, m.keys.ToggleAll):
		m.toggleAllFolders()
		m.flatTree = FlattenTree(m.tree)
		m.clampCursor()
		m.syncViewportWithCursor()
		return m, nil

	case key.Matches(msg, m.keys.Rename):
		return m.startRename()

	case key.Matches(msg, m.keys.Delete):
		return m.openDelete()

	case key.Matches(msg, m.keys.Visibility):
		return m.openToggleVisibility()

	case key.Matches(msg, m.keys.Move):
		return m.startMove()

	case key.Matches(msg, m.keys.Clone):
		return m.executeClone("")

	case key.Matches(msg, m.keys.CloneDir):
		return m.openCloneDir()
	}

	return m, nil
}

func (m Model) handleMoveMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		m.movingRepo = nil
		m.restoreFolderStates(m.tree)
		m.preMoveExpanded = nil
		m.flatTree = FlattenTree(m.tree)
		m.clampCursor()
		m.syncViewportWithCursor()
		m.statusMsg = "Move cancelled"
		return m, nil

	case key.Matches(msg, m.keys.Up):
		m.moveCursorToFolder(-1)
		m.ensureFolderContentsVisible()
		return m, nil

	case key.Matches(msg, m.keys.Down):
		m.moveCursorToFolder(1)
		m.ensureFolderContentsVisible()
		return m, nil

	case key.Matches(msg, m.keys.Left):
		m.setFolderExpanded(false)
		return m, nil

	case key.Matches(msg, m.keys.Right):
		m.setFolderExpanded(true)
		return m, nil

	case key.Matches(msg, m.keys.Select), key.Matches(msg, m.keys.Move):
		return m.confirmMove()
	}

	return m, nil
}

func (m Model) handleEditMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		m.editingNode = nil
		m.editText = ""
		return m, nil

	case tea.KeyEnter:
		return m.submitRename()

	case tea.KeyBackspace:
		if len(m.editText) > 0 {
			m.editText = m.editText[:len(m.editText)-1]
		}
		return m, nil

	case tea.KeyRunes:
		m.editText += string(msg.Runes)
		return m, nil
	}

	return m, nil
}

func (m Model) submitRename() (tea.Model, tea.Cmd) {
	newName := m.editText
	node := m.editingNode

	m.editingNode = nil
	m.editText = ""

	if newName == "" || newName == node.Name {
		return m, nil
	}

	if node.Kind == NodeFolder {
		return m, m.renameFolder(node.ID, newName)
	}
	return m, m.renameRepo(node.ID, newName)
}

func (m *Model) moveCursorToFolder(direction int) {
	for i := m.cursor + direction; i >= 0 && i < len(m.flatTree); i += direction {
		if m.flatTree[i].IsContainer() {
			m.cursor = i
			return
		}
	}
}

func (m *Model) setFolderExpanded(expanded bool) {
	if m.cursor >= len(m.flatTree) {
		return
	}
	node := m.flatTree[m.cursor]
	if node.Kind != NodeFolder || node.Expanded == expanded {
		return
	}
	if !expanded && len(node.Children) == 0 {
		return
	}
	node.Expanded = expanded
	m.flatTree = FlattenTree(m.tree)
}

func (m Model) handleModalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.dialog, cmd = m.dialog.Update(msg)
	return m, cmd
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

func (m Model) startRename() (tea.Model, tea.Cmd) {
	node := m.selectedNode()
	if node == nil || (node.Kind != NodeRepo && node.Kind != NodeFolder) {
		return m, nil
	}

	m.editingNode = node
	m.editText = node.Name
	return m, nil
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

func (m Model) startMove() (tea.Model, tea.Cmd) {
	node := m.selectedNode()
	if node == nil || node.Kind != NodeRepo {
		return m, nil
	}

	m.movingRepo = node
	m.statusMsg = "Moving " + node.Name + " - select destination folder"
	m.preMoveExpanded = m.saveFolderStates(m.tree)
	m.flatTree = FlattenTree(m.tree)
	m.selectContainingFolder(node)
	m.ensureFolderContentsVisible()
	return m, nil
}

func (m *Model) selectContainingFolder(repo *TreeNode) {
	if repo.Repo == nil || repo.Repo.FolderID == nil {
		m.cursor = 0
		return
	}

	folderID := *repo.Repo.FolderID
	for i, node := range m.flatTree {
		if node.Kind == NodeFolder && node.ID == folderID {
			m.cursor = i
			return
		}
	}
	m.cursor = 0
}

func (m Model) confirmMove() (tea.Model, tea.Cmd) {
	target := m.selectedNode()
	if target == nil {
		return m, nil
	}

	var folderID *string
	switch target.Kind {
	case NodeRoot:
		m.moveToRoot = true
		m.moveTargetID = ""
	case NodeFolder:
		folderID = &target.ID
		m.moveTargetID = target.ID
		m.moveToRoot = false
	default:
		m.statusMsg = "Select a folder as the destination"
		return m, nil
	}

	repoID := m.movingRepo.ID
	m.recentlyMovedID = repoID
	m.movingRepo = nil
	return m, m.moveRepo(repoID, folderID)
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
		if parentID != nil {
			m.expandAfterLoad = *parentID
		}
		return m, m.createFolder(msg.Value, parentID)

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

func (m Model) renameFolder(id, name string) tea.Cmd {
	return func() tea.Msg {
		folder, err := m.client.UpdateFolder(id, &name, nil)
		if err != nil {
			return ActionErrorMsg{Operation: "rename folder", Err: err}
		}
		return FolderUpdatedMsg{Folder: *folder}
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
		fid := ""
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

func (m *Model) toggleAllFolders() {
	expand := m.hasCollapsedFolder(m.tree)
	m.setAllFoldersExpanded(m.tree, expand)
}

func (m *Model) hasCollapsedFolder(nodes []*TreeNode) bool {
	for _, node := range nodes {
		if node.Kind == NodeFolder && !node.Expanded && len(node.Children) > 0 {
			return true
		}
		if node.IsContainer() && m.hasCollapsedFolder(node.Children) {
			return true
		}
	}
	return false
}

func (m *Model) setAllFoldersExpanded(nodes []*TreeNode, expanded bool) {
	for _, node := range nodes {
		if node.Kind == NodeFolder {
			if expanded || len(node.Children) > 0 {
				node.Expanded = expanded
			}
		}
		if node.IsContainer() {
			m.setAllFoldersExpanded(node.Children, expanded)
		}
	}
}

func (m *Model) saveFolderStates(nodes []*TreeNode) map[string]bool {
	states := make(map[string]bool)
	m.collectFolderStates(nodes, states)
	return states
}

func (m *Model) collectFolderStates(nodes []*TreeNode, states map[string]bool) {
	for _, node := range nodes {
		if node.Kind == NodeFolder {
			states[node.ID] = node.Expanded
		}
		if node.IsContainer() {
			m.collectFolderStates(node.Children, states)
		}
	}
}

func (m *Model) restoreFolderStates(nodes []*TreeNode) {
	if m.preMoveExpanded == nil {
		return
	}
	for _, node := range nodes {
		if node.Kind == NodeFolder {
			if expanded, ok := m.preMoveExpanded[node.ID]; ok {
				node.Expanded = expanded
			}
		}
		if node.IsContainer() {
			m.restoreFolderStates(node.Children)
		}
	}
}

func (m *Model) expandFolderAndAncestors(nodes []*TreeNode, id string) bool {
	for _, node := range nodes {
		if node.Kind == NodeFolder && node.ID == id {
			node.Expanded = true
			return true
		}
		if node.IsContainer() {
			if m.expandFolderAndAncestors(node.Children, id) {
				if node.Kind == NodeFolder {
					node.Expanded = true
				}
				return true
			}
		}
	}
	return false
}

func (m *Model) selectFolderByID(id string) {
	if id == "" {
		m.cursor = 0
		return
	}
	for i, node := range m.flatTree {
		if node.Kind == NodeFolder && node.ID == id {
			m.cursor = i
			return
		}
	}
	m.cursor = 0
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

	mainHeight := m.height - footerHeight

	sections := []string{
		m.mainContentView(mainHeight),
		m.footerView(),
	}

	base := lipgloss.JoinVertical(lipgloss.Left, sections...)

	if m.modal != modalNone {
		return m.overlayModal(base)
	}

	return base
}

func (m Model) overlayModal(background string) string {
	return overlay.Composite(m.dialog.View(), background, overlay.Center, overlay.Center, 0, 0)
}

func (m Model) mainContentView(height int) string {
	if m.loading {
		return lipgloss.NewStyle().Height(height).Render(m.loadingView())
	}

	if m.err != nil {
		return lipgloss.NewStyle().Height(height).Render(m.errorView())
	}

	return lipgloss.NewStyle().Height(height).Render(m.visibleTreeView(height))
}

func (m Model) visibleTreeView(height int) string {
	if len(m.flatTree) == 0 {
		return StyleSubtle.Render("  No repositories found")
	}

	var b strings.Builder

	end := m.scrollOffset + height
	if end > len(m.flatTree) {
		end = len(m.flatTree)
	}

	for i := m.scrollOffset; i < end; i++ {
		if i > m.scrollOffset {
			b.WriteString("\n")
		}
		line := m.renderNode(m.flatTree[i], i == m.cursor)
		b.WriteString(line)
	}

	return b.String()
}

func (m Model) loadingView() string {
	return fmt.Sprintf("\n  %s Loading...\n", m.spinner.View())
}

func (m Model) errorView() string {
	return StyleError.Render(fmt.Sprintf("\n  Error: %v\n", m.err))
}

func (m *Model) syncViewportWithCursor() {
	m.ensureCursorVisible()
}

func (m *Model) ensureCursorVisible() {
	if len(m.flatTree) == 0 {
		return
	}

	viewportHeight := m.height - footerHeight
	if viewportHeight <= 0 {
		return
	}

	top := m.scrollOffset
	bottom := top + viewportHeight - 1

	if m.cursor < top {
		m.scrollOffset = m.cursor
	} else if m.cursor > bottom {
		m.scrollOffset = m.cursor - viewportHeight + 1
	}
}

func (m *Model) ensureFolderContentsVisible() {
	if m.cursor < 0 || m.cursor >= len(m.flatTree) {
		return
	}

	viewportHeight := m.height - footerHeight
	if viewportHeight <= 0 {
		return
	}

	node := m.flatTree[m.cursor]
	if !node.IsContainer() {
		m.ensureCursorVisible()
		return
	}

	contentLines := m.countFolderContents(m.cursor)

	if contentLines <= viewportHeight {
		endLine := m.cursor + contentLines - 1
		if endLine > m.scrollOffset+viewportHeight-1 {
			m.scrollOffset = endLine - viewportHeight + 1
		}
		if m.cursor < m.scrollOffset {
			m.scrollOffset = m.cursor
		}
	} else {
		m.scrollOffset = m.cursor
	}
}

func (m *Model) countFolderContents(cursorPos int) int {
	node := m.flatTree[cursorPos]
	count := 1
	for i := cursorPos + 1; i < len(m.flatTree); i++ {
		if m.flatTree[i].Depth <= node.Depth {
			break
		}
		count++
	}
	return count
}

func (m Model) renderNode(node *TreeNode, selected bool) string {
	indent := strings.Repeat("  ", node.Depth)
	isMoving := m.movingRepo != nil && m.movingRepo.ID == node.ID
	isRecentlyMoved := m.recentlyMovedID != "" && m.recentlyMovedID == node.ID
	isEditing := m.editingNode != nil && m.editingNode.ID == node.ID

	icon, name, badge, details := m.nodeContent(node, isMoving)

	if isEditing {
		line := indent + icon + m.editText + "█"
		return StyleEditing.Width(m.width).Render(line)
	}

	left := indent + icon + name + badge

	if selected {
		return StyleSelected.Width(m.width).Render(m.renderWithDetails(left, details))
	}

	if isMoving {
		return StyleMoving.Render(left)
	}

	if isRecentlyMoved {
		return StyleRecentlyMoved.Width(m.width).Render(m.renderWithDetails(left, details))
	}

	icon, name, details = m.styledNodeContent(node, icon, name, details)
	left = indent + icon + name + badge
	return m.renderWithDetails(left, details)
}

func (m Model) nodeContent(node *TreeNode, isMoving bool) (icon, name, badge, details string) {
	name = node.Name

	switch node.Kind {
	case NodeRoot:
		icon = "◆ "
		details = "last push  repo size"

	case NodeFolder:
		icon = folderIcon(node.Expanded, len(node.Children) > 0)

	case NodeRepo:
		if isMoving {
			badge = " [moving]"
		} else if node.Repo != nil {
			details = fmt.Sprintf("%9s  %9s",
				formatRelativeTime(node.Repo.LastPushAt),
				formatSize(node.Repo.SizeBytes))
		}
	}

	return icon, name, badge, details
}

func folderIcon(expanded, hasChildren bool) string {
	if hasChildren {
		if expanded {
			return "▼ "
		}
		return "▶ "
	}
	if expanded {
		return "▽ "
	}
	return "▷ "
}

func (m Model) styledNodeContent(node *TreeNode, icon, name, details string) (string, string, string) {
	switch node.Kind {
	case NodeRoot:
		icon = StyleRootIcon.Render(icon)
		name = StyleRootName.Render(name)
		if details != "" {
			details = StyleColumnHeader.Render(details)
		}
	case NodeFolder:
		icon = StyleFolderIcon.Render(icon)
		name = StyleFolderName.Render(name)
	case NodeRepo:
		name = StyleRepoName.Render(name)
		if details != "" {
			details = StyleSubtle.Render(details)
		}
	}
	return icon, name, details
}

func (m Model) renderWithDetails(left, details string) string {
	if details == "" {
		return left
	}

	leftLen := lipgloss.Width(left)
	detailsLen := lipgloss.Width(details)
	padding := 1
	gap := m.width - leftLen - detailsLen - padding

	if gap < 2 {
		return left
	}

	return left + strings.Repeat(" ", gap) + details
}

func (m Model) footerView() string {
	namespaceBadge := StyleFooterNamespace.Render(m.namespace)
	badgeWidth := lipgloss.Width(namespaceBadge)

	var helpContent string
	if m.statusMsg != "" {
		helpContent = StyleStatusMsg.Render(m.statusMsg)
	} else {
		var nodeKind *NodeKind
		if node := m.selectedNode(); node != nil {
			nodeKind = &node.Kind
		}
		helpContent = m.keys.ShortHelp(nodeKind, m.movingRepo != nil)
	}

	return namespaceBadge + StyleFooterHelp.Width(m.width-badgeWidth).Render(helpContent)
}

func Run(c *client.Client, namespace, server string) error {
	p := tea.NewProgram(
		NewModel(c, namespace, server),
		tea.WithAltScreen(),
	)

	_, err := p.Run()
	return err
}

func formatSize(bytes int) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)

	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1fG", float64(bytes)/gb)
	case bytes >= mb:
		return fmt.Sprintf("%.1fM", float64(bytes)/mb)
	case bytes >= kb:
		return fmt.Sprintf("%.1fK", float64(bytes)/kb)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

func formatRelativeTime(t *time.Time) string {
	if t == nil {
		return "never"
	}

	elapsed := time.Since(*t)
	hours := elapsed.Hours()

	switch {
	case elapsed < time.Minute:
		return "now"
	case elapsed < time.Hour:
		return fmt.Sprintf("%dm", int(elapsed.Minutes()))
	case hours < 24:
		return fmt.Sprintf("%dh", int(hours))
	case hours < 24*30:
		return fmt.Sprintf("%dd", int(hours/24))
	case hours < 24*365:
		return fmt.Sprintf("%dmo", int(hours/24/30))
	default:
		return fmt.Sprintf("%dy", int(hours/24/365))
	}
}
