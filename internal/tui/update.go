package tui

import (
	"fmt"
	"net/url"
	"os"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/bantamhq/ephemeral/internal/client"
)

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
	if m.modal == modalHelp {
		return m.handleHelpKey(msg)
	}
	if m.modal != modalNone {
		return m.handleModalKey(msg)
	}
	return m.handleKey(msg)
}

func (m Model) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "?":
		m.modal = modalNone
	}
	return m, nil
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
	if msg.Operation == "load additional repos" {
		m.repoLoadingMore = false
	}
	m.statusMsg = "Could not " + msg.Operation + ": " + msg.Err.Error()
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
	m.cacheDetail(msg.RepoID, detail)
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

	case key.Matches(msg, m.keys.Help):
		m.modal = modalHelp
		return m, nil

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

	case key.Matches(msg, m.keys.EditDescription):
		return m.startEditDescription()

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
			m.repoScroll = 0
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
	switch {
	case key.Matches(msg, m.keys.Escape):
		m.editingFolder = nil
		m.editingRepo = nil
		m.editingDescription = false
		m.editText = ""
		return m, nil

	case key.Matches(msg, m.keys.Enter):
		return m.submitEdit()

	case msg.Type == tea.KeyBackspace:
		if len(m.editText) > 0 {
			m.editText = m.editText[:len(m.editText)-1]
		}
		return m, nil

	case msg.Type == tea.KeySpace:
		if m.editingDescription {
			m.editText += " "
		}
		return m, nil

	case msg.Type == tea.KeyRunes:
		if m.editingDescription {
			m.editText += string(msg.Runes)
		} else {
			if len(m.editText) >= nameMaxLength {
				return m, nil
			}
			if !validateNameInput(msg.Runes, m.editText) {
				return m, nil
			}
			m.editText += string(msg.Runes)
		}
		return m, nil
	}

	return m, nil
}

func (m Model) submitEdit() (tea.Model, tea.Cmd) {
	newValue := m.editText
	editingFolder := m.editingFolder
	editingRepo := m.editingRepo
	editingDescription := m.editingDescription

	m.editingFolder = nil
	m.editingRepo = nil
	m.editingDescription = false
	m.editText = ""

	if editingFolder != nil {
		if newValue == "" {
			m.statusMsg = "Empty name not allowed, folder name reset"
			return m, nil
		}
		if newValue == editingFolder.Name {
			return m, nil
		}
		return m, m.renameFolder(editingFolder.ID, newValue)
	}

	if editingRepo != nil {
		if editingDescription {
			if newValue == repoDescription(editingRepo) {
				return m, nil
			}
			m.updateLocalRepo(editingRepo.ID, func(r *client.Repo) { r.Description = &newValue })
			return m, m.updateRepoDescription(editingRepo.ID, newValue)
		}
		if newValue == "" {
			m.statusMsg = "Empty name not allowed, repo name reset"
			return m, nil
		}
		if newValue == editingRepo.Name {
			return m, nil
		}
		m.updateLocalRepo(editingRepo.ID, func(r *client.Repo) { r.Name = newValue })
		return m, m.renameRepo(editingRepo.ID, newValue)
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

func (m *Model) updateLocalRepo(id string, mutate func(*client.Repo)) {
	for i := range m.repos {
		if m.repos[i].ID == id {
			mutate(&m.repos[i])
			break
		}
	}
	for i := range m.filteredRepos {
		if m.filteredRepos[i].ID == id {
			mutate(&m.filteredRepos[i])
			break
		}
	}
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

func (m Model) startEditDescription() (tea.Model, tea.Cmd) {
	if m.focusedColumn == columnFolders {
		return m, nil
	}

	repo := m.selectedRepo()
	if repo == nil {
		return m, nil
	}

	m.editingRepo = repo
	m.editingDescription = true
	m.editText = repoDescription(repo)
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

func (m *Model) cacheDetail(repoID string, detail *RepoDetail) {
	if len(m.detailCache) >= maxDetailCacheSize {
		for k := range m.detailCache {
			delete(m.detailCache, k)
			break
		}
	}
	m.detailCache[repoID] = detail
}
