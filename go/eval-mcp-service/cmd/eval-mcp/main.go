package main

import (
	"context"
	"fmt"
	"log"
	"os"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/config"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/evalapi"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/evalworkflow"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/mcpserver"
	"github.com/kabradshaw1/portfolio/go/eval-mcp-service/internal/store"
)

type app struct {
	service *evalworkflow.Service
	cfg     config.Config
}

type serverRunner func(context.Context, *app) error

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
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate store: %w", err)
	}
	api := evalapi.New(cfg.EvalAPIURL, cfg.APIToken, nil)
	service := evalworkflow.New(api, db, cfg.PollInterval, cfg.WaitTimeout)
	logger.Printf("eval MCP server running on stdio eval_api_url=%s db_path=%s", cfg.EvalAPIURL, cfg.DBPath)
	return runServer(ctx, &app{service: service, cfg: cfg})
}

func runMCPServer(ctx context.Context, application *app) error {
	server := mcpserver.New(application.service)
	return server.Run(ctx, &sdkmcp.StdioTransport{})
}
