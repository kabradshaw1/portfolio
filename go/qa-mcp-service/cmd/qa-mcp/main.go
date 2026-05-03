package main

import (
	"context"
	"fmt"
	"log"
	"os"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/qa-mcp-service/internal/mcpserver"
	"github.com/kabradshaw1/portfolio/go/qa-mcp-service/internal/qa"
	"github.com/kabradshaw1/portfolio/go/qa-mcp-service/internal/store"
)

const (
	defaultDBPath       = "data/qa.db"
	defaultMaterialPath = "../../docs/interview-prep/qa"
)

type config struct {
	dbPath       string
	materialPath string
}

type app struct {
	service *qa.Service
}

type serverRunner func(context.Context, *app) error

func main() {
	ctx := context.Background()
	logger := log.New(os.Stderr, "", log.LstdFlags)
	cfg := config{
		dbPath:       getenv("QA_DB_PATH", defaultDBPath),
		materialPath: getenv("QA_MATERIAL_PATH", defaultMaterialPath),
	}

	if err := run(ctx, cfg, logger, runMCPServer); err != nil {
		logger.Fatalf("QA MCP server failed: %v", err)
	}
}

func run(ctx context.Context, cfg config, logger *log.Logger, runServer serverRunner) error {
	db, err := store.Open(cfg.dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate store: %w", err)
	}

	service := qa.New(db, cfg.materialPath)
	if result, err := service.ImportMaterial(ctx); err != nil {
		logger.Printf("initial material import failed: %v", err)
	} else {
		logger.Printf("initial material import complete imported_questions=%d", result.ImportedQuestions)
	}

	logger.Printf("QA MCP server running on stdio db_path=%s material_path=%s", cfg.dbPath, cfg.materialPath)
	if err := runServer(ctx, &app{service: service}); err != nil {
		return fmt.Errorf("mcp server: %w", err)
	}
	return nil
}

func runMCPServer(ctx context.Context, application *app) error {
	server := mcpserver.New(application.service)
	if err := server.Run(ctx, &sdkmcp.StdioTransport{}); err != nil {
		return err
	}
	return nil
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
