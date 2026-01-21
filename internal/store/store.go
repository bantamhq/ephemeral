package store

import (
	"database/sql"
	"time"
)

// Scope constants for token permissions.
const (
	ScopeReadOnly = "read-only"
	ScopeRepos    = "repos"
	ScopeFull     = "full"
	ScopeAdmin    = "admin"
)

// Store defines the database interface.
type Store interface {
	Initialize() error

	// Token operations
	CreateToken(token *Token) error
	GetTokenByLookup(lookup string) (*Token, error)
	GetTokenByID(id string) (*Token, error)
	ListTokens(cursor string, limit int) ([]Token, error)
	DeleteToken(id string) error
	GenerateAdminToken() (string, error)
	GenerateUserToken(namespaceID string, name *string, scope string) (string, *Token, error)

	// Token namespace access operations
	GrantTokenNamespaceAccess(access *TokenNamespaceAccess) error
	RevokeTokenNamespaceAccess(tokenID, namespaceID string) error
	GetTokenNamespaceAccess(tokenID, namespaceID string) (*TokenNamespaceAccess, error)
	ListTokenNamespaces(tokenID string) ([]NamespaceWithAccess, error)
	GetTokenPrimaryNamespace(tokenID string) (*Namespace, error)
	HasTokenNamespaceAccess(tokenID, namespaceID string) (bool, error)
	CountNamespaceTokens(namespaceID string) (int, error)

	// Repo operations
	CreateRepo(repo *Repo) error
	GetRepo(namespaceID, name string) (*Repo, error)
	GetRepoByID(id string) (*Repo, error)
	ListRepos(namespaceID, cursor string, limit int) ([]Repo, error)
	ListReposWithFolders(namespaceID, cursor string, limit int) ([]RepoWithFolders, error)
	UpdateRepo(repo *Repo) error
	DeleteRepo(id string) error
	UpdateRepoLastPush(id string, pushTime time.Time) error
	UpdateRepoSize(id string, sizeBytes int) error

	// Folder operations
	CreateFolder(folder *Folder) error
	GetFolderByID(id string) (*Folder, error)
	GetFolderByName(namespaceID, name string) (*Folder, error)
	ListFolders(namespaceID, cursor string, limit int) ([]Folder, error)
	UpdateFolder(folder *Folder) error
	DeleteFolder(id string) error
	CountFolderRepos(id string) (int, error)

	// Repo-Folder M2M operations
	AddRepoFolder(repoID, folderID string) error
	AddRepoFolders(repoID string, folderIDs []string) error
	RemoveRepoFolder(repoID, folderID string) error
	ListRepoFolders(repoID string) ([]Folder, error)
	ListFolderRepos(folderID string) ([]Repo, error)
	SetRepoFolders(repoID string, folderIDs []string) error

	// Namespace operations
	CreateNamespace(ns *Namespace) error
	GetNamespace(id string) (*Namespace, error)
	GetNamespaceByName(name string) (*Namespace, error)
	ListNamespaces(cursor string, limit int) ([]Namespace, error)
	DeleteNamespace(id string) error

	Close() error
}

type Namespace struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	CreatedAt         time.Time  `json:"created_at"`
	RepoLimit         *int       `json:"repo_limit,omitempty"`
	StorageLimitBytes *int       `json:"storage_limit_bytes,omitempty"`
	ExternalID        *string    `json:"external_id,omitempty"`
}

type Token struct {
	ID          string     `json:"id"`
	TokenHash   string     `json:"-"`
	TokenLookup string     `json:"-"`
	Name        *string    `json:"name,omitempty"`
	IsAdmin     bool       `json:"is_admin"`
	Scope       string     `json:"scope"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
}

type TokenNamespaceAccess struct {
	TokenID     string    `json:"token_id"`
	NamespaceID string    `json:"namespace_id"`
	IsPrimary   bool      `json:"is_primary"`
	CreatedAt   time.Time `json:"created_at"`
}

type NamespaceWithAccess struct {
	Namespace
	IsPrimary bool `json:"is_primary"`
}

type Repo struct {
	ID          string     `json:"id"`
	NamespaceID string     `json:"namespace_id"`
	Name        string     `json:"name"`
	Description *string    `json:"description,omitempty"`
	Public      bool       `json:"public"`
	SizeBytes   int        `json:"size_bytes"`
	LastPushAt  *time.Time `json:"last_push_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type Folder struct {
	ID          string    `json:"id"`
	NamespaceID string    `json:"namespace_id"`
	Name        string    `json:"name"`
	Color       *string   `json:"color,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type RepoWithFolders struct {
	Repo
	Folders []Folder `json:"folders,omitempty"`
}

func ToNullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

func FromNullString(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	return &ns.String
}

func ToNullInt64(i *int) sql.NullInt64 {
	if i == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*i), Valid: true}
}

func FromNullInt64(ni sql.NullInt64) *int {
	if !ni.Valid {
		return nil
	}
	i := int(ni.Int64)
	return &i
}

func ToNullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

func FromNullTime(nt sql.NullTime) *time.Time {
	if !nt.Valid {
		return nil
	}
	return &nt.Time
}
