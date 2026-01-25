package tui

import (
	_ "embed"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bantamhq/ephemeral/internal/client"
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
	modalHelp
)

type detailTab int

const (
	tabDetails detailTab = iota
	tabReadme
	tabActivity
	tabFiles
)

// Model represents the TUI application state.
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

	editingFolder      *client.Folder
	editingRepo        *client.Repo
	editingDescription bool
	editText           string

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
	help    help.Model
}

// NewModel creates a new TUI model with the given client and configuration.
func NewModel(c *client.Client, namespace, server string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	helpModel := help.New()

	return Model{
		client:         c,
		namespace:      namespace,
		server:         server,
		loading:        true,
		spinner:        s,
		keys:           DefaultKeyMap,
		help:           helpModel,
		repoFolders:    make(map[string][]client.Folder),
		folderCounts:   make(map[string]int),
		detailCache:    make(map[string]*RepoDetail),
		detailScroll:   make(map[detailTab]int),
		detailViewport: viewport.New(0, 0),
	}
}

// Init initializes the TUI model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.loadData())
}

// Run starts the TUI application with the given client and configuration.
func Run(c *client.Client, namespace, server string) error {
	p := tea.NewProgram(
		NewModel(c, namespace, server),
		tea.WithAltScreen(),
	)

	_, err := p.Run()
	return err
}
