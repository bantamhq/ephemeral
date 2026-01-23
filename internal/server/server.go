package server

import (
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/bantamhq/ephemeral/internal/lfs"
	"github.com/bantamhq/ephemeral/internal/store"
)

// AuthOptions configures platform authentication settings.
type AuthOptions struct {
	WebAuthURL            string
	ExchangeValidationURL string
	ExchangeSecret        string
}

// Server is the HTTP server for Ephemeral.
type Server struct {
	store       store.Store
	dataDir     string
	authOpts    AuthOptions
	lfsOpts     LFSOptions
	router      *chi.Mux
	permissions *store.PermissionChecker
	lfsHandler  *LFSHandler
}

// NewServer creates a new server instance.
func NewServer(st store.Store, dataDir string, authOpts AuthOptions, lfsOpts LFSOptions) *Server {
	s := &Server{
		store:       st,
		dataDir:     dataDir,
		authOpts:    authOpts,
		lfsOpts:     lfsOpts,
		router:      chi.NewRouter(),
		permissions: store.NewPermissionChecker(st),
	}

	if lfsOpts.Enabled {
		lfsPath := filepath.Join(dataDir, "lfs")
		storage := lfs.NewLocalStorage(lfsPath)
		s.lfsHandler = NewLFSHandler(st, storage, lfsOpts.BaseURL, lfsOpts.MaxFileSize)
	}

	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)

	s.router.Get("/health", s.handleHealth)

	s.router.Route("/api/v1", func(r chi.Router) {
		// Auth routes - no auth required
		r.Get("/auth/config", s.handleAuthConfig)
		r.Post("/auth/exchange", s.handleAuthExchange)

		// Admin routes - requires admin token
		r.Route("/admin", func(r chi.Router) {
			r.Use(BearerAuthMiddleware(s.store))

			// Namespaces
			r.Get("/namespaces", s.handleAdminListNamespaces)
			r.Post("/namespaces", s.handleAdminCreateNamespace)
			r.Get("/namespaces/{id}", s.handleAdminGetNamespace)
			r.Delete("/namespaces/{id}", s.handleAdminDeleteNamespace)

			// Tokens
			r.Get("/tokens", s.handleAdminListTokens)
			r.Post("/tokens", s.handleAdminCreateToken)
			r.Get("/tokens/{id}", s.handleAdminGetToken)
			r.Delete("/tokens/{id}", s.handleAdminDeleteToken)

			// Namespace grants
			r.Post("/tokens/{id}/namespace-grants", s.handleAdminCreateNamespaceGrant)
			r.Get("/tokens/{id}/namespace-grants", s.handleAdminListNamespaceGrants)
			r.Delete("/tokens/{id}/namespace-grants/{nsID}", s.handleAdminDeleteNamespaceGrant)

			// Repo grants
			r.Post("/tokens/{id}/repo-grants", s.handleAdminCreateRepoGrant)
			r.Get("/tokens/{id}/repo-grants", s.handleAdminListRepoGrants)
			r.Delete("/tokens/{id}/repo-grants/{repoID}", s.handleAdminDeleteRepoGrant)
		})

		// User routes - requires user token (non-admin)
		r.Group(func(r chi.Router) {
			r.Use(BearerAuthMiddleware(s.store))

			// Current user info
			r.Get("/namespaces", s.handleListNamespaces)
			r.Get("/namespace", s.handleGetCurrentNamespace)

			// Namespace-scoped admin routes (requires namespace:admin)
			r.Patch("/namespaces/{id}", s.handleUpdateNamespace)
			r.Delete("/namespaces/{id}", s.handleDeleteNamespaceScoped)
			r.Get("/namespaces/{id}/grants", s.handleListNamespaceGrants)

			// Repos
			r.Get("/repos", s.handleListRepos)
			r.Post("/repos", s.handleCreateRepo)
			r.Get("/repos/{id}", s.handleGetRepo)
			r.Delete("/repos/{id}", s.handleDeleteRepo)
			r.Patch("/repos/{id}", s.handleUpdateRepo)
			r.Post("/repos/{id}/refs", s.handleCreateRef)
			r.Patch("/repos/{id}/refs/{refType}/*", s.handleUpdateRef)
			r.Delete("/repos/{id}/refs/{refType}/*", s.handleDeleteRef)
			r.Put("/repos/{id}/default-branch", s.handleSetDefaultBranch)

			// Repo folders (M2M)
			r.Get("/repos/{id}/folders", s.handleListRepoFolders)
			r.Post("/repos/{id}/folders", s.handleAddRepoFolders)
			r.Put("/repos/{id}/folders", s.handleSetRepoFolders)
			r.Delete("/repos/{id}/folders/{folderID}", s.handleRemoveRepoFolder)

			// Folders
			r.Get("/folders", s.handleListFolders)
			r.Post("/folders", s.handleCreateFolder)
			r.Get("/folders/{id}", s.handleGetFolder)
			r.Patch("/folders/{id}", s.handleUpdateFolder)
			r.Delete("/folders/{id}", s.handleDeleteFolder)
		})

		// Content API - supports anonymous access for public repos
		r.Group(func(r chi.Router) {
			r.Use(OptionalBearerAuthMiddleware(s.store))
			r.Get("/repos/{id}/readme", s.handleGetReadme)
			r.Get("/repos/{id}/refs", s.handleListRefs)
			r.Get("/repos/{id}/commits", s.handleListCommits)
			r.Get("/repos/{id}/commits/{sha}/diff", s.handleGetCommitDiff)
			r.Get("/repos/{id}/commits/{sha}", s.handleGetCommit)
			r.Get("/repos/{id}/compare/{base}...{head}", s.handleCompareCommits)
			r.Get("/repos/{id}/tree/{ref}/*", s.handleGetTree)
			r.Get("/repos/{id}/blob/{ref}/*", s.handleGetBlob)
			r.Get("/repos/{id}/blame/{ref}/*", s.handleGetBlame)
			r.Get("/repos/{id}/archive/{ref}", s.handleGetArchive)
		})
	})

	gitHandler := NewGitHTTPHandler(s.store, s.dataDir)
	s.router.Route("/git", func(r chi.Router) {
		r.Use(OptionalAuthMiddleware(s.store))

		if s.lfsHandler != nil {
			r.Route("/{namespace}/{repo}.git/info/lfs", func(r chi.Router) {
				r.Mount("/", s.lfsHandler.Routes())
			})
		}

		r.HandleFunc("/*", gitHandler.ServeHTTP)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// Start starts the HTTP server on the given host and port.
func (s *Server) Start(host string, port int) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	fmt.Printf("Starting server on %s\n", addr)

	server := &http.Server{
		Addr:              addr,
		Handler:           s,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	return server.ListenAndServe()
}
