package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/bantamhq/ephemeral/internal/client"
)

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

func formatRepoDescription(repo client.Repo) string {
	desc := repoDescription(&repo)
	if desc == "" {
		return "No description"
	}
	return desc
}

func repoDescription(repo *client.Repo) string {
	if repo.Description == nil {
		return ""
	}
	return *repo.Description
}

func firstLine(s string) string {
	if idx := strings.Index(s, "\n"); idx != -1 {
		return s[:idx]
	}
	return s
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
