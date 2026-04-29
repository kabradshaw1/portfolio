package clients

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/jwtctx"
)

// UserResourceClient returns raw JSON for authenticated user resources. The
// userID argument is a guardrail from the resource layer; authorization is still
// enforced by forwarding the request JWT to the ecommerce backend.
type UserResourceClient struct {
	baseURL string
	http    *http.Client
}

func NewUserResourceClient(baseURL string) *UserResourceClient {
	return &UserResourceClient{
		baseURL: baseURL,
		http:    &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)},
	}
}

func (c *UserResourceClient) Orders(ctx context.Context, userID string) (string, error) {
	return c.getAuthed(ctx, "/orders", userID)
}

func (c *UserResourceClient) Cart(ctx context.Context, userID string) (string, error) {
	return c.getAuthed(ctx, "/cart", userID)
}

func (c *UserResourceClient) getAuthed(ctx context.Context, path, userID string) (string, error) {
	if userID == "" {
		return "", fmt.Errorf("%s: user_id required", path)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return "", err
	}
	if jwt := jwtctx.FromContext(ctx); jwt != "" {
		req.Header.Set("Authorization", "Bearer "+jwt)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("%s: read body: %w", path, err)
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("%s: status %d: %s", path, resp.StatusCode, string(body))
	}
	return string(body), nil
}
