package store

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/google/uuid"
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
		token_hash TEXT NOT NULL UNIQUE,  -- sha256 of actual token
		name TEXT,                         -- human-friendly label
		namespace_id TEXT NOT NULL REFERENCES namespaces(id),

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

	-- Folders for organizing repos
	CREATE TABLE IF NOT EXISTS folders (
		id TEXT PRIMARY KEY,
		namespace_id TEXT NOT NULL REFERENCES namespaces(id),
		name TEXT NOT NULL,
		parent_id TEXT REFERENCES folders(id) ON DELETE CASCADE,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

		UNIQUE(namespace_id, parent_id, name)
	);

	-- Repositories
	CREATE TABLE IF NOT EXISTS repos (
		id TEXT PRIMARY KEY,
		namespace_id TEXT NOT NULL REFERENCES namespaces(id),
		name TEXT NOT NULL,
		folder_id TEXT REFERENCES folders(id) ON DELETE SET NULL,

		-- Visibility
		public BOOLEAN DEFAULT FALSE,  -- If true, anonymous read access allowed

		-- Stats
		size_bytes INTEGER DEFAULT 0,
		last_push_at TIMESTAMP,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

		UNIQUE(namespace_id, name)
	);

	-- Tags for labeling repos
	CREATE TABLE IF NOT EXISTS tags (
		id TEXT PRIMARY KEY,
		namespace_id TEXT NOT NULL REFERENCES namespaces(id),
		name TEXT NOT NULL,
		color TEXT,  -- hex color for TUI
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

		UNIQUE(namespace_id, name)
	);

	-- Many-to-many relationship between repos and tags
	CREATE TABLE IF NOT EXISTS repo_tags (
		repo_id TEXT REFERENCES repos(id) ON DELETE CASCADE,
		tag_id TEXT REFERENCES tags(id) ON DELETE CASCADE,
		PRIMARY KEY (repo_id, tag_id)
	);

	-- Create indexes
	CREATE INDEX IF NOT EXISTS idx_repos_namespace ON repos(namespace_id);
	CREATE INDEX IF NOT EXISTS idx_tokens_hash ON tokens(token_hash);
	CREATE INDEX IF NOT EXISTS idx_repos_folder ON repos(folder_id);
	CREATE INDEX IF NOT EXISTS idx_folders_parent ON folders(parent_id);
	CREATE INDEX IF NOT EXISTS idx_tags_namespace ON tags(namespace_id);
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

	tokenValue := fmt.Sprintf("eph_default_%s", uuid.New().String())

	hasher := sha256.New()
	hasher.Write([]byte(tokenValue))
	tokenHash := fmt.Sprintf("%x", hasher.Sum(nil))

	name := "Root Token"
	token := &Token{
		ID:          uuid.New().String(),
		TokenHash:   tokenHash,
		Name:        &name,
		NamespaceID: "default",
		Scope:       "admin",
		CreatedAt:   time.Now(),
	}

	if err := s.CreateToken(token); err != nil {
		return "", fmt.Errorf("create root token: %w", err)
	}

	return tokenValue, nil
}