package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/bantamhq/ephemeral/internal/core"
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
		INSERT INTO tokens (id, token_hash, token_lookup, is_admin, user_id, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.Exec(query,
		token.ID,
		token.TokenHash,
		token.TokenLookup,
		token.IsAdmin,
		ToNullString(token.UserID),
		token.CreatedAt,
		ToNullTime(token.ExpiresAt),
	)

	if err != nil {
		if isTokenLookupCollision(err) {
			return ErrTokenLookupCollision
		}
		return fmt.Errorf("insert token: %w", err)
	}
	return nil
}

func isTokenLookupCollision(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}

	return sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}

// GetTokenByLookup retrieves a token by lookup key.
func (s *SQLiteStore) GetTokenByLookup(lookup string) (*Token, error) {
	query := `
		SELECT id, token_hash, token_lookup, is_admin, user_id,
			   created_at, expires_at, last_used_at
		FROM tokens
		WHERE token_lookup = ?
	`

	token, err := s.scanToken(s.db.QueryRow(query, lookup))
	if err != nil || token == nil {
		return token, err
	}

	go func() {
		if _, err := s.db.Exec("UPDATE tokens SET last_used_at = ? WHERE id = ?", time.Now(), token.ID); err != nil {
			fmt.Printf("Warning: failed to update token last_used_at: %v\n", err)
		}
	}()

	return token, nil
}

// CreateRepo creates a new repository.
func (s *SQLiteStore) CreateRepo(repo *Repo) error {
	query := `
		INSERT INTO repos (
			id, namespace_id, name, description, public,
			size_bytes, last_push_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.Exec(query,
		repo.ID,
		repo.NamespaceID,
		repo.Name,
		ToNullString(repo.Description),
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
		SELECT id, namespace_id, name, description, public,
			   size_bytes, last_push_at, created_at, updated_at
		FROM repos
		WHERE namespace_id = ? AND name = ?
	`
	return s.scanRepo(s.db.QueryRow(query, namespaceID, name))
}

// GetRepoByID retrieves a repository by ID.
func (s *SQLiteStore) GetRepoByID(id string) (*Repo, error) {
	query := `
		SELECT id, namespace_id, name, description, public,
			   size_bytes, last_push_at, created_at, updated_at
		FROM repos
		WHERE id = ?
	`
	return s.scanRepo(s.db.QueryRow(query, id))
}

func (s *SQLiteStore) scanRepo(row *sql.Row) (*Repo, error) {
	var repo Repo
	var description sql.NullString
	var lastPushAt sql.NullTime

	err := row.Scan(
		&repo.ID,
		&repo.NamespaceID,
		&repo.Name,
		&description,
		&repo.Public,
		&repo.SizeBytes,
		&lastPushAt,
		&repo.CreatedAt,
		&repo.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan repo: %w", err)
	}

	repo.Description = FromNullString(description)
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

// UpdateRepoSize updates the stored size of a repository.
func (s *SQLiteStore) UpdateRepoSize(id string, sizeBytes int) error {
	query := `
		UPDATE repos
		SET size_bytes = ?, updated_at = ?
		WHERE id = ?
	`

	result, err := s.db.Exec(query, sizeBytes, time.Now(), id)
	if err != nil {
		return fmt.Errorf("update repo size_bytes: %w", err)
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

	if errors.Is(err, sql.ErrNoRows) {
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

// UpdateNamespace updates a namespace's mutable fields.
func (s *SQLiteStore) UpdateNamespace(ns *Namespace) error {
	result, err := s.db.Exec(`
		UPDATE namespaces
		SET name = ?, repo_limit = ?, storage_limit_bytes = ?
		WHERE id = ?
	`, ns.Name, ToNullInt64(ns.RepoLimit), ToNullInt64(ns.StorageLimitBytes), ns.ID)
	if err != nil {
		return fmt.Errorf("update namespace: %w", err)
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

// DeleteNamespace deletes a namespace. Related tokens, repos, and folders are
// automatically deleted via ON DELETE CASCADE constraints.
func (s *SQLiteStore) DeleteNamespace(id string) error {
	result, err := s.db.Exec("DELETE FROM namespaces WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete namespace: %w", err)
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

// GetTokenByID retrieves a token by ID.
func (s *SQLiteStore) GetTokenByID(id string) (*Token, error) {
	query := `
		SELECT id, token_hash, token_lookup, is_admin, user_id,
			   created_at, expires_at, last_used_at
		FROM tokens
		WHERE id = ?
	`
	return s.scanToken(s.db.QueryRow(query, id))
}

func (s *SQLiteStore) scanToken(row *sql.Row) (*Token, error) {
	var token Token
	var userID sql.NullString
	var expiresAt, lastUsedAt sql.NullTime

	err := row.Scan(
		&token.ID,
		&token.TokenHash,
		&token.TokenLookup,
		&token.IsAdmin,
		&userID,
		&token.CreatedAt,
		&expiresAt,
		&lastUsedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan token: %w", err)
	}

	token.UserID = FromNullString(userID)
	token.ExpiresAt = FromNullTime(expiresAt)
	token.LastUsedAt = FromNullTime(lastUsedAt)

	return &token, nil
}

// ListTokens lists all tokens with cursor-based pagination.
func (s *SQLiteStore) ListTokens(cursor string, limit int) ([]Token, error) {
	query := `
		SELECT id, token_hash, token_lookup, is_admin, user_id,
			   created_at, expires_at, last_used_at
		FROM tokens
		WHERE id > ?
		ORDER BY id
		LIMIT ?
	`

	rows, err := s.db.Query(query, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("query tokens: %w", err)
	}
	defer rows.Close()

	var tokens []Token
	for rows.Next() {
		var token Token
		var userID sql.NullString
		var expiresAt, lastUsedAt sql.NullTime

		if err := rows.Scan(
			&token.ID,
			&token.TokenHash,
			&token.TokenLookup,
			&token.IsAdmin,
			&userID,
			&token.CreatedAt,
			&expiresAt,
			&lastUsedAt,
		); err != nil {
			return nil, fmt.Errorf("scan token: %w", err)
		}

		token.UserID = FromNullString(userID)
		token.ExpiresAt = FromNullTime(expiresAt)
		token.LastUsedAt = FromNullTime(lastUsedAt)
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
			SELECT id, namespace_id, name, description, public,
				   size_bytes, last_push_at, created_at, updated_at
			FROM repos
			WHERE namespace_id = ? AND name > ?
			ORDER BY name
			LIMIT ?
		`
		rows, err = s.db.Query(query, namespaceID, cursor, limit)
	} else {
		query := `
			SELECT id, namespace_id, name, description, public,
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
		var description sql.NullString
		var lastPushAt sql.NullTime

		if err := rows.Scan(
			&repo.ID,
			&repo.NamespaceID,
			&repo.Name,
			&description,
			&repo.Public,
			&repo.SizeBytes,
			&lastPushAt,
			&repo.CreatedAt,
			&repo.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan repo: %w", err)
		}

		repo.Description = FromNullString(description)
		repo.LastPushAt = FromNullTime(lastPushAt)
		repos = append(repos, repo)
	}

	return repos, rows.Err()
}

// ListReposWithFolders lists repos with their folder associations in a single query.
func (s *SQLiteStore) ListReposWithFolders(namespaceID, cursor string, limit int) ([]RepoWithFolders, error) {
	repos, err := s.ListRepos(namespaceID, cursor, limit)
	if err != nil {
		return nil, err
	}

	if len(repos) == 0 {
		return []RepoWithFolders{}, nil
	}

	repoIDs := make([]interface{}, len(repos))
	placeholders := make([]string, len(repos))
	for i, repo := range repos {
		repoIDs[i] = repo.ID
		placeholders[i] = "?"
	}

	query := fmt.Sprintf(`
		SELECT rf.repo_id, f.id, f.namespace_id, f.name, f.color, f.created_at
		FROM repo_folders rf
		JOIN folders f ON f.id = rf.folder_id
		WHERE rf.repo_id IN (%s)
		ORDER BY f.name
	`, strings.Join(placeholders, ","))

	rows, err := s.db.Query(query, repoIDs...)
	if err != nil {
		return nil, fmt.Errorf("query repo folders: %w", err)
	}
	defer rows.Close()

	folderMap := make(map[string][]Folder)
	for rows.Next() {
		var repoID string
		var folder Folder
		var color sql.NullString

		if err := rows.Scan(&repoID, &folder.ID, &folder.NamespaceID, &folder.Name, &color, &folder.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan folder: %w", err)
		}
		folder.Color = FromNullString(color)
		folderMap[repoID] = append(folderMap[repoID], folder)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate folders: %w", err)
	}

	result := make([]RepoWithFolders, len(repos))
	for i, repo := range repos {
		result[i] = RepoWithFolders{
			Repo:    repo,
			Folders: folderMap[repo.ID],
		}
	}

	return result, nil
}

// UpdateRepo updates a repository.
func (s *SQLiteStore) UpdateRepo(repo *Repo) error {
	query := `
		UPDATE repos
		SET name = ?, description = ?, public = ?, updated_at = ?
		WHERE id = ?
	`

	result, err := s.db.Exec(query,
		repo.Name,
		ToNullString(repo.Description),
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
	if errors.Is(err, sql.ErrNoRows) {
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
		SELECT r.id, r.namespace_id, r.name, r.description, r.public,
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
		var description sql.NullString
		var lastPushAt sql.NullTime

		if err := rows.Scan(
			&repo.ID,
			&repo.NamespaceID,
			&repo.Name,
			&description,
			&repo.Public,
			&repo.SizeBytes,
			&lastPushAt,
			&repo.CreatedAt,
			&repo.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan repo: %w", err)
		}

		repo.Description = FromNullString(description)
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

// AddRepoFolders adds multiple folders to a repo atomically.
func (s *SQLiteStore) AddRepoFolders(repoID string, folderIDs []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	for _, folderID := range folderIDs {
		if _, err := tx.Exec("INSERT OR IGNORE INTO repo_folders (repo_id, folder_id) VALUES (?, ?)", repoID, folderID); err != nil {
			return fmt.Errorf("add repo folder: %w", err)
		}
	}

	return tx.Commit()
}

// CreateLFSObject creates a new LFS object record.
func (s *SQLiteStore) CreateLFSObject(obj *LFSObject) error {
	query := `
		INSERT INTO lfs_objects (repo_id, oid, size, created_at)
		VALUES (?, ?, ?, ?)
	`

	_, err := s.db.Exec(query, obj.RepoID, obj.OID, obj.Size, obj.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert lfs object: %w", err)
	}
	return nil
}

// GetLFSObject retrieves an LFS object by repo and OID.
func (s *SQLiteStore) GetLFSObject(repoID, oid string) (*LFSObject, error) {
	query := `
		SELECT repo_id, oid, size, created_at
		FROM lfs_objects
		WHERE repo_id = ? AND oid = ?
	`

	var obj LFSObject
	err := s.db.QueryRow(query, repoID, oid).Scan(
		&obj.RepoID,
		&obj.OID,
		&obj.Size,
		&obj.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan lfs object: %w", err)
	}
	return &obj, nil
}

// ListLFSObjects lists all LFS objects for a repository.
func (s *SQLiteStore) ListLFSObjects(repoID string) ([]LFSObject, error) {
	query := `
		SELECT repo_id, oid, size, created_at
		FROM lfs_objects
		WHERE repo_id = ?
		ORDER BY created_at
	`

	rows, err := s.db.Query(query, repoID)
	if err != nil {
		return nil, fmt.Errorf("query lfs objects: %w", err)
	}
	defer rows.Close()

	var objects []LFSObject
	for rows.Next() {
		var obj LFSObject
		if err := rows.Scan(&obj.RepoID, &obj.OID, &obj.Size, &obj.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan lfs object: %w", err)
		}
		objects = append(objects, obj)
	}
	return objects, rows.Err()
}

// DeleteLFSObject deletes an LFS object record.
func (s *SQLiteStore) DeleteLFSObject(repoID, oid string) error {
	result, err := s.db.Exec("DELETE FROM lfs_objects WHERE repo_id = ? AND oid = ?", repoID, oid)
	if err != nil {
		return fmt.Errorf("delete lfs object: %w", err)
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

// GetRepoLFSSize returns the total size of LFS objects for a repository.
func (s *SQLiteStore) GetRepoLFSSize(repoID string) (int64, error) {
	var size sql.NullInt64
	err := s.db.QueryRow("SELECT SUM(size) FROM lfs_objects WHERE repo_id = ?", repoID).Scan(&size)
	if err != nil {
		return 0, fmt.Errorf("sum lfs size: %w", err)
	}
	if !size.Valid {
		return 0, nil
	}
	return size.Int64, nil
}

// CreateUser creates a new user.
func (s *SQLiteStore) CreateUser(user *User) error {
	query := `
		INSERT INTO users (id, primary_namespace_id, created_at, updated_at)
		VALUES (?, ?, ?, ?)
	`

	_, err := s.db.Exec(query,
		user.ID,
		user.PrimaryNamespaceID,
		user.CreatedAt,
		user.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

func (s *SQLiteStore) scanUser(row *sql.Row) (*User, error) {
	var user User

	err := row.Scan(
		&user.ID,
		&user.PrimaryNamespaceID,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}

	return &user, nil
}

// GetUser retrieves a user by ID.
func (s *SQLiteStore) GetUser(id string) (*User, error) {
	query := `
		SELECT id, primary_namespace_id, created_at, updated_at
		FROM users
		WHERE id = ?
	`
	return s.scanUser(s.db.QueryRow(query, id))
}

// ListUsers lists all users with cursor-based pagination.
func (s *SQLiteStore) ListUsers(cursor string, limit int) ([]User, error) {
	query := `
		SELECT id, primary_namespace_id, created_at, updated_at
		FROM users
		WHERE id > ?
		ORDER BY id
		LIMIT ?
	`

	rows, err := s.db.Query(query, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("query users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var user User

		if err := rows.Scan(
			&user.ID,
			&user.PrimaryNamespaceID,
			&user.CreatedAt,
			&user.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}

		users = append(users, user)
	}

	return users, rows.Err()
}

// UpdateUser updates an existing user.
func (s *SQLiteStore) UpdateUser(user *User) error {
	query := `
		UPDATE users
		SET primary_namespace_id = ?, updated_at = ?
		WHERE id = ?
	`

	result, err := s.db.Exec(query,
		user.PrimaryNamespaceID,
		user.UpdatedAt,
		user.ID,
	)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
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

// DeleteUser deletes a user by ID.
func (s *SQLiteStore) DeleteUser(id string) error {
	result, err := s.db.Exec("DELETE FROM users WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
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

// UpsertNamespaceGrant creates or updates a user namespace grant.
func (s *SQLiteStore) UpsertNamespaceGrant(grant *NamespaceGrant) error {
	query := `
		INSERT INTO user_namespace_grants (user_id, namespace_id, allow_bits, deny_bits, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id, namespace_id) DO UPDATE SET
			allow_bits = excluded.allow_bits,
			deny_bits = excluded.deny_bits,
			updated_at = excluded.updated_at
	`

	_, err := s.db.Exec(query,
		grant.UserID,
		grant.NamespaceID,
		grant.AllowBits,
		grant.DenyBits,
		grant.CreatedAt,
		grant.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert namespace grant: %w", err)
	}

	return nil
}

// DeleteNamespaceGrant removes a user's namespace grant.
func (s *SQLiteStore) DeleteNamespaceGrant(userID, namespaceID string) error {
	result, err := s.db.Exec(
		"DELETE FROM user_namespace_grants WHERE user_id = ? AND namespace_id = ?",
		userID, namespaceID,
	)
	if err != nil {
		return fmt.Errorf("delete namespace grant: %w", err)
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

// GetNamespaceGrant retrieves a user's namespace grant.
func (s *SQLiteStore) GetNamespaceGrant(userID, namespaceID string) (*NamespaceGrant, error) {
	query := `
		SELECT user_id, namespace_id, allow_bits, deny_bits, created_at, updated_at
		FROM user_namespace_grants
		WHERE user_id = ? AND namespace_id = ?
	`

	var grant NamespaceGrant
	err := s.db.QueryRow(query, userID, namespaceID).Scan(
		&grant.UserID,
		&grant.NamespaceID,
		&grant.AllowBits,
		&grant.DenyBits,
		&grant.CreatedAt,
		&grant.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan namespace grant: %w", err)
	}

	return &grant, nil
}

// ListUserNamespaceGrants lists all namespace grants for a user.
func (s *SQLiteStore) ListUserNamespaceGrants(userID string) ([]NamespaceGrant, error) {
	query := `
		SELECT user_id, namespace_id, allow_bits, deny_bits, created_at, updated_at
		FROM user_namespace_grants
		WHERE user_id = ?
		ORDER BY namespace_id
	`

	rows, err := s.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("query namespace grants: %w", err)
	}
	defer rows.Close()

	var grants []NamespaceGrant
	for rows.Next() {
		var grant NamespaceGrant
		if err := rows.Scan(
			&grant.UserID,
			&grant.NamespaceID,
			&grant.AllowBits,
			&grant.DenyBits,
			&grant.CreatedAt,
			&grant.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan namespace grant: %w", err)
		}
		grants = append(grants, grant)
	}

	return grants, rows.Err()
}

// CountNamespaceUsers counts the number of users with grants for a namespace.
func (s *SQLiteStore) CountNamespaceUsers(namespaceID string) (int, error) {
	query := `SELECT COUNT(*) FROM user_namespace_grants WHERE namespace_id = ?`
	var count int
	if err := s.db.QueryRow(query, namespaceID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count namespace users: %w", err)
	}
	return count, nil
}

// UpsertRepoGrant creates or updates a user repo grant.
func (s *SQLiteStore) UpsertRepoGrant(grant *RepoGrant) error {
	query := `
		INSERT INTO user_repo_grants (user_id, repo_id, allow_bits, deny_bits, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id, repo_id) DO UPDATE SET
			allow_bits = excluded.allow_bits,
			deny_bits = excluded.deny_bits,
			updated_at = excluded.updated_at
	`

	_, err := s.db.Exec(query,
		grant.UserID,
		grant.RepoID,
		grant.AllowBits,
		grant.DenyBits,
		grant.CreatedAt,
		grant.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert repo grant: %w", err)
	}

	return nil
}

// DeleteRepoGrant removes a user's repo grant.
func (s *SQLiteStore) DeleteRepoGrant(userID, repoID string) error {
	result, err := s.db.Exec(
		"DELETE FROM user_repo_grants WHERE user_id = ? AND repo_id = ?",
		userID, repoID,
	)
	if err != nil {
		return fmt.Errorf("delete repo grant: %w", err)
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

// GetRepoGrant retrieves a user's repo grant.
func (s *SQLiteStore) GetRepoGrant(userID, repoID string) (*RepoGrant, error) {
	query := `
		SELECT user_id, repo_id, allow_bits, deny_bits, created_at, updated_at
		FROM user_repo_grants
		WHERE user_id = ? AND repo_id = ?
	`

	var grant RepoGrant
	err := s.db.QueryRow(query, userID, repoID).Scan(
		&grant.UserID,
		&grant.RepoID,
		&grant.AllowBits,
		&grant.DenyBits,
		&grant.CreatedAt,
		&grant.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan repo grant: %w", err)
	}

	return &grant, nil
}

// ListUserRepoGrants lists all repo grants for a user.
func (s *SQLiteStore) ListUserRepoGrants(userID string) ([]RepoGrant, error) {
	query := `
		SELECT user_id, repo_id, allow_bits, deny_bits, created_at, updated_at
		FROM user_repo_grants
		WHERE user_id = ?
		ORDER BY repo_id
	`

	rows, err := s.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("query repo grants: %w", err)
	}
	defer rows.Close()

	var grants []RepoGrant
	for rows.Next() {
		var grant RepoGrant
		if err := rows.Scan(
			&grant.UserID,
			&grant.RepoID,
			&grant.AllowBits,
			&grant.DenyBits,
			&grant.CreatedAt,
			&grant.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan repo grant: %w", err)
		}
		grants = append(grants, grant)
	}

	return grants, rows.Err()
}

// ListUserReposWithGrants returns repos in a namespace that the user has repo grants for.
func (s *SQLiteStore) ListUserReposWithGrants(userID, namespaceID string) ([]Repo, error) {
	query := `
		SELECT r.id, r.namespace_id, r.name, r.description, r.public,
			   r.size_bytes, r.last_push_at, r.created_at, r.updated_at
		FROM repos r
		JOIN user_repo_grants g ON g.repo_id = r.id
		WHERE g.user_id = ? AND r.namespace_id = ?
		ORDER BY r.name
	`

	rows, err := s.db.Query(query, userID, namespaceID)
	if err != nil {
		return nil, fmt.Errorf("query repos with grants: %w", err)
	}
	defer rows.Close()

	var repos []Repo
	for rows.Next() {
		var repo Repo
		var description sql.NullString
		var lastPushAt sql.NullTime

		if err := rows.Scan(
			&repo.ID,
			&repo.NamespaceID,
			&repo.Name,
			&description,
			&repo.Public,
			&repo.SizeBytes,
			&lastPushAt,
			&repo.CreatedAt,
			&repo.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan repo: %w", err)
		}

		repo.Description = FromNullString(description)
		repo.LastPushAt = FromNullTime(lastPushAt)
		repos = append(repos, repo)
	}

	return repos, rows.Err()
}

// HasRepoGrantsInNamespace returns true if the user has any repo grants in the namespace.
func (s *SQLiteStore) HasRepoGrantsInNamespace(userID, namespaceID string) (bool, error) {
	query := `
		SELECT COUNT(*) FROM user_repo_grants g
		JOIN repos r ON r.id = g.repo_id
		WHERE g.user_id = ? AND r.namespace_id = ?
	`

	var count int
	err := s.db.QueryRow(query, userID, namespaceID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check repo grants in namespace: %w", err)
	}
	return count > 0, nil
}

// ListAllUserAccessibleRepos returns all repos the user can read across all namespaces.
func (s *SQLiteStore) ListAllUserAccessibleRepos(userID string) ([]Repo, error) {
	repoReadBits := int(PermRepoRead | PermRepoWrite | PermRepoAdmin)

	query := `
		SELECT DISTINCT r.id, r.namespace_id, r.name, r.description, r.public,
			   r.size_bytes, r.last_push_at, r.created_at, r.updated_at
		FROM repos r
		JOIN user_namespace_grants ng ON ng.namespace_id = r.namespace_id
		WHERE ng.user_id = ? AND (ng.allow_bits & ?) != 0 AND (ng.deny_bits & ?) = 0
		UNION
		SELECT DISTINCT r.id, r.namespace_id, r.name, r.description, r.public,
			   r.size_bytes, r.last_push_at, r.created_at, r.updated_at
		FROM repos r
		JOIN user_repo_grants rg ON rg.repo_id = r.id
		WHERE rg.user_id = ? AND (rg.allow_bits & ?) != 0 AND (rg.deny_bits & ?) = 0
		ORDER BY namespace_id, name
	`

	rows, err := s.db.Query(query, userID, repoReadBits, repoReadBits, userID, repoReadBits, repoReadBits)
	if err != nil {
		return nil, fmt.Errorf("query accessible repos: %w", err)
	}
	defer rows.Close()

	var repos []Repo
	for rows.Next() {
		var repo Repo
		var description sql.NullString
		var lastPushAt sql.NullTime

		if err := rows.Scan(
			&repo.ID,
			&repo.NamespaceID,
			&repo.Name,
			&description,
			&repo.Public,
			&repo.SizeBytes,
			&lastPushAt,
			&repo.CreatedAt,
			&repo.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan repo: %w", err)
		}

		repo.Description = FromNullString(description)
		repo.LastPushAt = FromNullTime(lastPushAt)
		repos = append(repos, repo)
	}

	return repos, rows.Err()
}

// ListUserTokens lists all tokens belonging to a user.
func (s *SQLiteStore) ListUserTokens(userID string) ([]Token, error) {
	query := `
		SELECT id, token_hash, token_lookup, is_admin, user_id,
			   created_at, expires_at, last_used_at
		FROM tokens
		WHERE user_id = ?
		ORDER BY created_at DESC
	`

	rows, err := s.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("query user tokens: %w", err)
	}
	defer rows.Close()

	var tokens []Token
	for rows.Next() {
		var token Token
		var userID sql.NullString
		var expiresAt, lastUsedAt sql.NullTime

		if err := rows.Scan(
			&token.ID,
			&token.TokenHash,
			&token.TokenLookup,
			&token.IsAdmin,
			&userID,
			&token.CreatedAt,
			&expiresAt,
			&lastUsedAt,
		); err != nil {
			return nil, fmt.Errorf("scan token: %w", err)
		}

		token.UserID = FromNullString(userID)
		token.ExpiresAt = FromNullTime(expiresAt)
		token.LastUsedAt = FromNullTime(lastUsedAt)
		tokens = append(tokens, token)
	}

	return tokens, rows.Err()
}

// GenerateUserToken creates a new token bound to a user.
func (s *SQLiteStore) GenerateUserToken(userID string, expiresAt *time.Time) (string, *Token, error) {
	const maxAttempts = 5

	for attempt := 0; attempt < maxAttempts; attempt++ {
		rawToken, token, err := s.generateUserTokenAttempt(userID, expiresAt)
		if err != nil {
			if errors.Is(err, ErrTokenLookupCollision) {
				continue
			}
			return "", nil, err
		}
		return rawToken, token, nil
	}

	return "", nil, fmt.Errorf("generate user token: %w", ErrTokenLookupCollision)
}

func (s *SQLiteStore) generateUserTokenAttempt(userID string, expiresAt *time.Time) (string, *Token, error) {
	now := time.Now()
	tokenID := uuid.New().String()
	tokenLookup := tokenID[:8]

	secret, err := core.GenerateTokenSecret(24)
	if err != nil {
		return "", nil, fmt.Errorf("generate token secret: %w", err)
	}

	rawToken := core.BuildToken(tokenLookup, secret)

	tokenHash, err := core.HashToken(rawToken)
	if err != nil {
		return "", nil, fmt.Errorf("hash token: %w", err)
	}

	token := &Token{
		ID:          tokenID,
		TokenHash:   tokenHash,
		TokenLookup: tokenLookup,
		IsAdmin:     false,
		UserID:      &userID,
		CreatedAt:   now,
		ExpiresAt:   expiresAt,
	}

	if err := s.CreateToken(token); err != nil {
		return "", nil, err
	}

	return rawToken, token, nil
}
