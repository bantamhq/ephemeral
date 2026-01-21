package store

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"ephemeral/internal/core"
)

// Initialize creates the database schema and initial data.
func (s *SQLiteStore) Initialize() error {
	if err := s.createSchema(); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}

	if err := s.createDefaultNamespace(); err != nil {
		return fmt.Errorf("create default namespace: %w", err)
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
		token_lookup TEXT NOT NULL,        -- first 8 chars of ID for fast lookup
		name TEXT,                         -- human-friendly label
		namespace_id TEXT NOT NULL REFERENCES namespaces(id) ON DELETE CASCADE,

		-- Scope
		scope TEXT NOT NULL DEFAULT 'full',  -- 'full' | 'repos' | 'read-only' | 'admin'

		-- Optional: limit to specific repos (NULL = all in namespace)
		repo_ids TEXT,  -- JSON array, NULL means all

		-- Lifecycle
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP,            -- NULL = never
		last_used_at TIMESTAMP,

		-- For platform correlation
		external_id TEXT                 -- e.g., platform token record id
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
	CREATE UNIQUE INDEX IF NOT EXISTS idx_tokens_lookup ON tokens(namespace_id, token_lookup);
	CREATE INDEX IF NOT EXISTS idx_folders_namespace ON folders(namespace_id);
	`

	_, err := s.db.Exec(schema)
	return err
}

func (s *SQLiteStore) createDefaultNamespace() error {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM namespaces WHERE id = 'default'").Scan(&count)
	if err != nil {
		return fmt.Errorf("check default namespace: %w", err)
	}

	if count > 0 {
		return nil
	}

	_, err = s.db.Exec(`
		INSERT INTO namespaces (id, name, created_at)
		VALUES ('default', 'default', ?)
	`, time.Now())
	if err != nil {
		return fmt.Errorf("insert default namespace: %w", err)
	}

	return nil
}

// GenerateRootToken creates and returns the root token for first-time setup.
// Returns empty string if a root token already exists.
func (s *SQLiteStore) GenerateRootToken() (string, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM tokens WHERE scope = 'admin' AND namespace_id = 'default'").Scan(&count)
	if err != nil {
		return "", fmt.Errorf("check existing root token: %w", err)
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

		tokenValue := core.BuildToken("default", tokenLookup, secret)

		tokenHash, err := core.HashToken(tokenValue)
		if err != nil {
			return "", fmt.Errorf("hash token: %w", err)
		}

		name := "Root Token"
		token := &Token{
			ID:          tokenID,
			TokenHash:   tokenHash,
			TokenLookup: tokenLookup,
			Name:        &name,
			NamespaceID: "default",
			Scope:       "admin",
			CreatedAt:   time.Now(),
		}

		if err := s.CreateToken(token); err != nil {
			if errors.Is(err, ErrTokenLookupCollision) {
				continue
			}
			return "", fmt.Errorf("create root token: %w", err)
		}

		return tokenValue, nil
	}

	return "", fmt.Errorf("create root token: %w", ErrTokenLookupCollision)
}
