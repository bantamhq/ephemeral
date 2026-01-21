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
	path := buildPaginatedPath("/api/v1/repos", cursor, limit)

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
		return nil, false, c.decodeError(resp, "list repos")
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
		return nil, c.decodeError(resp, "get namespace")
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

func (c *Client) CreateFolder(name string) (*Folder, error) {
	body := map[string]any{"name": name}

	resp, err := c.doRequestWithBody(http.MethodPost, "/api/v1/folders", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, c.decodeError(resp, "create folder")
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
		return c.decodeError(resp, "delete folder")
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
		return nil, c.decodeError(resp, "update folder")
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
		return nil, c.decodeError(resp, "update repo")
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
		return c.decodeError(resp, "delete repo")
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
		return nil, c.decodeError(resp, "list repo folders")
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
		return nil, c.decodeError(resp, "add repo folders")
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
		return c.decodeError(resp, "remove repo folder")
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
		return nil, c.decodeError(resp, "list refs")
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
		return nil, false, c.decodeError(resp, "list commits")
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
		return nil, c.decodeError(resp, "get tree")
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
		return nil, c.decodeError(resp, "get blob")
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
