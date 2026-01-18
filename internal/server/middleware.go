package server

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ephemeral/internal/store"
)

type contextKey string

const tokenContextKey contextKey = "token"

// AuthMiddleware validates token authentication via HTTP Basic Auth.
// Username must be "x-token" and password is the token value.
func AuthMiddleware(st store.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			username, password, ok := r.BasicAuth()
			if !ok {
				w.Header().Set("WWW-Authenticate", `Basic realm="Ephemeral"`)
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			if username != "x-token" {
				http.Error(w, "Invalid credentials", http.StatusUnauthorized)
				return
			}

			hasher := sha256.New()
			hasher.Write([]byte(password))
			tokenHash := fmt.Sprintf("%x", hasher.Sum(nil))

			token, err := st.GetTokenByHash(tokenHash)
			if err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			if token == nil {
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}

			if token.ExpiresAt != nil && token.ExpiresAt.Before(time.Now()) {
				http.Error(w, "Token expired", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), tokenContextKey, token)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetTokenFromContext retrieves the token from the request context.
func GetTokenFromContext(ctx context.Context) *store.Token {
	token, _ := ctx.Value(tokenContextKey).(*store.Token)
	return token
}

// ExtractRepoPath extracts namespace and repo name from URL path.
// Expected format: /git/{namespace}/{repo}.git/...
func ExtractRepoPath(path string) (namespace, repo string, err error) {
	path = strings.TrimPrefix(path, "/git/")

	gitIndex := strings.Index(path, ".git")
	if gitIndex == -1 {
		return "", "", fmt.Errorf("invalid git path: missing .git suffix")
	}

	repoPath := path[:gitIndex]
	parts := strings.SplitN(repoPath, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid git path: expected namespace/repo format")
	}

	return parts[0], parts[1], nil
}