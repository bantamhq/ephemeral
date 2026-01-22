package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/spf13/cobra"

	"github.com/bantamhq/ephemeral/internal/client"
	"github.com/bantamhq/ephemeral/internal/config"
)

func newNewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new [repo-name]",
		Short: "Create a new repository",
		Long: `Create a new repository on the server and initialize it locally.

If no repo-name is specified, uses the current directory name.
If repo-name is specified, creates a new subdirectory with that name.

For existing git repositories, only adds/updates the origin remote.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runNew,
	}

	cmd.Flags().StringP("namespace", "n", "", "namespace to create the repo in (defaults to your default namespace)")

	return cmd
}

func runNew(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return errNotLoggedIn
	}

	if !cfg.IsConfigured() {
		return errNotLoggedIn
	}

	namespace, _ := cmd.Flags().GetString("namespace")
	if namespace == "" {
		namespace = cfg.DefaultNamespace
	}

	c := client.New(cfg.Server, cfg.Token)
	if namespace != "" {
		c = c.WithNamespace(namespace)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	var repoName string
	var workDir string

	if len(args) > 0 {
		repoName = args[0]
		workDir = filepath.Join(cwd, repoName)

		if err := os.MkdirAll(workDir, 0755); err != nil {
			return fmt.Errorf("create directory: %w", err)
		}
	} else {
		repoName = filepath.Base(cwd)
		workDir = cwd
	}

	repo, err := c.CreateRepo(repoName, nil, false)
	if err != nil {
		return formatAPIError("create repo", err)
	}

	remoteURL := fmt.Sprintf("%s/git/%s/%s.git", cfg.Server, namespace, repoName)

	gitDir := filepath.Join(workDir, ".git")
	existingRepo := dirExists(gitDir)

	if existingRepo {
		return setupRemoteOnly(workDir, remoteURL, repo.Name)
	}

	return initAndPush(workDir, remoteURL, repoName, cfg.Token)
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func setupRemoteOnly(workDir, remoteURL, repoName string) error {
	repo, err := git.PlainOpen(workDir)
	if err != nil {
		return fmt.Errorf("open git repo: %w", err)
	}

	_, err = repo.Remote("origin")
	if errors.Is(err, git.ErrRemoteNotFound) {
		_, err = repo.CreateRemote(&gitconfig.RemoteConfig{
			Name: "origin",
			URLs: []string{remoteURL},
		})
		if err != nil {
			return fmt.Errorf("create remote: %w", err)
		}
		fmt.Printf("Created repository '%s'\n", repoName)
		fmt.Printf("Added remote: %s\n", remoteURL)
		return nil
	}
	if err != nil {
		return fmt.Errorf("check remote: %w", err)
	}

	if err := repo.DeleteRemote("origin"); err != nil {
		return fmt.Errorf("delete old remote: %w", err)
	}

	_, err = repo.CreateRemote(&gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{remoteURL},
	})
	if err != nil {
		return fmt.Errorf("create remote: %w", err)
	}

	fmt.Printf("Created repository '%s'\n", repoName)
	fmt.Printf("Updated remote: %s\n", remoteURL)
	return nil
}

func initAndPush(workDir, remoteURL, repoName, token string) error {
	repo, err := git.PlainInit(workDir, false)
	if err != nil {
		return fmt.Errorf("git init: %w", err)
	}

	_, err = repo.CreateRemote(&gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{remoteURL},
	})
	if err != nil {
		return fmt.Errorf("create remote: %w", err)
	}

	readmePath := filepath.Join(workDir, "README.md")
	readmeContent := fmt.Sprintf("# %s\n", repoName)
	if err := os.WriteFile(readmePath, []byte(readmeContent), 0644); err != nil {
		return fmt.Errorf("write README.md: %w", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("get worktree: %w", err)
	}

	if _, err := worktree.Add("README.md"); err != nil {
		return fmt.Errorf("stage README.md: %w", err)
	}

	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "eph",
			Email: "eph@localhost",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	err = repo.Push(&git.PushOptions{
		RemoteName: "origin",
		Auth: &http.BasicAuth{
			Username: "x-token",
			Password: token,
		},
	})
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}

	fmt.Printf("Created repository '%s'\n", repoName)
	fmt.Printf("Remote: %s\n", remoteURL)
	fmt.Println("Initialized with README.md and pushed to origin")

	return nil
}
