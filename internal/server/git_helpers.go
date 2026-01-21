package server

import (
	"net/http"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"ephemeral/internal/store"
)

func (s *Server) openGitRepoForRepo(w http.ResponseWriter, repo *store.Repo) (*git.Repository, bool) {
	gitRepo, err := s.openGitRepo(repo.NamespaceID, repo.Name)
	if err != nil {
		JSONError(w, http.StatusNotFound, "Repository not initialized")
		return nil, false
	}

	return gitRepo, true
}

func (s *Server) resolveRefForRepo(w http.ResponseWriter, gitRepo *git.Repository, refStr string) (*plumbing.Hash, bool) {
	hash, err := resolveRef(gitRepo, refStr)
	if err != nil {
		writeRefError(w, err, refStr)
		return nil, false
	}

	return hash, true
}

func (s *Server) loadCommitFromRef(w http.ResponseWriter, gitRepo *git.Repository, refStr string) (*object.Commit, *plumbing.Hash, bool) {
	hash, ok := s.resolveRefForRepo(w, gitRepo, refStr)
	if !ok {
		return nil, nil, false
	}

	commit, err := gitRepo.CommitObject(*hash)
	if err != nil {
		JSONError(w, http.StatusNotFound, "Commit not found")
		return nil, nil, false
	}

	return commit, hash, true
}
