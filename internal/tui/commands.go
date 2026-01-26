package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bantamhq/ephemeral/internal/client"
)

func (m Model) addRepoFolder(repoID, folderID string) tea.Cmd {
	return func() tea.Msg {
		_, err := m.client.AddRepoFolders(context.Background(), repoID, []string{folderID})
		if err != nil {
			return ActionErrorMsg{Operation: "add folder", Err: err}
		}
		return repoFolderAddedMsg{RepoID: repoID, FolderID: folderID}
	}
}

func (m Model) removeRepoFolder(repoID, folderID string) tea.Cmd {
	return func() tea.Msg {
		err := m.client.RemoveRepoFolder(context.Background(), repoID, folderID)
		if err != nil {
			return ActionErrorMsg{Operation: "remove folder", Err: err}
		}
		return repoFolderRemovedMsg{RepoID: repoID, FolderID: folderID}
	}
}

func (m Model) createFolder(name string) tea.Cmd {
	return func() tea.Msg {
		folder, err := m.client.CreateFolder(context.Background(), name)
		if err != nil {
			return ActionErrorMsg{Operation: "create folder", Err: err}
		}
		return FolderCreatedMsg{Folder: *folder}
	}
}

func (m Model) renameRepo(id, name string) tea.Cmd {
	return func() tea.Msg {
		repo, err := m.client.UpdateRepo(context.Background(), id, &name, nil, nil)
		if err != nil {
			return ActionErrorMsg{Operation: "rename repo", Err: err}
		}
		return RepoUpdatedMsg{Repo: *repo}
	}
}

func (m Model) updateRepoDescription(id, description string) tea.Cmd {
	return func() tea.Msg {
		repo, err := m.client.UpdateRepo(context.Background(), id, nil, &description, nil)
		if err != nil {
			return ActionErrorMsg{Operation: "update repo description", Err: err}
		}
		return RepoUpdatedMsg{Repo: *repo}
	}
}

func (m Model) renameFolder(id, name string) tea.Cmd {
	return func() tea.Msg {
		folder, err := m.client.UpdateFolder(context.Background(), id, &name)
		if err != nil {
			return ActionErrorMsg{Operation: "rename folder", Err: err}
		}
		return FolderUpdatedMsg{Folder: *folder}
	}
}

func (m Model) deleteRepo(id string) tea.Cmd {
	return func() tea.Msg {
		if err := m.client.DeleteRepo(context.Background(), id); err != nil {
			return ActionErrorMsg{Operation: "delete repo", Err: err}
		}
		return RepoDeletedMsg{ID: id}
	}
}

func (m Model) deleteFolder(id string) tea.Cmd {
	return func() tea.Msg {
		if err := m.client.DeleteFolder(context.Background(), id, true); err != nil {
			return ActionErrorMsg{Operation: "delete folder", Err: err}
		}
		return FolderDeletedMsg{ID: id}
	}
}

func (m Model) runClone(cloneURL, targetDir, repoName string) tea.Cmd {
	token := m.client.Token()
	return func() tea.Msg {
		destPath := filepath.Join(targetDir, repoName)

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
		folders, _, err := m.client.ListFolders(context.Background(), "", 0)
		if err != nil {
			return errMsg{err}
		}

		sort.Slice(folders, func(i, j int) bool {
			return strings.ToLower(folders[i].Name) < strings.ToLower(folders[j].Name)
		})

		reposWithFolders, hasMore, err := m.client.ListReposWithFolders(context.Background(), "", repoPageSize)
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
		reposWithFolders, hasMore, err := m.client.ListReposWithFolders(context.Background(), cursor, repoPageSize)
		if err != nil {
			return ActionErrorMsg{Operation: "load additional repos", Err: err}
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

func (m Model) loadNamespaces() tea.Cmd {
	return func() tea.Msg {
		namespaces, err := m.client.ListNamespaces(context.Background())
		if err != nil {
			return ActionErrorMsg{Operation: "load namespaces", Err: err}
		}
		return namespacesLoadedMsg{namespaces: namespaces}
	}
}

func (m Model) loadDetail(repoID string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		refs, err := m.client.ListRefs(ctx, repoID)
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
			commits, _, err = m.client.ListCommits(ctx, repoID, defaultRef, "", detailCommitsLimit)
			if err != nil {
				return DetailLoadedMsg{RepoID: repoID, Err: fmt.Errorf("list commits for %s: %w", defaultRef, err)}
			}
			tree, err = m.client.GetTreeWithDepth(ctx, repoID, defaultRef, "", 3)
			if err != nil {
				return DetailLoadedMsg{RepoID: repoID, Err: fmt.Errorf("get tree for %s: %w", defaultRef, err)}
			}

			readmeResp, err := m.client.GetReadme(ctx, repoID, defaultRef)
			if err != nil {
				return DetailLoadedMsg{RepoID: repoID, Err: fmt.Errorf("get readme: %w", err)}
			}
			if readmeResp != nil && !readmeResp.IsBinary {
				readme = &readmeResp.Content
				readmeFilename = readmeResp.Filename
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
