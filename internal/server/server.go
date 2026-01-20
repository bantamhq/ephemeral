package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"ephemeral/internal/store"
)

// Server is the HTTP server for Ephemeral.
type Server struct {
	store   store.Store
	dataDir string
	router  *chi.Mux
}

// NewServer creates a new server instance.
func NewServer(st store.Store, dataDir string) *Server {
	s := &Server{
		store:   st,
		dataDir: dataDir,
		router:  chi.NewRouter(),
	}
	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)

	s.router.Get("/health", s.handleHealth)

	s.router.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(BearerAuthMiddleware(s.store))

			// Repos
			r.Get("/repos", s.handleListRepos)
			r.Post("/repos", s.handleCreateRepo)
			r.Get("/repos/{id}", s.handleGetRepo)
			r.Delete("/repos/{id}", s.handleDeleteRepo)
			r.Patch("/repos/{id}", s.handleUpdateRepo)

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
			r.Get("/repos/{id}/refs", s.handleListRefs)
			r.Get("/repos/{id}/commits", s.handleListCommits)
			r.Get("/repos/{id}/tree/{ref}/*", s.handleGetTree)
			r.Get("/repos/{id}/blob/{ref}/*", s.handleGetBlob)
		})

		// Tokens - requires auth
		r.Group(func(r chi.Router) {
			r.Use(BearerAuthMiddleware(s.store))
			r.Get("/tokens", s.handleListTokens)
			r.Post("/tokens", s.handleCreateToken)
			r.Delete("/tokens/{id}", s.handleDeleteToken)
		})

		// Current namespace - requires auth (any scope)
		r.Group(func(r chi.Router) {
			r.Use(BearerAuthMiddleware(s.store))
			r.Get("/namespace", s.handleGetCurrentNamespace)
		})

		// Namespaces admin - requires admin scope
		r.Group(func(r chi.Router) {
			r.Use(BearerAuthMiddleware(s.store))
			r.Get("/namespaces", s.handleListNamespaces)
			r.Post("/namespaces", s.handleCreateNamespace)
			r.Get("/namespaces/{id}", s.handleGetNamespace)
			r.Delete("/namespaces/{id}", s.handleDeleteNamespace)
		})
	})

	gitHandler := NewGitHTTPHandler(s.store, s.dataDir)
	s.router.Route("/git", func(r chi.Router) {
		r.Use(OptionalAuthMiddleware(s.store))
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