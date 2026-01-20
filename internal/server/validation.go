package server

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Name validation constraints
const (
	maxNameLength = 128
	minNameLength = 1
)

// validNamePattern allows alphanumeric characters, dots, underscores, and hyphens.
// Must start with alphanumeric character.
var validNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// ValidateName validates a namespace or repository name.
func ValidateName(name string) error {
	if len(name) < minNameLength {
		return fmt.Errorf("name is required")
	}

	if len(name) > maxNameLength {
		return fmt.Errorf("name exceeds maximum length of %d characters", maxNameLength)
	}

	if strings.Contains(name, "..") {
		return fmt.Errorf("name cannot contain '..'")
	}

	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("name cannot contain path separators")
	}

	if !validNamePattern.MatchString(name) {
		return fmt.Errorf("name must start with alphanumeric and contain only alphanumeric, dots, underscores, or hyphens")
	}

	return nil
}

// SafeRepoPath constructs a safe repository path and validates it stays under dataDir.
func SafeRepoPath(dataDir, namespaceID, repoName string) (string, error) {
	if err := ValidateName(repoName); err != nil {
		return "", fmt.Errorf("invalid repo name: %w", err)
	}

	repoPath := filepath.Join(dataDir, "repos", namespaceID, repoName+".git")

	cleanPath := filepath.Clean(repoPath)
	expectedPrefix := filepath.Clean(filepath.Join(dataDir, "repos"))
	if !strings.HasPrefix(cleanPath, expectedPrefix+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid path: escapes data directory")
	}

	return cleanPath, nil
}

// SafeNamespacePath constructs a safe namespace directory path.
func SafeNamespacePath(dataDir, namespaceID string) (string, error) {
	nsPath := filepath.Join(dataDir, "repos", namespaceID)

	cleanPath := filepath.Clean(nsPath)

	expectedPrefix := filepath.Clean(filepath.Join(dataDir, "repos"))
	if !strings.HasPrefix(cleanPath, expectedPrefix+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid path: escapes data directory")
	}

	return cleanPath, nil
}

// parseLimit parses a limit string and returns a valid limit between 1-100.
// Returns defaultVal if empty, parsing fails, or value is out of range.
func parseLimit(limitStr string, defaultVal int) int {
	if limitStr == "" {
		return defaultVal
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 100 {
		return defaultVal
	}
	return limit
}
