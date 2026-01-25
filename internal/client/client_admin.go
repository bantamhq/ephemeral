package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// NamespaceGrantResponse represents a namespace grant in API responses.
type NamespaceGrantResponse struct {
	NamespaceID string   `json:"namespace_id"`
	Allow       []string `json:"allow"`
	Deny        []string `json:"deny,omitempty"`
	IsPrimary   bool     `json:"is_primary"`
}

// RepoGrantResponse represents a repo grant in API responses.
type RepoGrantResponse struct {
	RepoID string   `json:"repo_id"`
	Allow  []string `json:"allow"`
	Deny   []string `json:"deny,omitempty"`
}

// TokenListItem represents a token in list responses (without the secret).
type TokenListItem struct {
	ID              string                   `json:"id"`
	Name            *string                  `json:"name,omitempty"`
	IsAdmin         bool                     `json:"is_admin"`
	CreatedAt       time.Time                `json:"created_at"`
	ExpiresAt       *time.Time               `json:"expires_at,omitempty"`
	LastUsedAt      *time.Time               `json:"last_used_at,omitempty"`
	NamespaceGrants []NamespaceGrantResponse `json:"namespace_grants,omitempty"`
	RepoGrants      []RepoGrantResponse      `json:"repo_grants,omitempty"`
}

// AdminListNamespaces lists all namespaces (admin only).
func (c *Client) AdminListNamespaces(ctx context.Context) ([]Namespace, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/admin/namespaces")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.decodeError(resp)
	}

	var dataResp response
	if err := json.NewDecoder(resp.Body).Decode(&dataResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var namespaces []Namespace
	if err := json.Unmarshal(dataResp.Data, &namespaces); err != nil {
		return nil, fmt.Errorf("decode namespaces: %w", err)
	}

	return namespaces, nil
}

// AdminCreateNamespace creates a new namespace (admin only).
func (c *Client) AdminCreateNamespace(ctx context.Context, name string) (*Namespace, error) {
	body := map[string]any{"name": name}

	resp, err := c.doRequestWithBody(ctx, http.MethodPost, "/api/v1/admin/namespaces", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, c.decodeError(resp)
	}

	var dataResp response
	if err := json.NewDecoder(resp.Body).Decode(&dataResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var ns Namespace
	if err := json.Unmarshal(dataResp.Data, &ns); err != nil {
		return nil, fmt.Errorf("decode namespace: %w", err)
	}

	return &ns, nil
}

// AdminDeleteNamespace deletes a namespace (admin only).
func (c *Client) AdminDeleteNamespace(ctx context.Context, id string) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, "/api/v1/admin/namespaces/"+id)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return c.decodeError(resp)
	}

	return nil
}

// AdminGetToken retrieves a single token by ID with its grants (admin only).
func (c *Client) AdminGetToken(ctx context.Context, id string) (*TokenListItem, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/admin/tokens/"+id)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.decodeError(resp)
	}

	var dataResp response
	if err := json.NewDecoder(resp.Body).Decode(&dataResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var token TokenListItem
	if err := json.Unmarshal(dataResp.Data, &token); err != nil {
		return nil, fmt.Errorf("decode token: %w", err)
	}

	return &token, nil
}

// AdminListTokens lists all tokens (admin only).
func (c *Client) AdminListTokens(ctx context.Context) ([]TokenListItem, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/admin/tokens")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.decodeError(resp)
	}

	var dataResp response
	if err := json.NewDecoder(resp.Body).Decode(&dataResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var tokens []TokenListItem
	if err := json.Unmarshal(dataResp.Data, &tokens); err != nil {
		return nil, fmt.Errorf("decode tokens: %w", err)
	}

	return tokens, nil
}

// AdminDeleteToken deletes a token (admin only).
func (c *Client) AdminDeleteToken(ctx context.Context, id string) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, "/api/v1/admin/tokens/"+id)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return c.decodeError(resp)
	}

	return nil
}
