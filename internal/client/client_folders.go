package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Folder represents a folder for organizing repositories.
type Folder struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Color     *string   `json:"color,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ListFolders lists folders in the current namespace.
func (c *Client) ListFolders(ctx context.Context, cursor string, limit int) ([]Folder, bool, error) {
	path := buildPaginatedPath("/api/v1/folders", cursor, limit)

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

	var folders []Folder
	if err := json.Unmarshal(listResp.Data, &folders); err != nil {
		return nil, false, fmt.Errorf("decode folders: %w", err)
	}

	return folders, listResp.HasMore, nil
}

// CreateFolder creates a new folder.
func (c *Client) CreateFolder(ctx context.Context, name string) (*Folder, error) {
	body := map[string]any{"name": name}

	resp, err := c.doRequestWithBody(ctx, http.MethodPost, "/api/v1/folders", body)
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

// UpdateFolder updates a folder's metadata.
func (c *Client) UpdateFolder(ctx context.Context, id string, name *string) (*Folder, error) {
	body := make(map[string]any)
	if name != nil {
		body["name"] = *name
	}

	resp, err := c.doRequestWithBody(ctx, http.MethodPatch, "/api/v1/folders/"+id, body)
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

// DeleteFolder deletes a folder.
func (c *Client) DeleteFolder(ctx context.Context, id string, force bool) error {
	path := "/api/v1/folders/" + id
	if force {
		path += "?force=true"
	}

	resp, err := c.doRequest(ctx, http.MethodDelete, path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return c.decodeError(resp)
	}

	return nil
}
