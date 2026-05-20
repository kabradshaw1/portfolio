package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/authclient"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/authprovider"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/config"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/evalapi"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/evalworkflow"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/fixturecatalog"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/ingestionapi"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/mcpserver"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/tokenstore"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/triageapi"
)

type app struct {
	service *evalworkflow.Service
	cfg     config.Config
}

type serverRunner func(context.Context, *app) error

type staticTokenProvider struct {
	token string
}

func (p staticTokenProvider) Token(context.Context) (string, error) {
	return p.token, nil
}

func (p staticTokenProvider) Invalidate() {}

type triageAdapter struct {
	client *triageapi.Client
}

func (a triageAdapter) TriageRAGRegression(ctx context.Context, in evalworkflow.TriageInput) (map[string]any, error) {
	return a.client.TriageRAGRegression(ctx, triageapi.TriageRequest{
		EvalID:               in.EvalID,
		BaselineEvalID:       in.BaselineEvalID,
		Metric:               in.Metric,
		Limit:                in.Limit,
		IncludeObservability: in.IncludeObservability,
	})
}

func main() {
	logger := log.New(os.Stderr, "", log.LstdFlags)
	if err := run(context.Background(), logger, runMCPServer); err != nil {
		logger.Fatalf("eval MCP server failed: %v", err)
	}
}

func run(ctx context.Context, logger *log.Logger, runServer serverRunner) error {
	cfg, err := config.FromEnv()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	httpClient := &http.Client{Timeout: cfg.WaitTimeout}
	var api *evalapi.Client
	var tokenProvider triageapi.TokenProvider
	if cfg.APIToken != "" {
		api = evalapi.New(cfg.EvalAPIURL, cfg.APIToken, httpClient)
		tokenProvider = staticTokenProvider{token: cfg.APIToken}
	} else {
		authClient := authclient.New(cfg.AuthServiceURL, httpClient)
		tokenStore := tokenstore.NewFileStore(cfg.TokenCachePath)
		provider := authprovider.New(authClient, tokenStore, authprovider.Config{
			Email:          cfg.AuthEmail,
			Password:       cfg.AuthPassword,
			AuthServiceURL: cfg.AuthServiceURL,
			RefreshSkew:    cfg.TokenRefreshSkew,
		})
		api = evalapi.NewWithTokenProvider(cfg.EvalAPIURL, provider, httpClient)
		tokenProvider = provider
	}
	ingestion := ingestionapi.New(cfg.IngestionURL, cfg.APIToken, httpClient)
	triageClient := triageapi.New(cfg.TriageAPIURL, tokenProvider, httpClient)
	fixtures := fixturecatalog.New(cfg.DatasetFixtureRoots)
	service := evalworkflow.New(api, ingestion, fixtures, cfg.PollInterval, cfg.WaitTimeout, cfg.MaxBackoff).
		WithTriageAPI(triageAdapter{client: triageClient})
	logger.Printf("eval MCP server running on stdio eval_api_url=%s ingestion_url=%s triage_api_url=%s", cfg.EvalAPIURL, cfg.IngestionURL, cfg.TriageAPIURL)
	return runServer(ctx, &app{service: service, cfg: cfg})
}

func runMCPServer(ctx context.Context, application *app) error {
	server := mcpserver.New(application.service)
	return server.Run(ctx, &sdkmcp.StdioTransport{})
}
