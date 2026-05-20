package ingestionapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const errorExcerptLimit = 256

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

type Collection struct {
	Name        string `json:"name"`
	PointsCount int    `json:"points_count"`
}

type Source struct {
	Filename string `json:"filename"`
	Chunks   int    `json:"chunks"`
}

type HTTPError struct {
	Method     string
	Path       string
	StatusCode int
	Excerpt    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s %s: status %d: %s", e.Method, e.Path, e.StatusCode, e.Excerpt)
}

func New(baseURL, token string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), token: token, httpClient: httpClient}
}

func (c *Client) ListCollections(ctx context.Context) ([]Collection, error) {
	var response struct {
		Collections []Collection `json:"collections"`
	}
	if err := c.do(ctx, http.MethodGet, "/collections", nil, &response); err != nil {
		return nil, err
	}
	return response.Collections, nil
}

func (c *Client) GetCollectionConfig(ctx context.Context, name string) (map[string]any, error) {
	var response map[string]any
	path := "/collections/" + url.PathEscape(name) + "/config"
	if err := c.do(ctx, http.MethodGet, path, nil, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) ListCollectionSources(ctx context.Context, name string) ([]Source, error) {
	var response struct {
		Sources []Source `json:"sources"`
	}
	path := "/collections/" + url.PathEscape(name) + "/sources"
	if err := c.do(ctx, http.MethodGet, path, nil, &response); err != nil {
		return nil, err
	}
	return response.Sources, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, errorExcerptLimit))
		return &HTTPError{Method: method, Path: path, StatusCode: resp.StatusCode, Excerpt: string(data)}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
