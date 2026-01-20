package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ephemeral/internal/core"
	"ephemeral/internal/store"
)

type contextKey string

const tokenContextKey contextKey = "token"

// authError represents an authentication error with an associated HTTP status code.
type authError struct {
	message string
	status  int
}

func (e *authError) Error() string {
	return e.message
}

// writeAuthError writes an authentication error response with appropriate headers.
func writeAuthError(w http.ResponseWriter, err error, realm string) {
	if authErr, ok := err.(*authError); ok {
		if authErr.status == http.StatusUnauthorized {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`%s realm="Ephemeral"`, realm))
		}
		http.Error(w, authErr.message, authErr.status)
		return
	}
	http.Error(w, "Internal server error", http.StatusInternalServerError)
}

// AuthMiddleware validates token authentication via HTTP Basic Auth.
// Username must be "x-token" and password is the token value.
func AuthMiddleware(st store.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _, ok := r.BasicAuth()
			if !ok {
				w.Header().Set("WWW-Authenticate", `Basic realm="Ephemeral"`)
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			token, err := validateBasicAuth(st, r)
			if err != nil {
				writeAuthError(w, err, "Basic")
				return
			}

			ctx := context.WithValue(r.Context(), tokenContextKey, token)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// BearerAuthMiddleware validates token authentication via Bearer token header.
func BearerAuthMiddleware(st store.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, err := validateBearerToken(st, r)
			if err != nil {
				writeAuthError(w, err, "Bearer")
				return
			}

			if token == nil {
				w.Header().Set("WWW-Authenticate", `Bearer realm="Ephemeral"`)
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), tokenContextKey, token)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// lookupToken parses the token, looks up by namespace and lookup key, and verifies.
func lookupToken(st store.Store, rawToken string) (*store.Token, error) {
	namespaceID, lookup, _, err := core.ParseToken(rawToken)
	if err != nil {
		return nil, &authError{"Invalid token format", http.StatusUnauthorized}
	}

	token, err := st.GetTokenByLookup(namespaceID, lookup)
	if err != nil {
		return nil, &authError{"Internal server error", http.StatusInternalServerError}
	}
	if token == nil {
		return nil, &authError{"Invalid token", http.StatusUnauthorized}
	}

	if err := core.VerifyToken(rawToken, token.TokenHash); err != nil {
		return nil, &authError{"Invalid token", http.StatusUnauthorized}
	}

	if token.ExpiresAt != nil && token.ExpiresAt.Before(time.Now()) {
		return nil, &authError{"Token expired", http.StatusUnauthorized}
	}

	return token, nil
}

// validateBearerToken extracts and validates a token from the Bearer Auth header.
func validateBearerToken(st store.Store, r *http.Request) (*store.Token, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, nil
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, &authError{"Invalid authorization scheme, Bearer required", http.StatusUnauthorized}
	}

	rawToken := strings.TrimPrefix(authHeader, "Bearer ")
	return lookupToken(st, rawToken)
}

// validateBasicAuth extracts and validates a token from HTTP Basic Auth.
func validateBasicAuth(st store.Store, r *http.Request) (*store.Token, error) {
	username, password, _ := r.BasicAuth()

	if username != "x-token" {
		return nil, &authError{"Invalid credentials", http.StatusUnauthorized}
	}

	return lookupToken(st, password)
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

	slashIdx := strings.Index(path, "/")
	if slashIdx == -1 {
		return "", "", fmt.Errorf("invalid git path: expected namespace/repo format")
	}

	namespace = path[:slashIdx]
	repoPath := path[slashIdx+1:]

	parts := strings.SplitN(repoPath, "/", 2)
	repoSegment := parts[0]
	if !strings.HasSuffix(repoSegment, ".git") {
		return "", "", fmt.Errorf("invalid git path: missing .git suffix")
	}

	repo = strings.TrimSuffix(repoSegment, ".git")
	if repo == "" {
		return "", "", fmt.Errorf("invalid git path: empty repo name")
	}

	return namespace, repo, nil
}

// OptionalBearerAuthMiddleware sets token context if a valid Bearer token is provided.
// Continues without authentication if no token is present.
func OptionalBearerAuthMiddleware(st store.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, err := validateBearerToken(st, r)
			if err != nil {
				writeAuthError(w, err, "Bearer")
				return
			}

			if token != nil {
				ctx := context.WithValue(r.Context(), tokenContextKey, token)
				r = r.WithContext(ctx)
			}

			next.ServeHTTP(w, r)
		})
	}
}

// OptionalAuthMiddleware sets token context if valid Basic Auth credentials are provided.
// Continues without authentication if no credentials are present.
func OptionalAuthMiddleware(st store.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _, ok := r.BasicAuth()
			if !ok {
				next.ServeHTTP(w, r)
				return
			}

			token, err := validateBasicAuth(st, r)
			if err != nil {
				writeAuthError(w, err, "Basic")
				return
			}

			ctx := context.WithValue(r.Context(), tokenContextKey, token)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// scopeLevel returns the privilege level for a scope (higher = more privileges).
func scopeLevel(scope string) int {
	switch scope {
	case store.ScopeReadOnly:
		return 1
	case store.ScopeRepos:
		return 2
	case store.ScopeFull:
		return 3
	case store.ScopeAdmin:
		return 4
	default:
		return 0
	}
}

// HasScope checks if token has at least the required scope level.
func HasScope(token *store.Token, required string) bool {
	if token == nil {
		return false
	}
	return scopeLevel(token.Scope) >= scopeLevel(required)
}
