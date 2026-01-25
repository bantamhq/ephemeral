package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/bantamhq/ephemeral/internal/client"
	"github.com/bantamhq/ephemeral/internal/config"
)

func newCloneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clone [repo]",
		Short: "Clone a repository",
		Long: `Clone a repository from the server.

Examples:
  eph clone                   # Interactive repo selection
  eph clone myrepo            # Clone from default namespace
  eph clone namespace/myrepo  # Clone from specified namespace`,
		Args: cobra.MaximumNArgs(1),
		RunE: runClone,
	}
}

func runClone(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return errNotLoggedIn
	}

	if !cfg.IsConfigured() {
		return errNotLoggedIn
	}

	c := client.New(cfg.Server, cfg.Token)

	var namespace, repoName string

	if len(args) > 0 {
		namespace, repoName = parseRepoArg(args[0], cfg.DefaultNamespace)
	} else {
		selected, err := selectRepoInteractive(c)
		if err != nil {
			return err
		}
		namespace, repoName = selected.namespace, selected.repo
	}

	cloneURL := fmt.Sprintf("%s/git/%s/%s.git", cfg.Server, namespace, repoName)

	return runGitClone(cloneURL, repoName, cfg.Token)
}

func parseRepoArg(arg, defaultNamespace string) (namespace, repo string) {
	if strings.Contains(arg, "/") {
		parts := strings.SplitN(arg, "/", 2)
		return parts[0], parts[1]
	}
	return defaultNamespace, arg
}

type repoSelection struct {
	namespace string
	repo      string
}

func selectRepoInteractive(c *client.Client) (*repoSelection, error) {
	ctx := context.Background()
	namespaces, err := c.ListNamespaces(ctx)
	if err != nil {
		return nil, formatAPIError("list namespaces", err)
	}

	if len(namespaces) == 0 {
		return nil, fmt.Errorf("no namespaces available")
	}

	type repoOption struct {
		namespace string
		repo      client.Repo
	}

	var allRepos []repoOption

	for _, ns := range namespaces {
		nsClient := c.WithNamespace(ns.Name)
		repos, _, err := nsClient.ListRepos(ctx, "", 0)
		if err != nil {
			continue
		}

		for _, repo := range repos {
			allRepos = append(allRepos, repoOption{
				namespace: ns.Name,
				repo:      repo,
			})
		}
	}

	if len(allRepos) == 0 {
		return nil, fmt.Errorf("no repositories found")
	}

	options := make([]huh.Option[repoOption], len(allRepos))
	for i, r := range allRepos {
		label := fmt.Sprintf("%s/%s", r.namespace, r.repo.Name)
		options[i] = huh.NewOption(label, r)
	}

	var selected repoOption

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[repoOption]().
				Title("Select repository").
				Description("Use / to filter").
				Options(options...).
				Value(&selected).
				Height(15),
		),
	).WithTheme(huh.ThemeBase())

	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("select repo: %w", err)
	}

	return &repoSelection{
		namespace: selected.namespace,
		repo:      selected.repo.Name,
	}, nil
}

func runGitClone(url, dir, token string) error {
	authURL := strings.Replace(url, "://", fmt.Sprintf("://x-token:%s@", token), 1)

	cmd := exec.Command("git", "clone", "-c", "credential.helper=", authURL, dir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone: %w", err)
	}

	return nil
}
