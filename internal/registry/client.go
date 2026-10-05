// Copyright (c) 2026 Derick Schaefer
// Licensed under the MIT License. See LICENSE file for details.

// Package registry implements the read-only client for the RESERVE workflow
// registry API. It is intentionally separate from the FRED client and does not
// use the user's FRED API key for RESERVE requests.
package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.reservecli.dev/v1"

type Client struct {
	baseURL    string
	httpClient *http.Client
	// Token is reserved for the v1.2.3 RESERVE authentication flow. It is not
	// populated from or derived from the FRED API key.
	Token string
}

type Repository struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Visibility string `json:"visibility"`
	Default    bool   `json:"default,omitempty"`
}

type Workflow struct {
	ID             string `json:"id"`
	Collection     string `json:"collection"`
	Title          string `json:"title"`
	LatestVersion  string `json:"latest_version"`
	ReserveVersion string `json:"reserve_version"`
	ContentKey     string `json:"content_key"`
}

type Catalog struct {
	SchemaVersion int        `json:"schema_version"`
	Repository    Repository `json:"repository"`
	Workflows     []Workflow `json:"workflows"`
}

type Collection struct {
	ID            string     `json:"id"`
	WorkflowCount int        `json:"workflow_count"`
	Workflows     []Workflow `json:"workflows"`
}

type Artifact struct {
	Collection string
	WorkflowID string
	Version    string
	Content    []byte
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultBaseURL
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (c *Client) Browse(ctx context.Context) (*Catalog, error) {
	var catalog Catalog
	if err := c.getJSON(ctx, "/browse", &catalog); err != nil {
		return nil, fmt.Errorf("browse workflow registry: %w", err)
	}
	return &catalog, nil
}

func (c *Client) Collection(ctx context.Context, collectionID string) (*Collection, error) {
	var collection Collection
	path := "/collections/" + url.PathEscape(strings.TrimSpace(collectionID))
	if err := c.getJSON(ctx, path, &collection); err != nil {
		return nil, fmt.Errorf("get workflow collection %s: %w", collectionID, err)
	}
	return &collection, nil
}

func (c *Client) Workflow(ctx context.Context, collectionID, workflowID, version string) (*Artifact, error) {
	path := "/workflows/" + url.PathEscape(strings.TrimSpace(collectionID)) + "/" + url.PathEscape(strings.TrimSpace(workflowID))
	if strings.TrimSpace(version) != "" {
		path += "/" + url.PathEscape(strings.TrimSpace(version))
	}
	body, headers, err := c.get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("get workflow %s/%s: %w", collectionID, workflowID, err)
	}
	resolvedVersion := headers.Get("x-reserve-workflow-version")
	if resolvedVersion == "" {
		resolvedVersion = version
	}
	return &Artifact{
		Collection: collectionID,
		WorkflowID: workflowID,
		Version:    resolvedVersion,
		Content:    body,
	}, nil
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	body, _, err := c.get(ctx, path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode registry response: %w", err)
	}
	return nil
}

func (c *Client) get(ctx context.Context, path string) ([]byte, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("build registry request: %w", err)
	}
	req.Header.Set("Accept", "application/json, application/yaml, text/yaml")
	req.Header.Set("User-Agent", "reserve-cli/1.2.2")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("registry request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read registry response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.Header, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, resp.Header, nil
}
