package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/tree"
	overlay "github.com/rmhubbert/bubbletea-overlay"

	"github.com/bantamhq/ephemeral/internal/client"
)

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
	switch m.modal {
	case modalManageFolders:
		modalView = m.folderPicker.View()
	case modalNamespaceSwitcher:
		modalView = m.namespacePicker.View()
	case modalHelp:
		modalView = m.helpModalView()
	default:
		modalView = m.dialog.View()
	}
	return overlay.Composite(modalView, background, overlay.Center, overlay.Center, 0, 0)
}

func (m Model) mainContentView(height int) string {
	if m.loading {
		return lipgloss.NewStyle().Height(height).Padding(0, 1).Render(m.loadingView())
	}

	if m.err != nil {
		background := lipgloss.NewStyle().Width(m.width).Height(height).Render("")
		return overlay.Composite(m.errorView(), background, overlay.Center, overlay.Center, 0, 0)
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

	b.WriteString(Styles.Common.Header.Width(width).Render(" Folders"))
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
		b.WriteString(Styles.Folder.Selected.Width(width).Render(line))
	} else {
		left := "  " + allReposLabel
		line := m.rightAlignInWidth(left, Styles.Common.MetaText.Render(countStr), width)
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
			b.WriteString(Styles.Folder.Editing.Width(width).Render(line))
		} else if isSelected {
			name := truncateWithEllipsis(folder.Name, maxNameWidth)
			left := prefix + name
			line := m.rightAlignInWidth(left, countStr, width)
			b.WriteString(Styles.Folder.Selected.Width(width).Render(line))
		} else {
			name := truncateWithEllipsis(folder.Name, maxNameWidth)
			left := prefix + name
			line := m.rightAlignInWidth(left, Styles.Common.MetaText.Render(countStr), width)
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	return lipgloss.NewStyle().Width(width).Height(height).Render(b.String())
}

func (m Model) renderRepoColumn(width, height int) string {
	var b strings.Builder

	b.WriteString(Styles.Common.Header.Width(width).Render(" Repos"))
	b.WriteString("\n\n")

	if len(m.filteredRepos) == 0 {
		b.WriteString(Styles.Common.MetaText.Render("  No repositories"))
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
		description := formatRepoDescription(repo)
		maxNameWidth := width - 3

		b.WriteString(m.renderRepoItem(repo.Name, description, maxNameWidth, width, state))
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

	border := Styles.Detail.TabBorder
	if isActive {
		border = Styles.Detail.TabBorderActive
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

type repoItemState int

const (
	repoStateDefault repoItemState = iota
	repoStateCursor
	repoStateActive
	repoStateEditing
)

func (m Model) renderRepoItem(name, description string, maxNameWidth, width int, state repoItemState) string {
	if state == repoStateEditing {
		if m.editingDescription {
			truncName := truncateWithEllipsis(name, maxNameWidth)
			titleLine := Styles.Repo.Active.Base.Width(width).Render(Styles.Repo.Active.Title.Render(truncName))
			visibleText := truncateEditText(m.editText, maxNameWidth)
			descLine := Styles.Repo.Editing.Width(width).Render("  " + visibleText + "█")
			return titleLine + "\n" + descLine
		}
		visibleText := truncateEditText(m.editText, maxNameWidth)
		descText := truncateWithEllipsis(description, maxNameWidth)
		return Styles.Repo.Editing.Width(width).Render("  "+visibleText+"█") + "\n" + Styles.Common.MetaText.Render("  "+descText)
	}

	truncName := truncateWithEllipsis(name, maxNameWidth)
	descText := truncateWithEllipsis(description, maxNameWidth)

	var baseStyle lipgloss.Style
	var titleStyle lipgloss.Style
	switch state {
	case repoStateActive:
		baseStyle = Styles.Repo.Active.Base
		titleStyle = Styles.Repo.Active.Title
	case repoStateCursor:
		baseStyle = Styles.Repo.Cursor.Base
		titleStyle = Styles.Repo.Cursor.Title
	default:
		baseStyle = Styles.Repo.Normal.Base
		titleStyle = Styles.Repo.Normal.Title
	}

	titleLine := baseStyle.Width(width).Render(titleStyle.Render(truncName))
	descLine := baseStyle.Width(width).Render(Styles.Common.MetaText.Render(descText))

	return titleLine + "\n" + descLine
}

func (m Model) loadingView() string {
	return fmt.Sprintf("\n  %s Loading...\n", m.spinner.View())
}

func (m Model) errorView() string {
	title, message := friendlyError(m.err)

	width := errorDialogWidth
	if m.width > 0 && m.width < width+4 {
		width = max(m.width-4, 20)
	}

	var content strings.Builder
	content.WriteString(Styles.Error.Title.Render(title))
	content.WriteString("\n\n")
	content.WriteString(message)
	content.WriteString("\n\n")
	content.WriteString(Styles.Dialog.Hint.Render("q quit"))

	return Styles.Error.Box.Width(width).Render(content.String())
}

func (m Model) footerView() string {
	namespaceBadge := Styles.Footer.Namespace.Render(m.namespace)
	badgeWidth := lipgloss.Width(namespaceBadge)
	helpWidth := max(m.width-badgeWidth, 0)
	if helpWidth == 0 {
		return namespaceBadge
	}

	rightPadding := 1
	if helpWidth <= rightPadding {
		return namespaceBadge + Styles.Footer.Help.Width(helpWidth).MaxHeight(1).Render(strings.Repeat(" ", helpWidth))
	}

	contentWidth := helpWidth - rightPadding

	var content string
	var style lipgloss.Style
	if m.statusMsg != "" {
		content = m.statusMsg
		style = Styles.Footer.StatusMessage
	} else {
		content = "? Help"
		style = Styles.Footer.Help
	}

	if lipgloss.Width(content) > contentWidth {
		content = truncateWithEllipsis(content, contentWidth)
	}
	leftPadding := contentWidth - lipgloss.Width(content)
	if leftPadding < 0 {
		leftPadding = 0
	}
	content = strings.Repeat(" ", leftPadding) + content + strings.Repeat(" ", rightPadding)

	return namespaceBadge + style.Width(helpWidth).MaxHeight(1).Render(content)
}

func (m Model) helpKeyMap() help.KeyMap {
	return helpKeyMap{
		KeyMap:          m.keys,
		hasSelectedRepo: m.selectedRepo() != nil,
	}
}

func (m Model) helpContent(width int, showAll bool) string {
	helpModel := m.help
	helpModel.Width = width
	helpModel.ShowAll = showAll

	return strings.TrimRight(helpModel.View(m.helpKeyMap()), "\n")
}

func (m Model) helpModalView() string {
	width := m.helpModalWidth()
	innerWidth := max(width-4, 1)

	var content strings.Builder
	content.WriteString(Styles.Help.Title.Render("Help"))
	content.WriteString("\n\n")
	content.WriteString(m.helpContent(innerWidth, true))
	content.WriteString("\n\n")
	content.WriteString(Styles.Dialog.Hint.Render("esc close"))

	return Styles.Help.Box.Width(width).Render(content.String())
}

func (m Model) helpModalWidth() int {
	available := max(m.width-4, 1)
	width := min(available, helpDialogMaxWidth)
	if width < helpDialogMinWidth {
		return available
	}
	return width
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
		return " " + Styles.Common.MetaText.Render("Select a repo to see its details")
	}

	var b strings.Builder

	b.WriteString(" " + Styles.Common.MetaText.Render("Name") + "\n")
	b.WriteString(" " + repo.Name + "\n\n")

	b.WriteString(" " + Styles.Common.MetaText.Render("Description") + "\n")
	desc := repoDescription(repo)
	if desc == "" {
		b.WriteString(" " + Styles.Common.MetaText.Render("No description") + "\n\n")
	} else {
		wrapped := lipgloss.NewStyle().Width(width - 2).Render(desc)
		for _, line := range strings.Split(wrapped, "\n") {
			b.WriteString(" " + line + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(" " + Styles.Common.MetaText.Render("Size") + "\n")
	b.WriteString(" " + formatSize(repo.SizeBytes) + "\n\n")

	b.WriteString(" " + Styles.Common.MetaText.Render("Last Pushed") + "\n")
	b.WriteString(" " + formatRelativeTime(repo.LastPushAt) + "\n\n")

	b.WriteString(" " + Styles.Common.MetaText.Render("Created") + "\n")
	b.WriteString(" " + repo.CreatedAt.Format("Jan 2, 2006") + "\n\n")

	folders := m.repoFolders[repo.ID]
	b.WriteString(" " + Styles.Common.MetaText.Render("Folders") + "\n")
	if len(folders) == 0 {
		b.WriteString(" " + Styles.Common.MetaText.Render("(none)") + "\n")
	} else {
		for _, f := range folders {
			b.WriteString(" • " + f.Name + "\n")
		}
	}

	return b.String()
}

func (m Model) getReadmeContent(width int) string {
	if m.currentDetail == nil || m.currentDetail.Readme == nil {
		return " " + Styles.Common.MetaText.Render("No README found")
	}

	return m.renderReadme(*m.currentDetail.Readme, m.currentDetail.ReadmeFilename, width)
}

func (m Model) getActivityContent(width int) string {
	if m.currentDetail == nil || len(m.currentDetail.Commits) == 0 {
		return " " + Styles.Common.MetaText.Render("No commits")
	}

	t := tree.Root(" ⁜ Recent Commits")
	enumeratorWidth := lipgloss.Width(Styles.Tree.Enumerator.Render("├─"))
	for _, commit := range m.currentDetail.Commits {
		shortSHA := commit.SHA
		if len(shortSHA) > shortSHAWidth {
			shortSHA = shortSHA[:shortSHAWidth]
		}

		message := firstLine(commit.Message)
		timeAgo := formatRelativeTime(&commit.Author.Date)

		messageWidth := width - enumeratorWidth - lipgloss.Width(shortSHA) - lipgloss.Width("•") - lipgloss.Width(timeAgo) - 3
		if messageWidth < 1 {
			commitLine := Styles.Commit.Hash.Render(shortSHA) + " " + Styles.Common.MetaText.Render("•") + " " + Styles.Common.MetaText.Render(timeAgo)
			t.Child(tree.Root(commitLine).Child(renderCommitStats(commit.Stats)))
			continue
		}

		message = truncateWithEllipsis(message, messageWidth)

		commitLine := Styles.Commit.Hash.Render(shortSHA) + " " + Styles.Common.MetaText.Render("•") + " " + message + " " + Styles.Common.MetaText.Render(timeAgo)
		t.Child(tree.Root(commitLine).Child(renderCommitStats(commit.Stats)))
	}

	t.EnumeratorStyle(Styles.Tree.Enumerator).
		Enumerator(treeEnumerator).
		Indenter(treeIndenter)

	return t.String()
}

func treeEnumerator(children tree.Children, index int) string {
	if children.Length()-1 == index {
		return "└─"
	}
	return "├─"
}

func treeIndenter(children tree.Children, index int) string {
	if children.Length()-1 == index {
		return "  "
	}
	return "│ "
}

func renderCommitStats(stats *client.CommitStats) string {
	if stats == nil {
		return Styles.Commit.Stat.Render("(no stats)")
	}

	base := Styles.Commit.Stat.Render(fmt.Sprintf("%d files, %d(", stats.FilesChanged, stats.Additions))
	added := Styles.Commit.StatAdded.Render("+")
	middle := Styles.Commit.Stat.Render(fmt.Sprintf("), %d(", stats.Deletions))
	removed := Styles.Commit.StatRemoved.Render("-")
	end := Styles.Commit.Stat.Render(")")

	return base + added + middle + removed + end
}

func (m Model) getFilesContent(width int) string {
	if m.currentDetail == nil || len(m.currentDetail.Tree) == 0 {
		return " " + Styles.Common.MetaText.Render("No files")
	}

	entries := sortTreeEntries(m.currentDetail.Tree)
	children := buildFileTreeChildren(entries)

	t := tree.Root(" /")
	if len(children) > 0 {
		t.Child(children...)
	}

	t.EnumeratorStyle(Styles.Tree.Enumerator).
		Enumerator(treeEnumerator).
		Indenter(treeIndenter)

	return t.String()
}

func sortTreeEntries(entries []client.TreeEntry) []client.TreeEntry {
	sorted := make([]client.TreeEntry, len(entries))
	copy(sorted, entries)

	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Type != sorted[j].Type {
			return sorted[i].Type == "dir"
		}
		return strings.ToLower(sorted[i].Name) < strings.ToLower(sorted[j].Name)
	})

	for i := range sorted {
		if len(sorted[i].Children) > 0 {
			sorted[i].Children = sortTreeEntries(sorted[i].Children)
		}
	}

	return sorted
}

func buildFileTreeChildren(entries []client.TreeEntry) []any {
	children := make([]any, 0, len(entries))

	for _, entry := range entries {
		name := entry.Name
		if entry.Type == "dir" {
			name = Styles.Tree.Dir.Render(name)
		}

		if len(entry.Children) > 0 {
			node := tree.Root(name)
			nodeChildren := buildFileTreeChildren(entry.Children)
			if len(nodeChildren) > 0 {
				node.Child(nodeChildren...)
			}
			children = append(children, node)
			continue
		}

		children = append(children, name)
	}

	return children
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
