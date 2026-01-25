package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Repo represents a repository.
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

// RepoWithFolders is a repository with its associated folders.
type RepoWithFolders struct {
	Repo
	Folders []Folder `json:"folders,omitempty"`
}

// ListRepos lists repositories in the current namespace.
func (c *Client) ListRepos(ctx context.Context, cursor string, limit int) ([]Repo, bool, error) {
	path := buildPaginatedPath("/api/v1/repos", cursor, limit)

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

	var repos []Repo
	if err := json.Unmarshal(listResp.Data, &repos); err != nil {
		return nil, false, fmt.Errorf("decode repos: %w", err)
	}

	return repos, listResp.HasMore, nil
}

// ListReposWithFolders lists repositories with their folder associations.
func (c *Client) ListReposWithFolders(ctx context.Context, cursor string, limit int) ([]RepoWithFolders, bool, error) {
	path := buildPaginatedPath("/api/v1/repos", cursor, limit)
	if cursor != "" || limit > 0 {
		path += "&expand=folders"
	} else {
		path += "?expand=folders"
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

	var repos []RepoWithFolders
	if err := json.Unmarshal(listResp.Data, &repos); err != nil {
		return nil, false, fmt.Errorf("decode repos: %w", err)
	}

	return repos, listResp.HasMore, nil
}

// CreateRepo creates a new repository.
func (c *Client) CreateRepo(ctx context.Context, name string, description *string, public bool) (*Repo, error) {
	body := map[string]any{
		"name":   name,
		"public": public,
	}
	if description != nil {
		body["description"] = *description
	}

	resp, err := c.doRequestWithBody(ctx, http.MethodPost, "/api/v1/repos", body)
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

// UpdateRepo updates a repository's metadata.
func (c *Client) UpdateRepo(ctx context.Context, id string, name *string, description *string, public *bool) (*Repo, error) {
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

	resp, err := c.doRequestWithBody(ctx, http.MethodPatch, "/api/v1/repos/"+id, body)
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

// DeleteRepo deletes a repository.
func (c *Client) DeleteRepo(ctx context.Context, id string) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, "/api/v1/repos/"+id)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return c.decodeError(resp)
	}

	return nil
}

// ListRepoFolders lists folders associated with a repository.
func (c *Client) ListRepoFolders(ctx context.Context, repoID string) ([]Folder, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/repos/"+repoID+"/folders")
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

// AddRepoFolders adds folders to a repository.
func (c *Client) AddRepoFolders(ctx context.Context, repoID string, folderIDs []string) ([]Folder, error) {
	body := map[string]any{"folder_ids": folderIDs}

	resp, err := c.doRequestWithBody(ctx, http.MethodPost, "/api/v1/repos/"+repoID+"/folders", body)
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

// RemoveRepoFolder removes a folder from a repository.
func (c *Client) RemoveRepoFolder(ctx context.Context, repoID, folderID string) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, "/api/v1/repos/"+repoID+"/folders/"+folderID)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return c.decodeError(resp)
	}

	return nil
}
