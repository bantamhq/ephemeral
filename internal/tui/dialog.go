package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"ephemeral/internal/core"
)

type DialogMode int

const (
	DialogInput DialogMode = iota
	DialogConfirm
)

func validateNameInput(runes []rune, currentText string) bool {
	return core.ValidateNameInput(runes, currentText)
}

type DialogSubmitMsg struct {
	Value string
}

type DialogCancelMsg struct{}

type DialogModel struct {
	mode           DialogMode
	title          string
	message        string
	input          textinput.Model
	confirmText    string
	cancelText     string
	focused        int
	width          int
	filterNameChar bool
}

func NewInputDialog(title, message, placeholder string) DialogModel {
	return newInputDialog(title, message, placeholder, false)
}

func NewNameInputDialog(title, message, placeholder string) DialogModel {
	return newInputDialog(title, message, placeholder, true)
}

func newInputDialog(title, message, placeholder string, filterNameChar bool) DialogModel {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Focus()
	ti.CharLimit = nameMaxLength
	ti.Width = dialogInputWidth

	return DialogModel{
		mode:           DialogInput,
		title:          title,
		message:        message,
		input:          ti,
		width:          dialogWidth,
		filterNameChar: filterNameChar,
	}
}

func NewConfirmDialog(title, message string) DialogModel {
	return DialogModel{
		mode:        DialogConfirm,
		title:       title,
		message:     message,
		confirmText: "Confirm",
		cancelText:  "Cancel",
		focused:     1,
		width:       dialogWidth,
	}
}

func (d DialogModel) Init() tea.Cmd {
	if d.mode == DialogInput {
		return textinput.Blink
	}
	return nil
}

func (d DialogModel) Update(msg tea.Msg) (DialogModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return d, func() tea.Msg { return DialogCancelMsg{} }

		case "enter":
			if d.mode == DialogInput {
				return d, func() tea.Msg { return DialogSubmitMsg{Value: d.input.Value()} }
			}
			if d.focused == 0 {
				return d, func() tea.Msg { return DialogSubmitMsg{} }
			}
			return d, func() tea.Msg { return DialogCancelMsg{} }

		case "tab", "shift+tab", "left", "right":
			if d.mode == DialogConfirm {
				d.focused = 1 - d.focused
			}
			return d, nil
		}

		if d.mode == DialogInput && d.filterNameChar && len(msg.Runes) > 0 {
			if !validateNameInput(msg.Runes, d.input.Value()) {
				return d, nil
			}
		}
	}

	if d.mode == DialogInput {
		var cmd tea.Cmd
		d.input, cmd = d.input.Update(msg)
		return d, cmd
	}

	return d, nil
}

func (d DialogModel) View() string {
	var content strings.Builder

	content.WriteString(StyleHeader.Render(d.title))
	content.WriteString("\n\n")

	if d.message != "" {
		content.WriteString(d.message)
		content.WriteString("\n\n")
	}

	if d.mode == DialogInput {
		content.WriteString(d.input.View())
		content.WriteString("\n\n")
		content.WriteString(StyleDialogHint.Render("enter submit • esc cancel"))
	} else {
		confirmStyle := StyleDialogButton
		cancelStyle := StyleDialogButton
		if d.focused == 0 {
			confirmStyle = StyleDialogButtonFocused
		} else {
			cancelStyle = StyleDialogButtonFocused
		}

		buttons := lipgloss.JoinHorizontal(
			lipgloss.Center,
			confirmStyle.Render(d.confirmText),
			"  ",
			cancelStyle.Render(d.cancelText),
		)
		content.WriteString(buttons)
	}

	return StyleDialogBox.Width(d.width).Render(content.String())
}

func (d *DialogModel) SetValue(value string) {
	d.input.SetValue(value)
}

type FolderPickerItem struct {
	ID       string
	Name     string
	Selected bool
}

type FolderPickerCloseMsg struct{}

type FolderPickerToggleMsg struct {
	FolderID string
	Selected bool
}

type FolderPickerModel struct {
	repoID   string
	repoName string
	items    []FolderPickerItem
	cursor   int
	width    int
	height   int
}

func NewFolderPickerModel(repoID, repoName string, allFolders []FolderPickerItem) FolderPickerModel {
	return FolderPickerModel{
		repoID:   repoID,
		repoName: repoName,
		items:    allFolders,
		cursor:   0,
		width:    folderPickerWidth,
		height:   folderPickerHeight,
	}
}

func (f FolderPickerModel) Init() tea.Cmd {
	return nil
}

func (f FolderPickerModel) Update(msg tea.Msg) (FolderPickerModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return f, nil
	}

	switch keyMsg.String() {
	case "esc":
		return f, func() tea.Msg { return FolderPickerCloseMsg{} }

	case "up", "k":
		if f.cursor > 0 {
			f.cursor--
		}
		return f, nil

	case "down", "j":
		if f.cursor < len(f.items)-1 {
			f.cursor++
		}
		return f, nil

	case "enter":
		if f.cursor >= len(f.items) {
			return f, nil
		}
		item := &f.items[f.cursor]
		item.Selected = !item.Selected
		return f, func() tea.Msg {
			return FolderPickerToggleMsg{
				FolderID: item.ID,
				Selected: item.Selected,
			}
		}
	}

	return f, nil
}

func (f FolderPickerModel) View() string {
	var content strings.Builder

	title := "Manage Folders"
	if f.repoName != "" {
		title = "Folders: " + f.repoName
	}
	content.WriteString(StyleHeader.Render(title))
	content.WriteString("\n\n")

	if len(f.items) == 0 {
		content.WriteString(StyleMetaText.Render("No folders available"))
		content.WriteString("\n\n")
		content.WriteString(StyleDialogHint.Render("esc close"))
		return StyleDialogBox.Width(f.width).Render(content.String())
	}

	startIdx, endIdx := f.visibleRange()
	for i := startIdx; i < endIdx; i++ {
		item := f.items[i]
		line := f.renderItem(item, i == f.cursor)
		content.WriteString(line)
		content.WriteString("\n")
	}
	content.WriteString("\n")
	content.WriteString(StyleDialogHint.Render("enter toggle • esc close"))

	return StyleDialogBox.Width(f.width).Render(content.String())
}

func (f FolderPickerModel) visibleRange() (start, end int) {
	const maxVisible = folderPickerMaxItems
	start = 0
	if f.cursor >= maxVisible {
		start = f.cursor - maxVisible + 1
	}
	end = start + maxVisible
	if end > len(f.items) {
		end = len(f.items)
	}
	return start, end
}

func (f FolderPickerModel) renderItem(item FolderPickerItem, isCursor bool) string {
	check := " "
	if item.Selected {
		check = "✓"
	}

	line := " " + check + " " + item.Name

	if isCursor {
		return StylePickerSelected.Width(f.width - 4).Render(line)
	}
	return line
}

func (f FolderPickerModel) RepoID() string {
	return f.repoID
}
