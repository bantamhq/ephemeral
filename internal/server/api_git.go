package server

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type DiffResponse struct {
	BaseSHA string      `json:"base_sha,omitempty"`
	HeadSHA string      `json:"head_sha"`
	Stats   CommitStats `json:"stats"`
	Patch   string      `json:"patch"`
}

type CompareResponse struct {
	BaseRef      string           `json:"base_ref"`
	HeadRef      string           `json:"head_ref"`
	BaseSHA      string           `json:"base_sha"`
	HeadSHA      string           `json:"head_sha"`
	MergeBaseSHA string           `json:"merge_base_sha"`
	AheadBy      int              `json:"ahead_by"`
	BehindBy     int              `json:"behind_by"`
	Commits      []CommitResponse `json:"commits"`
	NextCursor   *string          `json:"next_cursor,omitempty"`
	HasMore      bool             `json:"has_more"`
	Diff         DiffResponse     `json:"diff"`
}

type BlameLineResponse struct {
	Line   int       `json:"line"`
	SHA    string    `json:"sha"`
	Author GitAuthor `json:"author"`
	Text   string    `json:"text"`
}

type BlameResponse struct {
	Path  string              `json:"path"`
	Ref   string              `json:"ref"`
	Lines []BlameLineResponse `json:"lines"`
}

type archiveFormat struct {
	Name        string
	ContentType string
	Extension   string
	GitFormat   string
	Gzip        bool
}

func (s *Server) handleGetCommit(w http.ResponseWriter, r *http.Request) {
	repo, _, ok := s.checkRepoAccess(w, r)
	if !ok {
		return
	}

	gitRepo, ok := s.openGitRepoForRepo(w, repo)
	if !ok {
		return
	}

	refStr := chi.URLParam(r, "sha")
	commit, _, ok := s.loadCommitFromRef(w, gitRepo, refStr)
	if !ok {
		return
	}

	JSON(w, http.StatusOK, commitToResponse(commit))
}

func (s *Server) handleGetCommitDiff(w http.ResponseWriter, r *http.Request) {
	repo, _, ok := s.checkRepoAccess(w, r)
	if !ok {
		return
	}

	gitRepo, ok := s.openGitRepoForRepo(w, repo)
	if !ok {
		return
	}

	refStr := chi.URLParam(r, "sha")
	commit, _, ok := s.loadCommitFromRef(w, gitRepo, refStr)
	if !ok {
		return
	}

	var baseSHA string
	var baseTree *object.Tree

	if commit.NumParents() > 0 {
		parent, err := commit.Parent(0)
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to get parent commit")
			return
		}
		baseSHA = parent.Hash.String()
		baseTree, err = parent.Tree()
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to get parent tree")
			return
		}
	}

	headTree, err := commit.Tree()
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get commit tree")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), gitCommandTimeout)
	defer cancel()

	resp, err := buildDiffResponse(ctx, baseSHA, baseTree, commit.Hash.String(), headTree)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to compute diff")
		return
	}

	JSON(w, http.StatusOK, resp)
}

func (s *Server) handleCompareCommits(w http.ResponseWriter, r *http.Request) {
	repo, _, ok := s.checkRepoAccess(w, r)
	if !ok {
		return
	}

	gitRepo, ok := s.openGitRepoForRepo(w, repo)
	if !ok {
		return
	}

	baseRef, err := decodeRefParam(chi.URLParam(r, "base"))
	if err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid base ref")
		return
	}
	headRef, err := decodeRefParam(chi.URLParam(r, "head"))
	if err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid head ref")
		return
	}
	if baseRef == "" || headRef == "" {
		JSONError(w, http.StatusBadRequest, "Base and head refs are required")
		return
	}

	baseHash, ok := s.resolveRefForRepo(w, gitRepo, baseRef)
	if !ok {
		return
	}

	headHash, ok := s.resolveRefForRepo(w, gitRepo, headRef)
	if !ok {
		return
	}

	repoPath, err := SafeRepoPath(s.dataDir, repo.NamespaceID, repo.Name)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to resolve repo path")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), gitCommandTimeout)
	defer cancel()

	mergeBaseSHA, err := gitMergeBase(ctx, repoPath, baseHash.String(), headHash.String())
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to compute merge base")
		return
	}

	behindBy, aheadBy, err := gitAheadBehind(ctx, repoPath, baseHash.String(), headHash.String())
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to compute ahead/behind")
		return
	}

	cursor := r.URL.Query().Get("cursor")
	limit := parseLimit(r.URL.Query().Get("limit"), defaultPageSize)

	commitSHAs, nextCursor, hasMore, err := gitRevList(ctx, repoPath, baseHash.String(), headHash.String(), cursor, limit)
	if err != nil {
		JSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	commits := make([]CommitResponse, 0, len(commitSHAs))
	for _, sha := range commitSHAs {
		commit, err := gitRepo.CommitObject(plumbing.NewHash(sha))
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to load commit")
			return
		}
		commits = append(commits, commitToResponse(commit))
	}

	baseCommit, err := gitRepo.CommitObject(*baseHash)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get base commit")
		return
	}
	headCommit, err := gitRepo.CommitObject(*headHash)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get head commit")
		return
	}

	baseTree, err := baseCommit.Tree()
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get base tree")
		return
	}
	headTree, err := headCommit.Tree()
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to get head tree")
		return
	}

	diffResp, err := buildDiffResponse(ctx, baseHash.String(), baseTree, headHash.String(), headTree)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to compute diff")
		return
	}

	resp := CompareResponse{
		BaseRef:      baseRef,
		HeadRef:      headRef,
		BaseSHA:      baseHash.String(),
		HeadSHA:      headHash.String(),
		MergeBaseSHA: mergeBaseSHA,
		AheadBy:      aheadBy,
		BehindBy:     behindBy,
		Commits:      commits,
		NextCursor:   nextCursor,
		HasMore:      hasMore,
		Diff:         diffResp,
	}

	JSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetBlame(w http.ResponseWriter, r *http.Request) {
	repo, _, ok := s.checkRepoAccess(w, r)
	if !ok {
		return
	}

	gitRepo, ok := s.openGitRepoForRepo(w, repo)
	if !ok {
		return
	}

	refStr := chi.URLParam(r, "ref")
	path := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	if path == "" {
		JSONError(w, http.StatusBadRequest, "Path is required")
		return
	}

	commit, _, ok := s.loadCommitFromRef(w, gitRepo, refStr)
	if !ok {
		return
	}

	blame, err := git.Blame(commit, path)
	if err != nil {
		JSONError(w, http.StatusNotFound, "Path not found")
		return
	}

	lines := make([]BlameLineResponse, len(blame.Lines))
	for i, line := range blame.Lines {
		lines[i] = BlameLineResponse{
			Line: i + 1,
			SHA:  line.Hash.String(),
			Author: GitAuthor{
				Name:  line.AuthorName,
				Email: line.Author,
				Date:  line.Date,
			},
			Text: line.Text,
		}
	}

	JSON(w, http.StatusOK, BlameResponse{
		Path:  path,
		Ref:   refStr,
		Lines: lines,
	})
}

func (s *Server) handleGetArchive(w http.ResponseWriter, r *http.Request) {
	repo, _, ok := s.checkRepoAccess(w, r)
	if !ok {
		return
	}

	gitRepo, ok := s.openGitRepoForRepo(w, repo)
	if !ok {
		return
	}

	refStr := chi.URLParam(r, "ref")
	hash, ok := s.resolveRefForRepo(w, gitRepo, refStr)
	if !ok {
		return
	}

	format, err := parseArchiveFormat(r.URL.Query().Get("format"))
	if err != nil {
		JSONError(w, http.StatusBadRequest, "Invalid archive format")
		return
	}

	path := strings.TrimPrefix(r.URL.Query().Get("path"), "/")
	if strings.Contains(path, "..") {
		JSONError(w, http.StatusBadRequest, "Invalid path")
		return
	}

	if path != "" {
		commit, err := gitRepo.CommitObject(*hash)
		if err != nil {
			JSONError(w, http.StatusNotFound, "Commit not found")
			return
		}
		tree, err := commit.Tree()
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "Failed to get tree")
			return
		}
		if _, err := tree.FindEntry(path); err != nil {
			JSONError(w, http.StatusNotFound, "Path not found")
			return
		}
	}

	repoPath, err := SafeRepoPath(s.dataDir, repo.NamespaceID, repo.Name)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to resolve repo path")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), gitCommandTimeout)
	defer cancel()

	filename := archiveFilename(repo.Name, refStr, format.Extension)
	w.Header().Set("Content-Type", format.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	args := []string{"-C", repoPath, "archive", "--format=" + format.GitFormat, hash.String()}
	if path != "" {
		args = append(args, path)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to build archive")
		return
	}

	if err := cmd.Start(); err != nil {
		JSONError(w, http.StatusInternalServerError, "Failed to build archive")
		return
	}

	if format.Gzip {
		gzipWriter := gzip.NewWriter(w)
		if _, err := io.Copy(gzipWriter, stdout); err != nil {
			gzipWriter.Close()
			cmd.Wait()
			return
		}
		if err := gzipWriter.Close(); err != nil {
			cmd.Wait()
			return
		}
	} else {
		if _, err := io.Copy(w, stdout); err != nil {
			cmd.Wait()
			return
		}
	}

	if err := cmd.Wait(); err != nil {
		fmt.Printf("git archive error: %v\n", err)
	}
}

func commitToResponse(commit *object.Commit) CommitResponse {
	parentSHAs := make([]string, len(commit.ParentHashes))
	for i, parent := range commit.ParentHashes {
		parentSHAs[i] = parent.String()
	}

	return CommitResponse{
		SHA:     commit.Hash.String(),
		Message: commit.Message,
		Author: GitAuthor{
			Name:  commit.Author.Name,
			Email: commit.Author.Email,
			Date:  commit.Author.When,
		},
		Committer: GitAuthor{
			Name:  commit.Committer.Name,
			Email: commit.Committer.Email,
			Date:  commit.Committer.When,
		},
		ParentSHAs: parentSHAs,
		TreeSHA:    commit.TreeHash.String(),
		Stats:      computeCommitStats(commit),
	}
}

func buildPatch(ctx context.Context, baseTree, headTree *object.Tree) (*object.Patch, *CommitStats, error) {
	changes, err := object.DiffTreeWithOptions(ctx, baseTree, headTree, object.DefaultDiffTreeOptions)
	if err != nil {
		return nil, nil, err
	}

	patch, err := changes.PatchContext(ctx)
	if err != nil {
		return nil, nil, err
	}

	stats := patch.Stats()
	totals := CommitStats{FilesChanged: len(stats)}
	for _, stat := range stats {
		totals.Additions += stat.Addition
		totals.Deletions += stat.Deletion
	}

	return patch, &totals, nil
}

func buildDiffResponse(ctx context.Context, baseSHA string, baseTree *object.Tree, headSHA string, headTree *object.Tree) (DiffResponse, error) {
	patch, stats, err := buildPatch(ctx, baseTree, headTree)
	if err != nil {
		return DiffResponse{}, err
	}

	return DiffResponse{
		BaseSHA: baseSHA,
		HeadSHA: headSHA,
		Stats:   *stats,
		Patch:   patch.String(),
	}, nil
}

func gitCommandOutput(ctx context.Context, repoPath string, args ...string) ([]byte, error) {
	cmdArgs := append([]string{"-C", repoPath}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(output)))
	}

	return output, nil
}

func gitMergeBase(ctx context.Context, repoPath, base, head string) (string, error) {
	output, err := gitCommandOutput(ctx, repoPath, "merge-base", base, head)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

func gitAheadBehind(ctx context.Context, repoPath, base, head string) (int, int, error) {
	output, err := gitCommandOutput(ctx, repoPath, "rev-list", "--left-right", "--count", base+"..."+head)
	if err != nil {
		return 0, 0, err
	}

	parts := strings.Fields(string(output))
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("unexpected rev-list output")
	}

	behindBy, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}

	aheadBy, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, err
	}

	return behindBy, aheadBy, nil
}

func gitRevList(ctx context.Context, repoPath, base, head, cursor string, limit int) ([]string, *string, bool, error) {
	output, err := gitCommandOutput(ctx, repoPath, "rev-list", base+".."+head)
	if err != nil {
		return nil, nil, false, err
	}

	list := strings.Fields(string(output))
	start := 0
	if cursor != "" {
		index := -1
		for i, sha := range list {
			if sha == cursor {
				index = i
				break
			}
		}
		if index == -1 {
			return nil, nil, false, fmt.Errorf("Invalid cursor: commit not found")
		}
		start = index + 1
	}

	if start >= len(list) {
		return []string{}, nil, false, nil
	}

	end := start + limit
	if end > len(list) {
		end = len(list)
	}

	hasMore := end < len(list)
	var nextCursor *string
	if hasMore {
		c := list[end]
		nextCursor = &c
	}

	return list[start:end], nextCursor, hasMore, nil
}

func parseArchiveFormat(value string) (archiveFormat, error) {
	if strings.TrimSpace(value) == "" {
		return archiveFormat{
			Name:        "zip",
			ContentType: "application/zip",
			Extension:   "zip",
			GitFormat:   "zip",
		}, nil
	}

	switch strings.ToLower(value) {
	case "zip":
		return archiveFormat{
			Name:        "zip",
			ContentType: "application/zip",
			Extension:   "zip",
			GitFormat:   "zip",
		}, nil
	case "tar.gz", "tgz":
		return archiveFormat{
			Name:        "tar.gz",
			ContentType: "application/gzip",
			Extension:   "tar.gz",
			GitFormat:   "tar",
			Gzip:        true,
		}, nil
	default:
		return archiveFormat{}, errors.New("invalid format")
	}
}

func archiveFilename(repoName, ref, extension string) string {
	cleanRef := strings.ReplaceAll(ref, "/", "-")
	cleanRef = strings.TrimSpace(cleanRef)
	if cleanRef == "" {
		cleanRef = "archive"
	}
	return fmt.Sprintf("%s-%s.%s", repoName, cleanRef, extension)
}

func decodeRefParam(value string) (string, error) {
	if value == "" {
		return "", nil
	}

	decoded, err := url.PathUnescape(value)
	if err != nil {
		return "", err
	}

	return decoded, nil
}
