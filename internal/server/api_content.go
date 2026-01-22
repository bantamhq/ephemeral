package server

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"

	"ephemeral/internal/store"
)

type RefResponse struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	CommitSHA string `json:"commit_sha"`
	IsDefault bool   `json:"is_default"`
}

type CommitResponse struct {
	SHA        string       `json:"sha"`
	Message    string       `json:"message"`
	Author     GitAuthor    `json:"author"`
	Committer  GitAuthor    `json:"committer"`
	ParentSHAs []string     `json:"parent_shas"`
	TreeSHA    string       `json:"tree_sha"`
	Stats      *CommitStats `json:"stats,omitempty"`
}

type GitAuthor struct {
	Name  string    `json:"name"`
	Email string    `json:"email"`
	Date  time.Time `json:"date"`
}

type CommitStats struct {
	FilesChanged int `json:"files_changed"`
	Additions    int `json:"additions"`
	Deletions    int `json:"deletions"`
}

type TreeEntryResponse struct {
	Name        string              `json:"name"`
	Path        string              `json:"path"`
	Type        string              `json:"type"`
	Mode        string              `json:"mode"`
	SHA         string              `json:"sha"`
	Size        *int64              `json:"size,omitempty"`
	HasChildren *bool               `json:"has_children,omitempty"`
	Children    []TreeEntryResponse `json:"children,omitempty"`
}

type BlobResponse struct {
	SHA       string  `json:"sha"`
	Size      int64   `json:"size"`
	Content   *string `json:"content,omitempty"`
	Encoding  string  `json:"encoding"`
	IsBinary  bool    `json:"is_binary"`
	Truncated bool    `json:"truncated"`
}

const maxBlobSize = 1024 * 1024

const (
	defaultTreeDepth = 1
	maxTreeDepth     = 10
)

var mimeTypesByExt = map[string]string{
	".go":   "text/plain; charset=utf-8",
	".js":   "text/javascript; charset=utf-8",
	".ts":   "text/typescript; charset=utf-8",
	".py":   "text/x-python; charset=utf-8",
	".rb":   "text/x-ruby; charset=utf-8",
	".rs":   "text/x-rust; charset=utf-8",
	".java": "text/x-java; charset=utf-8",
	".c":    "text/x-c; charset=utf-8",
	".cpp":  "text/x-c++; charset=utf-8",
	".h":    "text/x-c; charset=utf-8",
	".hpp":  "text/x-c++; charset=utf-8",
	".md":   "text/markdown; charset=utf-8",
	".json": "application/json",
	".yaml": "text/yaml; charset=utf-8",
	".yml":  "text/yaml; charset=utf-8",
	".xml":  "application/xml",
	".html": "text/html; charset=utf-8",
	".css":  "text/css; charset=utf-8",
	".txt":  "text/plain; charset=utf-8",
	".sh":   "text/x-shellscript; charset=utf-8",
	".sql":  "text/x-sql; charset=utf-8",
}

// checkRepoAccess verifies access to a repository.
// Returns the repo and namespace if access is allowed, otherwise writes an error response.
func (s *Server) checkRepoAccess(w http.ResponseWriter, r *http.Request) (*store.Repo, *store.Namespace, bool) {
	id := chi.URLParam(r, "id")
	repo, err := s.store.GetRepoByID(id)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get repository")
		return nil, nil, false
	}
	if repo == nil {
		JSONError(w, http.StatusNotFound, "Repository not found")
		return nil, nil, false
	}

	ns, err := s.store.GetNamespace(repo.NamespaceID)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get namespace")
		return nil, nil, false
	}
	if ns == nil {
		JSONError(w, http.StatusNotFound, "Repository not found")
		return nil, nil, false
	}

	token := GetTokenFromContext(r.Context())

	if repo.Public {
		return repo, ns, true
	}

	if token == nil {
		JSONError(w, http.StatusUnauthorized, "Authentication required")
		return nil, nil, false
	}

	// Admin tokens cannot be used for content access
	if token.IsAdmin {
		JSONError(w, http.StatusForbidden, "Admin token cannot be used for this operation")
		return nil, nil, false
	}

	hasRead, err := s.permissions.CheckRepoPermission(token.ID, repo, store.PermRepoRead)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to check access")
		return nil, nil, false
	}
	if !hasRead {
		JSONError(w, http.StatusForbidden, "Access denied")
		return nil, nil, false
	}

	return repo, ns, true
}

// openGitRepo opens a bare git repository from disk.
func (s *Server) openGitRepo(namespaceID, repoName string) (*git.Repository, error) {
	repoPath := filepath.Join(s.dataDir, "repos", namespaceID, repoName+".git")
	return git.PlainOpen(repoPath)
}

// resolveRef resolves a reference string (branch name, tag name, or SHA) to a commit hash.
func resolveRef(repo *git.Repository, refStr string) (*plumbing.Hash, error) {
	if refStr == "" {
		refStr = "HEAD"
	}

	if len(refStr) == 40 {
		hash := plumbing.NewHash(refStr)
		_, err := repo.CommitObject(hash)
		if err == nil {
			return &hash, nil
		}
	}

	ref, err := repo.Reference(plumbing.NewBranchReferenceName(refStr), true)
	if err == nil {
		hash := ref.Hash()
		return &hash, nil
	}

	ref, err = repo.Reference(plumbing.NewTagReferenceName(refStr), true)
	if err == nil {
		hash := ref.Hash()
		tagObj, err := repo.TagObject(hash)
		if err == nil {
			commitHash := tagObj.Target
			return &commitHash, nil
		}
		return &hash, nil
	}

	if refStr == "HEAD" {
		ref, err := repo.Head()
		if err != nil {
			return nil, fmt.Errorf("repository is empty")
		}
		hash := ref.Hash()
		return &hash, nil
	}

	return nil, fmt.Errorf("reference not found: %s", refStr)
}

func writeRefError(w http.ResponseWriter, err error, refStr string) {
	if strings.Contains(err.Error(), "empty") {
		JSONError(w, http.StatusNotFound, "Repository is empty")
		return
	}
	JSONError(w, http.StatusNotFound, fmt.Sprintf("Reference not found: %s", refStr))
}

func (s *Server) handleListRefs(w http.ResponseWriter, r *http.Request) {
	repo, _, ok := s.checkRepoAccess(w, r)
	if !ok {
		return
	}

	gitRepo, err := s.openGitRepo(repo.NamespaceID, repo.Name)
	if err != nil {
		JSONError(w, http.StatusNotFound, "Repository not initialized")
		return
	}

	var refs []RefResponse

	headRef, err := gitRepo.Head()
	var defaultBranch string
	if err == nil {
		defaultBranch = headRef.Name().Short()
	}

	branchIter, err := gitRepo.Branches()
	if err == nil {
		branchIter.ForEach(func(ref *plumbing.Reference) error {
			refs = append(refs, RefResponse{
				Name:      ref.Name().Short(),
				Type:      "branch",
				CommitSHA: ref.Hash().String(),
				IsDefault: ref.Name().Short() == defaultBranch,
			})
			return nil
		})
	}

	tagIter, err := gitRepo.Tags()
	if err == nil {
		tagIter.ForEach(func(ref *plumbing.Reference) error {
			commitSHA := ref.Hash().String()

			tagObj, err := gitRepo.TagObject(ref.Hash())
			if err == nil {
				commitSHA = tagObj.Target.String()
			}

			refs = append(refs, RefResponse{
				Name:      ref.Name().Short(),
				Type:      "tag",
				CommitSHA: commitSHA,
				IsDefault: false,
			})
			return nil
		})
	}

	if len(refs) == 0 {
		JSONError(w, http.StatusNotFound, "Repository is empty")
		return
	}

	sort.Slice(refs, func(i, j int) bool {
		if refs[i].IsDefault != refs[j].IsDefault {
			return refs[i].IsDefault
		}
		if refs[i].Type != refs[j].Type {
			return refs[i].Type == "branch"
		}
		return refs[i].Name < refs[j].Name
	})

	JSON(w, http.StatusOK, refs)
}

func computeCommitStats(commit *object.Commit) *CommitStats {
	stats := &CommitStats{}

	currentTree, err := commit.Tree()
	if err != nil {
		return stats
	}

	var parentTree *object.Tree
	if commit.NumParents() > 0 {
		parent, err := commit.Parent(0)
		if err == nil {
			parentTree, _ = parent.Tree()
		}
	}

	changes, err := object.DiffTree(parentTree, currentTree)
	if err != nil {
		return stats
	}

	stats.FilesChanged = len(changes)

	for _, change := range changes {
		patch, err := change.Patch()
		if err != nil {
			continue
		}

		for _, fileStat := range patch.Stats() {
			stats.Additions += fileStat.Addition
			stats.Deletions += fileStat.Deletion
		}
	}

	return stats
}

func (s *Server) handleListCommits(w http.ResponseWriter, r *http.Request) {
	repo, _, ok := s.checkRepoAccess(w, r)
	if !ok {
		return
	}

	gitRepo, ok := s.openGitRepoForRepo(w, repo)
	if !ok {
		return
	}

	refStr := r.URL.Query().Get("ref")
	cursor := r.URL.Query().Get("cursor")
	pathQuery := strings.TrimPrefix(r.URL.Query().Get("path"), "/")
	limit := parseLimit(r.URL.Query().Get("limit"), defaultPageSize)

	commit, hash, ok := s.loadCommitFromRef(w, gitRepo, refStr)
	if !ok {
		return
	}

	startFrom := *hash
	if cursor != "" {
		startFrom = plumbing.NewHash(cursor)
		if _, err := gitRepo.CommitObject(startFrom); err != nil {
			JSONError(w, http.StatusBadRequest, "Invalid cursor: commit not found")
			return
		}
	}

	var pathFilter func(string) bool
	if pathQuery != "" {
		tree, err := commit.Tree()
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to get tree")
			return
		}
		entry, err := tree.FindEntry(pathQuery)
		if err != nil {
			JSONError(w, http.StatusNotFound, fmt.Sprintf("Path not found: %s", pathQuery))
			return
		}

		if entry.Mode == filemode.Dir {
			prefix := strings.TrimSuffix(pathQuery, "/") + "/"
			pathFilter = func(path string) bool {
				return strings.HasPrefix(path, prefix)
			}
		} else if entry.Mode == filemode.Submodule || entry.Mode == filemode.Symlink || entry.Mode.IsFile() {
			pathFilter = func(path string) bool {
				return path == pathQuery
			}
		} else {
			pathFilter = func(path string) bool {
				return path == pathQuery
			}
		}
	}

	commitIter, err := gitRepo.Log(&git.LogOptions{From: startFrom, PathFilter: pathFilter})
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get commit history")
		return
	}
	defer commitIter.Close()

	if cursor != "" {
		if _, err := commitIter.Next(); err != nil {
			JSONList(w, []CommitResponse{}, nil, false)
			return
		}
	}

	var commits []CommitResponse

	for i := 0; i <= limit; i++ {
		commit, err := commitIter.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to iterate commits")
			return
		}

		commits = append(commits, commitToResponse(commit))
	}

	hasMore := len(commits) > limit

	var nextCursor *string
	if hasMore {
		nextCursor = &commits[limit].SHA
		commits = commits[:limit]
	}

	JSONList(w, commits, nextCursor, hasMore)
}

func buildTreeEntries(gitRepo *git.Repository, tree *object.Tree, basePath string, depth int) []TreeEntryResponse {
	var entries []TreeEntryResponse

	for _, entry := range tree.Entries {
		entryPath := entry.Name
		if basePath != "" {
			entryPath = basePath + "/" + entry.Name
		}

		resp := TreeEntryResponse{
			Name: entry.Name,
			Path: entryPath,
			Mode: fmt.Sprintf("%06o", entry.Mode),
			SHA:  entry.Hash.String(),
		}

		switch {
		case entry.Mode.IsFile():
			resp.Type = "file"
			blob, err := gitRepo.BlobObject(entry.Hash)
			if err == nil {
				size := blob.Size
				resp.Size = &size
			}
		case entry.Mode == 0040000:
			resp.Type = "dir"
			if depth > 1 {
				subTree, err := gitRepo.TreeObject(entry.Hash)
				if err == nil {
					hasChildren := len(subTree.Entries) > 0
					resp.HasChildren = &hasChildren
					resp.Children = buildTreeEntries(gitRepo, subTree, entryPath, depth-1)
				}
			}
		case entry.Mode == 0120000:
			resp.Type = "symlink"
		case entry.Mode == 0160000:
			resp.Type = "submodule"
		default:
			resp.Type = "file"
		}

		entries = append(entries, resp)
	}

	return entries
}

func sortTreeEntries(entries []TreeEntryResponse) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Type == "dir" && entries[j].Type != "dir" {
			return true
		}
		if entries[i].Type != "dir" && entries[j].Type == "dir" {
			return false
		}
		return entries[i].Name < entries[j].Name
	})

	for i := range entries {
		if len(entries[i].Children) > 0 {
			sortTreeEntries(entries[i].Children)
		}
	}
}

func (s *Server) handleGetTree(w http.ResponseWriter, r *http.Request) {
	repo, _, ok := s.checkRepoAccess(w, r)
	if !ok {
		return
	}

	gitRepo, ok := s.openGitRepoForRepo(w, repo)
	if !ok {
		return
	}

	refStr := chi.URLParam(r, "ref")
	pathParam := strings.Trim(chi.URLParam(r, "*"), "/")

	depth := defaultTreeDepth
	if depthStr := r.URL.Query().Get("depth"); depthStr != "" {
		if d, err := strconv.Atoi(depthStr); err == nil && d > 0 {
			depth = d
			if depth > maxTreeDepth {
				depth = maxTreeDepth
			}
		}
	}

	commit, _, ok := s.loadCommitFromRef(w, gitRepo, refStr)
	if !ok {
		return
	}

	tree, err := commit.Tree()
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get tree")
		return
	}

	if pathParam != "" {
		entry, err := tree.FindEntry(pathParam)
		if err != nil {
			JSONError(w, http.StatusNotFound, fmt.Sprintf("Path not found: %s", pathParam))
			return
		}

		if !entry.Mode.IsFile() {
			subTree, err := tree.Tree(pathParam)
			if err != nil {
				JSONError(w, http.StatusNotFound, fmt.Sprintf("Path not found: %s", pathParam))
				return
			}
			tree = subTree
		} else {
			JSONError(w, http.StatusBadRequest, "Path is a file, not a directory")
			return
		}
	}

	entries := buildTreeEntries(gitRepo, tree, pathParam, depth)
	sortTreeEntries(entries)

	JSON(w, http.StatusOK, entries)
}

func (s *Server) handleGetBlob(w http.ResponseWriter, r *http.Request) {
	repo, _, ok := s.checkRepoAccess(w, r)
	if !ok {
		return
	}

	gitRepo, ok := s.openGitRepoForRepo(w, repo)
	if !ok {
		return
	}

	refStr := chi.URLParam(r, "ref")
	pathParam := chi.URLParam(r, "*")
	pathParam = strings.TrimPrefix(pathParam, "/")
	raw := r.URL.Query().Get("raw") == "true"

	if pathParam == "" {
		JSONError(w, http.StatusBadRequest, "Path is required")
		return
	}

	commit, _, ok := s.loadCommitFromRef(w, gitRepo, refStr)
	if !ok {
		return
	}

	tree, err := commit.Tree()
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get tree")
		return
	}

	file, err := tree.File(pathParam)
	if err != nil {
		if _, treeErr := tree.Tree(pathParam); treeErr == nil {
			JSONError(w, http.StatusBadRequest, "Path is a directory, not a file")
			return
		}
		JSONError(w, http.StatusNotFound, fmt.Sprintf("Path not found: %s", pathParam))
		return
	}

	blob := &file.Blob

	if raw {
		s.serveRawBlob(w, r, blob, pathParam)
		return
	}

	reader, err := blob.Reader()
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to read file")
		return
	}
	defer reader.Close()

	size := blob.Size
	truncated := false
	readSize := size
	if readSize > maxBlobSize {
		readSize = maxBlobSize
		truncated = true
	}

	content := make([]byte, readSize)
	n, err := io.ReadFull(reader, content)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		JSONError(w, http.StatusInternalServerError, "Failed to read file")
		return
	}
	content = content[:n]

	isBinary := isBinaryContent(content)

	resp := BlobResponse{
		SHA:       blob.Hash.String(),
		Size:      size,
		IsBinary:  isBinary,
		Truncated: truncated,
	}

	if isBinary {
		encoded := base64.StdEncoding.EncodeToString(content)
		resp.Content = &encoded
		resp.Encoding = "base64"
	} else {
		str := string(content)
		resp.Content = &str
		resp.Encoding = "utf-8"
	}

	JSON(w, http.StatusOK, resp)
}

func (s *Server) serveRawBlob(w http.ResponseWriter, r *http.Request, blob *object.Blob, filename string) {
	reader, err := blob.Reader()
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to read file")
		return
	}
	defer reader.Close()

	contentType := detectContentType(filename, reader)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.FormatInt(blob.Size, 10))

	reader, err = blob.Reader()
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to read file")
		return
	}
	defer reader.Close()

	if _, err := io.Copy(w, reader); err != nil {
		fmt.Printf("Error streaming blob: %v\n", err)
	}
}

func isBinaryContent(content []byte) bool {
	if !utf8.Valid(content) {
		return true
	}

	for _, b := range content {
		if b == 0 {
			return true
		}
	}

	return false
}

func detectContentType(filename string, reader io.Reader) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if mime, ok := mimeTypesByExt[ext]; ok {
		return mime
	}

	buf := make([]byte, 512)
	n, _ := reader.Read(buf)
	if n > 0 {
		return http.DetectContentType(buf[:n])
	}

	return "application/octet-stream"
}
