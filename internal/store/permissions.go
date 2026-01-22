package store

import (
	"fmt"
	"strings"
)

// Permission represents a bitmask of granted permissions.
type Permission uint32

const (
	PermRepoRead        Permission = 1 << 0 // 1
	PermRepoWrite       Permission = 1 << 1 // 2
	PermRepoAdmin       Permission = 1 << 2 // 4
	PermNamespaceRead   Permission = 1 << 3 // 8
	PermNamespaceWrite  Permission = 1 << 4 // 16
	PermNamespaceAdmin  Permission = 1 << 5 // 32
)

// Permission string constants for API serialization.
const (
	PermStringRepoRead        = "repo:read"
	PermStringRepoWrite       = "repo:write"
	PermStringRepoAdmin       = "repo:admin"
	PermStringNamespaceRead   = "namespace:read"
	PermStringNamespaceWrite  = "namespace:write"
	PermStringNamespaceAdmin  = "namespace:admin"
)

var permissionStrings = map[Permission]string{
	PermRepoRead:        PermStringRepoRead,
	PermRepoWrite:       PermStringRepoWrite,
	PermRepoAdmin:       PermStringRepoAdmin,
	PermNamespaceRead:   PermStringNamespaceRead,
	PermNamespaceWrite:  PermStringNamespaceWrite,
	PermNamespaceAdmin:  PermStringNamespaceAdmin,
}

var stringToPermission = map[string]Permission{
	PermStringRepoRead:        PermRepoRead,
	PermStringRepoWrite:       PermRepoWrite,
	PermStringRepoAdmin:       PermRepoAdmin,
	PermStringNamespaceRead:   PermNamespaceRead,
	PermStringNamespaceWrite:  PermNamespaceWrite,
	PermStringNamespaceAdmin:  PermNamespaceAdmin,
}

// Has returns true if the permission bitmask contains the required permission.
func (p Permission) Has(required Permission) bool {
	return p&required == required
}

// String returns a comma-separated list of permission strings.
func (p Permission) String() string {
	if p == 0 {
		return ""
	}

	var perms []string
	for bit, str := range permissionStrings {
		if p.Has(bit) {
			perms = append(perms, str)
		}
	}
	return strings.Join(perms, ", ")
}

// ToStrings returns a slice of permission strings.
func (p Permission) ToStrings() []string {
	if p == 0 {
		return nil
	}

	var perms []string
	for bit, str := range permissionStrings {
		if p.Has(bit) {
			perms = append(perms, str)
		}
	}
	return perms
}

// ParsePermission converts a permission string to its bitmask value.
func ParsePermission(s string) (Permission, error) {
	p, ok := stringToPermission[s]
	if !ok {
		return 0, fmt.Errorf("unknown permission: %s", s)
	}
	return p, nil
}

// ParsePermissions converts a slice of permission strings to a combined bitmask.
func ParsePermissions(strs []string) (Permission, error) {
	var result Permission
	for _, s := range strs {
		p, err := ParsePermission(s)
		if err != nil {
			return 0, err
		}
		result |= p
	}
	return result, nil
}

// PermissionsFromStrings converts a slice of permission strings to a combined bitmask.
// Unknown permission strings are silently ignored.
func PermissionsFromStrings(strs []string) Permission {
	var result Permission
	for _, s := range strs {
		if p, ok := stringToPermission[s]; ok {
			result |= p
		}
	}
	return result
}

// ExpandImplied expands a permission bitmask to include implied permissions.
// admin implies write implies read, for both repo and namespace permissions.
// This should only be used for ALLOW permissions, never for DENY.
func ExpandImplied(p Permission) Permission {
	result := p

	if result.Has(PermRepoAdmin) {
		result |= PermRepoWrite
	}
	if result.Has(PermRepoWrite) {
		result |= PermRepoRead
	}

	if result.Has(PermNamespaceAdmin) {
		result |= PermNamespaceWrite
	}
	if result.Has(PermNamespaceWrite) {
		result |= PermNamespaceRead
	}

	return result
}

// DefaultNamespaceGrant returns the default permissions for simple token creation:
// namespace:write + repo:admin (which implies namespace:read, repo:read, repo:write).
func DefaultNamespaceGrant() Permission {
	return PermNamespaceWrite | PermRepoAdmin
}

// PermissionChecker provides methods to check token permissions against grants.
type PermissionChecker struct {
	store Store
}

// NewPermissionChecker creates a new permission checker.
func NewPermissionChecker(store Store) *PermissionChecker {
	return &PermissionChecker{store: store}
}

// CheckNamespacePermission checks if a token has the required permission for a namespace.
// It evaluates namespace grants only, expanding allow bits but not deny bits.
func (pc *PermissionChecker) CheckNamespacePermission(tokenID, namespaceID string, required Permission) (bool, error) {
	grant, err := pc.store.GetNamespaceGrant(tokenID, namespaceID)
	if err != nil {
		return false, err
	}

	if grant == nil {
		return false, nil
	}

	allow := ExpandImplied(grant.AllowBits)
	deny := grant.DenyBits
	effective := allow &^ deny

	return effective.Has(required), nil
}

// CheckRepoPermission checks if a token has the required permission for a repo.
// It combines namespace grants and repo grants, expanding allow bits but not deny bits.
func (pc *PermissionChecker) CheckRepoPermission(tokenID string, repo *Repo, required Permission) (bool, error) {
	nsGrant, err := pc.store.GetNamespaceGrant(tokenID, repo.NamespaceID)
	if err != nil {
		return false, err
	}

	repoGrant, err := pc.store.GetRepoGrant(tokenID, repo.ID)
	if err != nil {
		return false, err
	}

	var allowNS, denyNS Permission
	if nsGrant != nil {
		allowNS = ExpandImplied(nsGrant.AllowBits)
		denyNS = nsGrant.DenyBits
	}

	var allowRepo, denyRepo Permission
	if repoGrant != nil {
		allowRepo = ExpandImplied(repoGrant.AllowBits)
		denyRepo = repoGrant.DenyBits
	}

	allow := allowNS | allowRepo
	deny := denyNS | denyRepo
	effective := allow &^ deny

	return effective.Has(required), nil
}

// HasAnyRepoGrants checks if a token has any repo grants in a namespace.
func (pc *PermissionChecker) HasAnyRepoGrants(tokenID, namespaceID string) (bool, error) {
	return pc.store.HasRepoGrantsInNamespace(tokenID, namespaceID)
}

// CanAccessNamespace checks if a token can access a namespace at all.
// Returns true if the token has a namespace grant OR has any repo grants in the namespace.
func (pc *PermissionChecker) CanAccessNamespace(tokenID, namespaceID string) (bool, error) {
	grant, err := pc.store.GetNamespaceGrant(tokenID, namespaceID)
	if err != nil {
		return false, err
	}
	if grant != nil {
		return true, nil
	}

	return pc.store.HasRepoGrantsInNamespace(tokenID, namespaceID)
}
