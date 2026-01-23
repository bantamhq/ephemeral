package lfs

import (
	"context"
	"io"
)

type Storage interface {
	Exists(ctx context.Context, repoID, oid string) (bool, error)
	Get(ctx context.Context, repoID, oid string) (io.ReadCloser, int64, error)
	Put(ctx context.Context, repoID, oid string, content io.Reader, size int64) error
	Delete(ctx context.Context, repoID, oid string) error
	Size(ctx context.Context, repoID, oid string) (int64, error)
}
