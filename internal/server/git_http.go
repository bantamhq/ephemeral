package server

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/google/uuid"

	"ephemeral/internal/store"
)

// GitHTTPHandler handles Git HTTP smart protocol requests.
type GitHTTPHandler struct {
	store   store.Store
	dataDir string
}

// NewGitHTTPHandler creates a new Git HTTP handler.
func NewGitHTTPHandler(st store.Store, dataDir string) *GitHTTPHandler {
	return &GitHTTPHandler{
		store:   st,
		dataDir: dataDir,
	}
}

func (h *GitHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	namespace, repoName, err := ExtractRepoPath(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	token := GetTokenFromContext(r.Context())
	if token == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	if token.NamespaceID != namespace {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	switch {
	case strings.HasSuffix(r.URL.Path, "/info/refs"):
		h.handleInfoRefs(w, r, namespace, repoName, token)
	case strings.HasSuffix(r.URL.Path, "/git-upload-pack"):
		h.handleUploadPack(w, r, namespace, repoName, token)
	case strings.HasSuffix(r.URL.Path, "/git-receive-pack"):
		h.handleReceivePack(w, r, namespace, repoName, token)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

func (h *GitHTTPHandler) handleInfoRefs(w http.ResponseWriter, r *http.Request, namespace, repoName string, token *store.Token) {
	service := r.URL.Query().Get("service")
	isWrite := service == "git-receive-pack"

	if isWrite && token.Scope == "read-only" {
		http.Error(w, "Write access denied", http.StatusForbidden)
		return
	}

	repo, err := h.getOrCreateRepo(namespace, repoName, isWrite)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get repository: %v", err), http.StatusInternalServerError)
		return
	}
	if repo == nil {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	repoPath := h.getRepoPath(namespace, repoName)

	var cmd *exec.Cmd
	switch service {
	case "git-upload-pack":
		cmd = exec.Command("git-upload-pack", "--stateless-rpc", "--advertise-refs", repoPath)
	case "git-receive-pack":
		cmd = exec.Command("git-receive-pack", "--stateless-rpc", "--advertise-refs", repoPath)
	default:
		http.Error(w, "Invalid service", http.StatusBadRequest)
		return
	}

	output, err := cmd.Output()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get refs: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", fmt.Sprintf("application/x-%s-advertisement", service))
	w.Header().Set("Cache-Control", "no-cache")

	serviceLine := fmt.Sprintf("# service=%s\n", service)
	fmt.Fprintf(w, "%04x%s", len(serviceLine)+4, serviceLine)
	w.Write([]byte("0000"))
	w.Write(output)
}

func (h *GitHTTPHandler) handleUploadPack(w http.ResponseWriter, r *http.Request, namespace, repoName string, token *store.Token) {
	repo, err := h.store.GetRepo(namespace, repoName)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if repo == nil {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	repoPath := h.getRepoPath(namespace, repoName)

	w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
	w.Header().Set("Cache-Control", "no-cache")

	cmd := exec.Command("git-upload-pack", "--stateless-rpc", repoPath)
	cmd.Stdin = r.Body
	cmd.Stdout = w

	if err := cmd.Run(); err != nil {
		// Client disconnect is common and not an error worth logging loudly
		fmt.Printf("git-upload-pack error: %v\n", err)
	}
}

func (h *GitHTTPHandler) handleReceivePack(w http.ResponseWriter, r *http.Request, namespace, repoName string, token *store.Token) {
	if token.Scope == "read-only" {
		http.Error(w, "Write access denied", http.StatusForbidden)
		return
	}

	repo, err := h.getOrCreateRepo(namespace, repoName, true)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get repository: %v", err), http.StatusInternalServerError)
		return
	}

	repoPath := h.getRepoPath(namespace, repoName)

	bodyReader := h.getRequestBody(r)
	if closer, ok := bodyReader.(io.Closer); ok && bodyReader != r.Body {
		defer closer.Close()
	}

	w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
	w.Header().Set("Cache-Control", "no-cache")

	cmd := exec.Command("git-receive-pack", "--stateless-rpc", repoPath)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := cmd.Start(); err != nil {
		http.Error(w, "Failed to start git-receive-pack", http.StatusInternalServerError)
		return
	}

	go func() {
		io.Copy(stdin, bodyReader)
		stdin.Close()
	}()

	io.Copy(w, stdout)

	if err := cmd.Wait(); err != nil {
		// Non-zero exit may be normal for rejected pushes
		fmt.Printf("git-receive-pack error: %v\n", err)
	}

	h.store.UpdateRepoLastPush(repo.ID, time.Now())
}

func (h *GitHTTPHandler) getRequestBody(r *http.Request) io.Reader {
	if r.Header.Get("Content-Encoding") != "gzip" {
		return r.Body
	}

	gzipReader, err := gzip.NewReader(r.Body)
	if err != nil {
		return r.Body
	}
	return gzipReader
}

// getOrCreateRepo gets a repo, optionally creating it if autoCreate is true.
func (h *GitHTTPHandler) getOrCreateRepo(namespace, repoName string, autoCreate bool) (*store.Repo, error) {
	repo, err := h.store.GetRepo(namespace, repoName)
	if err != nil {
		return nil, fmt.Errorf("get repo: %w", err)
	}

	if repo != nil || !autoCreate {
		return repo, nil
	}

	return h.createRepo(namespace, repoName)
}

func (h *GitHTTPHandler) createRepo(namespace, repoName string) (*store.Repo, error) {
	now := time.Now()
	repo := &store.Repo{
		ID:          uuid.New().String(),
		NamespaceID: namespace,
		Name:        repoName,
		Public:      false,
		SizeBytes:   0,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := h.store.CreateRepo(repo); err != nil {
		return nil, fmt.Errorf("save repo: %w", err)
	}

	repoPath := h.getRepoPath(namespace, repoName)
	if err := os.MkdirAll(filepath.Dir(repoPath), 0755); err != nil {
		return nil, fmt.Errorf("create repo directory: %w", err)
	}

	if _, err := git.PlainInit(repoPath, true); err != nil {
		return nil, fmt.Errorf("init bare repo: %w", err)
	}

	fmt.Printf("Created new repository: %s/%s\n", namespace, repoName)
	return repo, nil
}

func (h *GitHTTPHandler) getRepoPath(namespace, repoName string) string {
	return filepath.Join(h.dataDir, "repos", namespace, repoName+".git")
}