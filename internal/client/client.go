package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client is the HTTP client for the ephemeral API.
type Client struct {
	baseURL   string
	token     string
	namespace string
	http      *http.Client
}

// New creates a new API client.
func New(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// WithNamespace returns a new client configured to use the specified namespace.
func (c *Client) WithNamespace(namespace string) *Client {
	return &Client{
		baseURL:   c.baseURL,
		token:     c.token,
		namespace: namespace,
		http:      c.http,
	}
}

// Namespace returns the currently configured namespace.
func (c *Client) Namespace() string {
	return c.namespace
}

// Token returns the authentication token.
func (c *Client) Token() string {
	return c.token
}

// BaseURL returns the base URL of the API.
func (c *Client) BaseURL() string {
	return c.baseURL
}

// Namespace represents a namespace in the system.
type Namespace struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	CreatedAt         time.Time `json:"created_at"`
	RepoLimit         *int      `json:"repo_limit,omitempty"`
	StorageLimitBytes *int      `json:"storage_limit_bytes,omitempty"`
}

// NamespaceWithAccess represents a namespace with access information.
type NamespaceWithAccess struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	IsPrimary bool      `json:"is_primary"`
}

// response wraps single-item API responses.
type response struct {
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

// listResponse wraps paginated API responses.
type listResponse struct {
	Data       json.RawMessage `json:"data"`
	NextCursor *string         `json:"next_cursor,omitempty"`
	HasMore    bool            `json:"has_more"`
}

func (c *Client) doRequest(ctx context.Context, method, path string) (*http.Response, error) {
	return c.doRequestWithBody(ctx, method, path, nil)
}

func (c *Client) doRequestWithBody(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return nil, fmt.Errorf("encode body: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, &buf)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.namespace != "" {
		req.Header.Set("X-Namespace", c.namespace)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}

	return resp, nil
}

func buildPaginatedPath(basePath, cursor string, limit int) string {
	params := url.Values{}
	if cursor != "" {
		params.Set("cursor", cursor)
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if len(params) > 0 {
		return basePath + "?" + params.Encode()
	}
	return basePath
}

func (c *Client) decodeError(resp *http.Response) error {
	var errResp response
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	if errResp.Error != "" {
		return errors.New(errResp.Error)
	}
	return fmt.Errorf("status %d", resp.StatusCode)
}

// GetNamespaceInfo retrieves information about the current namespace.
func (c *Client) GetNamespaceInfo(ctx context.Context) (*Namespace, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/namespace")
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

	var ns Namespace
	if err := json.Unmarshal(dataResp.Data, &ns); err != nil {
		return nil, fmt.Errorf("decode namespace: %w", err)
	}

	return &ns, nil
}

// ListNamespaces lists all namespaces the current token has access to.
func (c *Client) ListNamespaces(ctx context.Context) ([]NamespaceWithAccess, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/namespaces")
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

	var namespaces []NamespaceWithAccess
	if err := json.Unmarshal(dataResp.Data, &namespaces); err != nil {
		return nil, fmt.Errorf("decode namespaces: %w", err)
	}

	return namespaces, nil
}
