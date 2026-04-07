package google

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

// UserInfo is the subset of Google's /userinfo response we care about.
type UserInfo struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

// Client exchanges an OAuth authorization code for a Google UserInfo.
type Client struct {
	clientID     string
	clientSecret string
	tokenURL     string
	userinfoURL  string
	http         *http.Client
}

// NewClient constructs a Google OAuth client.
func NewClient(clientID, clientSecret, tokenURL, userinfoURL string) *Client {
	return &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		tokenURL:     tokenURL,
		userinfoURL:  userinfoURL,
		http:         &http.Client{Timeout: 10 * time.Second},
	}
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
}

// ExchangeCode exchanges an authorization code for user profile information.
func (c *Client) ExchangeCode(ctx context.Context, code, redirectURI string) (*UserInfo, error) {
	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", c.clientID)
	form.Set("client_secret", c.clientSecret)
	form.Set("redirect_uri", redirectURI)
	form.Set("grant_type", "authorization_code")

	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build token request: %w", err)
	}
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	tokenResp, err := c.http.Do(tokenReq)
	if err != nil {
		return nil, fmt.Errorf("google token request: %w", err)
	}
	defer tokenResp.Body.Close()

	if tokenResp.StatusCode >= 400 {
		body, _ := io.ReadAll(tokenResp.Body)
		return nil, fmt.Errorf("google token endpoint returned %d: %s", tokenResp.StatusCode, string(body))
	}

	var tr tokenResponse
	if err := json.NewDecoder(tokenResp.Body).Decode(&tr); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	if tr.AccessToken == "" {
		return nil, fmt.Errorf("google token response missing access_token")
	}

	userReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.userinfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build userinfo request: %w", err)
	}
	userReq.Header.Set("Authorization", "Bearer "+tr.AccessToken)

	userResp, err := c.http.Do(userReq)
	if err != nil {
		return nil, fmt.Errorf("google userinfo request: %w", err)
	}
	defer userResp.Body.Close()

	if userResp.StatusCode >= 400 {
		body, _ := io.ReadAll(userResp.Body)
		return nil, fmt.Errorf("google userinfo endpoint returned %d: %s", userResp.StatusCode, string(body))
	}

	var info UserInfo
	if err := json.NewDecoder(userResp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decode userinfo response: %w", err)
	}
	return &info, nil
}
