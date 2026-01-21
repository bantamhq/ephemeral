package server

import (
	"compress/gzip"
	"context"
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

const gitCommandTimeout = 5 * time.Minute

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
	namespaceName, repoName, err := ExtractRepoPath(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := ValidateName(repoName); err != nil {
		http.Error(w, fmt.Sprintf("Invalid repository name: %v", err), http.StatusBadRequest)
		return
	}

	ns, err := h.store.GetNamespaceByName(namespaceName)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if ns == nil {
		http.Error(w, "Namespace not found", http.StatusNotFound)
		return
	}

	token := GetTokenFromContext(r.Context())
	isWrite := h.isWriteOperation(r)

	if isWrite {
		if token == nil {
			w.Header().Set("WWW-Authenticate", `Basic realm="Ephemeral"`)
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}
		if token.NamespaceID != ns.ID {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}
	} else {
		if token != nil && token.NamespaceID != ns.ID {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}

		if token == nil {
			repo, err := h.store.GetRepo(ns.ID, repoName)
			if err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			if repo == nil || !repo.Public {
				w.Header().Set("WWW-Authenticate", `Basic realm="Ephemeral"`)
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}
		}
	}

	switch {
	case strings.HasSuffix(r.URL.Path, "/info/refs"):
		h.handleInfoRefs(w, r, ns.ID, repoName, token)
	case strings.HasSuffix(r.URL.Path, "/git-upload-pack"):
		h.handleUploadPack(w, r, ns.ID, repoName)
	case strings.HasSuffix(r.URL.Path, "/git-receive-pack"):
		h.handleReceivePack(w, r, ns.ID, repoName, token)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

func (h *GitHTTPHandler) isWriteOperation(r *http.Request) bool {
	if strings.HasSuffix(r.URL.Path, "/git-receive-pack") {
		return true
	}
	if strings.HasSuffix(r.URL.Path, "/info/refs") {
		return r.URL.Query().Get("service") == "git-receive-pack"
	}
	return false
}

func (h *GitHTTPHandler) handleInfoRefs(w http.ResponseWriter, r *http.Request, namespaceID, repoName string, token *store.Token) {
	service := r.URL.Query().Get("service")
	isWrite := service == "git-receive-pack"

	if isWrite && token != nil && token.Scope == store.ScopeReadOnly {
		http.Error(w, "Write access denied", http.StatusForbidden)
		return
	}

	repo, err := h.getOrCreateRepo(namespaceID, repoName, isWrite)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get repository: %v", err), http.StatusInternalServerError)
		return
	}
	if repo == nil {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	repoPath, err := h.getRepoPath(namespaceID, repoName)
	if err != nil {
		http.Error(w, "Failed to resolve repository path", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), gitCommandTimeout)
	defer cancel()

	var cmd *exec.Cmd
	switch service {
	case "git-upload-pack":
		cmd = exec.CommandContext(ctx, "git-upload-pack", "--stateless-rpc", "--advertise-refs", repoPath)
	case "git-receive-pack":
		cmd = exec.CommandContext(ctx, "git-receive-pack", "--stateless-rpc", "--advertise-refs", repoPath)
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

func (h *GitHTTPHandler) handleUploadPack(w http.ResponseWriter, r *http.Request, namespaceID, repoName string) {
	repo, err := h.store.GetRepo(namespaceID, repoName)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if repo == nil {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	repoPath, err := h.getRepoPath(namespaceID, repoName)
	if err != nil {
		http.Error(w, "Failed to resolve repository path", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), gitCommandTimeout)
	defer cancel()

	w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
	w.Header().Set("Cache-Control", "no-cache")

	cmd := exec.CommandContext(ctx, "git-upload-pack", "--stateless-rpc", repoPath)
	cmd.Stdin = r.Body
	cmd.Stdout = w

	if err := cmd.Run(); err != nil {
		fmt.Printf("git-upload-pack error: %v\n", err)
	}
}

func (h *GitHTTPHandler) handleReceivePack(w http.ResponseWriter, r *http.Request, namespaceID, repoName string, token *store.Token) {
	if token.Scope == store.ScopeReadOnly {
		http.Error(w, "Write access denied", http.StatusForbidden)
		return
	}

	repo, err := h.getOrCreateRepo(namespaceID, repoName, true)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get repository: %v", err), http.StatusInternalServerError)
		return
	}

	repoPath, err := h.getRepoPath(namespaceID, repoName)
	if err != nil {
		http.Error(w, "Failed to resolve repository path", http.StatusInternalServerError)
		return
	}

	bodyReader, err := h.getRequestBody(w, r)
	if err != nil {
		return
	}
	if closer, ok := bodyReader.(io.Closer); ok && bodyReader != r.Body {
		defer closer.Close()
	}

	ctx, cancel := context.WithTimeout(r.Context(), gitCommandTimeout)
	defer cancel()

	w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
	w.Header().Set("Cache-Control", "no-cache")

	cmd := exec.CommandContext(ctx, "git-receive-pack", "--stateless-rpc", repoPath)

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
		fmt.Printf("git-receive-pack error: %v\n", err)
	}

	if err := h.store.UpdateRepoLastPush(repo.ID, time.Now()); err != nil {
		fmt.Printf("update repo last_push_at error: %v\n", err)
	}

	sizeBytes, err := repoDiskUsage(repoPath)
	if err != nil {
		fmt.Printf("compute repo size error: %v\n", err)
		return
	}

	if err := h.store.UpdateRepoSize(repo.ID, sizeBytes); err != nil {
		fmt.Printf("update repo size_bytes error: %v\n", err)
	}
}

func (h *GitHTTPHandler) getRequestBody(w http.ResponseWriter, r *http.Request) (io.Reader, error) {
	if r.Header.Get("Content-Encoding") != "gzip" {
		return r.Body, nil
	}

	gzipReader, err := gzip.NewReader(r.Body)
	if err != nil {
		http.Error(w, "Invalid gzip body", http.StatusBadRequest)
		return nil, err
	}
	return gzipReader, nil
}

// getOrCreateRepo gets a repo, optionally creating it if autoCreate is true.
func (h *GitHTTPHandler) getOrCreateRepo(namespaceID, repoName string, autoCreate bool) (*store.Repo, error) {
	repoName = strings.ToLower(repoName)
	repo, err := h.store.GetRepo(namespaceID, repoName)
	if err != nil {
		return nil, fmt.Errorf("get repo: %w", err)
	}

	if repo != nil || !autoCreate {
		return repo, nil
	}

	return h.createRepo(namespaceID, repoName)
}

func (h *GitHTTPHandler) createRepo(namespaceID, repoName string) (*store.Repo, error) {
	if err := ValidateName(repoName); err != nil {
		return nil, fmt.Errorf("invalid repo name: %w", err)
	}
	repoName = strings.ToLower(repoName)

	repoPath, err := h.getRepoPath(namespaceID, repoName)
	if err != nil {
		return nil, fmt.Errorf("resolve repo path: %w", err)
	}

	now := time.Now()
	repo := &store.Repo{
		ID:          uuid.New().String(),
		NamespaceID: namespaceID,
		Name:        repoName,
		Public:      false,
		SizeBytes:   0,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := h.store.CreateRepo(repo); err != nil {
		return nil, fmt.Errorf("save repo: %w", err)
	}

	if err := initBareRepo(repoPath); err != nil {
		h.store.DeleteRepo(repo.ID)
		return nil, fmt.Errorf("init bare repo: %w", err)
	}

	fmt.Printf("Created new repository: %s/%s\n", namespaceID, repoName)
	return repo, nil
}

func (h *GitHTTPHandler) getRepoPath(namespaceID, repoName string) (string, error) {
	return SafeRepoPath(h.dataDir, namespaceID, repoName)
}

// initBareRepo creates and initializes a bare git repository with main as the default branch.
func initBareRepo(repoPath string) error {
	if err := os.MkdirAll(filepath.Dir(repoPath), 0755); err != nil {
		return fmt.Errorf("create repo directory: %w", err)
	}

	if _, err := git.PlainInit(repoPath, true); err != nil {
		return fmt.Errorf("init bare repo: %w", err)
	}

	headPath := filepath.Join(repoPath, "HEAD")
	if err := os.WriteFile(headPath, []byte("ref: refs/heads/main\n"), 0644); err != nil {
		return fmt.Errorf("set default branch: %w", err)
	}

	return nil
}

func repoDiskUsage(path string) (int, error) {
	var size int64

	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		size += info.Size()
		return nil
	})
	if err != nil {
		return 0, err
	}

	return int(size), nil
}
