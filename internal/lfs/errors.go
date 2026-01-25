package lfs

import "errors"

var (
	ErrObjectNotFound = errors.New("object not found")
	ErrInvalidOID     = errors.New("invalid object ID")
	ErrHashMismatch   = errors.New("content hash does not match OID")
)
