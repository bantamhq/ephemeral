package server

import (
	"fmt"
	"net/http"

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
		r.Use(AuthMiddleware(s.store))

		r.Route("/repos", func(r chi.Router) {
			r.Get("/", s.handleListRepos)
			r.Post("/", s.handleCreateRepo)
			r.Get("/{id}", s.handleGetRepo)
			r.Delete("/{id}", s.handleDeleteRepo)
			r.Patch("/{id}", s.handleUpdateRepo)
		})

		r.Route("/tokens", func(r chi.Router) {
			r.Get("/", s.handleListTokens)
			r.Post("/", s.handleCreateToken)
			r.Delete("/{id}", s.handleDeleteToken)
		})

		r.Route("/namespaces", func(r chi.Router) {
			r.Get("/", s.handleListNamespaces)
			r.Post("/", s.handleCreateNamespace)
			r.Get("/{id}", s.handleGetNamespace)
			r.Delete("/{id}", s.handleDeleteNamespace)
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
	return http.ListenAndServe(addr, s)
}