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
	baseURL   string
	token     string
	namespace string
	http      *http.Client
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

type Repo struct {
	ID          string     `json:"id"`
	NamespaceID string     `json:"namespace_id"`
	Name        string     `json:"name"`
	Description *string    `json:"description,omitempty"`
	Public      bool       `json:"public"`
	SizeBytes   int        `json:"size_bytes"`
	LastPushAt  *time.Time `json:"last_push_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type RepoWithFolders struct {
	Repo
	Folders []Folder `json:"folders,omitempty"`
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
	Color     *string   `json:"color,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Ref struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	CommitSHA string `json:"commit_sha"`
	IsDefault bool   `json:"is_default"`
}

type Commit struct {
	SHA        string       `json:"sha"`
	Message    string       `json:"message"`
	Author     GitAuthor    `json:"author"`
	Committer  GitAuthor    `json:"committer"`
	ParentSHAs []string     `json:"parent_shas"`
	TreeSHA    string       `json:"tree_sha"`
	Stats      *CommitStats `json:"stats,omitempty"`
}

type GitAuthor struct {
	Name  string    `json:"name"`
	Email string    `json:"email"`
	Date  time.Time `json:"date"`
}

type CommitStats struct {
	FilesChanged int `json:"files_changed"`
	Additions    int `json:"additions"`
	Deletions    int `json:"deletions"`
}

type TreeEntry struct {
	Name        string      `json:"name"`
	Path        string      `json:"path"`
	Type        string      `json:"type"`
	Mode        string      `json:"mode"`
	SHA         string      `json:"sha"`
	Size        *int64      `json:"size,omitempty"`
	HasChildren *bool       `json:"has_children,omitempty"`
	Children    []TreeEntry `json:"children,omitempty"`
}

type Blob struct {
	SHA       string  `json:"sha"`
	Size      int64   `json:"size"`
	Content   *string `json:"content,omitempty"`
	Encoding  string  `json:"encoding"`
	IsBinary  bool    `json:"is_binary"`
	Truncated bool    `json:"truncated"`
}

type Readme struct {
	Filename  string `json:"filename"`
	Content   string `json:"content"`
	Size      int64  `json:"size"`
	SHA       string `json:"sha"`
	IsBinary  bool   `json:"is_binary"`
	Truncated bool   `json:"truncated"`
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

func (c *Client) ListRepos(cursor string, limit int) ([]Repo, bool, error) {
	path := buildPaginatedPath("/api/v1/repos", cursor, limit)

	resp, err := c.doRequest(http.MethodGet, path)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false, c.decodeError(resp)
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

func (c *Client) ListReposWithFolders(cursor string, limit int) ([]RepoWithFolders, bool, error) {
	path := buildPaginatedPath("/api/v1/repos", cursor, limit)
	if cursor != "" || limit > 0 {
		path += "&expand=folders"
	} else {
		path += "?expand=folders"
	}

	resp, err := c.doRequest(http.MethodGet, path)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false, c.decodeError(resp)
	}

	var listResp listResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, false, fmt.Errorf("decode response: %w", err)
	}

	var repos []RepoWithFolders
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

func (c *Client) ListFolders(cursor string, limit int) ([]Folder, bool, error) {
	path := buildPaginatedPath("/api/v1/folders", cursor, limit)

	resp, err := c.doRequest(http.MethodGet, path)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false, c.decodeError(resp)
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
	if c.namespace != "" {
		req.Header.Set("X-Namespace", c.namespace)
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

func (c *Client) CreateFolder(name string) (*Folder, error) {
	body := map[string]any{"name": name}

	resp, err := c.doRequestWithBody(http.MethodPost, "/api/v1/folders", body)
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

	var folder Folder
	if err := json.Unmarshal(dataResp.Data, &folder); err != nil {
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
		return c.decodeError(resp)
	}

	return nil
}

func (c *Client) UpdateFolder(id string, name *string) (*Folder, error) {
	body := make(map[string]any)
	if name != nil {
		body["name"] = *name
	}

	resp, err := c.doRequestWithBody(http.MethodPatch, "/api/v1/folders/"+id, body)
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

	var folder Folder
	if err := json.Unmarshal(dataResp.Data, &folder); err != nil {
		return nil, fmt.Errorf("decode folder: %w", err)
	}

	return &folder, nil
}

func (c *Client) UpdateRepo(id string, name *string, description *string, public *bool) (*Repo, error) {
	body := make(map[string]any)
	if name != nil {
		body["name"] = *name
	}
	if description != nil {
		body["description"] = *description
	}
	if public != nil {
		body["public"] = *public
	}

	resp, err := c.doRequestWithBody(http.MethodPatch, "/api/v1/repos/"+id, body)
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

	var repo Repo
	if err := json.Unmarshal(dataResp.Data, &repo); err != nil {
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
		return c.decodeError(resp)
	}

	return nil
}

func (c *Client) ListRepoFolders(repoID string) ([]Folder, error) {
	resp, err := c.doRequest(http.MethodGet, "/api/v1/repos/"+repoID+"/folders")
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

	var folders []Folder
	if err := json.Unmarshal(dataResp.Data, &folders); err != nil {
		return nil, fmt.Errorf("decode folders: %w", err)
	}

	return folders, nil
}

func (c *Client) AddRepoFolders(repoID string, folderIDs []string) ([]Folder, error) {
	body := map[string]any{"folder_ids": folderIDs}

	resp, err := c.doRequestWithBody(http.MethodPost, "/api/v1/repos/"+repoID+"/folders", body)
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

	var folders []Folder
	if err := json.Unmarshal(dataResp.Data, &folders); err != nil {
		return nil, fmt.Errorf("decode folders: %w", err)
	}

	return folders, nil
}

func (c *Client) RemoveRepoFolder(repoID, folderID string) error {
	resp, err := c.doRequest(http.MethodDelete, "/api/v1/repos/"+repoID+"/folders/"+folderID)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return c.decodeError(resp)
	}

	return nil
}

func (c *Client) ListRefs(repoID string) ([]Ref, error) {
	resp, err := c.doRequest(http.MethodGet, "/api/v1/repos/"+repoID+"/refs")
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

	var refs []Ref
	if err := json.Unmarshal(dataResp.Data, &refs); err != nil {
		return nil, fmt.Errorf("decode refs: %w", err)
	}

	return refs, nil
}

func (c *Client) ListCommits(repoID, ref, cursor string, limit int) ([]Commit, bool, error) {
	params := url.Values{}
	if ref != "" {
		params.Set("ref", ref)
	}
	if cursor != "" {
		params.Set("cursor", cursor)
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	path := "/api/v1/repos/" + repoID + "/commits"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	resp, err := c.doRequest(http.MethodGet, path)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false, c.decodeError(resp)
	}

	var listResp listResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, false, fmt.Errorf("decode response: %w", err)
	}

	var commits []Commit
	if err := json.Unmarshal(listResp.Data, &commits); err != nil {
		return nil, false, fmt.Errorf("decode commits: %w", err)
	}

	return commits, listResp.HasMore, nil
}

func (c *Client) GetTree(repoID, ref, path string) ([]TreeEntry, error) {
	return c.GetTreeWithDepth(repoID, ref, path, 0)
}

func (c *Client) GetTreeWithDepth(repoID, ref, path string, depth int) ([]TreeEntry, error) {
	apiPath := "/api/v1/repos/" + repoID + "/tree/" + ref + "/" + path
	if depth > 0 {
		apiPath += "?depth=" + strconv.Itoa(depth)
	}

	resp, err := c.doRequest(http.MethodGet, apiPath)
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

	var entries []TreeEntry
	if err := json.Unmarshal(dataResp.Data, &entries); err != nil {
		return nil, fmt.Errorf("decode tree: %w", err)
	}

	return entries, nil
}

func (c *Client) GetBlob(repoID, ref, path string) (*Blob, error) {
	apiPath := "/api/v1/repos/" + repoID + "/blob/" + ref + "/" + path

	resp, err := c.doRequest(http.MethodGet, apiPath)
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

	var blob Blob
	if err := json.Unmarshal(dataResp.Data, &blob); err != nil {
		return nil, fmt.Errorf("decode blob: %w", err)
	}

	return &blob, nil
}

func (c *Client) GetReadme(repoID, ref string) (*Readme, error) {
	apiPath := "/api/v1/repos/" + repoID + "/readme"
	if ref != "" {
		apiPath += "?ref=" + ref
	}

	resp, err := c.doRequest(http.MethodGet, apiPath)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, c.decodeError(resp)
	}

	var dataResp response
	if err := json.NewDecoder(resp.Body).Decode(&dataResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var readme Readme
	if err := json.Unmarshal(dataResp.Data, &readme); err != nil {
		return nil, fmt.Errorf("decode readme: %w", err)
	}

	return &readme, nil
}

type NamespaceWithAccess struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	IsPrimary bool      `json:"is_primary"`
}

// ListNamespaces lists all namespaces the current token has access to.
func (c *Client) ListNamespaces() ([]NamespaceWithAccess, error) {
	resp, err := c.doRequest(http.MethodGet, "/api/v1/namespaces")
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

type TokenResponse struct {
	ID              string                   `json:"id"`
	Name            *string                  `json:"name,omitempty"`
	IsAdmin         bool                     `json:"is_admin"`
	CreatedAt       time.Time                `json:"created_at"`
	ExpiresAt       *time.Time               `json:"expires_at,omitempty"`
	Token           string                   `json:"token"`
	NamespaceGrants []NamespaceGrantResponse `json:"namespace_grants,omitempty"`
	RepoGrants      []RepoGrantResponse      `json:"repo_grants,omitempty"`
}

// AdminListNamespaces lists all namespaces (admin only).
func (c *Client) AdminListNamespaces() ([]Namespace, error) {
	resp, err := c.doRequest(http.MethodGet, "/api/v1/admin/namespaces")
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
func (c *Client) AdminCreateNamespace(name string) (*Namespace, error) {
	body := map[string]any{"name": name}

	resp, err := c.doRequestWithBody(http.MethodPost, "/api/v1/admin/namespaces", body)
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
func (c *Client) AdminDeleteNamespace(id string) error {
	resp, err := c.doRequest(http.MethodDelete, "/api/v1/admin/namespaces/"+id)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return c.decodeError(resp)
	}

	return nil
}

// AdminCreateToken creates a new user token with access to a namespace (admin only).
// Uses simple mode which grants namespace:write + repo:admin on the primary namespace.
func (c *Client) AdminCreateToken(namespaceID string, name *string) (*TokenResponse, error) {
	body := map[string]any{
		"namespace_id": namespaceID,
	}
	if name != nil {
		body["name"] = *name
	}

	resp, err := c.doRequestWithBody(http.MethodPost, "/api/v1/admin/tokens", body)
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

	var token TokenResponse
	if err := json.Unmarshal(dataResp.Data, &token); err != nil {
		return nil, fmt.Errorf("decode token: %w", err)
	}

	return &token, nil
}

// AdminUpsertNamespaceGrant creates or updates a namespace grant for a token (admin only).
func (c *Client) AdminUpsertNamespaceGrant(tokenID, namespaceID string, allow []string, deny []string, isPrimary bool) error {
	body := map[string]any{
		"namespace_id": namespaceID,
		"allow":        allow,
		"is_primary":   isPrimary,
	}
	if len(deny) > 0 {
		body["deny"] = deny
	}

	resp, err := c.doRequestWithBody(http.MethodPost, "/api/v1/admin/tokens/"+tokenID+"/namespace-grants", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return c.decodeError(resp)
	}

	return nil
}

// AdminDeleteNamespaceGrant removes a namespace grant from a token (admin only).
func (c *Client) AdminDeleteNamespaceGrant(tokenID, namespaceID string) error {
	resp, err := c.doRequest(http.MethodDelete, "/api/v1/admin/tokens/"+tokenID+"/namespace-grants/"+namespaceID)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return c.decodeError(resp)
	}

	return nil
}

// AdminUpsertRepoGrant creates or updates a repo grant for a token (admin only).
func (c *Client) AdminUpsertRepoGrant(tokenID, repoID string, allow []string, deny []string) error {
	body := map[string]any{
		"repo_id": repoID,
		"allow":   allow,
	}
	if len(deny) > 0 {
		body["deny"] = deny
	}

	resp, err := c.doRequestWithBody(http.MethodPost, "/api/v1/admin/tokens/"+tokenID+"/repo-grants", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return c.decodeError(resp)
	}

	return nil
}

// AdminDeleteRepoGrant removes a repo grant from a token (admin only).
func (c *Client) AdminDeleteRepoGrant(tokenID, repoID string) error {
	resp, err := c.doRequest(http.MethodDelete, "/api/v1/admin/tokens/"+tokenID+"/repo-grants/"+repoID)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return c.decodeError(resp)
	}

	return nil
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

// AdminListTokens lists all tokens (admin only).
func (c *Client) AdminListTokens() ([]TokenListItem, error) {
	resp, err := c.doRequest(http.MethodGet, "/api/v1/admin/tokens")
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
func (c *Client) AdminDeleteToken(id string) error {
	resp, err := c.doRequest(http.MethodDelete, "/api/v1/admin/tokens/"+id)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return c.decodeError(resp)
	}

	return nil
}

// CreateRepo creates a new repository.
func (c *Client) CreateRepo(name string, description *string, public bool) (*Repo, error) {
	body := map[string]any{
		"name":   name,
		"public": public,
	}
	if description != nil {
		body["description"] = *description
	}

	resp, err := c.doRequestWithBody(http.MethodPost, "/api/v1/repos", body)
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

	var repo Repo
	if err := json.Unmarshal(dataResp.Data, &repo); err != nil {
		return nil, fmt.Errorf("decode repo: %w", err)
	}

	return &repo, nil
}
