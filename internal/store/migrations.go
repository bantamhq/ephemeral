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
		external_id TEXT              -- e.g., platform user_id or org_id
	);

	-- Tokens are the only auth primitive
	CREATE TABLE IF NOT EXISTS tokens (
		id TEXT PRIMARY KEY,
		token_hash TEXT NOT NULL,          -- argon2id hash with embedded salt
		token_lookup TEXT NOT NULL,        -- first 8 chars of ID for fast lookup (global, not per-namespace)
		name TEXT,                         -- human-friendly label
		is_admin BOOLEAN NOT NULL DEFAULT FALSE,  -- admin tokens only access /api/v1/admin/* routes

		-- Lifecycle
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP,            -- NULL = never
		last_used_at TIMESTAMP
	);

	-- Namespace grants: permissions a token has for a namespace
	CREATE TABLE IF NOT EXISTS token_namespace_grants (
		token_id TEXT NOT NULL REFERENCES tokens(id) ON DELETE CASCADE,
		namespace_id TEXT NOT NULL REFERENCES namespaces(id) ON DELETE CASCADE,
		allow_bits INTEGER NOT NULL DEFAULT 0,
		deny_bits INTEGER NOT NULL DEFAULT 0,
		is_primary BOOLEAN NOT NULL DEFAULT FALSE,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (token_id, namespace_id)
	);

	-- Repo grants: permissions a token has for a specific repo
	CREATE TABLE IF NOT EXISTS token_repo_grants (
		token_id TEXT NOT NULL REFERENCES tokens(id) ON DELETE CASCADE,
		repo_id TEXT NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
		allow_bits INTEGER NOT NULL DEFAULT 0,
		deny_bits INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (token_id, repo_id)
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

	-- Create indexes
	CREATE INDEX IF NOT EXISTS idx_repos_namespace ON repos(namespace_id);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_tokens_lookup ON tokens(token_lookup);
	CREATE INDEX IF NOT EXISTS idx_folders_namespace ON folders(namespace_id);
	CREATE INDEX IF NOT EXISTS idx_lfs_objects_repo ON lfs_objects(repo_id);

	-- Ensure each token has at most one primary namespace
	CREATE UNIQUE INDEX IF NOT EXISTS idx_token_primary_ns
		ON token_namespace_grants(token_id) WHERE is_primary = TRUE;
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

// GenerateUserTokenWithGrants creates a new user token with the specified namespace grants.
// Returns both the raw token (for display) and the Token struct.
// This creates the token and grants in a single transaction.
func (s *SQLiteStore) GenerateUserTokenWithGrants(name *string, expiresAt *time.Time, namespaceGrants []NamespaceGrant, repoGrants []RepoGrant) (string, *Token, error) {
	const tokenCreateAttempts = 5

	for attempt := 0; attempt < tokenCreateAttempts; attempt++ {
		tokenID := uuid.New().String()
		tokenLookup := tokenID[:8]

		secret, err := core.GenerateTokenSecret(24)
		if err != nil {
			return "", nil, fmt.Errorf("generate token secret: %w", err)
		}

		tokenValue := core.BuildToken(tokenLookup, secret)

		tokenHash, err := core.HashToken(tokenValue)
		if err != nil {
			return "", nil, fmt.Errorf("hash token: %w", err)
		}

		now := time.Now()
		token := &Token{
			ID:          tokenID,
			TokenHash:   tokenHash,
			TokenLookup: tokenLookup,
			Name:        name,
			IsAdmin:     false,
			CreatedAt:   now,
			ExpiresAt:   expiresAt,
		}

		tx, err := s.db.Begin()
		if err != nil {
			return "", nil, fmt.Errorf("begin transaction: %w", err)
		}
		defer tx.Rollback()

		if err := s.createTokenTx(tx, token); err != nil {
			if errors.Is(err, ErrTokenLookupCollision) {
				continue
			}
			return "", nil, fmt.Errorf("create user token: %w", err)
		}

		for _, grant := range namespaceGrants {
			grant.TokenID = tokenID
			grant.CreatedAt = now
			grant.UpdatedAt = now
			if err := s.upsertNamespaceGrantTx(tx, &grant); err != nil {
				return "", nil, fmt.Errorf("create namespace grant: %w", err)
			}
		}

		for _, grant := range repoGrants {
			grant.TokenID = tokenID
			grant.CreatedAt = now
			grant.UpdatedAt = now
			if err := s.upsertRepoGrantTx(tx, &grant); err != nil {
				return "", nil, fmt.Errorf("create repo grant: %w", err)
			}
		}

		if err := tx.Commit(); err != nil {
			return "", nil, fmt.Errorf("commit transaction: %w", err)
		}

		return tokenValue, token, nil
	}

	return "", nil, fmt.Errorf("create user token: %w", ErrTokenLookupCollision)
}
