package tui

import "ephemeral/internal/client"

type FolderCreatedMsg struct {
	Folder client.Folder
}

type RepoUpdatedMsg struct {
	Repo client.Repo
}

type RepoDeletedMsg struct {
	ID string
}

type FolderDeletedMsg struct {
	ID string
}

type CloneStartedMsg struct {
	RepoName string
	Dir      string
}

type CloneCompletedMsg struct {
	RepoName string
	Dir      string
}

type CloneFailedMsg struct {
	RepoName string
	Err      error
}

type ActionErrorMsg struct {
	Operation string
	Err       error
}
