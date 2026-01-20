package tui

import (
	_ "embed"
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
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/rmhubbert/bubbletea-overlay"

	"ephemeral/internal/client"
)

//go:embed ephemeral.json
var glamourStyle []byte

type modalState int

const (
	modalNone modalState = iota
	modalCreateFolder
	modalDeleteRepo
	modalDeleteFolder
	modalCloneDir
	modalManageFolders
)

type detailTab int

const (
	tabDetails detailTab = iota
	tabReadme
	tabActivity
	tabFiles
)

type Model struct {
	client    *client.Client
	namespace string
	server    string

	folders      []client.Folder
	repos        []client.Repo
	repoFolders  map[string][]client.Folder
	folderCounts map[string]int

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

	detailTab      detailTab
	detailViewport viewport.Model
	detailCache    map[string]*RepoDetail
	detailScroll   map[detailTab]int
	currentDetail  *RepoDetail
	lastLoadedRepo string

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
		client:         c,
		namespace:      namespace,
		server:         server,
		loading:        true,
		spinner:        s,
		keys:           DefaultKeyMap,
		repoFolders:    make(map[string][]client.Folder),
		folderCounts:   make(map[string]int),
		detailCache:    make(map[string]*RepoDetail),
		detailScroll:   make(map[detailTab]int),
		detailViewport: viewport.New(0, 0),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.loadData())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg)

	case dataLoadedMsg:
		return m.handleDataLoaded(msg)

	case moreReposLoadedMsg:
		return m.handleMoreReposLoaded(msg)

	case errMsg:
		return m.handleLoadError(msg)

	case spinner.TickMsg:
		return m.handleSpinnerTick(msg)

	case DialogSubmitMsg:
		return m.handleDialogSubmit(msg)

	case DialogCancelMsg:
		m.modal = modalNone
		return m, nil

	case FolderCreatedMsg:
		return m.reloadWithStatus("Folder created: " + msg.Folder.Name)

	case FolderUpdatedMsg:
		return m.reloadWithStatus("Folder renamed: " + msg.Folder.Name)

	case RepoUpdatedMsg:
		return m.reloadWithStatus("Repo updated: " + msg.Repo.Name)

	case RepoDeletedMsg:
		return m.handleRepoDeleted(msg)

	case FolderDeletedMsg:
		return m.handleFolderDeleted(msg)

	case CloneStartedMsg:
		return m.setStatus("Cloning " + msg.RepoName + "...")

	case CloneCompletedMsg:
		return m.setStatus("Cloned " + msg.RepoName + " to " + msg.Dir)

	case CloneFailedMsg:
		return m.setStatus("Clone failed: " + msg.Err.Error())

	case ActionErrorMsg:
		return m.handleActionError(msg)

	case FolderPickerCloseMsg:
		m.modal = modalNone
		return m, nil

	case FolderPickerToggleMsg:
		return m.handleFolderToggle(msg)

	case repoFolderAddedMsg:
		return m.handleRepoFolderAdded(msg)

	case repoFolderRemovedMsg:
		return m.handleRepoFolderRemoved(msg)

	case DetailLoadedMsg:
		return m.handleDetailLoaded(msg)
	}

	return m, nil
}

func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.modal != modalNone {
		return m.handleModalKey(msg)
	}
	return m.handleKey(msg)
}

func (m Model) handleWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	m.updateViewportSize()
	m.setViewportContent()
	return m, nil
}

func (m Model) handleDataLoaded(msg dataLoadedMsg) (tea.Model, tea.Cmd) {
	m.loading = false
	m.err = nil
	m.folders = msg.folders
	m.repos = msg.repos
	m.repoFolders = msg.repoFolders
	m.repoNextCursor = msg.repoNextCursor
	m.repoHasMore = msg.repoHasMore
	m.rebuildFolderCounts()
	m.filterRepos()
	m.setViewportContent()
	return m, nil
}

func (m Model) handleMoreReposLoaded(msg moreReposLoadedMsg) (tea.Model, tea.Cmd) {
	m.repoLoadingMore = false
	for _, rwf := range msg.repos {
		m.repos = append(m.repos, rwf.Repo)
		m.repoFolders[rwf.ID] = rwf.Folders
	}
	m.repoNextCursor = msg.nextCursor
	m.repoHasMore = msg.hasMore
	m.rebuildFolderCounts()
	m.filterRepos()
	return m, nil
}

func (m Model) handleLoadError(msg errMsg) (tea.Model, tea.Cmd) {
	m.loading = false
	m.err = msg.err
	return m, nil
}

func (m Model) handleSpinnerTick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	if m.loading {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) reloadWithStatus(status string) (tea.Model, tea.Cmd) {
	m.statusMsg = status
	return m, m.loadData()
}

func (m Model) setStatus(status string) (tea.Model, tea.Cmd) {
	m.statusMsg = status
	return m, nil
}

func (m Model) handleRepoDeleted(_ RepoDeletedMsg) (tea.Model, tea.Cmd) {
	m.statusMsg = "Repo deleted"
	if len(m.filteredRepos) <= 1 {
		m.focusedColumn = columnFolders
		m.deselectRepo()
	} else if m.repoCursor >= len(m.filteredRepos)-1 {
		m.repoCursor--
	}
	return m, m.loadData()
}

func (m Model) handleFolderDeleted(_ FolderDeletedMsg) (tea.Model, tea.Cmd) {
	m.statusMsg = "Folder deleted"
	if m.folderCursor > 0 {
		m.folderCursor--
	}
	return m, m.loadData()
}

func (m Model) handleActionError(msg ActionErrorMsg) (tea.Model, tea.Cmd) {
	if msg.Operation == "load more repos" {
		m.repoLoadingMore = false
	}
	m.statusMsg = msg.Operation + " failed: " + msg.Err.Error()
	return m, nil
}

func (m Model) handleRepoFolderAdded(msg repoFolderAddedMsg) (tea.Model, tea.Cmd) {
	m.applyRepoFolderAdded(msg.RepoID, msg.FolderID)
	return m, nil
}

func (m Model) handleRepoFolderRemoved(msg repoFolderRemovedMsg) (tea.Model, tea.Cmd) {
	m.applyRepoFolderRemoved(msg.RepoID, msg.FolderID)
	return m, nil
}

func (m *Model) applyRepoFolderAdded(repoID, folderID string) {
	folder := m.findFolder(folderID)
	if folder == nil {
		return
	}
	m.repoFolders[repoID] = append(m.repoFolders[repoID], *folder)
	m.adjustFolderCount(folderID, 1)
	m.setViewportContent()
}

func (m *Model) applyRepoFolderRemoved(repoID, folderID string) {
	m.repoFolders[repoID] = m.removeFolderFromList(m.repoFolders[repoID], folderID)
	m.adjustFolderCount(folderID, -1)
	m.setViewportContent()
}

func (m Model) handleDetailLoaded(msg DetailLoadedMsg) (tea.Model, tea.Cmd) {
	detail := &RepoDetail{
		RepoID:         msg.RepoID,
		Refs:           msg.Refs,
		Commits:        msg.Commits,
		Tree:           msg.Tree,
		Readme:         msg.Readme,
		ReadmeFilename: msg.ReadmeFilename,
	}
	for _, ref := range msg.Refs {
		if ref.IsDefault {
			detail.DefaultRef = ref.Name
			break
		}
	}
	m.detailCache[msg.RepoID] = detail
	m.currentDetail = detail
	m.setViewportContent()
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

	case key.Matches(msg, m.keys.Escape):
		return m.handleEscape()

	case key.Matches(msg, m.keys.Up):
		if m.focusedColumn == columnDetail {
			m.detailViewport.LineUp(1)
			m.recordDetailScroll()
			return m, nil
		}
		m.moveCursor(-1)
		return m, m.maybeLoadDetail()

	case key.Matches(msg, m.keys.Down):
		if m.focusedColumn == columnDetail {
			m.detailViewport.LineDown(1)
			m.recordDetailScroll()
			return m, nil
		}
		m.moveCursor(1)
		if cmd := m.maybeLoadMoreRepos(); cmd != nil {
			return m, tea.Batch(cmd, m.maybeLoadDetail())
		}
		return m, m.maybeLoadDetail()

	case key.Matches(msg, m.keys.Left):
		return m.handleLeft()

	case key.Matches(msg, m.keys.Right):
		return m.handleRight()

	case key.Matches(msg, m.keys.Enter):
		if m.focusedColumn == columnFolders {
			m.focusedColumn = columnRepos
			return m, m.maybeLoadDetail()
		} else if m.focusedColumn == columnRepos {
			m.focusedColumn = columnDetail
			m.switchDetailTab(tabDetails)
			return m, m.maybeLoadDetail()
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

func (m *Model) resetRepoCursor() {
	m.repoCursor = 0
	m.repoScroll = 0
}

func (m Model) handleEscape() (tea.Model, tea.Cmd) {
	switch m.focusedColumn {
	case columnDetail:
		m.focusedColumn = columnRepos
		m.recordDetailScroll()
		m.switchDetailTab(tabDetails)
		return m, nil
	case columnRepos:
		m.focusedColumn = columnFolders
		m.deselectRepo()
		return m, nil
	}
	return m, nil
}

func (m Model) handleLeft() (tea.Model, tea.Cmd) {
	switch m.focusedColumn {
	case columnDetail:
		if m.detailTab > tabDetails {
			m.switchDetailTab(m.detailTab - 1)
			return m, nil
		}
		m.focusedColumn = columnRepos
		m.recordDetailScroll()
	case columnRepos:
		m.focusedColumn = columnFolders
		m.deselectRepo()
	}
	return m, nil
}

func (m Model) handleRight() (tea.Model, tea.Cmd) {
	switch m.focusedColumn {
	case columnFolders:
		m.focusedColumn = columnRepos
		if m.repoCursor < 0 && len(m.filteredRepos) > 0 {
			m.repoCursor = 0
		}
		return m, m.maybeLoadDetail()
	case columnRepos:
		m.focusedColumn = columnDetail
		m.switchDetailTab(tabDetails)
		return m, m.maybeLoadDetail()
	case columnDetail:
		if m.detailTab < tabFiles {
			m.switchDetailTab(m.detailTab + 1)
		}
		return m, nil
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
		if len(m.editText) >= nameMaxLength {
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
		m.resetRepoCursor()

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
	viewportHeight := listViewportHeight(m.mainHeight())

	if m.folderCursor < m.folderScroll {
		m.folderScroll = m.folderCursor
	} else if m.folderCursor >= m.folderScroll+viewportHeight {
		m.folderScroll = m.folderCursor - viewportHeight + 1
	}
}

func (m *Model) syncRepoScroll() {
	viewportHeight := repoViewportHeight(m.mainHeight())

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
	if distanceFromEnd > repoLoadMoreThreshold {
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

func (m *Model) rebuildFolderCounts() {
	counts := make(map[string]int, len(m.folders))
	for _, folder := range m.folders {
		counts[folder.ID] = 0
	}

	for _, folders := range m.repoFolders {
		for _, folder := range folders {
			if _, ok := counts[folder.ID]; ok {
				counts[folder.ID]++
			}
		}
	}

	m.folderCounts = counts
}

func (m *Model) adjustFolderCount(folderID string, delta int) {
	if m.folderCounts == nil {
		m.rebuildFolderCounts()
		return
	}

	count, ok := m.folderCounts[folderID]
	if !ok {
		m.rebuildFolderCounts()
		return
	}

	count += delta
	if count < 0 {
		count = 0
	}
	if count == 0 {
		delete(m.folderCounts, folderID)
		return
	}
	m.folderCounts[folderID] = count
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

func (m *Model) deselectRepo() {
	m.repoCursor = -1
	m.currentDetail = nil
	m.lastLoadedRepo = ""
	m.setViewportContent()
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

	repo := m.selectedRepo()
	if repo == nil {
		return m, nil
	}
	m.editingRepo = repo
	m.editText = repo.Name
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

func (m Model) openCloneDir() (tea.Model, tea.Cmd) {
	if m.focusedColumn == columnFolders {
		return m, nil
	}

	repo := m.selectedRepo()
	if repo == nil {
		return m, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		m.statusMsg = "Failed to get current directory: " + err.Error()
		return m, nil
	}
	m.modal = modalCloneDir
	m.dialog = NewInputDialog("Clone to Directory", "Enter path:", cwd)
	m.dialog.SetValue(cwd)
	return m, m.dialog.Init()
}

func (m Model) openManageFolders() (tea.Model, tea.Cmd) {
	if m.focusedColumn == columnFolders {
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
	for i := range m.folders {
		if m.folders[i].ID == id {
			return &m.folders[i]
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

func (m *Model) maybeLoadDetail() tea.Cmd {
	repo := m.selectedRepo()
	if repo == nil {
		m.resetDetailScroll()
		m.currentDetail = nil
		m.lastLoadedRepo = ""
		m.setViewportContent()
		return nil
	}

	if m.lastLoadedRepo == repo.ID {
		return nil
	}

	m.resetDetailScroll()

	if cached, ok := m.detailCache[repo.ID]; ok {
		m.currentDetail = cached
		m.lastLoadedRepo = repo.ID
		m.setViewportContent()
		return nil
	}

	m.detailViewport.Height = m.detailViewportHeight()
	m.lastLoadedRepo = repo.ID
	return m.loadDetail(repo.ID)
}

func (m Model) loadDetail(repoID string) tea.Cmd {
	return func() tea.Msg {
		refs, err := m.client.ListRefs(repoID)
		if err != nil {
			return DetailLoadedMsg{RepoID: repoID, Err: fmt.Errorf("list refs: %w", err)}
		}

		var defaultRef string
		for _, ref := range refs {
			if ref.IsDefault {
				defaultRef = ref.Name
				break
			}
		}

		if defaultRef == "" && len(refs) > 0 {
			defaultRef = refs[0].Name
		}

		var commits []client.Commit
		var tree []client.TreeEntry
		var readme *string
		var readmeFilename string

		if defaultRef != "" {
			commits, _, err = m.client.ListCommits(repoID, defaultRef, "", detailCommitsLimit)
			if err != nil {
				return DetailLoadedMsg{RepoID: repoID, Err: fmt.Errorf("list commits for %s: %w", defaultRef, err)}
			}
			tree, err = m.client.GetTree(repoID, defaultRef, "")
			if err != nil {
				return DetailLoadedMsg{RepoID: repoID, Err: fmt.Errorf("get tree for %s: %w", defaultRef, err)}
			}

			for _, entry := range tree {
				nameLower := strings.ToLower(entry.Name)
				if nameLower == "readme.md" || nameLower == "readme" || nameLower == "readme.txt" {
					blob, err := m.client.GetBlob(repoID, defaultRef, entry.Name)
					if err != nil {
						return DetailLoadedMsg{RepoID: repoID, Err: fmt.Errorf("get readme %q: %w", entry.Name, err)}
					}
					if blob.Content != nil && !blob.IsBinary {
						readme = blob.Content
						readmeFilename = entry.Name
					}
					break
				}
			}
		}

		return DetailLoadedMsg{
			RepoID:         repoID,
			Refs:           refs,
			Commits:        commits,
			Tree:           tree,
			Readme:         readme,
			ReadmeFilename: readmeFilename,
		}
	}
}

func (m Model) mainHeight() int {
	return m.height - footerHeight
}

func (m *Model) updateViewportSize() {
	layout := m.layoutSizes()
	viewportHeight := m.detailViewportHeight()

	m.detailViewport.Width = max(layout.detailWidth-detailViewportBorderWidth, 1)
	m.detailViewport.Height = viewportHeight
}

func (m *Model) setViewportContent() {
	m.detailViewport.Height = m.detailViewportHeight()
	width := m.detailViewport.Width

	var content string
	switch m.detailTab {
	case tabDetails:
		content = m.getDetailsContent(width)
	case tabReadme:
		content = m.getReadmeContent(width)
	case tabActivity:
		content = m.getActivityContent(width)
	case tabFiles:
		content = m.getFilesContent(width)
	}

	trimmed := strings.TrimRight(content, "\n")
	padded := strings.Repeat("\n", detailViewportTopPadding) + trimmed + strings.Repeat("\n", detailViewportBottomPadding)
	m.detailViewport.SetContent(padded)
	m.restoreDetailScroll(m.detailTab)
}

func (m *Model) switchDetailTab(tab detailTab) {
	if m.detailTab == tab {
		return
	}

	m.recordDetailScroll()
	m.detailTab = tab
	m.setViewportContent()
}

func (m *Model) resetDetailScroll() {
	m.detailScroll = make(map[detailTab]int)
	m.detailViewport.GotoTop()
}

func (m *Model) recordDetailScroll() {
	if m.detailScroll == nil {
		m.detailScroll = make(map[detailTab]int)
	}
	m.detailScroll[m.detailTab] = m.detailViewport.YOffset
}

func (m *Model) restoreDetailScroll(tab detailTab) {
	if m.detailScroll == nil {
		m.detailScroll = make(map[detailTab]int)
	}

	offset := m.detailScroll[tab]
	maxOffset := max(m.detailViewport.TotalLineCount()-m.detailViewport.Height, 0)
	if offset > maxOffset {
		offset = maxOffset
	}
	if offset < 0 {
		offset = 0
	}
	m.detailViewport.YOffset = offset
	m.detailScroll[tab] = offset
}

func (m Model) getDetailsContent(width int) string {
	repo := m.selectedRepo()
	if repo == nil {
		return " " + StyleMetaText.Render("Select a repo to see its details")
	}

	var b strings.Builder

	b.WriteString(" " + StyleMetaText.Render("Name") + "\n")
	b.WriteString(" " + repo.Name + "\n\n")

	b.WriteString(" " + StyleMetaText.Render("Size") + "\n")
	b.WriteString(" " + formatSize(repo.SizeBytes) + "\n\n")

	b.WriteString(" " + StyleMetaText.Render("Last Pushed") + "\n")
	b.WriteString(" " + formatRelativeTime(repo.LastPushAt) + "\n\n")

	b.WriteString(" " + StyleMetaText.Render("Created") + "\n")
	b.WriteString(" " + repo.CreatedAt.Format("Jan 2, 2006") + "\n\n")

	folders := m.repoFolders[repo.ID]
	b.WriteString(" " + StyleMetaText.Render("Folders") + "\n")
	if len(folders) == 0 {
		b.WriteString(" " + StyleMetaText.Render("(none)") + "\n")
	} else {
		for _, f := range folders {
			b.WriteString(" • " + f.Name + "\n")
		}
	}

	return b.String()
}

func (m Model) getReadmeContent(width int) string {
	if m.currentDetail == nil || m.currentDetail.Readme == nil {
		return " " + StyleMetaText.Render("No README found")
	}

	return m.renderReadme(*m.currentDetail.Readme, m.currentDetail.ReadmeFilename, width)
}

func (m Model) getActivityContent(width int) string {
	if m.currentDetail == nil || len(m.currentDetail.Commits) == 0 {
		return " " + StyleMetaText.Render("No commits")
	}

	maxMsgLen := max(width-activityMessagePadding, activityMessageMinWidth)
	var b strings.Builder

	for i, commit := range m.currentDetail.Commits {
		shortSHA := commit.SHA
		if len(shortSHA) > shortSHAWidth {
			shortSHA = shortSHA[:shortSHAWidth]
		}

		message := strings.Split(commit.Message, "\n")[0]
		if len(message) > maxMsgLen {
			message = message[:maxMsgLen-1] + "…"
		}

		timeAgo := formatRelativeTime(&commit.Author.Date)

		b.WriteString(" " + StyleMetaText.Render(shortSHA) + " " + message + "\n")
		b.WriteString(" " + StyleMetaText.Render(fmt.Sprintf("  %s • %s", commit.Author.Name, timeAgo)))

		if i < len(m.currentDetail.Commits)-1 {
			b.WriteString("\n\n")
		}
	}

	return b.String()
}

func (m Model) getFilesContent(width int) string {
	if m.currentDetail == nil || len(m.currentDetail.Tree) == 0 {
		return " " + StyleMetaText.Render("No files")
	}

	var lines []string
	for _, entry := range m.currentDetail.Tree {
		icon := "[f]"
		if entry.Type == "dir" {
			icon = "[d]"
		}

		name := entry.Name
		maxNameLen := width - fileNamePadding
		if maxNameLen < fileNameMinWidth {
			maxNameLen = fileNameMinWidth
		}
		if len(name) > maxNameLen {
			name = name[:maxNameLen-1] + "…"
		}

		line := " " + icon + " " + name
		if entry.Size != nil {
			line = m.rightAlignInWidth(line, formatSize(int(*entry.Size)), width-1)
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	mainHeight := m.mainHeight()

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

	layout := m.layoutSizes()

	listHeight := height - headerHeight
	folderColumn := "\n" + m.renderFolderColumn(layout.folderWidth, listHeight)
	repoColumn := "\n" + m.renderRepoColumn(layout.repoWidth, listHeight)
	detailColumn := m.renderDetailColumn(layout.detailWidth, height)

	gap := strings.Repeat(" ", columnGapWidth)
	content := lipgloss.JoinHorizontal(lipgloss.Top, folderColumn, gap, repoColumn, gap, detailColumn)
	return lipgloss.NewStyle().Padding(0, 1).Render(content)
}

func (m Model) renderFolderColumn(width, height int) string {
	var b strings.Builder

	b.WriteString(StyleHeader.Width(width).Render(" Folders"))
	b.WriteString("\n\n")

	viewportHeight := listViewportHeight(height)

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

		count := m.folderCounts[folder.ID]

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

	b.WriteString(StyleHeader.Width(width).Render(" Repos"))
	b.WriteString("\n\n")

	if len(m.filteredRepos) == 0 {
		b.WriteString(StyleMetaText.Render("  No repositories"))
		b.WriteString("\n")
		return lipgloss.NewStyle().Width(width).Height(height).Render(b.String())
	}

	viewportHeight := repoViewportHeight(height)
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
		state := m.repoItemState(i, repo.ID)
		meta := formatRepoMeta(repo)
		maxNameWidth := width - 3

		b.WriteString(m.renderRepoItem(repo.Name, meta, maxNameWidth, width, state))
		b.WriteString("\n\n")
	}

	return lipgloss.NewStyle().Width(width).Height(height).Render(b.String())
}

func (m Model) renderDetailColumn(width, height int) string {
	isActive := m.focusedColumn == columnDetail
	tabbedHeight := detailTabContainerHeight(height)
	tabbedContainer := m.renderTabbedContainer(width, tabbedHeight, isActive)

	return lipgloss.NewStyle().Width(width).Height(height).Render(tabbedContainer)
}

func (m Model) renderTabbedContainer(width, height int, isActive bool) string {
	if height < detailTabMinHeight {
		height = detailTabMinHeight
	}

	border := StyleTabBorder
	if isActive {
		border = StyleTabBorderActive
	}

	tabHeader := m.renderTabHeader(width, border, isActive)
	contentHeight := height - detailTabFrameRows
	if contentHeight < 1 {
		contentHeight = 1
	}

	content := m.renderTabContent(width, contentHeight, border)
	bottomBorder := m.renderTabBottomBorder(width, border)

	return tabHeader + content + "\n" + bottomBorder
}

func (m Model) renderTabHeader(width int, border lipgloss.Style, isActive bool) string {
	tabs := []string{"Details", "Readme", "Activity", "Files"}

	var topRow, midRow, botRow strings.Builder

	for i, tab := range tabs {
		innerWidth := len(tab) + 2
		isActiveTab := detailTab(i) == m.detailTab

		tabText := tab
		if !isActive || !isActiveTab {
			tabText = lipgloss.NewStyle().Faint(true).Render(tab)
		}

		topRow.WriteString(border.Render("╭" + strings.Repeat("─", innerWidth) + "╮"))
		midRow.WriteString(border.Render("│") + " " + tabText + " " + border.Render("│"))

		leftCorner := m.tabLeftCorner(i, isActiveTab)
		if isActiveTab {
			botRow.WriteString(border.Render(leftCorner) + strings.Repeat(" ", innerWidth) + border.Render("└"))
		} else {
			botRow.WriteString(border.Render(leftCorner + strings.Repeat("─", innerWidth) + "┴"))
		}

		if i < len(tabs)-1 {
			topRow.WriteString(" ")
			midRow.WriteString(" ")
			botRow.WriteString(border.Render("─"))
		}
	}

	remaining := width - lipgloss.Width(botRow.String()) - 1
	if remaining > 0 {
		botRow.WriteString(border.Render(strings.Repeat("─", remaining) + "╮"))
	} else {
		botRow.WriteString(border.Render("╮"))
	}

	return topRow.String() + "\n" + midRow.String() + "\n" + botRow.String() + "\n"
}

func (m Model) tabLeftCorner(index int, isActive bool) string {
	if index == 0 {
		if isActive {
			return "│"
		}
		return "├"
	}
	if isActive {
		return "┘"
	}
	return "┴"
}

func (m Model) renderTabContent(width, height int, border lipgloss.Style) string {
	contentWidth := max(width-tabContentBorderWidth, 1)

	rawContent := m.detailViewport.View()

	lines := strings.Split(rawContent, "\n")
	var b strings.Builder

	for i := 0; i < height; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}

		padding := max(contentWidth-lipgloss.Width(line), 0)
		b.WriteString(border.Render("│") + line + strings.Repeat(" ", padding) + border.Render("│"))

		if i < height-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (m Model) renderTabBottomBorder(width int, border lipgloss.Style) string {
	borderWidth := max(width-tabContentBorderWidth, 1)
	if m.detailViewport.TotalLineCount() <= m.detailViewport.Height {
		return border.Render("╰" + strings.Repeat("─", borderWidth) + "╯")
	}

	pct := int(m.detailViewport.ScrollPercent() * 100)
	pctStr := fmt.Sprintf("[%d%%]", pct)
	leftDashes := max(borderWidth-len(pctStr)-tabScrollIndicatorPadding, 1)

	return border.Render("╰" + strings.Repeat("─", leftDashes+1) + pctStr + "─" + "╯")
}

func (m Model) renderReadme(content, filename string, width int) string {
	if !strings.HasSuffix(strings.ToLower(filename), ".md") {
		return content
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithWordWrap(width),
		glamour.WithStylesFromJSONBytes(glamourStyle),
	)
	if err != nil {
		return content
	}

	rendered, err := renderer.Render(content)
	if err != nil {
		return content
	}

	return strings.TrimSpace(rendered)
}

type repoItemState int

const (
	repoStateDefault repoItemState = iota
	repoStateCursor
	repoStateActive
	repoStateEditing
)

func (m Model) renderRepoItem(name, meta string, maxNameWidth, width int, state repoItemState) string {
	if state == repoStateEditing {
		visibleText := truncateEditText(m.editText, maxNameWidth)
		return StyleEditing.Width(width).Render("  "+visibleText+"█") + "\n" + StyleMetaText.Render(meta)
	}

	truncName := truncateWithEllipsis(name, maxNameWidth)
	metaText := strings.TrimPrefix(meta, "  ")

	var style lipgloss.Style
	switch state {
	case repoStateActive:
		style = StyleRepoActive
	case repoStateCursor:
		style = StyleRepoCursor
	default:
		style = StyleRepoDefault
	}

	titleLine := style.Width(width).Render(StyleRepoTitle.Render(truncName))
	metaLine := style.Width(width).Render(StyleMetaText.Render(metaText))

	return titleLine + "\n" + metaLine
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
		helpContent = m.keys.ShortHelp(m.selectedRepo() != nil)
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

func (m Model) repoItemState(index int, repoID string) repoItemState {
	if m.editingRepo != nil && m.editingRepo.ID == repoID {
		return repoStateEditing
	}

	if index != m.repoCursor {
		return repoStateDefault
	}

	switch m.focusedColumn {
	case columnRepos:
		return repoStateActive
	case columnDetail:
		return repoStateCursor
	default:
		return repoStateDefault
	}
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
