package store

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"ephemeral/internal/core"
)

// Initialize creates the database schema.
func (s *SQLiteStore) Initialize() error {
	if err := s.createSchema(); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}

	return nil
}

func (s *SQLiteStore) createSchema() error {
	schema := `
	-- Namespaces provide isolation
	CREATE TABLE IF NOT EXISTS namespaces (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

		-- Soft limits (enforced by platform, tracked by core)
		repo_limit INTEGER,           -- NULL = unlimited
		storage_limit_bytes INTEGER,  -- NULL = unlimited

		-- For platform correlation (opaque to core)
		external_id TEXT              -- e.g., platform user_id or org_id
	);

	-- Tokens are the only auth primitive
	CREATE TABLE IF NOT EXISTS tokens (
		id TEXT PRIMARY KEY,
		token_hash TEXT NOT NULL,          -- argon2id hash with embedded salt
		token_lookup TEXT NOT NULL,        -- first 8 chars of ID for fast lookup (global, not per-namespace)
		name TEXT,                         -- human-friendly label
		is_admin BOOLEAN NOT NULL DEFAULT FALSE,  -- admin tokens only access /api/v1/admin/* routes

		-- Scope for user tokens
		scope TEXT NOT NULL DEFAULT 'full',  -- 'full' | 'repos' | 'read-only'

		-- Lifecycle
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP,            -- NULL = never
		last_used_at TIMESTAMP
	);

	-- Junction table for token namespace access (user tokens only)
	CREATE TABLE IF NOT EXISTS token_namespace_access (
		token_id TEXT NOT NULL REFERENCES tokens(id) ON DELETE CASCADE,
		namespace_id TEXT NOT NULL REFERENCES namespaces(id) ON DELETE RESTRICT,
		is_primary BOOLEAN NOT NULL DEFAULT FALSE,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (token_id, namespace_id)
	);

	-- Folders for organizing repos (flat, no nesting)
	CREATE TABLE IF NOT EXISTS folders (
		id TEXT PRIMARY KEY,
		namespace_id TEXT NOT NULL REFERENCES namespaces(id) ON DELETE CASCADE,
		name TEXT NOT NULL,
		color TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

		UNIQUE(namespace_id, name)
	);

	-- Repositories
	CREATE TABLE IF NOT EXISTS repos (
		id TEXT PRIMARY KEY,
		namespace_id TEXT NOT NULL REFERENCES namespaces(id) ON DELETE CASCADE,
		name TEXT NOT NULL,
		description TEXT,

		-- Visibility
		public BOOLEAN DEFAULT FALSE,  -- If true, anonymous read access allowed

		-- Stats
		size_bytes INTEGER DEFAULT 0,
		last_push_at TIMESTAMP,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

		UNIQUE(namespace_id, name)
	);

	-- Many-to-many relationship between repos and folders
	CREATE TABLE IF NOT EXISTS repo_folders (
		repo_id TEXT REFERENCES repos(id) ON DELETE CASCADE,
		folder_id TEXT REFERENCES folders(id) ON DELETE CASCADE,
		PRIMARY KEY (repo_id, folder_id)
	);

	-- Create indexes
	CREATE INDEX IF NOT EXISTS idx_repos_namespace ON repos(namespace_id);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_tokens_lookup ON tokens(token_lookup);
	CREATE INDEX IF NOT EXISTS idx_folders_namespace ON folders(namespace_id);

	-- Ensure each token has at most one primary namespace
	CREATE UNIQUE INDEX IF NOT EXISTS idx_token_primary_ns
		ON token_namespace_access(token_id) WHERE is_primary = TRUE;
	`

	_, err := s.db.Exec(schema)
	return err
}


// GenerateAdminToken creates and returns an admin token for first-time setup.
// Returns empty string if an admin token already exists.
func (s *SQLiteStore) GenerateAdminToken() (string, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM tokens WHERE is_admin = TRUE").Scan(&count)
	if err != nil {
		return "", fmt.Errorf("check existing admin token: %w", err)
	}

	if count > 0 {
		return "", nil
	}

	const tokenCreateAttempts = 5

	for attempt := 0; attempt < tokenCreateAttempts; attempt++ {
		tokenID := uuid.New().String()
		tokenLookup := tokenID[:8]

		secret, err := core.GenerateTokenSecret(24)
		if err != nil {
			return "", fmt.Errorf("generate token secret: %w", err)
		}

		tokenValue := core.BuildToken(tokenLookup, secret)

		tokenHash, err := core.HashToken(tokenValue)
		if err != nil {
			return "", fmt.Errorf("hash token: %w", err)
		}

		name := "Admin Token"
		token := &Token{
			ID:          tokenID,
			TokenHash:   tokenHash,
			TokenLookup: tokenLookup,
			Name:        &name,
			IsAdmin:     true,
			Scope:       ScopeFull,
			CreatedAt:   time.Now(),
		}

		if err := s.CreateToken(token); err != nil {
			if errors.Is(err, ErrTokenLookupCollision) {
				continue
			}
			return "", fmt.Errorf("create admin token: %w", err)
		}

		return tokenValue, nil
	}

	return "", fmt.Errorf("create admin token: %w", ErrTokenLookupCollision)
}
