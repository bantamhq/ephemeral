package server

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bantamhq/ephemeral/internal/core"
)

// ValidateName validates a namespace or repository name.
func ValidateName(name string) error {
	return core.ValidateName(name)
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
