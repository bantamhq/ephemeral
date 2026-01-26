package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
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
	cmd.Flags().Bool("no-push", false, "skip pushing after adding remote")

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

	repo, err := c.CreateRepo(context.Background(), repoName, nil, false)
	if err != nil {
		return formatAPIError("create repo", err)
	}

	remoteURL := fmt.Sprintf("%s/git/%s/%s.git", cfg.Server, namespace, repoName)

	gitDir := filepath.Join(workDir, ".git")
	existingRepo := dirExists(gitDir)

	if existingRepo {
		noPush, _ := cmd.Flags().GetBool("no-push")
		return setupRemoteOnly(workDir, remoteURL, cfg.Server, repo.Name, cfg.Token, noPush)
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

func setupRemoteOnly(workDir, remoteURL, serverURL, repoName, token string, noPush bool) error {
	repo, err := git.PlainOpen(workDir)
	if err != nil {
		return fmt.Errorf("open git repo: %w", err)
	}

	remoteName, remoteAction, err := configureRemote(repo, remoteURL, serverURL)
	if err != nil {
		return err
	}

	if remoteName == "" {
		return nil
	}

	fmt.Printf("Created repository '%s'\n", repoName)
	fmt.Printf("%s remote '%s': %s\n", remoteAction, remoteName, remoteURL)

	if noPush {
		return nil
	}

	if err := pushToRemote(repo, remoteName, token); err != nil {
		if errors.Is(err, git.NoErrAlreadyUpToDate) {
			fmt.Println("Already up to date")
			return nil
		}
		return err
	}

	fmt.Printf("Pushed current branch to %s\n", remoteName)
	return nil
}

func configureRemote(repo *git.Repository, remoteURL, serverURL string) (remoteName, action string, err error) {
	existingRemote, err := repo.Remote("origin")
	if errors.Is(err, git.ErrRemoteNotFound) {
		if err := createRemote(repo, "origin", remoteURL); err != nil {
			return "", "", err
		}
		return "origin", "Added", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("check remote: %w", err)
	}

	existingURL := remoteFirstURL(existingRemote)
	ephGitPrefix := serverURL + "/git/"

	if strings.HasPrefix(existingURL, ephGitPrefix) {
		if err := replaceRemote(repo, "origin", remoteURL); err != nil {
			return "", "", err
		}
		return "origin", "Updated", nil
	}

	replaceOrigin, customName, err := promptRemoteConflict(existingURL)
	if err != nil {
		return "", "", err
	}

	if customName == "" {
		fmt.Println("Cancelled")
		return "", "", nil
	}

	if replaceOrigin {
		if err := replaceRemote(repo, "origin", remoteURL); err != nil {
			return "", "", err
		}
		return "origin", "Replaced", nil
	}

	existingCustom, err := repo.Remote(customName)
	if err == nil {
		customURL := remoteFirstURL(existingCustom)
		if strings.HasPrefix(customURL, ephGitPrefix) {
			if err := replaceRemote(repo, customName, remoteURL); err != nil {
				return "", "", err
			}
			return customName, "Updated", nil
		}
		return "", "", fmt.Errorf("remote %q already exists", customName)
	}

	if err := createRemote(repo, customName, remoteURL); err != nil {
		return "", "", err
	}
	return customName, "Added", nil
}

func remoteFirstURL(remote *git.Remote) string {
	if urls := remote.Config().URLs; len(urls) > 0 {
		return urls[0]
	}
	return ""
}

func createRemote(repo *git.Repository, name, url string) error {
	_, err := repo.CreateRemote(&gitconfig.RemoteConfig{
		Name: name,
		URLs: []string{url},
	})
	if err != nil {
		return fmt.Errorf("create remote: %w", err)
	}
	return nil
}

func replaceRemote(repo *git.Repository, name, url string) error {
	if err := repo.DeleteRemote(name); err != nil {
		return fmt.Errorf("delete old remote: %w", err)
	}
	return createRemote(repo, name, url)
}

func promptRemoteConflict(existingURL string) (replaceOrigin bool, customName string, err error) {
	var replace bool
	var remoteName string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Remote 'origin' already exists").
				Description(existingURL).
				Affirmative("Replace origin").
				Negative("Use different name").
				Value(&replace),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Remote name").
				Placeholder("eph").
				Value(&remoteName),
		).WithHideFunc(func() bool {
			return replace
		}),
	).WithTheme(huh.ThemeBase())

	if err := form.Run(); err != nil {
		return false, "", fmt.Errorf("configure remote: %w", err)
	}

	if replace {
		return true, "origin", nil
	}

	if remoteName == "" {
		remoteName = "eph"
	}

	return false, remoteName, nil
}

func pushToRemote(repo *git.Repository, remoteName, token string) error {
	err := repo.Push(&git.PushOptions{
		RemoteName: remoteName,
		Auth: &http.BasicAuth{
			Username: "x-token",
			Password: token,
		},
	})
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}
	return nil
}

func initAndPush(workDir, remoteURL, repoName, token string) error {
	repo, err := git.PlainInit(workDir, false)
	if err != nil {
		return fmt.Errorf("git init: %w", err)
	}

	headPath := filepath.Join(workDir, ".git", "HEAD")
	if err := os.WriteFile(headPath, []byte("ref: refs/heads/main\n"), 0644); err != nil {
		return fmt.Errorf("set default branch: %w", err)
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

	if err := pushToRemote(repo, "origin", token); err != nil {
		return err
	}

	fmt.Printf("Created repository '%s'\n", repoName)
	fmt.Printf("Remote: %s\n", remoteURL)
	fmt.Println("Initialized with README.md and pushed to origin")

	return nil
}
