package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteStore implements the Store interface using SQLite.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore creates a new SQLite store.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath+"?_journal=WAL")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// SQLite only supports one writer at a time
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// CreateToken creates a new token.
func (s *SQLiteStore) CreateToken(token *Token) error {
	query := `
		INSERT INTO tokens (
			id, token_hash, name, namespace_id, scope,
			repo_ids, created_at, expires_at, external_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.Exec(query,
		token.ID,
		token.TokenHash,
		ToNullString(token.Name),
		token.NamespaceID,
		token.Scope,
		ToNullString(token.RepoIDs),
		token.CreatedAt,
		ToNullTime(token.ExpiresAt),
		ToNullString(token.ExternalID),
	)

	if err != nil {
		return fmt.Errorf("insert token: %w", err)
	}
	return nil
}

// GetTokenByHash retrieves a token by its hash.
func (s *SQLiteStore) GetTokenByHash(hash string) (*Token, error) {
	query := `
		SELECT id, token_hash, name, namespace_id, scope,
			   repo_ids, created_at, expires_at, last_used_at, external_id
		FROM tokens
		WHERE token_hash = ?
	`

	token, err := s.scanToken(s.db.QueryRow(query, hash))
	if err != nil || token == nil {
		return token, err
	}

	go func() {
		s.db.Exec("UPDATE tokens SET last_used_at = ? WHERE id = ?", time.Now(), token.ID)
	}()

	return token, nil
}

// CreateRepo creates a new repository.
func (s *SQLiteStore) CreateRepo(repo *Repo) error {
	query := `
		INSERT INTO repos (
			id, namespace_id, name, folder_id, public,
			size_bytes, last_push_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.Exec(query,
		repo.ID,
		repo.NamespaceID,
		repo.Name,
		ToNullString(repo.FolderID),
		repo.Public,
		repo.SizeBytes,
		ToNullTime(repo.LastPushAt),
		repo.CreatedAt,
		repo.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert repo: %w", err)
	}
	return nil
}

// GetRepo retrieves a repository by namespace and name.
func (s *SQLiteStore) GetRepo(namespaceID, name string) (*Repo, error) {
	query := `
		SELECT id, namespace_id, name, folder_id, public,
			   size_bytes, last_push_at, created_at, updated_at
		FROM repos
		WHERE namespace_id = ? AND name = ?
	`
	return s.scanRepo(s.db.QueryRow(query, namespaceID, name))
}

// GetRepoByID retrieves a repository by ID.
func (s *SQLiteStore) GetRepoByID(id string) (*Repo, error) {
	query := `
		SELECT id, namespace_id, name, folder_id, public,
			   size_bytes, last_push_at, created_at, updated_at
		FROM repos
		WHERE id = ?
	`
	return s.scanRepo(s.db.QueryRow(query, id))
}

func (s *SQLiteStore) scanRepo(row *sql.Row) (*Repo, error) {
	var repo Repo
	var folderID sql.NullString
	var lastPushAt sql.NullTime

	err := row.Scan(
		&repo.ID,
		&repo.NamespaceID,
		&repo.Name,
		&folderID,
		&repo.Public,
		&repo.SizeBytes,
		&lastPushAt,
		&repo.CreatedAt,
		&repo.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan repo: %w", err)
	}

	repo.FolderID = FromNullString(folderID)
	repo.LastPushAt = FromNullTime(lastPushAt)

	return &repo, nil
}

// UpdateRepoLastPush updates the last push time for a repository.
func (s *SQLiteStore) UpdateRepoLastPush(id string, pushTime time.Time) error {
	query := `
		UPDATE repos
		SET last_push_at = ?, updated_at = ?
		WHERE id = ?
	`
	_, err := s.db.Exec(query, pushTime, time.Now(), id)
	if err != nil {
		return fmt.Errorf("update repo last_push_at: %w", err)
	}
	return nil
}

// GetNamespace retrieves a namespace by ID.
func (s *SQLiteStore) GetNamespace(id string) (*Namespace, error) {
	query := `
		SELECT id, name, created_at, repo_limit, storage_limit_bytes, external_id
		FROM namespaces
		WHERE id = ?
	`
	return s.scanNamespace(s.db.QueryRow(query, id))
}

// GetNamespaceByName retrieves a namespace by name.
func (s *SQLiteStore) GetNamespaceByName(name string) (*Namespace, error) {
	query := `
		SELECT id, name, created_at, repo_limit, storage_limit_bytes, external_id
		FROM namespaces
		WHERE name = ?
	`
	return s.scanNamespace(s.db.QueryRow(query, name))
}

func (s *SQLiteStore) scanNamespace(row *sql.Row) (*Namespace, error) {
	var ns Namespace
	var repoLimit, storageLimit sql.NullInt64
	var externalID sql.NullString

	err := row.Scan(
		&ns.ID,
		&ns.Name,
		&ns.CreatedAt,
		&repoLimit,
		&storageLimit,
		&externalID,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan namespace: %w", err)
	}

	ns.RepoLimit = FromNullInt64(repoLimit)
	ns.StorageLimitBytes = FromNullInt64(storageLimit)
	ns.ExternalID = FromNullString(externalID)

	return &ns, nil
}

// CreateNamespace creates a new namespace.
func (s *SQLiteStore) CreateNamespace(ns *Namespace) error {
	query := `
		INSERT INTO namespaces (id, name, created_at, repo_limit, storage_limit_bytes, external_id)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.Exec(query,
		ns.ID,
		ns.Name,
		ns.CreatedAt,
		ToNullInt64(ns.RepoLimit),
		ToNullInt64(ns.StorageLimitBytes),
		ToNullString(ns.ExternalID),
	)
	if err != nil {
		return fmt.Errorf("insert namespace: %w", err)
	}
	return nil
}

// ListNamespaces lists all namespaces with cursor-based pagination.
func (s *SQLiteStore) ListNamespaces(cursor string, limit int) ([]Namespace, error) {
	query := `
		SELECT id, name, created_at, repo_limit, storage_limit_bytes, external_id
		FROM namespaces
		WHERE id > ?
		ORDER BY id
		LIMIT ?
	`

	rows, err := s.db.Query(query, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("query namespaces: %w", err)
	}
	defer rows.Close()

	var namespaces []Namespace
	for rows.Next() {
		var ns Namespace
		var repoLimit, storageLimit sql.NullInt64
		var externalID sql.NullString

		if err := rows.Scan(
			&ns.ID,
			&ns.Name,
			&ns.CreatedAt,
			&repoLimit,
			&storageLimit,
			&externalID,
		); err != nil {
			return nil, fmt.Errorf("scan namespace: %w", err)
		}

		ns.RepoLimit = FromNullInt64(repoLimit)
		ns.StorageLimitBytes = FromNullInt64(storageLimit)
		ns.ExternalID = FromNullString(externalID)
		namespaces = append(namespaces, ns)
	}

	return namespaces, rows.Err()
}

// DeleteNamespace deletes a namespace and cascades to repos and tokens.
func (s *SQLiteStore) DeleteNamespace(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM tokens WHERE namespace_id = ?", id); err != nil {
		return fmt.Errorf("delete tokens: %w", err)
	}

	if _, err := tx.Exec("DELETE FROM repos WHERE namespace_id = ?", id); err != nil {
		return fmt.Errorf("delete repos: %w", err)
	}

	if _, err := tx.Exec("DELETE FROM namespaces WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete namespace: %w", err)
	}

	return tx.Commit()
}

// GetTokenByID retrieves a token by ID.
func (s *SQLiteStore) GetTokenByID(id string) (*Token, error) {
	query := `
		SELECT id, token_hash, name, namespace_id, scope,
			   repo_ids, created_at, expires_at, last_used_at, external_id
		FROM tokens
		WHERE id = ?
	`
	return s.scanToken(s.db.QueryRow(query, id))
}

func (s *SQLiteStore) scanToken(row *sql.Row) (*Token, error) {
	var token Token
	var name, repoIDs, externalID sql.NullString
	var expiresAt, lastUsedAt sql.NullTime

	err := row.Scan(
		&token.ID,
		&token.TokenHash,
		&name,
		&token.NamespaceID,
		&token.Scope,
		&repoIDs,
		&token.CreatedAt,
		&expiresAt,
		&lastUsedAt,
		&externalID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan token: %w", err)
	}

	token.Name = FromNullString(name)
	token.RepoIDs = FromNullString(repoIDs)
	token.ExpiresAt = FromNullTime(expiresAt)
	token.LastUsedAt = FromNullTime(lastUsedAt)
	token.ExternalID = FromNullString(externalID)

	return &token, nil
}

// ListTokens lists tokens in a namespace with cursor-based pagination.
func (s *SQLiteStore) ListTokens(namespaceID, cursor string, limit int) ([]Token, error) {
	query := `
		SELECT id, token_hash, name, namespace_id, scope,
			   repo_ids, created_at, expires_at, last_used_at, external_id
		FROM tokens
		WHERE namespace_id = ? AND id > ?
		ORDER BY id
		LIMIT ?
	`

	rows, err := s.db.Query(query, namespaceID, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("query tokens: %w", err)
	}
	defer rows.Close()

	var tokens []Token
	for rows.Next() {
		var token Token
		var name, repoIDs, externalID sql.NullString
		var expiresAt, lastUsedAt sql.NullTime

		if err := rows.Scan(
			&token.ID,
			&token.TokenHash,
			&name,
			&token.NamespaceID,
			&token.Scope,
			&repoIDs,
			&token.CreatedAt,
			&expiresAt,
			&lastUsedAt,
			&externalID,
		); err != nil {
			return nil, fmt.Errorf("scan token: %w", err)
		}

		token.Name = FromNullString(name)
		token.RepoIDs = FromNullString(repoIDs)
		token.ExpiresAt = FromNullTime(expiresAt)
		token.LastUsedAt = FromNullTime(lastUsedAt)
		token.ExternalID = FromNullString(externalID)
		tokens = append(tokens, token)
	}

	return tokens, rows.Err()
}

// DeleteToken deletes a token by ID.
func (s *SQLiteStore) DeleteToken(id string) error {
	result, err := s.db.Exec("DELETE FROM tokens WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete token: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// ListRepos lists repos in a namespace with cursor-based pagination.
func (s *SQLiteStore) ListRepos(namespaceID, cursor string, limit int) ([]Repo, error) {
	query := `
		SELECT id, namespace_id, name, folder_id, public,
			   size_bytes, last_push_at, created_at, updated_at
		FROM repos
		WHERE namespace_id = ? AND id > ?
		ORDER BY id
		LIMIT ?
	`

	rows, err := s.db.Query(query, namespaceID, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("query repos: %w", err)
	}
	defer rows.Close()

	var repos []Repo
	for rows.Next() {
		var repo Repo
		var folderID sql.NullString
		var lastPushAt sql.NullTime

		if err := rows.Scan(
			&repo.ID,
			&repo.NamespaceID,
			&repo.Name,
			&folderID,
			&repo.Public,
			&repo.SizeBytes,
			&lastPushAt,
			&repo.CreatedAt,
			&repo.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan repo: %w", err)
		}

		repo.FolderID = FromNullString(folderID)
		repo.LastPushAt = FromNullTime(lastPushAt)
		repos = append(repos, repo)
	}

	return repos, rows.Err()
}

// UpdateRepo updates a repository.
func (s *SQLiteStore) UpdateRepo(repo *Repo) error {
	query := `
		UPDATE repos
		SET name = ?, folder_id = ?, public = ?, updated_at = ?
		WHERE id = ?
	`

	result, err := s.db.Exec(query,
		repo.Name,
		ToNullString(repo.FolderID),
		repo.Public,
		time.Now(),
		repo.ID,
	)
	if err != nil {
		return fmt.Errorf("update repo: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// DeleteRepo deletes a repository by ID.
func (s *SQLiteStore) DeleteRepo(id string) error {
	result, err := s.db.Exec("DELETE FROM repos WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete repo: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}

	return nil
}