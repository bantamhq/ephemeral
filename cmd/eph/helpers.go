package main

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// parseCredentialInput parses git credential protocol input (key=value pairs terminated by empty line).
func parseCredentialInput(r io.Reader) map[string]string {
	result := make(map[string]string)
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}

	return result
}

func hostMatches(serverURL, gitHost string) bool {
	parsed, err := url.Parse(serverURL)
	if err != nil {
		return false
	}

	serverHost := parsed.Host
	serverHost = stripDefaultPort(serverHost, parsed.Scheme)
	gitHost = stripDefaultPort(gitHost, "")

	return strings.EqualFold(serverHost, gitHost)
}

func stripDefaultPort(host, scheme string) string {
	if strings.HasSuffix(host, ":80") && (scheme == "" || scheme == "http") {
		return strings.TrimSuffix(host, ":80")
	}
	if strings.HasSuffix(host, ":443") && (scheme == "" || scheme == "https") {
		return strings.TrimSuffix(host, ":443")
	}
	return host
}

func readToken() (string, error) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		tokenBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(tokenBytes)), nil
	}

	reader := bufio.NewReader(os.Stdin)
	token, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(token), nil
}

func configureGitHelper(serverURL string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	helper := fmt.Sprintf("!%s credential", exePath)
	cmd := exec.Command("git", "config", "--global",
		"credential."+serverURL+".helper", helper)
	return cmd.Run()
}

func unconfigureGitHelper(serverURL string) error {
	cmd := exec.Command("git", "config", "--global", "--unset",
		"credential."+serverURL+".helper")
	return cmd.Run()
}

var errNotLoggedIn = fmt.Errorf("not logged in - run 'eph login' to authenticate")

func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline exceeded") ||
		strings.Contains(errStr, "network is unreachable") ||
		strings.Contains(errStr, "no route to host")
}

func formatAPIError(context string, err error) error {
	errStr := err.Error()

	if strings.Contains(errStr, "connection refused") {
		return fmt.Errorf("%s: could not connect to server", context)
	}

	if strings.Contains(errStr, "no such host") {
		return fmt.Errorf("%s: server not found", context)
	}

	if strings.Contains(errStr, "timeout") {
		return fmt.Errorf("%s: connection timed out", context)
	}

	if strings.Contains(errStr, "401") || strings.Contains(errStr, "unauthorized") {
		return fmt.Errorf("%s: unauthorized", context)
	}

	if strings.Contains(errStr, "403") || strings.Contains(errStr, "forbidden") {
		return fmt.Errorf("%s: permission denied", context)
	}

	if strings.Contains(errStr, "404") || strings.Contains(errStr, "not found") {
		return fmt.Errorf("%s: not found", context)
	}

	if strings.Contains(errStr, "409") || strings.Contains(errStr, "already exists") {
		return fmt.Errorf("%s: already exists", context)
	}

	return fmt.Errorf("%s: %s", context, err.Error())
}

var styleCheckmark = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("✓")

func runSpinner(spinnerTitle, completedTitle string, action func() error) error {
	var actionErr error
	err := spinner.New().
		Title(spinnerTitle).
		Action(func() {
			actionErr = action()
		}).
		Run()
	if err != nil {
		if strings.Contains(err.Error(), "interrupt") || strings.Contains(err.Error(), "killed") {
			os.Exit(0)
		}
		return err
	}
	if actionErr == nil {
		fmt.Printf("%s %s\n", styleCheckmark, completedTitle)
	}
	return actionErr
}
