package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type Repo struct {
	ID          string     `json:"id"`
	NamespaceID string     `json:"namespace_id"`
	Name        string     `json:"name"`
	FolderID    *string    `json:"folder_id,omitempty"`
	Public      bool       `json:"public"`
	SizeBytes   int        `json:"size_bytes"`
	LastPushAt  *time.Time `json:"last_push_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type Namespace struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	CreatedAt         time.Time `json:"created_at"`
	RepoLimit         *int      `json:"repo_limit,omitempty"`
	StorageLimitBytes *int      `json:"storage_limit_bytes,omitempty"`
}

type Folder struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	ParentID  *string   `json:"parent_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type response struct {
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

type listResponse struct {
	Data       json.RawMessage `json:"data"`
	NextCursor *string         `json:"next_cursor,omitempty"`
	HasMore    bool            `json:"has_more"`
}

func (c *Client) doRequest(method, path string) (*http.Response, error) {
	return c.doRequestWithBody(method, path, nil)
}

func (c *Client) decodeError(resp *http.Response, operation string) error {
	var errResp response
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		return fmt.Errorf("%s: status %d", operation, resp.StatusCode)
	}
	if errResp.Error != "" {
		return errors.New(errResp.Error)
	}
	return fmt.Errorf("%s: status %d", operation, resp.StatusCode)
}

func (c *Client) ListRepos(cursor string, limit int) ([]Repo, bool, error) {
	params := url.Values{}
	if cursor != "" {
		params.Set("cursor", cursor)
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	path := "/api/v1/repos"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	resp, err := c.doRequest(http.MethodGet, path)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false, c.decodeError(resp, "list repos")
	}

	var listResp listResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, false, fmt.Errorf("decode response: %w", err)
	}

	var repos []Repo
	if err := json.Unmarshal(listResp.Data, &repos); err != nil {
		return nil, false, fmt.Errorf("decode repos: %w", err)
	}

	return repos, listResp.HasMore, nil
}

func (c *Client) GetNamespaceInfo() (*Namespace, error) {
	resp, err := c.doRequest(http.MethodGet, "/api/v1/namespace")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.decodeError(resp, "get namespace")
	}

	var ns Namespace
	if err := json.NewDecoder(resp.Body).Decode(&ns); err != nil {
		return nil, fmt.Errorf("decode namespace: %w", err)
	}

	return &ns, nil
}

func (c *Client) ListFolders(cursor string, limit int) ([]Folder, bool, error) {
	params := url.Values{}
	if cursor != "" {
		params.Set("cursor", cursor)
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	path := "/api/v1/folders"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	resp, err := c.doRequest(http.MethodGet, path)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false, c.decodeError(resp, "list folders")
	}

	var listResp listResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, false, fmt.Errorf("decode response: %w", err)
	}

	var folders []Folder
	if err := json.Unmarshal(listResp.Data, &folders); err != nil {
		return nil, false, fmt.Errorf("decode folders: %w", err)
	}

	return folders, listResp.HasMore, nil
}

func (c *Client) doRequestWithBody(method, path string, body any) (*http.Response, error) {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return nil, fmt.Errorf("encode body: %w", err)
		}
	}

	req, err := http.NewRequest(method, c.baseURL+path, &buf)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}

	return resp, nil
}

func (c *Client) Token() string {
	return c.token
}

func (c *Client) BaseURL() string {
	return c.baseURL
}

func (c *Client) CreateFolder(name string, parentID *string) (*Folder, error) {
	body := map[string]any{"name": name}
	if parentID != nil {
		body["parent_id"] = *parentID
	}

	resp, err := c.doRequestWithBody(http.MethodPost, "/api/v1/folders", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, c.decodeError(resp, "create folder")
	}

	var folder Folder
	if err := json.NewDecoder(resp.Body).Decode(&folder); err != nil {
		return nil, fmt.Errorf("decode folder: %w", err)
	}

	return &folder, nil
}

func (c *Client) DeleteFolder(id string, force bool) error {
	path := "/api/v1/folders/" + id
	if force {
		path += "?force=true"
	}

	resp, err := c.doRequest(http.MethodDelete, path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return c.decodeError(resp, "delete folder")
	}

	return nil
}

func (c *Client) UpdateFolder(id string, name *string, parentID *string) (*Folder, error) {
	body := make(map[string]any)
	if name != nil {
		body["name"] = *name
	}
	if parentID != nil {
		body["parent_id"] = *parentID
	}

	resp, err := c.doRequestWithBody(http.MethodPatch, "/api/v1/folders/"+id, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.decodeError(resp, "update folder")
	}

	var folder Folder
	if err := json.NewDecoder(resp.Body).Decode(&folder); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &folder, nil
}

func (c *Client) UpdateRepo(id string, name *string, public *bool, folderID *string) (*Repo, error) {
	body := make(map[string]any)
	if name != nil {
		body["name"] = *name
	}
	if public != nil {
		body["public"] = *public
	}
	if folderID != nil {
		body["folder_id"] = *folderID
	}

	resp, err := c.doRequestWithBody(http.MethodPatch, "/api/v1/repos/"+id, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.decodeError(resp, "update repo")
	}

	var repo Repo
	if err := json.NewDecoder(resp.Body).Decode(&repo); err != nil {
		return nil, fmt.Errorf("decode repo: %w", err)
	}

	return &repo, nil
}

func (c *Client) DeleteRepo(id string) error {
	resp, err := c.doRequest(http.MethodDelete, "/api/v1/repos/"+id)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return c.decodeError(resp, "delete repo")
	}

	return nil
}
