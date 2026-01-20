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
			id, namespace_id, name, public,
			size_bytes, last_push_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.Exec(query,
		repo.ID,
		repo.NamespaceID,
		repo.Name,
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
		SELECT id, namespace_id, name, public,
			   size_bytes, last_push_at, created_at, updated_at
		FROM repos
		WHERE namespace_id = ? AND name = ?
	`
	return s.scanRepo(s.db.QueryRow(query, namespaceID, name))
}

// GetRepoByID retrieves a repository by ID.
func (s *SQLiteStore) GetRepoByID(id string) (*Repo, error) {
	query := `
		SELECT id, namespace_id, name, public,
			   size_bytes, last_push_at, created_at, updated_at
		FROM repos
		WHERE id = ?
	`
	return s.scanRepo(s.db.QueryRow(query, id))
}

func (s *SQLiteStore) scanRepo(row *sql.Row) (*Repo, error) {
	var repo Repo
	var lastPushAt sql.NullTime

	err := row.Scan(
		&repo.ID,
		&repo.NamespaceID,
		&repo.Name,
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

// DeleteNamespace deletes a namespace and cascades to all related data.
func (s *SQLiteStore) DeleteNamespace(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete repo_folders for repos in this namespace
	if _, err := tx.Exec(`
		DELETE FROM repo_folders
		WHERE repo_id IN (SELECT id FROM repos WHERE namespace_id = ?)
	`, id); err != nil {
		return fmt.Errorf("delete repo folders: %w", err)
	}

	// Delete folders in this namespace
	if _, err := tx.Exec("DELETE FROM folders WHERE namespace_id = ?", id); err != nil {
		return fmt.Errorf("delete folders: %w", err)
	}

	// Delete tokens in this namespace
	if _, err := tx.Exec("DELETE FROM tokens WHERE namespace_id = ?", id); err != nil {
		return fmt.Errorf("delete tokens: %w", err)
	}

	// Delete repos in this namespace
	if _, err := tx.Exec("DELETE FROM repos WHERE namespace_id = ?", id); err != nil {
		return fmt.Errorf("delete repos: %w", err)
	}

	// Finally delete the namespace itself
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
	var rows *sql.Rows
	var err error

	if limit > 0 {
		query := `
			SELECT id, namespace_id, name, public,
				   size_bytes, last_push_at, created_at, updated_at
			FROM repos
			WHERE namespace_id = ? AND name > ?
			ORDER BY name
			LIMIT ?
		`
		rows, err = s.db.Query(query, namespaceID, cursor, limit)
	} else {
		query := `
			SELECT id, namespace_id, name, public,
				   size_bytes, last_push_at, created_at, updated_at
			FROM repos
			WHERE namespace_id = ? AND name > ?
			ORDER BY name
		`
		rows, err = s.db.Query(query, namespaceID, cursor)
	}
	if err != nil {
		return nil, fmt.Errorf("query repos: %w", err)
	}
	defer rows.Close()

	var repos []Repo
	for rows.Next() {
		var repo Repo
		var lastPushAt sql.NullTime

		if err := rows.Scan(
			&repo.ID,
			&repo.NamespaceID,
			&repo.Name,
			&repo.Public,
			&repo.SizeBytes,
			&lastPushAt,
			&repo.CreatedAt,
			&repo.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan repo: %w", err)
		}

		repo.LastPushAt = FromNullTime(lastPushAt)
		repos = append(repos, repo)
	}

	return repos, rows.Err()
}

// UpdateRepo updates a repository.
func (s *SQLiteStore) UpdateRepo(repo *Repo) error {
	query := `
		UPDATE repos
		SET name = ?, public = ?, updated_at = ?
		WHERE id = ?
	`

	result, err := s.db.Exec(query,
		repo.Name,
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

// CreateFolder creates a new folder.
func (s *SQLiteStore) CreateFolder(folder *Folder) error {
	query := `
		INSERT INTO folders (id, namespace_id, name, color, created_at)
		VALUES (?, ?, ?, ?, ?)
	`

	_, err := s.db.Exec(query,
		folder.ID,
		folder.NamespaceID,
		folder.Name,
		ToNullString(folder.Color),
		folder.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert folder: %w", err)
	}
	return nil
}

// GetFolderByID retrieves a folder by ID.
func (s *SQLiteStore) GetFolderByID(id string) (*Folder, error) {
	query := `
		SELECT id, namespace_id, name, color, created_at
		FROM folders
		WHERE id = ?
	`
	return s.scanFolder(s.db.QueryRow(query, id))
}

// GetFolderByName retrieves a folder by namespace and name.
func (s *SQLiteStore) GetFolderByName(namespaceID, name string) (*Folder, error) {
	query := `
		SELECT id, namespace_id, name, color, created_at
		FROM folders
		WHERE namespace_id = ? AND name = ?
	`
	return s.scanFolder(s.db.QueryRow(query, namespaceID, name))
}

func (s *SQLiteStore) scanFolder(row *sql.Row) (*Folder, error) {
	var folder Folder
	var color sql.NullString

	err := row.Scan(
		&folder.ID,
		&folder.NamespaceID,
		&folder.Name,
		&color,
		&folder.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan folder: %w", err)
	}

	folder.Color = FromNullString(color)
	return &folder, nil
}

// ListFolders lists all folders in a namespace.
func (s *SQLiteStore) ListFolders(namespaceID, cursor string, limit int) ([]Folder, error) {
	var rows *sql.Rows
	var err error

	if limit > 0 {
		query := `
			SELECT id, namespace_id, name, color, created_at
			FROM folders
			WHERE namespace_id = ? AND name > ?
			ORDER BY name
			LIMIT ?
		`
		rows, err = s.db.Query(query, namespaceID, cursor, limit)
	} else {
		query := `
			SELECT id, namespace_id, name, color, created_at
			FROM folders
			WHERE namespace_id = ? AND name > ?
			ORDER BY name
		`
		rows, err = s.db.Query(query, namespaceID, cursor)
	}
	if err != nil {
		return nil, fmt.Errorf("query folders: %w", err)
	}
	defer rows.Close()

	var folders []Folder
	for rows.Next() {
		var folder Folder
		var color sql.NullString

		if err := rows.Scan(
			&folder.ID,
			&folder.NamespaceID,
			&folder.Name,
			&color,
			&folder.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan folder: %w", err)
		}

		folder.Color = FromNullString(color)
		folders = append(folders, folder)
	}

	return folders, rows.Err()
}

// UpdateFolder updates a folder.
func (s *SQLiteStore) UpdateFolder(folder *Folder) error {
	query := `
		UPDATE folders
		SET name = ?, color = ?
		WHERE id = ?
	`

	result, err := s.db.Exec(query,
		folder.Name,
		ToNullString(folder.Color),
		folder.ID,
	)
	if err != nil {
		return fmt.Errorf("update folder: %w", err)
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

// DeleteFolder deletes a folder by ID.
func (s *SQLiteStore) DeleteFolder(id string) error {
	result, err := s.db.Exec("DELETE FROM folders WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete folder: %w", err)
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

// CountFolderRepos counts repos in a folder.
func (s *SQLiteStore) CountFolderRepos(id string) (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM repo_folders WHERE folder_id = ?", id).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count folder repos: %w", err)
	}
	return count, nil
}

// AddRepoFolder adds a folder to a repo.
func (s *SQLiteStore) AddRepoFolder(repoID, folderID string) error {
	query := `INSERT OR IGNORE INTO repo_folders (repo_id, folder_id) VALUES (?, ?)`
	_, err := s.db.Exec(query, repoID, folderID)
	if err != nil {
		return fmt.Errorf("add repo folder: %w", err)
	}
	return nil
}

// RemoveRepoFolder removes a folder from a repo.
func (s *SQLiteStore) RemoveRepoFolder(repoID, folderID string) error {
	result, err := s.db.Exec("DELETE FROM repo_folders WHERE repo_id = ? AND folder_id = ?", repoID, folderID)
	if err != nil {
		return fmt.Errorf("remove repo folder: %w", err)
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

// ListRepoFolders lists all folders for a repo.
func (s *SQLiteStore) ListRepoFolders(repoID string) ([]Folder, error) {
	query := `
		SELECT f.id, f.namespace_id, f.name, f.color, f.created_at
		FROM folders f
		JOIN repo_folders rf ON f.id = rf.folder_id
		WHERE rf.repo_id = ?
		ORDER BY f.name
	`

	rows, err := s.db.Query(query, repoID)
	if err != nil {
		return nil, fmt.Errorf("query repo folders: %w", err)
	}
	defer rows.Close()

	var folders []Folder
	for rows.Next() {
		var folder Folder
		var color sql.NullString

		if err := rows.Scan(
			&folder.ID,
			&folder.NamespaceID,
			&folder.Name,
			&color,
			&folder.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan folder: %w", err)
		}

		folder.Color = FromNullString(color)
		folders = append(folders, folder)
	}

	return folders, rows.Err()
}

// ListFolderRepos lists all repos in a folder.
func (s *SQLiteStore) ListFolderRepos(folderID string) ([]Repo, error) {
	query := `
		SELECT r.id, r.namespace_id, r.name, r.public,
			   r.size_bytes, r.last_push_at, r.created_at, r.updated_at
		FROM repos r
		JOIN repo_folders rf ON r.id = rf.repo_id
		WHERE rf.folder_id = ?
		ORDER BY r.name
	`

	rows, err := s.db.Query(query, folderID)
	if err != nil {
		return nil, fmt.Errorf("query folder repos: %w", err)
	}
	defer rows.Close()

	var repos []Repo
	for rows.Next() {
		var repo Repo
		var lastPushAt sql.NullTime

		if err := rows.Scan(
			&repo.ID,
			&repo.NamespaceID,
			&repo.Name,
			&repo.Public,
			&repo.SizeBytes,
			&lastPushAt,
			&repo.CreatedAt,
			&repo.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan repo: %w", err)
		}

		repo.LastPushAt = FromNullTime(lastPushAt)
		repos = append(repos, repo)
	}

	return repos, rows.Err()
}

// SetRepoFolders replaces all folders for a repo with the given folder IDs.
func (s *SQLiteStore) SetRepoFolders(repoID string, folderIDs []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM repo_folders WHERE repo_id = ?", repoID); err != nil {
		return fmt.Errorf("delete existing repo folders: %w", err)
	}

	for _, folderID := range folderIDs {
		if _, err := tx.Exec("INSERT INTO repo_folders (repo_id, folder_id) VALUES (?, ?)", repoID, folderID); err != nil {
			return fmt.Errorf("insert repo folder: %w", err)
		}
	}

	return tx.Commit()
}