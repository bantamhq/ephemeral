/*
Package store tests.

These tests serve as lightweight smoke tests and living documentation of expected
store behavior. They verify happy paths, basic error cases, and cascade/constraint
behavior using an in-memory SQLite database.

This file is intentionally minimal. Comprehensive behavioral testing happens at
the API integration layer (tests/api/). Only add tests here when:
  - Documenting non-obvious store behavior that the API doesn't expose
  - Catching a regression that slipped through API tests
  - Testing complex queries that warrant isolated verification

Do not expand this into exhaustive unit test coverage.
*/
package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := NewSQLiteStore(":memory:")
	require.NoError(t, err, "create store")
	require.NoError(t, s.Initialize(), "initialize store")
	t.Cleanup(func() { s.Close() })
	return s
}

func createTestNamespace(t *testing.T, s *SQLiteStore, id string) *Namespace {
	t.Helper()
	ns := &Namespace{ID: id, Name: "ns-" + id, CreatedAt: time.Now()}
	require.NoError(t, s.CreateNamespace(ns))
	return ns
}

func createTestRepo(t *testing.T, s *SQLiteStore, nsID, name string) *Repo {
	t.Helper()
	repo := &Repo{
		ID:          "repo-" + name,
		NamespaceID: nsID,
		Name:        name,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	require.NoError(t, s.CreateRepo(repo))
	return repo
}

func createTestFolder(t *testing.T, s *SQLiteStore, nsID, name string, color *string) *Folder {
	t.Helper()
	folder := &Folder{
		ID:          "folder-" + name,
		NamespaceID: nsID,
		Name:        name,
		Color:       color,
		CreatedAt:   time.Now(),
	}
	require.NoError(t, s.CreateFolder(folder))
	return folder
}

func createTestToken(t *testing.T, s *SQLiteStore, id, lookup, hash string, isAdmin bool) *Token {
	t.Helper()
	token := &Token{
		ID:          id,
		TokenHash:   hash,
		TokenLookup: lookup,
		IsAdmin:     isAdmin,
		CreatedAt:   time.Now(),
	}
	require.NoError(t, s.CreateToken(token))
	return token
}

func repoNames(repos []Repo) []string {
	names := make([]string, len(repos))
	for i, r := range repos {
		names[i] = r.Name
	}
	return names
}

func TestStore_NamespaceLifecycle(t *testing.T) {
	s := newTestStore(t)

	var ns *Namespace

	t.Run("create", func(t *testing.T) {
		ns = &Namespace{ID: "ns-1", Name: "test-ns", CreatedAt: time.Now()}
		require.NoError(t, s.CreateNamespace(ns))
	})

	t.Run("get by ID", func(t *testing.T) {
		got, err := s.GetNamespace("ns-1")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "test-ns", got.Name)
	})

	t.Run("get by name", func(t *testing.T) {
		got, err := s.GetNamespaceByName("test-ns")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "ns-1", got.ID)
	})

	t.Run("list", func(t *testing.T) {
		namespaces, err := s.ListNamespaces("", 10)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(namespaces), 1)
	})

	t.Run("delete cascades to repos and grants", func(t *testing.T) {
		repo := createTestRepo(t, s, ns.ID, "cascade-test")
		folder := createTestFolder(t, s, ns.ID, "cascade-folder", nil)
		s.AddRepoFolder(repo.ID, folder.ID)

		// Create user with namespace grant
		user := createTestUser(t, s, "cascade-user", ns.ID)
		require.NoError(t, s.UpsertNamespaceGrant(&NamespaceGrant{
			UserID:      user.ID,
			NamespaceID: ns.ID,
			AllowBits:   DefaultNamespaceGrant(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}))

		// Verify grant exists
		grant, err := s.GetNamespaceGrant(user.ID, ns.ID)
		require.NoError(t, err)
		require.NotNil(t, grant)

		// Delete namespace - should cascade delete repos and grants
		require.NoError(t, s.DeleteNamespace("ns-1"))

		got, _ := s.GetNamespace("ns-1")
		assert.Nil(t, got, "namespace should be deleted")

		r, _ := s.GetRepoByID(repo.ID)
		assert.Nil(t, r, "repo should be cascade deleted")

		g, _ := s.GetNamespaceGrant(user.ID, ns.ID)
		assert.Nil(t, g, "grant should be cascade deleted")
	})
}

func TestStore_RepoLifecycle(t *testing.T) {
	s := newTestStore(t)
	ns := createTestNamespace(t, s, "ns-1")

	var repo *Repo

	t.Run("create", func(t *testing.T) {
		repo = &Repo{
			ID:          "repo-1",
			NamespaceID: ns.ID,
			Name:        "my-repo",
			Public:      false,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		require.NoError(t, s.CreateRepo(repo))
	})

	t.Run("get by ID", func(t *testing.T) {
		got, err := s.GetRepoByID("repo-1")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "my-repo", got.Name)
	})

	t.Run("get by namespace and name", func(t *testing.T) {
		got, err := s.GetRepo(ns.ID, "my-repo")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "repo-1", got.ID)
	})

	t.Run("update", func(t *testing.T) {
		repo.Name = "renamed"
		repo.Public = true
		require.NoError(t, s.UpdateRepo(repo))

		got, _ := s.GetRepoByID("repo-1")
		assert.Equal(t, "renamed", got.Name)
		assert.True(t, got.Public)
	})

	t.Run("update last push", func(t *testing.T) {
		pushTime := time.Now()
		require.NoError(t, s.UpdateRepoLastPush("repo-1", pushTime))

		got, _ := s.GetRepoByID("repo-1")
		require.NotNil(t, got.LastPushAt)
		assert.WithinDuration(t, pushTime, *got.LastPushAt, time.Second)
	})

	t.Run("update size", func(t *testing.T) {
		require.NoError(t, s.UpdateRepoSize("repo-1", 2048))

		got, _ := s.GetRepoByID("repo-1")
		assert.Equal(t, int64(2048), got.SizeBytes)
	})

	t.Run("list", func(t *testing.T) {
		repos, err := s.ListRepos(ns.ID, "", 10)
		require.NoError(t, err)
		assert.Len(t, repos, 1)
	})

	t.Run("delete", func(t *testing.T) {
		require.NoError(t, s.DeleteRepo("repo-1"))

		got, err := s.GetRepoByID("repo-1")
		require.NoError(t, err)
		assert.Nil(t, got)
	})
}

func TestStore_FolderLifecycle(t *testing.T) {
	s := newTestStore(t)
	ns := createTestNamespace(t, s, "ns-1")

	var folder *Folder

	t.Run("create folder with color", func(t *testing.T) {
		color := "#ff0000"
		folder = createTestFolder(t, s, ns.ID, "projects", &color)
	})

	t.Run("get by ID", func(t *testing.T) {
		got, err := s.GetFolderByID(folder.ID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "projects", got.Name)
		require.NotNil(t, got.Color)
		assert.Equal(t, "#ff0000", *got.Color)
	})

	t.Run("get by name", func(t *testing.T) {
		got, err := s.GetFolderByName(ns.ID, "projects")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, folder.ID, got.ID)
	})

	t.Run("count repos", func(t *testing.T) {
		repo := createTestRepo(t, s, ns.ID, "in-folder")
		s.AddRepoFolder(repo.ID, folder.ID)

		count, err := s.CountFolderRepos(folder.ID)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("update", func(t *testing.T) {
		folder.Name = "renamed"
		newColor := "#00ff00"
		folder.Color = &newColor
		require.NoError(t, s.UpdateFolder(folder))

		got, _ := s.GetFolderByID(folder.ID)
		assert.Equal(t, "renamed", got.Name)
		assert.Equal(t, "#00ff00", *got.Color)
	})

	t.Run("list", func(t *testing.T) {
		createTestFolder(t, s, ns.ID, "another", nil)
		folders, err := s.ListFolders(ns.ID, "", 10)
		require.NoError(t, err)
		assert.Len(t, folders, 2)
	})

	t.Run("delete", func(t *testing.T) {
		other, _ := s.GetFolderByName(ns.ID, "another")
		require.NoError(t, s.DeleteFolder(other.ID))

		got, _ := s.GetFolderByID(other.ID)
		assert.Nil(t, got)
	})
}

func TestStore_RepoFolderM2M(t *testing.T) {
	s := newTestStore(t)
	ns := createTestNamespace(t, s, "ns-1")
	repo := createTestRepo(t, s, ns.ID, "my-repo")
	folder1 := createTestFolder(t, s, ns.ID, "folder1", nil)
	folder2 := createTestFolder(t, s, ns.ID, "folder2", nil)

	t.Run("add folder to repo", func(t *testing.T) {
		require.NoError(t, s.AddRepoFolder(repo.ID, folder1.ID))

		folders, err := s.ListRepoFolders(repo.ID)
		require.NoError(t, err)
		assert.Len(t, folders, 1)
		assert.Equal(t, "folder1", folders[0].Name)
	})

	t.Run("add same folder twice is idempotent", func(t *testing.T) {
		require.NoError(t, s.AddRepoFolder(repo.ID, folder1.ID))

		folders, _ := s.ListRepoFolders(repo.ID)
		assert.Len(t, folders, 1)
	})

	t.Run("list repos in folder", func(t *testing.T) {
		repos, err := s.ListFolderRepos(folder1.ID)
		require.NoError(t, err)
		assert.Len(t, repos, 1)
		assert.Equal(t, repo.ID, repos[0].ID)
	})

	t.Run("repo can belong to multiple folders", func(t *testing.T) {
		require.NoError(t, s.AddRepoFolder(repo.ID, folder2.ID))

		folders, err := s.ListRepoFolders(repo.ID)
		require.NoError(t, err)
		assert.Len(t, folders, 2)
	})

	t.Run("set repo folders replaces all", func(t *testing.T) {
		require.NoError(t, s.SetRepoFolders(repo.ID, []string{folder2.ID}))

		folders, err := s.ListRepoFolders(repo.ID)
		require.NoError(t, err)
		assert.Len(t, folders, 1)
		assert.Equal(t, "folder2", folders[0].Name)
	})

	t.Run("remove folder from repo", func(t *testing.T) {
		require.NoError(t, s.RemoveRepoFolder(repo.ID, folder2.ID))

		folders, _ := s.ListRepoFolders(repo.ID)
		assert.Len(t, folders, 0)
	})

	t.Run("remove non-existent folder returns error", func(t *testing.T) {
		err := s.RemoveRepoFolder(repo.ID, folder1.ID)
		assert.Error(t, err)
	})

	t.Run("list folders for repo with none", func(t *testing.T) {
		repo2 := createTestRepo(t, s, ns.ID, "no-folders")
		folders, err := s.ListRepoFolders(repo2.ID)
		require.NoError(t, err)
		assert.Len(t, folders, 0)
	})
}

func TestStore_TokenLifecycle(t *testing.T) {
	s := newTestStore(t)
	ns := createTestNamespace(t, s, "ns-1")
	user := createTestUser(t, s, "user-1", ns.ID)

	var token *Token
	userID := user.ID

	t.Run("create user token", func(t *testing.T) {
		token = &Token{
			ID:          "token-1",
			TokenHash:   "hash123",
			TokenLookup: "lookup01",
			IsAdmin:     false,
			UserID:      &userID,
			CreatedAt:   time.Now(),
		}
		require.NoError(t, s.CreateToken(token))
	})

	t.Run("upsert namespace grant", func(t *testing.T) {
		require.NoError(t, s.UpsertNamespaceGrant(&NamespaceGrant{
			UserID:      user.ID,
			NamespaceID: ns.ID,
			AllowBits:   DefaultNamespaceGrant(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}))
	})

	t.Run("get by lookup", func(t *testing.T) {
		got, err := s.GetTokenByLookup("lookup01")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "token-1", got.ID)
	})

	t.Run("get by ID", func(t *testing.T) {
		got, err := s.GetTokenByID("token-1")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.False(t, got.IsAdmin)
		assert.Equal(t, user.ID, *got.UserID)
	})

	t.Run("list", func(t *testing.T) {
		tokens, err := s.ListTokens("", 10)
		require.NoError(t, err)
		assert.Len(t, tokens, 1)
	})

	t.Run("list user namespace grants", func(t *testing.T) {
		grants, err := s.ListUserNamespaceGrants(user.ID)
		require.NoError(t, err)
		assert.Len(t, grants, 1)
		assert.True(t, grants[0].AllowBits.Has(PermNamespaceWrite))
	})

	t.Run("get namespace grant", func(t *testing.T) {
		grant, err := s.GetNamespaceGrant(user.ID, ns.ID)
		require.NoError(t, err)
		require.NotNil(t, grant)

		grant, err = s.GetNamespaceGrant(user.ID, "nonexistent")
		require.NoError(t, err)
		assert.Nil(t, grant)
	})

	t.Run("delete", func(t *testing.T) {
		require.NoError(t, s.DeleteToken("token-1"))

		got, _ := s.GetTokenByID("token-1")
		assert.Nil(t, got)
	})
}

func TestStore_Pagination(t *testing.T) {
	s := newTestStore(t)
	ns := createTestNamespace(t, s, "ns-1")

	for _, name := range []string{"alpha", "bravo", "charlie", "delta", "echo"} {
		createTestRepo(t, s, ns.ID, name)
	}

	t.Run("first page", func(t *testing.T) {
		repos, err := s.ListRepos(ns.ID, "", 2)
		require.NoError(t, err)
		assert.Equal(t, []string{"alpha", "bravo"}, repoNames(repos))
	})

	t.Run("second page", func(t *testing.T) {
		repos, err := s.ListRepos(ns.ID, "bravo", 2)
		require.NoError(t, err)
		assert.Equal(t, []string{"charlie", "delta"}, repoNames(repos))
	})

	t.Run("last page", func(t *testing.T) {
		repos, err := s.ListRepos(ns.ID, "delta", 2)
		require.NoError(t, err)
		assert.Equal(t, []string{"echo"}, repoNames(repos))
	})

	t.Run("past end", func(t *testing.T) {
		repos, err := s.ListRepos(ns.ID, "echo", 2)
		require.NoError(t, err)
		assert.Len(t, repos, 0)
	})

	t.Run("unlimited", func(t *testing.T) {
		repos, err := s.ListRepos(ns.ID, "", 0)
		require.NoError(t, err)
		assert.Len(t, repos, 5)
	})
}

func TestStore_DuplicateNames(t *testing.T) {
	s := newTestStore(t)
	ns := createTestNamespace(t, s, "ns-1")

	createTestRepo(t, s, ns.ID, "dupe")

	t.Run("same namespace rejects duplicate", func(t *testing.T) {
		repo := &Repo{
			ID:          "repo-dupe-2",
			NamespaceID: ns.ID,
			Name:        "dupe",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		err := s.CreateRepo(repo)
		assert.Error(t, err)
	})

	t.Run("different namespace allows same name", func(t *testing.T) {
		ns2 := createTestNamespace(t, s, "ns-2")
		repo := &Repo{
			ID:          "repo-dupe-other",
			NamespaceID: ns2.ID,
			Name:        "dupe",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		require.NoError(t, s.CreateRepo(repo))
	})
}

func TestStore_NotFound(t *testing.T) {
	s := newTestStore(t)

	t.Run("get returns nil", func(t *testing.T) {
		ns, err := s.GetNamespace("nope")
		require.NoError(t, err)
		assert.Nil(t, ns)

		repo, err := s.GetRepoByID("nope")
		require.NoError(t, err)
		assert.Nil(t, repo)

		folder, err := s.GetFolderByID("nope")
		require.NoError(t, err)
		assert.Nil(t, folder)

		token, err := s.GetTokenByID("nope")
		require.NoError(t, err)
		assert.Nil(t, token)
	})

	t.Run("delete returns error", func(t *testing.T) {
		assert.Error(t, s.DeleteRepo("nope"))
		assert.Error(t, s.DeleteFolder("nope"))
		assert.Error(t, s.DeleteToken("nope"))
	})

	t.Run("update returns error", func(t *testing.T) {
		assert.Error(t, s.UpdateRepo(&Repo{ID: "nope"}))
		assert.Error(t, s.UpdateFolder(&Folder{ID: "nope"}))
	})
}

func TestStore_GenerateAdminToken(t *testing.T) {
	s := newTestStore(t)

	t.Run("creates admin token", func(t *testing.T) {
		rawToken, err := s.GenerateAdminToken()
		require.NoError(t, err)
		assert.NotEmpty(t, rawToken)

		// Should have one admin token
		tokens, err := s.ListTokens("", 10)
		require.NoError(t, err)
		assert.Len(t, tokens, 1)
		assert.True(t, tokens[0].IsAdmin)
	})

	t.Run("second call returns empty string", func(t *testing.T) {
		token, err := s.GenerateAdminToken()
		require.NoError(t, err)
		assert.Empty(t, token)
	})
}

func TestStore_OptionalFields(t *testing.T) {
	s := newTestStore(t)

	t.Run("namespace with limits", func(t *testing.T) {
		repoLimit := 100
		storageLimit := 1000000
		ns := &Namespace{
			ID:                "ns-limits",
			Name:              "limited",
			CreatedAt:         time.Now(),
			RepoLimit:         &repoLimit,
			StorageLimitBytes: &storageLimit,
		}
		require.NoError(t, s.CreateNamespace(ns))

		got, _ := s.GetNamespace("ns-limits")
		require.NotNil(t, got.RepoLimit)
		require.NotNil(t, got.StorageLimitBytes)
		assert.Equal(t, 100, *got.RepoLimit)
		assert.Equal(t, 1000000, *got.StorageLimitBytes)
	})

	t.Run("token with expiry", func(t *testing.T) {
		expiry := time.Now().Add(24 * time.Hour)
		token := &Token{
			ID:          "token-expiry",
			TokenHash:   "hash-expiry",
			TokenLookup: "token-ex",
			IsAdmin:     false,
			ExpiresAt:   &expiry,
			CreatedAt:   time.Now(),
		}
		require.NoError(t, s.CreateToken(token))

		got, _ := s.GetTokenByID("token-expiry")
		require.NotNil(t, got.ExpiresAt)
	})

	t.Run("folder without color", func(t *testing.T) {
		ns := createTestNamespace(t, s, "ns-folder")
		folder := &Folder{
			ID:          "folder-no-color",
			NamespaceID: ns.ID,
			Name:        "plain",
			CreatedAt:   time.Now(),
		}
		require.NoError(t, s.CreateFolder(folder))

		got, _ := s.GetFolderByID("folder-no-color")
		assert.Nil(t, got.Color)
	})
}

func TestPermission_BitOperations(t *testing.T) {
	t.Run("Has checks single permission", func(t *testing.T) {
		p := PermRepoRead | PermRepoWrite
		assert.True(t, p.Has(PermRepoRead))
		assert.True(t, p.Has(PermRepoWrite))
		assert.False(t, p.Has(PermRepoAdmin))
	})

	t.Run("Has checks combined permissions", func(t *testing.T) {
		p := PermRepoRead | PermRepoWrite | PermRepoAdmin
		assert.True(t, p.Has(PermRepoRead|PermRepoWrite))
		assert.False(t, (PermRepoRead | PermRepoWrite).Has(PermRepoAdmin))
	})

	t.Run("ToStrings returns permission names", func(t *testing.T) {
		p := PermRepoRead | PermNamespaceWrite
		strs := p.ToStrings()
		assert.Contains(t, strs, "repo:read")
		assert.Contains(t, strs, "namespace:write")
		assert.Len(t, strs, 2)
	})

	t.Run("ParsePermissions converts strings to bits", func(t *testing.T) {
		p, err := ParsePermissions([]string{"repo:read", "namespace:admin"})
		require.NoError(t, err)
		assert.True(t, p.Has(PermRepoRead))
		assert.True(t, p.Has(PermNamespaceAdmin))
		assert.False(t, p.Has(PermRepoWrite))
	})

	t.Run("ParsePermissions rejects invalid", func(t *testing.T) {
		_, err := ParsePermissions([]string{"invalid:perm"})
		assert.Error(t, err)
	})
}

func TestPermission_ExpandImplied(t *testing.T) {
	t.Run("repo:admin implies repo:write and repo:read", func(t *testing.T) {
		p := ExpandImplied(PermRepoAdmin)
		assert.True(t, p.Has(PermRepoAdmin))
		assert.True(t, p.Has(PermRepoWrite))
		assert.True(t, p.Has(PermRepoRead))
	})

	t.Run("repo:write implies repo:read", func(t *testing.T) {
		p := ExpandImplied(PermRepoWrite)
		assert.True(t, p.Has(PermRepoWrite))
		assert.True(t, p.Has(PermRepoRead))
		assert.False(t, p.Has(PermRepoAdmin))
	})

	t.Run("namespace:admin implies namespace:write and namespace:read", func(t *testing.T) {
		p := ExpandImplied(PermNamespaceAdmin)
		assert.True(t, p.Has(PermNamespaceAdmin))
		assert.True(t, p.Has(PermNamespaceWrite))
		assert.True(t, p.Has(PermNamespaceRead))
	})

	t.Run("repo perms don't imply namespace perms", func(t *testing.T) {
		p := ExpandImplied(PermRepoAdmin)
		assert.False(t, p.Has(PermNamespaceRead))
	})
}

func TestPermissionChecker_DenyBehavior(t *testing.T) {
	s := newTestStore(t)
	ns := createTestNamespace(t, s, "ns-deny")
	user := createTestUser(t, s, "user-deny", ns.ID)
	userID := user.ID
	token := &Token{
		ID:          "token-deny",
		TokenHash:   "hash",
		TokenLookup: "denytest",
		IsAdmin:     false,
		UserID:      &userID,
		CreatedAt:   time.Now(),
	}
	require.NoError(t, s.CreateToken(token))

	t.Run("deny blocks specific permission without expanding", func(t *testing.T) {
		// Grant namespace:admin but deny namespace:admin specifically
		// Deny should NOT expand, so namespace:write should still work
		require.NoError(t, s.UpsertNamespaceGrant(&NamespaceGrant{
			UserID:      user.ID,
			NamespaceID: ns.ID,
			AllowBits:   PermNamespaceAdmin,
			DenyBits:    PermNamespaceAdmin,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}))

		checker := NewPermissionChecker(s)

		// namespace:admin is explicitly denied
		has, err := checker.CheckNamespacePermission(token.ID, ns.ID, PermNamespaceAdmin)
		require.NoError(t, err)
		assert.False(t, has, "namespace:admin should be denied")

		// namespace:write should be allowed (deny doesn't expand)
		has, err = checker.CheckNamespacePermission(token.ID, ns.ID, PermNamespaceWrite)
		require.NoError(t, err)
		assert.True(t, has, "namespace:write should be allowed since deny doesn't expand")

		// namespace:read should be allowed
		has, err = checker.CheckNamespacePermission(token.ID, ns.ID, PermNamespaceRead)
		require.NoError(t, err)
		assert.True(t, has, "namespace:read should be allowed")
	})
}

func TestPermissionChecker_RepoOnlyListing(t *testing.T) {
	s := newTestStore(t)
	ns := createTestNamespace(t, s, "ns-repo-only")
	repo1 := createTestRepo(t, s, ns.ID, "repo1")
	repo2 := createTestRepo(t, s, ns.ID, "repo2")
	createTestRepo(t, s, ns.ID, "repo3") // no grant

	userNs := createTestNamespace(t, s, "ns-user-primary")
	user := createTestUser(t, s, "user-repo-only", userNs.ID)
	userID := user.ID
	token := &Token{
		ID:          "token-repo-only",
		TokenHash:   "hash",
		TokenLookup: "repoonly",
		IsAdmin:     false,
		UserID:      &userID,
		CreatedAt:   time.Now(),
	}
	require.NoError(t, s.CreateToken(token))

	t.Run("repo grants without namespace grant", func(t *testing.T) {
		// Grant repo:read on repo1 and repo2, but no namespace grant
		require.NoError(t, s.UpsertRepoGrant(&RepoGrant{
			UserID:    user.ID,
			RepoID:    repo1.ID,
			AllowBits: PermRepoRead,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}))
		require.NoError(t, s.UpsertRepoGrant(&RepoGrant{
			UserID:    user.ID,
			RepoID:    repo2.ID,
			AllowBits: PermRepoRead | PermRepoWrite,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}))

		checker := NewPermissionChecker(s)

		// Token has repo grants in namespace
		hasGrants, err := checker.HasAnyRepoGrants(token.ID, ns.ID)
		require.NoError(t, err)
		assert.True(t, hasGrants)

		// Token cannot access namespace directly
		has, err := checker.CheckNamespacePermission(token.ID, ns.ID, PermNamespaceRead)
		require.NoError(t, err)
		assert.False(t, has, "should not have namespace:read without grant")

		// But can access specific repos
		has, err = checker.CheckRepoPermission(token.ID, repo1, PermRepoRead)
		require.NoError(t, err)
		assert.True(t, has, "should have repo:read on repo1")

		has, err = checker.CheckRepoPermission(token.ID, repo2, PermRepoWrite)
		require.NoError(t, err)
		assert.True(t, has, "should have repo:write on repo2")

		// List repos with grants should return only granted repos
		repos, err := s.ListUserReposWithGrants(user.ID, ns.ID)
		require.NoError(t, err)
		assert.Len(t, repos, 2)

		names := make([]string, len(repos))
		for i, r := range repos {
			names[i] = r.Name
		}
		assert.Contains(t, names, "repo1")
		assert.Contains(t, names, "repo2")
		assert.NotContains(t, names, "repo3")
	})
}

func createTestUser(t *testing.T, s *SQLiteStore, id string, primaryNamespaceID string) *User {
	t.Helper()
	user := &User{
		ID:                 id,
		PrimaryNamespaceID: primaryNamespaceID,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
	require.NoError(t, s.CreateUser(user))
	return user
}

func TestStore_UserLifecycle(t *testing.T) {
	s := newTestStore(t)
	ns := createTestNamespace(t, s, "ns-user-lifecycle")

	t.Run("create", func(t *testing.T) {
		createTestUser(t, s, "user-1", ns.ID)
	})

	t.Run("get by ID", func(t *testing.T) {
		got, err := s.GetUser("user-1")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, ns.ID, got.PrimaryNamespaceID)
	})

	t.Run("list", func(t *testing.T) {
		users, err := s.ListUsers("", 10)
		require.NoError(t, err)
		assert.Len(t, users, 1)
	})

	t.Run("delete", func(t *testing.T) {
		require.NoError(t, s.DeleteUser("user-1"))

		got, err := s.GetUser("user-1")
		require.NoError(t, err)
		assert.Nil(t, got)
	})
}

func TestStore_UserGrants(t *testing.T) {
	s := newTestStore(t)
	ns := createTestNamespace(t, s, "ns-1")
	repo := createTestRepo(t, s, ns.ID, "test-repo")
	user := createTestUser(t, s, "user-grants", ns.ID)

	t.Run("namespace grant CRUD", func(t *testing.T) {
		grant := &NamespaceGrant{
			UserID:      user.ID,
			NamespaceID: ns.ID,
			AllowBits:   PermNamespaceWrite | PermRepoAdmin,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		require.NoError(t, s.UpsertNamespaceGrant(grant))

		got, err := s.GetNamespaceGrant(user.ID, ns.ID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.True(t, got.AllowBits.Has(PermNamespaceWrite))

		grants, err := s.ListUserNamespaceGrants(user.ID)
		require.NoError(t, err)
		assert.Len(t, grants, 1)

		require.NoError(t, s.DeleteNamespaceGrant(user.ID, ns.ID))
		got, _ = s.GetNamespaceGrant(user.ID, ns.ID)
		assert.Nil(t, got)
	})

	t.Run("repo grant CRUD", func(t *testing.T) {
		grant := &RepoGrant{
			UserID:    user.ID,
			RepoID:    repo.ID,
			AllowBits: PermRepoRead | PermRepoWrite,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		require.NoError(t, s.UpsertRepoGrant(grant))

		got, err := s.GetRepoGrant(user.ID, repo.ID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.True(t, got.AllowBits.Has(PermRepoWrite))

		grants, err := s.ListUserRepoGrants(user.ID)
		require.NoError(t, err)
		assert.Len(t, grants, 1)

		repos, err := s.ListUserReposWithGrants(user.ID, ns.ID)
		require.NoError(t, err)
		assert.Len(t, repos, 1)

		has, err := s.HasRepoGrantsInNamespace(user.ID, ns.ID)
		require.NoError(t, err)
		assert.True(t, has)

		require.NoError(t, s.DeleteRepoGrant(user.ID, repo.ID))
		got, _ = s.GetRepoGrant(user.ID, repo.ID)
		assert.Nil(t, got)
	})
}

func TestStore_PrimaryNamespaceGrantProtection(t *testing.T) {
	s := newTestStore(t)

	ns1 := createTestNamespace(t, s, "user1-ns")
	user1 := createTestUser(t, s, "user1", ns1.ID)

	ns2 := createTestNamespace(t, s, "user2-ns")
	user2 := createTestUser(t, s, "user2", ns2.ID)

	sharedNs := createTestNamespace(t, s, "shared-ns")

	t.Run("owner can grant themselves access to their primary namespace", func(t *testing.T) {
		grant := &NamespaceGrant{
			UserID:      user1.ID,
			NamespaceID: ns1.ID,
			AllowBits:   PermNamespaceWrite | PermRepoAdmin,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		err := s.UpsertNamespaceGrant(grant)
		require.NoError(t, err)
	})

	t.Run("cannot grant other user access to primary namespace", func(t *testing.T) {
		grant := &NamespaceGrant{
			UserID:      user2.ID,
			NamespaceID: ns1.ID,
			AllowBits:   PermNamespaceWrite | PermRepoAdmin,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		err := s.UpsertNamespaceGrant(grant)
		require.ErrorIs(t, err, ErrPrimaryNamespaceGrant)
	})

	t.Run("can grant any user access to non-primary namespace", func(t *testing.T) {
		grant1 := &NamespaceGrant{
			UserID:      user1.ID,
			NamespaceID: sharedNs.ID,
			AllowBits:   PermNamespaceWrite | PermRepoAdmin,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		require.NoError(t, s.UpsertNamespaceGrant(grant1))

		grant2 := &NamespaceGrant{
			UserID:      user2.ID,
			NamespaceID: sharedNs.ID,
			AllowBits:   PermNamespaceWrite | PermRepoAdmin,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		require.NoError(t, s.UpsertNamespaceGrant(grant2))
	})

	t.Run("GetUserByPrimaryNamespaceID returns correct user", func(t *testing.T) {
		owner, err := s.GetUserByPrimaryNamespaceID(ns1.ID)
		require.NoError(t, err)
		require.NotNil(t, owner)
		assert.Equal(t, user1.ID, owner.ID)
	})

	t.Run("GetUserByPrimaryNamespaceID returns nil for non-primary namespace", func(t *testing.T) {
		owner, err := s.GetUserByPrimaryNamespaceID(sharedNs.ID)
		require.NoError(t, err)
		assert.Nil(t, owner)
	})
}

func TestStore_UserBoundTokens(t *testing.T) {
	s := newTestStore(t)
	ns := createTestNamespace(t, s, "ns-user-tokens")
	user := createTestUser(t, s, "user-tokens", ns.ID)

	t.Run("generate user bound token", func(t *testing.T) {
		rawToken, token, err := s.GenerateUserToken(user.ID, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, rawToken)
		require.NotNil(t, token.UserID)
		assert.Equal(t, user.ID, *token.UserID)
	})

	t.Run("list user tokens", func(t *testing.T) {
		tokens, err := s.ListUserTokens(user.ID)
		require.NoError(t, err)
		assert.Len(t, tokens, 1)
	})

	t.Run("token with user_id", func(t *testing.T) {
		userID := user.ID
		token := &Token{
			ID:          "token-with-user",
			TokenHash:   "hash",
			TokenLookup: "usertok1",
			IsAdmin:     false,
			UserID:      &userID,
			CreatedAt:   time.Now(),
		}
		require.NoError(t, s.CreateToken(token))

		got, err := s.GetTokenByID("token-with-user")
		require.NoError(t, err)
		require.NotNil(t, got.UserID)
		assert.Equal(t, user.ID, *got.UserID)
	})
}

func TestPermissionChecker_UserBoundTokenPermissions(t *testing.T) {
	s := newTestStore(t)
	ns := createTestNamespace(t, s, "ns-user-perms")
	repo := createTestRepo(t, s, ns.ID, "test-repo")
	user := createTestUser(t, s, "user-perms", ns.ID)

	userID := user.ID
	token := &Token{
		ID:          "user-bound-token",
		TokenHash:   "hash",
		TokenLookup: "userbndt",
		IsAdmin:     false,
		UserID:      &userID,
		CreatedAt:   time.Now(),
	}
	require.NoError(t, s.CreateToken(token))

	t.Run("user namespace grant gives token access", func(t *testing.T) {
		require.NoError(t, s.UpsertNamespaceGrant(&NamespaceGrant{
			UserID:      user.ID,
			NamespaceID: ns.ID,
			AllowBits:   PermNamespaceWrite | PermRepoAdmin,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}))

		checker := NewPermissionChecker(s)

		has, err := checker.CheckNamespacePermission(token.ID, ns.ID, PermNamespaceRead)
		require.NoError(t, err)
		assert.True(t, has, "user-bound token should inherit user's namespace:read via write")

		has, err = checker.CheckNamespacePermission(token.ID, ns.ID, PermNamespaceWrite)
		require.NoError(t, err)
		assert.True(t, has, "user-bound token should inherit user's namespace:write")

		has, err = checker.CheckRepoPermission(token.ID, repo, PermRepoAdmin)
		require.NoError(t, err)
		assert.True(t, has, "user-bound token should inherit user's repo:admin")
	})

	t.Run("user repo grant gives token access", func(t *testing.T) {
		require.NoError(t, s.DeleteNamespaceGrant(user.ID, ns.ID))

		require.NoError(t, s.UpsertRepoGrant(&RepoGrant{
			UserID:    user.ID,
			RepoID:    repo.ID,
			AllowBits: PermRepoRead,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}))

		checker := NewPermissionChecker(s)

		has, err := checker.CheckRepoPermission(token.ID, repo, PermRepoRead)
		require.NoError(t, err)
		assert.True(t, has, "user-bound token should inherit user's repo:read")

		has, err = checker.CheckRepoPermission(token.ID, repo, PermRepoWrite)
		require.NoError(t, err)
		assert.False(t, has, "user-bound token should not have repo:write without grant")
	})

	t.Run("can access namespace via user grants", func(t *testing.T) {
		checker := NewPermissionChecker(s)

		canAccess, err := checker.CanAccessNamespace(token.ID, ns.ID)
		require.NoError(t, err)
		assert.True(t, canAccess, "user-bound token should access namespace via user repo grants")
	})
}

func TestStore_UserCascadeDelete(t *testing.T) {
	s := newTestStore(t)
	userNs := createTestNamespace(t, s, "ns-user-cascade-primary")
	ns := createTestNamespace(t, s, "ns-cascade")
	repo := createTestRepo(t, s, ns.ID, "test-repo")
	user := createTestUser(t, s, "user-cascade", userNs.ID)

	require.NoError(t, s.UpsertNamespaceGrant(&NamespaceGrant{
		UserID:      user.ID,
		NamespaceID: ns.ID,
		AllowBits:   PermNamespaceWrite,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}))

	require.NoError(t, s.UpsertRepoGrant(&RepoGrant{
		UserID:    user.ID,
		RepoID:    repo.ID,
		AllowBits: PermRepoRead,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}))

	_, token, err := s.GenerateUserToken(user.ID, nil)
	require.NoError(t, err)

	t.Run("deleting user cascades grants and tokens", func(t *testing.T) {
		require.NoError(t, s.DeleteUser(user.ID))

		got, _ := s.GetNamespaceGrant(user.ID, ns.ID)
		assert.Nil(t, got, "user namespace grant should be deleted")

		repoGrant, _ := s.GetRepoGrant(user.ID, repo.ID)
		assert.Nil(t, repoGrant, "user repo grant should be deleted")

		tokenGot, _ := s.GetTokenByID(token.ID)
		assert.Nil(t, tokenGot, "user-bound token should be deleted")
	})
}
