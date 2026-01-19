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
	ListFolders(namespaceID string) ([]Folder, error)
	UpdateFolder(folder *Folder) error
	DeleteFolder(id string) error
	CountFolderContents(id string) (repos int, subfolders int, err error)

	// Label operations
	CreateLabel(label *Label) error
	GetLabelByID(id string) (*Label, error)
	GetLabelByName(namespaceID, name string) (*Label, error)
	ListLabels(namespaceID string) ([]Label, error)
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
	ID               string
	Name             string
	CreatedAt        time.Time
	RepoLimit        *int
	StorageLimitBytes *int
	ExternalID       *string
}

type Token struct {
	ID          string
	TokenHash   string
	Name        *string
	NamespaceID string
	Scope       string
	RepoIDs     *string // JSON array
	CreatedAt   time.Time
	ExpiresAt   *time.Time
	LastUsedAt  *time.Time
	ExternalID  *string
}

type Repo struct {
	ID          string
	NamespaceID string
	Name        string
	FolderID    *string
	Public      bool
	SizeBytes   int
	LastPushAt  *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Folder struct {
	ID          string
	NamespaceID string
	Name        string
	ParentID    *string
	CreatedAt   time.Time
}

type Label struct {
	ID          string
	NamespaceID string
	Name        string
	Color       *string
	CreatedAt   time.Time
}

// SQL null type conversions for optional model fields.

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