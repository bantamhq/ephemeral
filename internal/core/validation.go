package core

import (
	"fmt"
	"regexp"
	"strings"
)

// Name validation constraints
const (
	MaxNameLength = 128
	MinNameLength = 1
)

// validNamePattern allows alphanumeric characters, dots, underscores, and hyphens.
// Must start with alphanumeric character.
var validNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// ValidateName validates a namespace or repository name.
func ValidateName(name string) error {
	if len(name) < MinNameLength {
		return fmt.Errorf("name is required")
	}

	if len(name) > MaxNameLength {
		return fmt.Errorf("name exceeds maximum length of %d characters", MaxNameLength)
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

// IsValidNameChar returns true if the rune is valid in a name.
// For the first character, only ASCII letters and digits are allowed.
// For subsequent characters, dots, underscores, and hyphens are also allowed.
func IsValidNameChar(r rune, isFirst bool) bool {
	if isFirst {
		return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
	}
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
}

// ValidateNameInput validates a sequence of runes being added to an existing name.
// Returns true if all runes are valid for their position in the resulting string.
func ValidateNameInput(runes []rune, currentText string) bool {
	baseRunes := []rune(currentText)
	baseLen := len(baseRunes)
	prevDot := baseLen > 0 && baseRunes[baseLen-1] == '.'

	for i, r := range runes {
		isFirst := baseLen+i == 0
		if !IsValidNameChar(r, isFirst) {
			return false
		}
		if r == '.' && prevDot {
			return false
		}
		prevDot = r == '.'
	}
	return true
}
