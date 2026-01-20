package tui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type DialogMode int

const (
	DialogInput DialogMode = iota
	DialogConfirm
)

func isValidNameChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-'
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
	ti.CharLimit = 128
	ti.Width = 30

	return DialogModel{
		mode:           DialogInput,
		title:          title,
		message:        message,
		input:          ti,
		width:          40,
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
		width:       40,
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

		if d.mode == DialogInput && d.filterNameChar {
			if len(msg.Runes) > 0 {
				for _, r := range msg.Runes {
					if !isValidNameChar(r) {
						return d, nil
					}
				}
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
