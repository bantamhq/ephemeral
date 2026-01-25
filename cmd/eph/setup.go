package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"

	"github.com/bantamhq/ephemeral/internal/store"
)


var validNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

func validateNamespaceName(s string) error {
	if s == "" {
		return nil
	}

	if strings.Contains(s, "..") {
		return fmt.Errorf("cannot contain '..'")
	}

	if strings.Contains(s, "/") || strings.Contains(s, "\\") {
		return fmt.Errorf("cannot contain path separators")
	}

	if !validNamePattern.MatchString(s) {
		return fmt.Errorf("must start with letter/number, use only letters, numbers, dots, underscores, hyphens")
	}

	return nil
}

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
	var (
		createNamespace bool
		namespaceName   string
	)

	// Single input form
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Welcome to Ephemeral").
				Description("Let's set up your git hosting server.").
				Next(true).
				NextLabel("Start"),
		),

		huh.NewGroup(
			huh.NewConfirm().
				Title("Create a user?").
				Description("Creates a user with their own namespace for repositories and a token for daily use.").
				Affirmative("Yes").
				Negative("No").
				Value(&createNamespace).
				WithButtonAlignment(lipgloss.Left),
		),

		huh.NewGroup(
			huh.NewInput().
				Title("Username").
				Description("This will also be the name of your primary namespace.").
				Placeholder("default").
				CharLimit(128).
				Validate(validateNamespaceName).
				Value(&namespaceName),
		).WithHideFunc(func() bool {
			return !createNamespace
		}),
	).WithTheme(huh.ThemeBase()).Run()
	if err != nil {
		return nil, fmt.Errorf("form: %w", err)
	}

	if createNamespace && namespaceName == "" {
		namespaceName = "default"
	}

	// Do all work
	var result SetupResult
	var setupErr error
	err = spinner.New().
		Title("Initializing server...").
		Action(func() {
			time.Sleep(2 * time.Second)
			result, setupErr = w.createResources(createNamespace, namespaceName)
		}).
		Run()
	if err != nil {
		return nil, fmt.Errorf("spinner: %w", err)
	}
	if setupErr != nil {
		return nil, setupErr
	}

	// Print results
	w.printResults(&result)

	return &result, nil
}

func (w *SetupWizard) printResults(result *SetupResult) {
	tokenPath := filepath.Join(w.dataDir, "admin-token")

	var sb strings.Builder
	fmt.Fprintf(&sb, "Setup Complete\n\n")
	fmt.Fprintf(&sb, "Admin token (saved to %s):\n", tokenPath)
	fmt.Fprintf(&sb, "%s\n\n", result.AdminToken)
	fmt.Fprintf(&sb, "Start your server with: eph serve")

	if result.UserToken != "" {
		fmt.Fprintf(&sb, "\n\nUser '%s' created.\n\n", result.NamespaceName)
		fmt.Fprintf(&sb, "User token:\n")
		fmt.Fprintf(&sb, "%s\n\n", result.UserToken)
		fmt.Fprintf(&sb, "Save this token - you'll need it to login.")
	}

	fmt.Println(
		lipgloss.NewStyle().
			Width(60).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("7")).
			Padding(1, 2).
			Render(sb.String()),
	)
}

func (w *SetupWizard) createResources(createNamespace bool, namespaceName string) (SetupResult, error) {
	var result SetupResult

	adminToken, err := w.store.GenerateAdminToken()
	if err != nil {
		return result, fmt.Errorf("generate admin token: %w", err)
	}

	tokenPath := filepath.Join(w.dataDir, "admin-token")
	if err := os.WriteFile(tokenPath, []byte(adminToken), 0600); err != nil {
		return result, fmt.Errorf("save admin token: %w", err)
	}

	result.AdminToken = adminToken

	if !createNamespace {
		return result, nil
	}

	existing, err := w.store.GetNamespaceByName(namespaceName)
	if err != nil {
		return result, fmt.Errorf("check namespace: %w", err)
	}
	if existing != nil {
		return result, fmt.Errorf("namespace %q already exists", namespaceName)
	}

	now := time.Now()

	ns := &store.Namespace{
		ID:        uuid.New().String(),
		Name:      namespaceName,
		CreatedAt: now,
	}
	if err := w.store.CreateNamespace(ns); err != nil {
		return result, fmt.Errorf("create namespace: %w", err)
	}

	user := &store.User{
		ID:                 uuid.New().String(),
		PrimaryNamespaceID: ns.ID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := w.store.CreateUser(user); err != nil {
		return result, fmt.Errorf("create user: %w", err)
	}

	grant := &store.NamespaceGrant{
		UserID:      user.ID,
		NamespaceID: ns.ID,
		AllowBits:   store.DefaultNamespaceGrant(),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := w.store.UpsertNamespaceGrant(grant); err != nil {
		return result, fmt.Errorf("create grant: %w", err)
	}

	userToken, _, err := w.store.GenerateUserToken(user.ID, nil)
	if err != nil {
		return result, fmt.Errorf("create token: %w", err)
	}

	result.NamespaceName = namespaceName
	result.UserToken = userToken

	return result, nil
}
