package authclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultHTTPTimeout = 30 * time.Second
	errorExcerptLimit  = 256
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type TokenResponse struct {
	AccessToken      string `json:"accessToken"`
	RefreshToken     string `json:"refreshToken"`
	ExpiresInSeconds int64  `json:"expiresInSeconds"`
}

func New(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

func (c *Client) Login(ctx context.Context, email, password string) (TokenResponse, error) {
	body := struct {
		Email         string `json:"email"`
		Password      string `json:"password"`
		IncludeTokens bool   `json:"includeTokens"`
	}{
		Email:         email,
		Password:      password,
		IncludeTokens: true,
	}

	var response TokenResponse
	if err := c.do(ctx, http.MethodPost, "/login", body, &response, password); err != nil {
		return TokenResponse{}, err
	}
	if err := validateTokenResponse(response); err != nil {
		return TokenResponse{}, fmt.Errorf("POST /login: %w", err)
	}
	return response, nil
}

func (c *Client) Refresh(ctx context.Context, refreshToken string) (TokenResponse, error) {
	body := struct {
		RefreshToken  string `json:"refreshToken"`
		IncludeTokens bool   `json:"includeTokens"`
	}{
		RefreshToken:  refreshToken,
		IncludeTokens: true,
	}

	var response TokenResponse
	if err := c.do(ctx, http.MethodPost, "/refresh", body, &response, refreshToken); err != nil {
		return TokenResponse{}, err
	}
	if err := validateTokenResponse(response); err != nil {
		return TokenResponse{}, fmt.Errorf("POST /refresh: %w", err)
	}
	return response, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any, redactions ...string) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return fmt.Errorf("%s %s: encode request: %w", method, path, err)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, &buf)
	if err != nil {
		return fmt.Errorf("%s %s: create request: %w", method, path, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		excerpt, _ := io.ReadAll(io.LimitReader(resp.Body, errorExcerptLimit))
		return fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, redact(strings.TrimSpace(string(excerpt)), redactions...))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%s %s: decode response: %w", method, path, err)
	}
	return nil
}

func redact(value string, redactions ...string) string {
	for _, secret := range redactions {
		if secret == "" {
			continue
		}
		for _, variant := range secretVariants(secret) {
			value = strings.ReplaceAll(value, variant, "[REDACTED]")
			for prefixLen := len(variant) - 1; prefixLen >= 8; prefixLen-- {
				value = strings.ReplaceAll(value, variant[:prefixLen], "[REDACTED]")
			}
		}
	}
	if len(value) > errorExcerptLimit {
		return value[:errorExcerptLimit]
	}
	return value
}

func secretVariants(secret string) []string {
	quoted, err := json.Marshal(secret)
	if err != nil {
		return []string{secret}
	}

	escaped := strings.Trim(string(quoted), `"`)
	if escaped == secret {
		return []string{secret}
	}
	return []string{secret, escaped}
}

func validateTokenResponse(response TokenResponse) error {
	switch {
	case response.AccessToken == "":
		return fmt.Errorf("missing access token")
	case response.RefreshToken == "":
		return fmt.Errorf("missing refresh token")
	case response.ExpiresInSeconds <= 0:
		return fmt.Errorf("expiresInSeconds must be positive")
	default:
		return nil
	}
}
