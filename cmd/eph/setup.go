package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"

	"ephemeral/internal/store"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12"))

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("8")).
			Padding(1, 2)

	sectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("10"))

	tokenStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("14"))

	subtleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	dividerChar = "-"
	equalChar   = "="
)

type SetupWizard struct {
	store   store.Store
	dataDir string
}

type SetupResult struct {
	NamespaceName string
	AdminToken    string
	UserToken     string
}

func NewSetupWizard(st store.Store, dataDir string) *SetupWizard {
	return &SetupWizard{
		store:   st,
		dataDir: dataDir,
	}
}

func (w *SetupWizard) Run() (*SetupResult, error) {
	w.showWelcome()

	namespaceName, err := w.promptNamespace()
	if err != nil {
		return nil, err
	}

	result, err := w.createResources(namespaceName)
	if err != nil {
		return nil, err
	}

	w.showResults(result)

	return result, nil
}

func (w *SetupWizard) showWelcome() {
	welcome := []string{
		titleStyle.Render("Welcome to Ephemeral"),
		"",
		"Let's set up your git hosting server.",
		"",
		"This wizard will create:",
		subtleStyle.Render("  * A namespace for your repositories"),
		subtleStyle.Render("  * An admin token (for server management)"),
		subtleStyle.Render("  * A user token (for git operations)"),
	}

	fmt.Println()
	fmt.Println(boxStyle.Render(strings.Join(welcome, "\n")))
	fmt.Println()
}

func (w *SetupWizard) promptNamespace() (string, error) {
	var namespaceName string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Namespace name").
				Description("The namespace for your repositories").
				Placeholder("default").
				Value(&namespaceName),
		),
	)

	if err := form.Run(); err != nil {
		return "", fmt.Errorf("form input: %w", err)
	}

	if namespaceName == "" {
		namespaceName = "default"
	}

	return namespaceName, nil
}

func (w *SetupWizard) createResources(namespaceName string) (*SetupResult, error) {
	ns, err := w.store.GetNamespaceByName(namespaceName)
	if err != nil {
		return nil, fmt.Errorf("check namespace: %w", err)
	}

	var namespaceID string
	if ns == nil {
		ns = &store.Namespace{
			ID:        uuid.New().String(),
			Name:      namespaceName,
			CreatedAt: time.Now(),
		}
		if err := w.store.CreateNamespace(ns); err != nil {
			return nil, fmt.Errorf("create namespace: %w", err)
		}
	}
	namespaceID = ns.ID

	adminToken, err := w.store.GenerateAdminToken()
	if err != nil {
		return nil, fmt.Errorf("generate admin token: %w", err)
	}

	if adminToken != "" {
		tokenPath := filepath.Join(w.dataDir, "admin-token")
		if err := os.WriteFile(tokenPath, []byte(adminToken), 0600); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save admin token to file: %v\n", err)
		}
	}

	userName := "User Token"
	userToken, _, err := w.store.GenerateUserToken(namespaceID, &userName, store.ScopeFull)
	if err != nil {
		return nil, fmt.Errorf("generate user token: %w", err)
	}

	return &SetupResult{
		NamespaceName: namespaceName,
		AdminToken:    adminToken,
		UserToken:     userToken,
	}, nil
}

func (w *SetupWizard) showResults(result *SetupResult) {
	width := 60

	fmt.Println()
	fmt.Println(strings.Repeat(equalChar, width))
	fmt.Println(sectionStyle.Render("SETUP COMPLETE"))
	fmt.Println(strings.Repeat(equalChar, width))
	fmt.Println()

	if result.AdminToken != "" {
		fmt.Printf("Admin token (saved to %s):\n", filepath.Join(w.dataDir, "admin-token"))
		fmt.Printf("  %s\n", tokenStyle.Render(result.AdminToken))
		fmt.Println()
	}

	fmt.Println("User token (for daily use):")
	fmt.Printf("  %s\n", tokenStyle.Render(result.UserToken))

	fmt.Println()
	fmt.Println(strings.Repeat(dividerChar, width))
	fmt.Println(sectionStyle.Render("NEXT STEPS") + " - Run on your " + titleStyle.Render("LOCAL") + " machine:")
	fmt.Println(strings.Repeat(dividerChar, width))
	fmt.Println()
	fmt.Println("  eph login https://your-server.example.com")
	fmt.Println()
	fmt.Println("When prompted, paste the USER TOKEN above.")
	fmt.Println(strings.Repeat(equalChar, width))
	fmt.Println()
}

