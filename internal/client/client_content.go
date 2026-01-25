package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Ref represents a git reference (branch or tag).
type Ref struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	CommitSHA string `json:"commit_sha"`
	IsDefault bool   `json:"is_default"`
}

// Commit represents a git commit.
type Commit struct {
	SHA        string       `json:"sha"`
	Message    string       `json:"message"`
	Author     GitAuthor    `json:"author"`
	Committer  GitAuthor    `json:"committer"`
	ParentSHAs []string     `json:"parent_shas"`
	TreeSHA    string       `json:"tree_sha"`
	Stats      *CommitStats `json:"stats,omitempty"`
}

// GitAuthor represents a git author or committer.
type GitAuthor struct {
	Name  string    `json:"name"`
	Email string    `json:"email"`
	Date  time.Time `json:"date"`
}

// CommitStats represents statistics for a commit.
type CommitStats struct {
	FilesChanged int `json:"files_changed"`
	Additions    int `json:"additions"`
	Deletions    int `json:"deletions"`
}

// TreeEntry represents an entry in a git tree.
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

// Blob represents a git blob (file content).
type Blob struct {
	SHA       string  `json:"sha"`
	Size      int64   `json:"size"`
	Content   *string `json:"content,omitempty"`
	Encoding  string  `json:"encoding"`
	IsBinary  bool    `json:"is_binary"`
	Truncated bool    `json:"truncated"`
}

// Readme represents a repository's README file.
type Readme struct {
	Filename  string `json:"filename"`
	Content   string `json:"content"`
	Size      int64  `json:"size"`
	SHA       string `json:"sha"`
	IsBinary  bool   `json:"is_binary"`
	Truncated bool   `json:"truncated"`
}

// ListRefs lists all refs (branches and tags) for a repository.
func (c *Client) ListRefs(ctx context.Context, repoID string) ([]Ref, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/repos/"+repoID+"/refs")
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

// ListCommits lists commits for a repository.
func (c *Client) ListCommits(ctx context.Context, repoID, ref, cursor string, limit int) ([]Commit, bool, error) {
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

	resp, err := c.doRequest(ctx, http.MethodGet, path)
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

// GetTree retrieves the tree for a repository at a given ref and path.
func (c *Client) GetTree(ctx context.Context, repoID, ref, path string) ([]TreeEntry, error) {
	return c.GetTreeWithDepth(ctx, repoID, ref, path, 0)
}

// GetTreeWithDepth retrieves the tree with a specified depth for recursive expansion.
func (c *Client) GetTreeWithDepth(ctx context.Context, repoID, ref, path string, depth int) ([]TreeEntry, error) {
	apiPath := "/api/v1/repos/" + repoID + "/tree/" + ref + "/" + path
	if depth > 0 {
		apiPath += "?depth=" + strconv.Itoa(depth)
	}

	resp, err := c.doRequest(ctx, http.MethodGet, apiPath)
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

// GetBlob retrieves the content of a file at a given ref and path.
func (c *Client) GetBlob(ctx context.Context, repoID, ref, path string) (*Blob, error) {
	apiPath := "/api/v1/repos/" + repoID + "/blob/" + ref + "/" + path

	resp, err := c.doRequest(ctx, http.MethodGet, apiPath)
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

// GetReadme retrieves the README file for a repository.
func (c *Client) GetReadme(ctx context.Context, repoID, ref string) (*Readme, error) {
	apiPath := "/api/v1/repos/" + repoID + "/readme"
	if ref != "" {
		apiPath += "?ref=" + ref
	}

	resp, err := c.doRequest(ctx, http.MethodGet, apiPath)
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
