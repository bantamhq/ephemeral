package lfs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

var oidPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type LocalStorage struct {
	basePath string
}

func NewLocalStorage(basePath string) *LocalStorage {
	return &LocalStorage{basePath: basePath}
}

// objectPath uses 2-level directory prefix to avoid filesystem performance issues with large directories.
func (s *LocalStorage) objectPath(repoID, oid string) string {
	return filepath.Join(s.basePath, repoID, "objects", oid[:2], oid[2:4], oid)
}

func (s *LocalStorage) tmpPath(repoID string) string {
	return filepath.Join(s.basePath, repoID, "tmp")
}

func ValidateOID(oid string) error {
	if !oidPattern.MatchString(oid) {
		return ErrInvalidOID
	}
	return nil
}

func (s *LocalStorage) Exists(ctx context.Context, repoID, oid string) (bool, error) {
	if err := ValidateOID(oid); err != nil {
		return false, err
	}

	_, err := os.Stat(s.objectPath(repoID, oid))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat object: %w", err)
	}
	return true, nil
}

func (s *LocalStorage) Get(ctx context.Context, repoID, oid string) (io.ReadCloser, int64, error) {
	if err := ValidateOID(oid); err != nil {
		return nil, 0, err
	}

	path := s.objectPath(repoID, oid)
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil, 0, ErrObjectNotFound
	}
	if err != nil {
		return nil, 0, fmt.Errorf("stat object: %w", err)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("open object: %w", err)
	}

	return file, info.Size(), nil
}

// Put verifies the SHA-256 hash matches the OID before committing to storage.
func (s *LocalStorage) Put(ctx context.Context, repoID, oid string, content io.Reader, size int64) error {
	if err := ValidateOID(oid); err != nil {
		return err
	}

	tmpDir := s.tmpPath(repoID)
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return fmt.Errorf("create tmp directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(tmpDir, "upload-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	hasher := sha256.New()
	writer := io.MultiWriter(tmpFile, hasher)

	written, err := io.Copy(writer, content)
	if err != nil {
		tmpFile.Close()
		return fmt.Errorf("write content: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if written != size {
		return fmt.Errorf("size mismatch: expected %d, got %d", size, written)
	}

	computedHash := hex.EncodeToString(hasher.Sum(nil))
	if computedHash != oid {
		return ErrHashMismatch
	}

	finalPath := s.objectPath(repoID, oid)
	if err := os.MkdirAll(filepath.Dir(finalPath), 0755); err != nil {
		return fmt.Errorf("create object directory: %w", err)
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		return fmt.Errorf("move to final path: %w", err)
	}

	return nil
}

func (s *LocalStorage) Delete(ctx context.Context, repoID, oid string) error {
	if err := ValidateOID(oid); err != nil {
		return err
	}

	path := s.objectPath(repoID, oid)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return ErrObjectNotFound
		}
		return fmt.Errorf("remove object: %w", err)
	}
	return nil
}

func (s *LocalStorage) Size(ctx context.Context, repoID, oid string) (int64, error) {
	if err := ValidateOID(oid); err != nil {
		return 0, err
	}

	info, err := os.Stat(s.objectPath(repoID, oid))
	if os.IsNotExist(err) {
		return 0, ErrObjectNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("stat object: %w", err)
	}
	return info.Size(), nil
}
