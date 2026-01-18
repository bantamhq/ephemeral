package store

import (
	"database/sql"
	"time"
)

// Store defines the database interface.
type Store interface {
	Initialize() error

	CreateToken(token *Token) error
	GetTokenByHash(hash string) (*Token, error)
	GenerateRootToken() (string, error)

	CreateRepo(repo *Repo) error
	GetRepo(namespaceID, name string) (*Repo, error)
	GetRepoByID(id string) (*Repo, error)
	UpdateRepoLastPush(id string, pushTime time.Time) error

	GetNamespace(id string) (*Namespace, error)

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

type Tag struct {
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