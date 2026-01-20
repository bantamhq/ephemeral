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

func createTestToken(t *testing.T, s *SQLiteStore, nsID, id, hash string) *Token {
	t.Helper()
	token := &Token{
		ID:          id,
		TokenHash:   hash,
		NamespaceID: nsID,
		Scope:       ScopeFull,
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

	t.Run("delete cascades", func(t *testing.T) {
		repo := createTestRepo(t, s, ns.ID, "cascade-test")
		folder := createTestFolder(t, s, ns.ID, "cascade-folder", nil)
		s.AddRepoFolder(repo.ID, folder.ID)
		createTestToken(t, s, ns.ID, "cascade-token", "hash")

		require.NoError(t, s.DeleteNamespace("ns-1"))

		got, _ := s.GetNamespace("ns-1")
		assert.Nil(t, got, "namespace should be deleted")

		r, _ := s.GetRepoByID(repo.ID)
		assert.Nil(t, r, "repo should be cascade deleted")
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

	var token *Token

	t.Run("create", func(t *testing.T) {
		token = &Token{
			ID:          "token-1",
			TokenHash:   "hash123",
			NamespaceID: ns.ID,
			Scope:       ScopeFull,
			CreatedAt:   time.Now(),
		}
		require.NoError(t, s.CreateToken(token))
	})

	t.Run("get by hash", func(t *testing.T) {
		got, err := s.GetTokenByHash("hash123")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "token-1", got.ID)
	})

	t.Run("get by ID", func(t *testing.T) {
		got, err := s.GetTokenByID("token-1")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, ScopeFull, got.Scope)
	})

	t.Run("list", func(t *testing.T) {
		tokens, err := s.ListTokens(ns.ID, "", 10)
		require.NoError(t, err)
		assert.Len(t, tokens, 1)
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

func TestStore_GenerateRootToken(t *testing.T) {
	s := newTestStore(t)

	t.Run("creates default namespace and token", func(t *testing.T) {
		rawToken, err := s.GenerateRootToken()
		require.NoError(t, err)
		assert.NotEmpty(t, rawToken)

		ns, err := s.GetNamespaceByName("default")
		require.NoError(t, err)
		require.NotNil(t, ns)

		tokens, err := s.ListTokens(ns.ID, "", 10)
		require.NoError(t, err)
		assert.Len(t, tokens, 1)
		assert.Equal(t, ScopeAdmin, tokens[0].Scope)
	})

	t.Run("second call returns empty string", func(t *testing.T) {
		token, err := s.GenerateRootToken()
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
		ns := createTestNamespace(t, s, "ns-token")
		expiry := time.Now().Add(24 * time.Hour)
		name := "test-token"
		token := &Token{
			ID:          "token-expiry",
			TokenHash:   "hash-expiry",
			NamespaceID: ns.ID,
			Scope:       ScopeReadOnly,
			Name:        &name,
			ExpiresAt:   &expiry,
			CreatedAt:   time.Now(),
		}
		require.NoError(t, s.CreateToken(token))

		got, _ := s.GetTokenByID("token-expiry")
		require.NotNil(t, got.Name)
		require.NotNil(t, got.ExpiresAt)
		assert.Equal(t, "test-token", *got.Name)
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

