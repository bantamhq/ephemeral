package tui

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	modalCloneDir
	modalManageFolders
)

const (
	columnFolders = 0
	columnRepos   = 1
)

const footerHeight = 1
const repoPageSize = 50

type Model struct {
	client    *client.Client
	namespace string
	server    string

	folders     []client.Folder
	repos       []client.Repo
	repoFolders map[string][]client.Folder

	focusedColumn int
	folderCursor  int
	repoCursor    int
	folderScroll  int
	repoScroll    int

	filteredRepos []client.Repo

	repoNextCursor  string
	repoHasMore     bool
	repoLoadingMore bool

	editingFolder *client.Folder
	editingRepo   *client.Repo
	editText      string

	modal        modalState
	dialog       DialogModel
	folderPicker FolderPickerModel

	loading   bool
	err       error
	statusMsg string

	spinner spinner.Model
	width   int
	height  int
	keys    KeyMap
}

func NewModel(c *client.Client, namespace, server string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return Model{
		client:      c,
		namespace:   namespace,
		server:      server,
		loading:     true,
		spinner:     s,
		keys:        DefaultKeyMap,
		repoFolders: make(map[string][]client.Folder),
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
		m.err = nil
		m.folders = msg.folders
		m.repos = msg.repos
		m.repoFolders = msg.repoFolders
		m.repoNextCursor = msg.repoNextCursor
		m.repoHasMore = msg.repoHasMore
		m.filterRepos()
		return m, nil

	case moreReposLoadedMsg:
		m.repoLoadingMore = false
		for _, rwf := range msg.repos {
			m.repos = append(m.repos, rwf.Repo)
			m.repoFolders[rwf.ID] = rwf.Folders
		}
		m.repoNextCursor = msg.nextCursor
		m.repoHasMore = msg.hasMore
		m.filterRepos()
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
		return m, nil

	case FolderCreatedMsg:
		m.statusMsg = "Folder created: " + msg.Folder.Name
		return m, m.loadData()

	case FolderUpdatedMsg:
		m.statusMsg = "Folder renamed: " + msg.Folder.Name
		return m, m.loadData()

	case RepoUpdatedMsg:
		m.statusMsg = "Repo updated: " + msg.Repo.Name
		return m, m.loadData()

	case RepoDeletedMsg:
		m.statusMsg = "Repo deleted"
		if len(m.filteredRepos) <= 1 {
			m.focusedColumn = columnFolders
			m.repoCursor = 0
		} else if m.repoCursor >= len(m.filteredRepos)-1 {
			m.repoCursor--
		}
		return m, m.loadData()

	case FolderDeletedMsg:
		m.statusMsg = "Folder deleted"
		if m.folderCursor > 0 {
			m.folderCursor--
		}
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
		if msg.Operation == "load more repos" {
			m.repoLoadingMore = false
		}
		m.statusMsg = msg.Operation + " failed: " + msg.Err.Error()
		return m, nil

	case FolderPickerCloseMsg:
		m.modal = modalNone
		return m, nil

	case FolderPickerToggleMsg:
		return m.handleFolderToggle(msg)

	case repoFolderAddedMsg:
		if folder := m.findFolder(msg.FolderID); folder != nil {
			m.repoFolders[msg.RepoID] = append(m.repoFolders[msg.RepoID], *folder)
		}
		return m, nil

	case repoFolderRemovedMsg:
		m.repoFolders[msg.RepoID] = m.removeFolderFromList(m.repoFolders[msg.RepoID], msg.FolderID)
		return m, nil
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.statusMsg = ""

	if m.editingFolder != nil || m.editingRepo != nil {
		return m.handleEditMode(msg)
	}

	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit

	case key.Matches(msg, m.keys.Up):
		m.moveCursor(-1)
		return m, nil

	case key.Matches(msg, m.keys.Down):
		m.moveCursor(1)
		if cmd := m.maybeLoadMoreRepos(); cmd != nil {
			return m, cmd
		}
		return m, nil

	case key.Matches(msg, m.keys.Left):
		if m.focusedColumn == columnRepos {
			m.focusedColumn = columnFolders
		}
		return m, nil

	case key.Matches(msg, m.keys.Right):
		if m.focusedColumn == columnFolders && len(m.filteredRepos) > 0 {
			m.focusedColumn = columnRepos
		}
		return m, nil

	case key.Matches(msg, m.keys.Enter):
		if m.focusedColumn == columnFolders && len(m.filteredRepos) > 0 {
			m.focusedColumn = columnRepos
			m.repoCursor = 0
			m.repoScroll = 0
		}
		return m, nil

	case key.Matches(msg, m.keys.NewFolder):
		if m.focusedColumn == columnFolders {
			return m.openCreateFolder()
		}
		return m, nil

	case key.Matches(msg, m.keys.Rename):
		return m.startRename()

	case key.Matches(msg, m.keys.Delete):
		return m.openDelete()

	case key.Matches(msg, m.keys.Clone):
		return m.executeClone("")

	case key.Matches(msg, m.keys.CloneDir):
		return m.openCloneDir()

	case key.Matches(msg, m.keys.ManageFolders):
		return m.openManageFolders()
	}

	return m, nil
}

func (m Model) handleEditMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		m.editingFolder = nil
		m.editingRepo = nil
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
		if len(m.editText) >= 128 {
			return m, nil
		}
		if !validateNameInput(msg.Runes, m.editText) {
			return m, nil
		}
		m.editText += string(msg.Runes)
		return m, nil
	}

	return m, nil
}

func (m Model) submitRename() (tea.Model, tea.Cmd) {
	newName := m.editText
	editingFolder := m.editingFolder
	editingRepo := m.editingRepo

	m.editingFolder = nil
	m.editingRepo = nil
	m.editText = ""

	if newName == "" {
		return m, nil
	}

	if editingFolder != nil {
		if newName == editingFolder.Name {
			return m, nil
		}
		return m, m.renameFolder(editingFolder.ID, newName)
	}

	if editingRepo != nil {
		if newName == editingRepo.Name {
			return m, nil
		}
		return m, m.renameRepo(editingRepo.ID, newName)
	}

	return m, nil
}

func (m *Model) moveCursor(delta int) {
	switch m.focusedColumn {
	case columnFolders:
		m.folderCursor += delta
		if m.folderCursor < 0 {
			m.folderCursor = 0
		}
		maxFolder := len(m.folders)
		if m.folderCursor > maxFolder {
			m.folderCursor = maxFolder
		}
		m.syncFolderScroll()
		m.filterRepos()
		m.repoCursor = 0
		m.repoScroll = 0

	case columnRepos:
		m.repoCursor += delta
		maxRepo := len(m.filteredRepos) - 1
		if m.repoCursor < 0 {
			m.repoCursor = 0
		} else if m.repoCursor > maxRepo && maxRepo >= 0 {
			m.repoCursor = maxRepo
		}
		m.syncRepoScroll()
	}
}

func (m *Model) syncFolderScroll() {
	viewportHeight := m.mainHeight() - 2
	if viewportHeight < 1 {
		viewportHeight = 1
	}

	if m.folderCursor < m.folderScroll {
		m.folderScroll = m.folderCursor
	} else if m.folderCursor >= m.folderScroll+viewportHeight {
		m.folderScroll = m.folderCursor - viewportHeight + 1
	}
}

func (m *Model) syncRepoScroll() {
	viewportHeight := (m.mainHeight() - 2) / 3
	if viewportHeight < 1 {
		viewportHeight = 1
	}

	if m.repoCursor < m.repoScroll {
		m.repoScroll = m.repoCursor
	} else if m.repoCursor >= m.repoScroll+viewportHeight {
		m.repoScroll = m.repoCursor - viewportHeight + 1
	}
}

func (m *Model) maybeLoadMoreRepos() tea.Cmd {
	if m.focusedColumn != columnRepos || !m.repoHasMore || m.repoLoadingMore {
		return nil
	}

	distanceFromEnd := len(m.filteredRepos) - m.repoCursor - 1
	if distanceFromEnd > 5 {
		return nil
	}

	m.repoLoadingMore = true
	return m.loadMoreRepos()
}

func (m *Model) filterRepos() {
	if m.folderCursor == 0 {
		m.filteredRepos = m.repos
		return
	}

	folderIdx := m.folderCursor - 1
	if folderIdx >= len(m.folders) {
		m.filteredRepos = nil
		return
	}

	selectedFolder := m.folders[folderIdx]
	var filtered []client.Repo

	for _, repo := range m.repos {
		folders := m.repoFolders[repo.ID]
		for _, f := range folders {
			if f.ID == selectedFolder.ID {
				filtered = append(filtered, repo)
				break
			}
		}
	}

	m.filteredRepos = filtered
}

func (m Model) handleModalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.modal == modalManageFolders {
		var cmd tea.Cmd
		m.folderPicker, cmd = m.folderPicker.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.dialog, cmd = m.dialog.Update(msg)
	return m, cmd
}

func (m Model) selectedFolder() *client.Folder {
	if m.folderCursor == 0 || m.folderCursor > len(m.folders) {
		return nil
	}
	return &m.folders[m.folderCursor-1]
}

func (m Model) selectedRepo() *client.Repo {
	if m.repoCursor < 0 || m.repoCursor >= len(m.filteredRepos) {
		return nil
	}
	return &m.filteredRepos[m.repoCursor]
}

func (m Model) openCreateFolder() (tea.Model, tea.Cmd) {
	m.modal = modalCreateFolder
	m.dialog = NewNameInputDialog("New Folder", "Enter folder name:", "folder-name")
	return m, m.dialog.Init()
}

func (m Model) startRename() (tea.Model, tea.Cmd) {
	if m.focusedColumn == columnFolders {
		folder := m.selectedFolder()
		if folder == nil {
			return m, nil
		}
		m.editingFolder = folder
		m.editText = folder.Name
		return m, nil
	}

	if m.focusedColumn == columnRepos {
		repo := m.selectedRepo()
		if repo == nil {
			return m, nil
		}
		m.editingRepo = repo
		m.editText = repo.Name
		return m, nil
	}

	return m, nil
}

func (m Model) openDelete() (tea.Model, tea.Cmd) {
	if m.focusedColumn == columnFolders {
		folder := m.selectedFolder()
		if folder == nil {
			return m, nil
		}
		m.modal = modalDeleteFolder
		m.dialog = NewConfirmDialog(
			"Delete Folder",
			fmt.Sprintf("Delete folder '%s'?\nRepos will be unlinked, not deleted.", folder.Name),
		)
		return m, nil
	}

	if m.focusedColumn == columnRepos {
		repo := m.selectedRepo()
		if repo == nil {
			return m, nil
		}
		m.modal = modalDeleteRepo
		m.dialog = NewConfirmDialog(
			"Delete Repo",
			fmt.Sprintf("Delete '%s'?\nThis cannot be undone.", repo.Name),
		)
		return m, nil
	}

	return m, nil
}

func (m Model) openCloneDir() (tea.Model, tea.Cmd) {
	if m.focusedColumn != columnRepos {
		return m, nil
	}

	repo := m.selectedRepo()
	if repo == nil {
		return m, nil
	}

	cwd, _ := os.Getwd()
	m.modal = modalCloneDir
	m.dialog = NewInputDialog("Clone to Directory", "Enter path:", cwd)
	m.dialog.SetValue(cwd)
	return m, m.dialog.Init()
}

func (m Model) openManageFolders() (tea.Model, tea.Cmd) {
	if m.focusedColumn != columnRepos {
		return m, nil
	}

	repo := m.selectedRepo()
	if repo == nil {
		return m, nil
	}

	items := make([]FolderPickerItem, len(m.folders))
	for i, folder := range m.folders {
		items[i] = FolderPickerItem{
			ID:       folder.ID,
			Name:     folder.Name,
			Selected: m.repoHasFolder(repo.ID, folder.ID),
		}
	}

	m.modal = modalManageFolders
	m.folderPicker = NewFolderPickerModel(repo.ID, repo.Name, items)
	return m, nil
}

func (m Model) repoHasFolder(repoID, folderID string) bool {
	for _, f := range m.repoFolders[repoID] {
		if f.ID == folderID {
			return true
		}
	}
	return false
}

func (m Model) handleFolderToggle(msg FolderPickerToggleMsg) (tea.Model, tea.Cmd) {
	repoID := m.folderPicker.RepoID()

	if msg.Selected {
		return m, m.addRepoFolder(repoID, msg.FolderID)
	}
	return m, m.removeRepoFolder(repoID, msg.FolderID)
}

func (m Model) addRepoFolder(repoID, folderID string) tea.Cmd {
	return func() tea.Msg {
		_, err := m.client.AddRepoFolders(repoID, []string{folderID})
		if err != nil {
			return ActionErrorMsg{Operation: "add folder", Err: err}
		}
		return repoFolderAddedMsg{RepoID: repoID, FolderID: folderID}
	}
}

func (m Model) removeRepoFolder(repoID, folderID string) tea.Cmd {
	return func() tea.Msg {
		err := m.client.RemoveRepoFolder(repoID, folderID)
		if err != nil {
			return ActionErrorMsg{Operation: "remove folder", Err: err}
		}
		return repoFolderRemovedMsg{RepoID: repoID, FolderID: folderID}
	}
}

func (m Model) findFolder(id string) *client.Folder {
	for _, f := range m.folders {
		if f.ID == id {
			return &f
		}
	}
	return nil
}

func (m Model) removeFolderFromList(folders []client.Folder, id string) []client.Folder {
	for i, f := range folders {
		if f.ID == id {
			return append(folders[:i], folders[i+1:]...)
		}
	}
	return folders
}

func (m Model) handleDialogSubmit(msg DialogSubmitMsg) (tea.Model, tea.Cmd) {
	modal := m.modal
	m.modal = modalNone

	switch modal {
	case modalCreateFolder:
		if msg.Value == "" {
			return m, nil
		}
		return m, m.createFolder(msg.Value)

	case modalDeleteRepo:
		repo := m.selectedRepo()
		if repo == nil {
			return m, nil
		}
		return m, m.deleteRepo(repo.ID)

	case modalDeleteFolder:
		folder := m.selectedFolder()
		if folder == nil {
			return m, nil
		}
		return m, m.deleteFolder(folder.ID)

	case modalCloneDir:
		if msg.Value == "" {
			return m, nil
		}
		return m.executeClone(msg.Value)
	}

	return m, nil
}

func (m Model) createFolder(name string) tea.Cmd {
	return func() tea.Msg {
		folder, err := m.client.CreateFolder(name)
		if err != nil {
			return ActionErrorMsg{Operation: "create folder", Err: err}
		}
		return FolderCreatedMsg{Folder: *folder}
	}
}

func (m Model) renameRepo(id, name string) tea.Cmd {
	return func() tea.Msg {
		repo, err := m.client.UpdateRepo(id, &name, nil)
		if err != nil {
			return ActionErrorMsg{Operation: "rename repo", Err: err}
		}
		return RepoUpdatedMsg{Repo: *repo}
	}
}

func (m Model) renameFolder(id, name string) tea.Cmd {
	return func() tea.Msg {
		folder, err := m.client.UpdateFolder(id, &name)
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

func (m Model) executeClone(targetDir string) (tea.Model, tea.Cmd) {
	repo := m.selectedRepo()
	if repo == nil {
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

	cloneURL := m.buildCloneURL(repo.Name)
	repoName := repo.Name

	return m, tea.Batch(
		func() tea.Msg {
			return CloneStartedMsg{RepoName: repoName, Dir: targetDir}
		},
		m.runClone(cloneURL, targetDir, repoName),
	)
}

func (m Model) buildCloneURL(repoName string) string {
	baseURL := m.client.BaseURL()

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return baseURL + "/git/" + m.namespace + "/" + repoName + ".git"
	}

	parsed.Path = "/git/" + m.namespace + "/" + repoName + ".git"
	return parsed.String()
}

func (m Model) runClone(cloneURL, targetDir, repoName string) tea.Cmd {
	token := m.client.Token()
	return func() tea.Msg {
		destPath := filepath.Join(targetDir, repoName)

		// Create temp askpass script to avoid token in command line args
		askpassFile, err := os.CreateTemp("", "eph-askpass-*")
		if err != nil {
			return CloneFailedMsg{RepoName: repoName, Err: fmt.Errorf("create askpass script: %w", err)}
		}
		defer os.Remove(askpassFile.Name())

		script := fmt.Sprintf(`#!/bin/sh
case "$1" in
    *[Uu]sername*) echo "x-token" ;;
    *[Pp]assword*) echo "%s" ;;
esac
`, token)

		if _, err := askpassFile.WriteString(script); err != nil {
			return CloneFailedMsg{RepoName: repoName, Err: fmt.Errorf("write askpass script: %w", err)}
		}
		askpassFile.Close()

		if err := os.Chmod(askpassFile.Name(), 0700); err != nil {
			return CloneFailedMsg{RepoName: repoName, Err: fmt.Errorf("chmod askpass script: %w", err)}
		}

		cmd := exec.Command("git", "clone", cloneURL, destPath)
		cmd.Env = append(os.Environ(),
			"GIT_ASKPASS="+askpassFile.Name(),
			"GIT_TERMINAL_PROMPT=0",
		)

		if err := cmd.Run(); err != nil {
			return CloneFailedMsg{RepoName: repoName, Err: err}
		}
		return CloneCompletedMsg{RepoName: repoName, Dir: destPath}
	}
}

func (m Model) loadData() tea.Cmd {
	return func() tea.Msg {
		folders, _, err := m.client.ListFolders("", 0)
		if err != nil {
			return errMsg{err}
		}

		sort.Slice(folders, func(i, j int) bool {
			return strings.ToLower(folders[i].Name) < strings.ToLower(folders[j].Name)
		})

		reposWithFolders, hasMore, err := m.client.ListReposWithFolders("", repoPageSize)
		if err != nil {
			return errMsg{err}
		}

		repos := make([]client.Repo, len(reposWithFolders))
		repoFolders := make(map[string][]client.Folder)
		for i, rwf := range reposWithFolders {
			repos[i] = rwf.Repo
			repoFolders[rwf.ID] = rwf.Folders
		}

		var nextCursor string
		if len(repos) > 0 {
			nextCursor = repos[len(repos)-1].Name
		}

		return dataLoadedMsg{
			folders:        folders,
			repos:          repos,
			repoFolders:    repoFolders,
			repoNextCursor: nextCursor,
			repoHasMore:    hasMore,
		}
	}
}

func (m Model) loadMoreRepos() tea.Cmd {
	cursor := m.repoNextCursor
	return func() tea.Msg {
		reposWithFolders, hasMore, err := m.client.ListReposWithFolders(cursor, repoPageSize)
		if err != nil {
			return ActionErrorMsg{Operation: "load more repos", Err: err}
		}

		var nextCursor string
		if len(reposWithFolders) > 0 {
			nextCursor = reposWithFolders[len(reposWithFolders)-1].Name
		}

		return moreReposLoadedMsg{
			repos:      reposWithFolders,
			nextCursor: nextCursor,
			hasMore:    hasMore,
		}
	}
}

const headerHeight = 1

func (m Model) mainHeight() int {
	return m.height - footerHeight - headerHeight
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	mainHeight := m.mainHeight()

	sections := []string{
		"",
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
	var modalView string
	if m.modal == modalManageFolders {
		modalView = m.folderPicker.View()
	} else {
		modalView = m.dialog.View()
	}
	return overlay.Composite(modalView, background, overlay.Center, overlay.Center, 0, 0)
}

func (m Model) mainContentView(height int) string {
	if m.loading {
		return lipgloss.NewStyle().Height(height).Padding(0, 1).Render(m.loadingView())
	}

	if m.err != nil {
		return lipgloss.NewStyle().Height(height).Padding(0, 1).Render(m.errorView())
	}

	contentWidth := m.width - 2 - 4
	if contentWidth < 30 {
		contentWidth = 30
	}
	folderWidth := contentWidth / 4
	if folderWidth < 10 {
		folderWidth = 10
	}
	repoWidth := contentWidth / 3
	if repoWidth < 10 {
		repoWidth = 10
	}
	detailWidth := contentWidth - folderWidth - repoWidth
	if detailWidth < 10 {
		detailWidth = 10
	}

	folderColumn := m.renderFolderColumn(folderWidth, height)
	repoColumn := m.renderRepoColumn(repoWidth, height)
	detailColumn := m.renderDetailColumn(detailWidth, height)

	gap := "  "
	content := lipgloss.JoinHorizontal(lipgloss.Top, folderColumn, gap, repoColumn, gap, detailColumn)
	return lipgloss.NewStyle().Padding(0, 1).Render(content)
}

func (m Model) renderFolderColumn(width, height int) string {
	var b strings.Builder

	b.WriteString(StyleHeader.Width(width).Render(" Folders"))
	b.WriteString("\n\n")

	viewportHeight := height - 2

	allReposLabel := "All Repos"
	countStr := fmt.Sprintf("%d", len(m.repos))

	if m.folderCursor == 0 {
		prefix := "  "
		if m.focusedColumn == columnFolders {
			prefix = "→ "
		}
		left := prefix + allReposLabel
		line := m.rightAlignInWidth(left, countStr, width)
		b.WriteString(StyleFolderSelected.Width(width).Render(line))
	} else {
		left := "  " + allReposLabel
		line := m.rightAlignInWidth(left, StyleMetaText.Render(countStr), width)
		b.WriteString(line)
	}
	b.WriteString("\n")

	endIdx := m.folderScroll + viewportHeight - 1
	if endIdx > len(m.folders) {
		endIdx = len(m.folders)
	}
	startIdx := m.folderScroll
	if startIdx < 0 {
		startIdx = 0
	}

	for i := startIdx; i < endIdx; i++ {
		folder := m.folders[i]
		cursorIdx := i + 1

		count := 0
		for _, repo := range m.repos {
			for _, f := range m.repoFolders[repo.ID] {
				if f.ID == folder.ID {
					count++
					break
				}
			}
		}

		countStr := fmt.Sprintf("%d", count)

		isEditing := m.editingFolder != nil && m.editingFolder.ID == folder.ID
		isSelected := cursorIdx == m.folderCursor

		prefix := "  "
		if isSelected && m.focusedColumn == columnFolders {
			prefix = "→ "
		}

		countWidth := lipgloss.Width(countStr)
		maxNameWidth := width - lipgloss.Width(prefix) - countWidth - 2

		if isEditing {
			visibleText := truncateEditText(m.editText, maxNameWidth)
			line := prefix + visibleText + "█"
			b.WriteString(StyleEditing.Width(width).Render(line))
		} else if isSelected {
			name := truncateWithEllipsis(folder.Name, maxNameWidth)
			left := prefix + name
			line := m.rightAlignInWidth(left, countStr, width)
			b.WriteString(StyleFolderSelected.Width(width).Render(line))
		} else {
			name := truncateWithEllipsis(folder.Name, maxNameWidth)
			left := prefix + name
			line := m.rightAlignInWidth(left, StyleMetaText.Render(countStr), width)
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	return lipgloss.NewStyle().Width(width).Height(height).Render(b.String())
}

func (m Model) renderRepoColumn(width, height int) string {
	var b strings.Builder

	b.WriteString(StyleHeader.Width(width).Render(" Repositories"))
	b.WriteString("\n\n")

	if len(m.filteredRepos) == 0 {
		b.WriteString(StyleMetaText.Render("  No repositories"))
		b.WriteString("\n")
		return lipgloss.NewStyle().Width(width).Height(height).Render(b.String())
	}

	viewportHeight := (height - 2) / 3
	startIdx := m.repoScroll
	if startIdx < 0 {
		startIdx = 0
	}
	endIdx := startIdx + viewportHeight
	if endIdx > len(m.filteredRepos) {
		endIdx = len(m.filteredRepos)
	}

	for i := startIdx; i < endIdx; i++ {
		repo := m.filteredRepos[i]
		isEditing := m.editingRepo != nil && m.editingRepo.ID == repo.ID
		isFocused := i == m.repoCursor && m.focusedColumn == columnRepos

		meta := formatRepoMeta(repo)
		maxNameWidth := width - 3

		b.WriteString(m.renderRepoItem(repo.Name, meta, maxNameWidth, width, isEditing, isFocused))
		b.WriteString("\n\n")
	}

	return lipgloss.NewStyle().Width(width).Height(height).Render(b.String())
}

func (m Model) renderDetailColumn(width, height int) string {
	var b strings.Builder

	repo := m.selectedRepo()
	if repo != nil && m.focusedColumn == columnRepos {
		name := truncateWithEllipsis(repo.Name, width-2)
		b.WriteString(StyleHeader.Width(width).Render(" " + name))
	} else {
		b.WriteString(StyleHeader.Width(width).Render(""))
	}

	return lipgloss.NewStyle().Width(width).Height(height).Render(b.String())
}

func (m Model) renderRepoItem(name, meta string, maxNameWidth, width int, isEditing, isFocused bool) string {
	var lines strings.Builder

	if isEditing {
		visibleText := truncateEditText(m.editText, maxNameWidth)
		lines.WriteString(StyleEditing.Width(width).Render("  " + visibleText + "█"))
		lines.WriteString("\n")
		lines.WriteString(StyleMetaText.Render(meta))
		return lines.String()
	}

	if isFocused {
		truncName := truncateWithEllipsis(name, maxNameWidth)
		lines.WriteString(StyleRepoSelected.Width(width).Render(StyleRepoTitle.Render(truncName)))
		lines.WriteString("\n")
		lines.WriteString(StyleRepoSelected.Width(width).Render(StyleMetaText.Render(strings.TrimPrefix(meta, "  "))))
		return lines.String()
	}

	truncName := truncateWithEllipsis(name, maxNameWidth)
	lines.WriteString(StyleRepoTitle.Faint(true).Render("  " + truncName))
	lines.WriteString("\n")
	lines.WriteString(StyleMetaText.Faint(true).Render(meta))
	return lines.String()
}

func (m Model) loadingView() string {
	return fmt.Sprintf("\n  %s Loading...\n", m.spinner.View())
}

func (m Model) errorView() string {
	return StyleError.Render(fmt.Sprintf("\n  Error: %v\n", m.err))
}

func (m Model) footerView() string {
	namespaceBadge := StyleFooterNamespace.Render(m.namespace)
	badgeWidth := lipgloss.Width(namespaceBadge)

	var helpContent string
	if m.repoLoadingMore {
		helpContent = StyleStatusMsg.Render("Loading more repos...")
	} else if m.statusMsg != "" {
		helpContent = StyleStatusMsg.Render(m.statusMsg)
	} else {
		helpContent = m.keys.ShortHelp(m.focusedColumn)
	}

	return namespaceBadge + StyleFooterHelp.Width(m.width-badgeWidth).MaxHeight(1).Render(helpContent)
}

func Run(c *client.Client, namespace, server string) error {
	p := tea.NewProgram(
		NewModel(c, namespace, server),
		tea.WithAltScreen(),
	)

	_, err := p.Run()
	return err
}

func formatRepoMeta(repo client.Repo) string {
	return fmt.Sprintf("  size: %s • pushed: %s", formatSize(repo.SizeBytes), formatRelativeTime(repo.LastPushAt))
}

func (m Model) rightAlignInWidth(left, right string, width int) string {
	if width < 1 {
		width = 1
	}
	leftLen := lipgloss.Width(left)
	rightLen := lipgloss.Width(right)
	gap := width - leftLen - rightLen
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func truncateWithEllipsis(s string, maxWidth int) string {
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	runes := []rune(s)
	for i := len(runes) - 1; i >= 0; i-- {
		truncated := string(runes[:i]) + "…"
		if lipgloss.Width(truncated) <= maxWidth {
			return truncated
		}
	}
	return "…"
}

func truncateEditText(s string, maxWidth int) string {
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	runes := []rune(s)
	for i := 1; i < len(runes); i++ {
		visible := string(runes[i:])
		if lipgloss.Width(visible) <= maxWidth {
			return visible
		}
	}
	if len(runes) > 0 {
		return string(runes[len(runes)-1:])
	}
	return ""
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
