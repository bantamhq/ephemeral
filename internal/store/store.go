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
	GetTokenByHash(hash string) (*Token, error)
	GetTokenByID(id string) (*Token, error)
	ListTokens(namespaceID, cursor string, limit int) ([]Token, error)
	DeleteToken(id string) error
	GenerateRootToken() (string, error)

	// Repo operations
	CreateRepo(repo *Repo) error
	GetRepo(namespaceID, name string) (*Repo, error)
	GetRepoByID(id string) (*Repo, error)
	ListRepos(namespaceID, cursor string, limit int) ([]Repo, error)
	UpdateRepo(repo *Repo) error
	DeleteRepo(id string) error
	UpdateRepoLastPush(id string, pushTime time.Time) error

	// Folder operations
	CreateFolder(folder *Folder) error
	GetFolderByID(id string) (*Folder, error)
	GetFolderByName(namespaceID, name string, parentID *string) (*Folder, error)
	ListFolders(namespaceID, cursor string, limit int) ([]Folder, error)
	UpdateFolder(folder *Folder) error
	DeleteFolder(id string) error
	CountFolderContents(id string) (repos int, subfolders int, err error)

	// Label operations
	CreateLabel(label *Label) error
	GetLabelByID(id string) (*Label, error)
	GetLabelByName(namespaceID, name string) (*Label, error)
	ListLabels(namespaceID, cursor string, limit int) ([]Label, error)
	UpdateLabel(label *Label) error
	DeleteLabel(id string) error

	// Repo-Label operations
	AddRepoLabel(repoID, labelID string) error
	RemoveRepoLabel(repoID, labelID string) error
	ListRepoLabels(repoID string) ([]Label, error)
	ListLabelRepos(labelID string) ([]Repo, error)

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
	Name        *string    `json:"name,omitempty"`
	NamespaceID string     `json:"namespace_id"`
	Scope       string     `json:"scope"`
	RepoIDs     *string    `json:"repo_ids,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	ExternalID  *string    `json:"external_id,omitempty"`
}

type Repo struct {
	ID          string     `json:"id"`
	NamespaceID string     `json:"namespace_id"`
	Name        string     `json:"name"`
	FolderID    *string    `json:"folder_id,omitempty"`
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
	ParentID    *string   `json:"parent_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type Label struct {
	ID          string    `json:"id"`
	NamespaceID string    `json:"namespace_id"`
	Name        string    `json:"name"`
	Color       *string   `json:"color,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
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