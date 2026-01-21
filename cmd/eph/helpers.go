package main

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"

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
		return string(tokenBytes), nil
	}

	reader := bufio.NewReader(os.Stdin)
	token, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(token), nil
}

func configureGitHelper(serverURL string) error {
	cmd := exec.Command("git", "config", "--global",
		"credential."+serverURL+".helper", "eph")
	return cmd.Run()
}

func unconfigureGitHelper(serverURL string) error {
	cmd := exec.Command("git", "config", "--global", "--unset",
		"credential."+serverURL+".helper")
	return cmd.Run()
}

func generateSessionID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

func generateCodeVerifier() (string, error) {
	bytes := make([]byte, 48)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func generateCodeChallenge(codeVerifier string) string {
	hash := sha256.Sum256([]byte(codeVerifier))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

func readLine() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
