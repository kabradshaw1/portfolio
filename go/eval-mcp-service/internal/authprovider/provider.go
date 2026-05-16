package authprovider

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/authclient"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/tokenstore"
)

type AuthClient interface {
	Login(context.Context, string, string) (authclient.TokenResponse, error)
	Refresh(context.Context, string) (authclient.TokenResponse, error)
}

type Store interface {
	Load(context.Context) (tokenstore.State, bool, error)
	Save(context.Context, tokenstore.State) error
}

type Config struct {
	Email          string
	Password       string
	AuthServiceURL string
	RefreshSkew    time.Duration
	Now            func() time.Time
}

type Provider struct {
	client AuthClient
	store  Store
	cfg    Config

	mu          sync.Mutex
	invalidated bool
}

func New(client AuthClient, store Store, cfg Config) *Provider {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Provider{
		client: client,
		store:  store,
		cfg:    cfg,
	}
}

func (p *Provider) Token(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	state, ok, err := p.store.Load(ctx)
	if err != nil {
		return "", fmt.Errorf("load token state: %w", err)
	}

	now := p.cfg.Now().UTC()
	if ok && p.cacheCanReuseAccess(state) && !p.invalidated && now.Add(p.cfg.RefreshSkew).Before(state.AccessTokenExp) {
		return state.AccessToken, nil
	}

	var refreshErr error
	if ok && p.cacheIdentityMatches(state) && state.RefreshToken != "" {
		response, err := p.client.Refresh(ctx, state.RefreshToken)
		if err == nil {
			return p.saveAndReturn(ctx, now, response)
		}
		refreshErr = err
	}

	response, err := p.client.Login(ctx, p.cfg.Email, p.cfg.Password)
	if err != nil {
		if refreshErr != nil {
			return "", fmt.Errorf("replace token: %w", errors.Join(
				fmt.Errorf("refresh: %w", refreshErr),
				fmt.Errorf("login: %w", err),
			))
		}
		return "", fmt.Errorf("login: %w", err)
	}
	return p.saveAndReturn(ctx, now, response)
}

func (p *Provider) Invalidate() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.invalidated = true
}

func (p *Provider) cacheCanReuseAccess(state tokenstore.State) bool {
	return p.cacheIdentityMatches(state) && state.AccessToken != ""
}

func (p *Provider) cacheIdentityMatches(state tokenstore.State) bool {
	return state.AuthEmail == p.cfg.Email &&
		state.AuthServiceURL == p.cfg.AuthServiceURL
}

func (p *Provider) saveAndReturn(ctx context.Context, now time.Time, response authclient.TokenResponse) (string, error) {
	state := tokenstore.State{
		AccessToken:    response.AccessToken,
		RefreshToken:   response.RefreshToken,
		AccessTokenExp: now.Add(time.Duration(response.ExpiresInSeconds) * time.Second),
		AuthEmail:      p.cfg.Email,
		AuthServiceURL: p.cfg.AuthServiceURL,
		WrittenAt:      now,
	}
	if err := p.store.Save(ctx, state); err != nil {
		return "", fmt.Errorf("save token state: %w", err)
	}
	p.invalidated = false
	return response.AccessToken, nil
}
