package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"ephemeral/internal/client"
)

type FolderSelectedMsg struct {
	FolderID *string
}

type FolderPickerCancelMsg struct{}

type folderOption struct {
	id    *string
	name  string
	depth int
}

type FolderPickerModel struct {
	title   string
	options []folderOption
	cursor  int
	width   int
}

func NewFolderPicker(title string, folders []client.Folder, excludeID *string) FolderPickerModel {
	options := []folderOption{
		{id: nil, name: "(Root - no folder)", depth: 0},
	}

	tree := buildFolderTree(folders, excludeID)
	options = append(options, flattenFolderOptions(tree, 0)...)

	return FolderPickerModel{
		title:   title,
		options: options,
		cursor:  0,
		width:   50,
	}
}

type folderTreeNode struct {
	folder   *client.Folder
	children []*folderTreeNode
}

func buildFolderTree(folders []client.Folder, excludeID *string) []*folderTreeNode {
	nodeMap := make(map[string]*folderTreeNode)
	var roots []*folderTreeNode

	for i := range folders {
		f := &folders[i]
		if excludeID != nil && f.ID == *excludeID {
			continue
		}
		nodeMap[f.ID] = &folderTreeNode{folder: f}
	}

	for i := range folders {
		f := &folders[i]
		if excludeID != nil && f.ID == *excludeID {
			continue
		}
		node := nodeMap[f.ID]
		if f.ParentID != nil {
			if parent, ok := nodeMap[*f.ParentID]; ok {
				parent.children = append(parent.children, node)
				continue
			}
		}
		roots = append(roots, node)
	}

	return roots
}

func flattenFolderOptions(nodes []*folderTreeNode, depth int) []folderOption {
	var result []folderOption
	for _, node := range nodes {
		id := node.folder.ID
		result = append(result, folderOption{
			id:    &id,
			name:  node.folder.Name,
			depth: depth,
		})
		result = append(result, flattenFolderOptions(node.children, depth+1)...)
	}
	return result
}

func (m FolderPickerModel) Init() tea.Cmd {
	return nil
}

func (m FolderPickerModel) Update(msg tea.Msg) (FolderPickerModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return FolderPickerCancelMsg{} }

		case "enter":
			selected := m.options[m.cursor]
			return m, func() tea.Msg { return FolderSelectedMsg{FolderID: selected.id} }

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
			return m, nil
		}
	}

	return m, nil
}

func (m FolderPickerModel) View() string {
	var content strings.Builder

	content.WriteString(StyleTitle.Render(m.title))
	content.WriteString("\n\n")

	for i, opt := range m.options {
		indent := strings.Repeat("  ", opt.depth)

		var icon string
		if opt.id == nil {
			icon = "○ "
		} else {
			icon = "▶ "
		}

		line := indent + icon + opt.name
		if i == m.cursor {
			content.WriteString(StylePickerSelected.Render("> " + line))
		} else {
			content.WriteString("  " + line)
		}
		content.WriteString("\n")
	}

	content.WriteString("\n")
	content.WriteString(StyleDialogHint.Render("j/k navigate • enter select • esc cancel"))

	return StyleDialogBox.Width(m.width).Render(content.String())
}
