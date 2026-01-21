package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"ephemeral/internal/store"
)

const (
	refTypeBranch = "branch"
	refTypeTag    = "tag"
)

type createRefRequest struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Target string `json:"target,omitempty"`
}

type updateRefRequest struct {
	Target  *string `json:"target,omitempty"`
	NewName *string `json:"new_name,omitempty"`
}

type defaultBranchRequest struct {
	Name string `json:"name"`
}

func normalizeRefType(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "branch", "branches":
		return refTypeBranch, nil
	case "tag", "tags":
		return refTypeTag, nil
	default:
		return "", fmt.Errorf("invalid ref type")
	}
}

func normalizeRefName(value string) string {
	return strings.Trim(strings.TrimSpace(value), "/")
}

func buildRefName(refType, name string) (plumbing.ReferenceName, error) {
	trimmed := normalizeRefName(name)
	if trimmed == "" {
		return "", fmt.Errorf("ref name is required")
	}

	var refName plumbing.ReferenceName
	switch refType {
	case refTypeBranch:
		refName = plumbing.NewBranchReferenceName(trimmed)
	case refTypeTag:
		refName = plumbing.NewTagReferenceName(trimmed)
	default:
		return "", fmt.Errorf("invalid ref type")
	}

	if err := refName.Validate(); err != nil {
		return "", fmt.Errorf("invalid ref name")
	}

	return refName, nil
}

func refExists(repo *git.Repository, refName plumbing.ReferenceName) (bool, error) {
	_, err := repo.Reference(refName, true)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return false, nil
	}
	return false, err
}

func (s *Server) handleCreateRef(w http.ResponseWriter, r *http.Request) {
	token := s.requireUserToken(w, r)
	if token == nil {
		return
	}
	if !s.requireScope(w, token, store.ScopeRepos) {
		return
	}

	repo := s.requireRepoAccess(w, r, token)
	if repo == nil {
		return
	}

	var req createRefRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	refType, err := normalizeRefType(req.Type)
	if err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid ref type")
		return
	}

	refName, err := buildRefName(refType, req.Name)
	if err != nil {
		JSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	gitRepo, ok := s.openGitRepoForRepo(w, repo)
	if !ok {
		return
	}

	exists, err := refExists(gitRepo, refName)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check ref")
		return
	}
	if exists {
		JSONError(w, http.StatusConflict, "Reference already exists")
		return
	}

	target := req.Target
	if target == "" {
		target = "HEAD"
	}

	hash, err := resolveRef(gitRepo, target)
	if err != nil {
		writeRefError(w, err, target)
		return
	}

	newRef := plumbing.NewHashReference(refName, *hash)
	if err := gitRepo.Storer.SetReference(newRef); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to create reference")
		return
	}

	resp := RefResponse{
		Name:      refName.Short(),
		Type:      refType,
		CommitSHA: hash.String(),
		IsDefault: false,
	}

	JSON(w, http.StatusCreated, resp)
}

func (s *Server) handleUpdateRef(w http.ResponseWriter, r *http.Request) {
	token := s.requireUserToken(w, r)
	if token == nil {
		return
	}
	if !s.requireScope(w, token, store.ScopeRepos) {
		return
	}

	repo := s.requireRepoAccess(w, r, token)
	if repo == nil {
		return
	}

	refType, name, refName, ok := parseRefParams(w, r)
	if !ok {
		return
	}

	var req updateRefRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Target == nil && req.NewName == nil {
		JSONError(w, http.StatusBadRequest, "No updates provided")
		return
	}

	gitRepo, ok := s.openGitRepoForRepo(w, repo)
	if !ok {
		return
	}

	existingRef, err := gitRepo.Reference(refName, true)
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			JSONError(w, http.StatusNotFound, "Reference not found")
			return
		}
		JSONError(w, http.StatusInternalServerError, "Failed to get reference")
		return
	}

	targetHash := existingRef.Hash()
	if req.Target != nil {
		hash, err := resolveRef(gitRepo, *req.Target)
		if err != nil {
			writeRefError(w, err, *req.Target)
			return
		}
		targetHash = *hash
	}

	newRefName := refName
	if req.NewName != nil {
		normalized := normalizeRefName(*req.NewName)
		if normalized != name {
			refNameCandidate, err := buildRefName(refType, normalized)
			if err != nil {
				JSONError(w, http.StatusBadRequest, err.Error())
				return
			}

			exists, err := refExists(gitRepo, refNameCandidate)
			if err != nil {
				JSONError(w, http.StatusInternalServerError, "Failed to check ref")
				return
			}
			if exists {
				JSONError(w, http.StatusConflict, "Reference already exists")
				return
			}
			newRefName = refNameCandidate
		}
	}

	updatedRef := plumbing.NewHashReference(newRefName, targetHash)
	if err := gitRepo.Storer.SetReference(updatedRef); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to update reference")
		return
	}

	if newRefName != refName {
		if err := gitRepo.Storer.RemoveReference(refName); err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to remove old reference")
			return
		}

		if err := updateHeadReference(gitRepo, refName, newRefName); err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to update default branch")
			return
		}
	}

	resp := RefResponse{
		Name:      newRefName.Short(),
		Type:      refType,
		CommitSHA: targetHash.String(),
		IsDefault: false,
	}

	JSON(w, http.StatusOK, resp)
}

func (s *Server) handleDeleteRef(w http.ResponseWriter, r *http.Request) {
	token := s.requireUserToken(w, r)
	if token == nil {
		return
	}
	if !s.requireScope(w, token, store.ScopeRepos) {
		return
	}

	repo := s.requireRepoAccess(w, r, token)
	if repo == nil {
		return
	}

	refType, _, refName, ok := parseRefParams(w, r)
	if !ok {
		return
	}

	gitRepo, ok := s.openGitRepoForRepo(w, repo)
	if !ok {
		return
	}

	if refType == refTypeBranch {
		isDefault, err := isDefaultBranch(gitRepo, refName)
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to check default branch")
			return
		}
		if isDefault {
			JSONError(w, http.StatusConflict, "Cannot delete default branch")
			return
		}
	}

	if err := gitRepo.Storer.RemoveReference(refName); err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			JSONError(w, http.StatusNotFound, "Reference not found")
			return
		}
		JSONError(w, http.StatusInternalServerError, "Failed to delete reference")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSetDefaultBranch(w http.ResponseWriter, r *http.Request) {
	token := s.requireUserToken(w, r)
	if token == nil {
		return
	}
	if !s.requireScope(w, token, store.ScopeRepos) {
		return
	}

	repo := s.requireRepoAccess(w, r, token)
	if repo == nil {
		return
	}

	var req defaultBranchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	refName, err := buildRefName(refTypeBranch, req.Name)
	if err != nil {
		JSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	gitRepo, ok := s.openGitRepoForRepo(w, repo)
	if !ok {
		return
	}

	ref, err := gitRepo.Reference(refName, true)
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			JSONError(w, http.StatusNotFound, "Branch not found")
			return
		}
		JSONError(w, http.StatusInternalServerError, "Failed to get branch")
		return
	}

	headRef := plumbing.NewSymbolicReference(plumbing.HEAD, refName)
	if err := gitRepo.Storer.SetReference(headRef); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to update default branch")
		return
	}

	resp := RefResponse{
		Name:      refName.Short(),
		Type:      refTypeBranch,
		CommitSHA: ref.Hash().String(),
		IsDefault: true,
	}

	JSON(w, http.StatusOK, resp)
}

func updateHeadReference(repo *git.Repository, oldRef, newRef plumbing.ReferenceName) error {
	headRef, err := repo.Reference(plumbing.HEAD, false)
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return nil
		}
		return err
	}

	if headRef.Type() != plumbing.SymbolicReference {
		return nil
	}
	if headRef.Target() != oldRef {
		return nil
	}

	updated := plumbing.NewSymbolicReference(plumbing.HEAD, newRef)
	return repo.Storer.SetReference(updated)
}

func isDefaultBranch(repo *git.Repository, refName plumbing.ReferenceName) (bool, error) {
	headRef, err := repo.Reference(plumbing.HEAD, false)
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return false, nil
		}
		return false, err
	}

	if headRef.Type() == plumbing.SymbolicReference {
		return headRef.Target() == refName, nil
	}

	return false, nil
}

func parseRefParams(w http.ResponseWriter, r *http.Request) (string, string, plumbing.ReferenceName, bool) {
	refType, err := normalizeRefType(chi.URLParam(r, "refType"))
	if err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid ref type")
		return "", "", "", false
	}

	name := normalizeRefName(chi.URLParam(r, "*"))
	refName, err := buildRefName(refType, name)
	if err != nil {
		JSONError(w, http.StatusBadRequest, err.Error())
		return "", "", "", false
	}

	return refType, name, refName, true
}
