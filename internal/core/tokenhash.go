package core

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argon2Time    = 1
	argon2Memory  = 64 * 1024
	argon2Threads = 4
	argon2KeyLen  = 32
	saltLen       = 16
)

var (
	ErrInvalidHash   = errors.New("invalid hash format")
	ErrInvalidToken  = errors.New("invalid token format")
	ErrHashMismatch  = errors.New("token does not match hash")
)

// HashToken creates an argon2id hash of the token with a random salt.
// Returns the hash in PHC string format: $argon2id$v=19$m=65536,t=1,p=4$<salt>$<hash>
func HashToken(token string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	hash := argon2.IDKey([]byte(token), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argon2Memory,
		argon2Time,
		argon2Threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// VerifyToken checks if a token matches an argon2id hash.
// Returns nil if the token matches, ErrHashMismatch if it doesn't.
func VerifyToken(token, encodedHash string) error {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return ErrInvalidHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return ErrInvalidHash
	}

	var memory, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return ErrInvalidHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return ErrInvalidHash
	}

	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return ErrInvalidHash
	}

	computedHash := argon2.IDKey([]byte(token), salt, time, memory, threads, uint32(len(expectedHash)))

	if subtle.ConstantTimeCompare(computedHash, expectedHash) != 1 {
		return ErrHashMismatch
	}

	return nil
}

// GenerateTokenSecret generates a cryptographically secure random hex string.
func GenerateTokenSecret(length int) (string, error) {
	bytes := make([]byte, (length+1)/2)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return fmt.Sprintf("%x", bytes)[:length], nil
}

// BuildToken constructs a token string from its components.
// Format: eph_<namespace>_<lookup>_<secret>
func BuildToken(namespacePrefix, lookup, secret string) string {
	return fmt.Sprintf("eph_%s_%s_%s", namespacePrefix, lookup, secret)
}

// ParseToken extracts components from a token string.
// Returns namespace prefix, lookup key, and secret.
func ParseToken(token string) (namespacePrefix, lookup, secret string, err error) {
	if !strings.HasPrefix(token, "eph_") {
		return "", "", "", ErrInvalidToken
	}

	parts := strings.Split(token, "_")
	if len(parts) != 4 {
		return "", "", "", ErrInvalidToken
	}

	return parts[1], parts[2], parts[3], nil
}
