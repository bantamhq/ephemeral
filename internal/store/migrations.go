package store

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/bantamhq/ephemeral/internal/core"
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
		external_id TEXT
	);

	-- Users own permissions; tokens are just auth credentials for users
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		primary_namespace_id TEXT NOT NULL UNIQUE REFERENCES namespaces(id) ON DELETE CASCADE,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- Namespace grants: permissions a user has for a namespace
	CREATE TABLE IF NOT EXISTS user_namespace_grants (
		user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		namespace_id TEXT NOT NULL REFERENCES namespaces(id) ON DELETE CASCADE,
		allow_bits INTEGER NOT NULL DEFAULT 0,
		deny_bits INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (user_id, namespace_id)
	);

	-- Repo grants: permissions a user has for a specific repo
	CREATE TABLE IF NOT EXISTS user_repo_grants (
		user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		repo_id TEXT NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
		allow_bits INTEGER NOT NULL DEFAULT 0,
		deny_bits INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (user_id, repo_id)
	);

	-- Tokens are auth credentials; non-admin tokens must belong to a user
	CREATE TABLE IF NOT EXISTS tokens (
		id TEXT PRIMARY KEY,
		token_hash TEXT NOT NULL,          -- argon2id hash with embedded salt
		token_lookup TEXT NOT NULL,        -- first 8 chars of ID for fast lookup
		is_admin BOOLEAN NOT NULL DEFAULT FALSE,  -- admin tokens only access /api/v1/admin/* routes

		-- User binding (required for non-admin tokens, NULL only for admin tokens)
		user_id TEXT REFERENCES users(id) ON DELETE CASCADE,

		-- Lifecycle
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP,            -- NULL = never
		last_used_at TIMESTAMP
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

	-- LFS objects
	CREATE TABLE IF NOT EXISTS lfs_objects (
		repo_id TEXT NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
		oid TEXT NOT NULL,
		size INTEGER NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (repo_id, oid)
	);

	-- Auth sessions for CLI polling-based web auth flow
	CREATE TABLE IF NOT EXISTS auth_sessions (
		id TEXT PRIMARY KEY,
		user_id TEXT REFERENCES users(id) ON DELETE CASCADE,
		token TEXT,
		status TEXT NOT NULL DEFAULT 'pending',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP NOT NULL
	);

	-- Create indexes
	CREATE INDEX IF NOT EXISTS idx_repos_namespace ON repos(namespace_id);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_tokens_lookup ON tokens(token_lookup);
	CREATE INDEX IF NOT EXISTS idx_tokens_user ON tokens(user_id);
	CREATE INDEX IF NOT EXISTS idx_folders_namespace ON folders(namespace_id);
	CREATE INDEX IF NOT EXISTS idx_lfs_objects_repo ON lfs_objects(repo_id);
	CREATE INDEX IF NOT EXISTS idx_namespace_grants_user ON user_namespace_grants(user_id);
	CREATE INDEX IF NOT EXISTS idx_repo_grants_user ON user_repo_grants(user_id);
	CREATE INDEX IF NOT EXISTS idx_users_primary_namespace ON users(primary_namespace_id);
	CREATE INDEX IF NOT EXISTS idx_auth_sessions_expires ON auth_sessions(expires_at);
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

		token := &Token{
			ID:          tokenID,
			TokenHash:   tokenHash,
			TokenLookup: tokenLookup,
			IsAdmin:     true,
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

// HasAdminToken checks if any admin token exists in the database.
func (s *SQLiteStore) HasAdminToken() (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM tokens WHERE is_admin = TRUE").Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check admin token: %w", err)
	}
	return count > 0, nil
}

