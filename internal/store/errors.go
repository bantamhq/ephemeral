package store

import "errors"

var ErrTokenLookupCollision = errors.New("token lookup collision")
var ErrPrimaryNamespaceGrant = errors.New("cannot grant other users access to a primary namespace")
