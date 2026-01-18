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

	gitHandler := NewGitHTTPHandler(s.store, s.dataDir)
	s.router.Route("/git", func(r chi.Router) {
		r.Use(AuthMiddleware(s.store))
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