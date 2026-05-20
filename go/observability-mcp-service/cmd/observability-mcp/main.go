package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/config"
	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/history"
	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/mcpserver"
	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/observability"
	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/workflows"
)

type app struct {
	service *workflows.Service
	cfg     config.Config
}

type serverRunner func(context.Context, *app) error

func main() {
	logger := log.New(os.Stderr, "", log.LstdFlags)
	if err := run(context.Background(), logger, runMCPServer); err != nil {
		logger.Fatalf("observability MCP server failed: %v", err)
	}
}

func run(ctx context.Context, logger *log.Logger, runServer serverRunner) error {
	cfg, err := config.FromEnv()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	httpClient := &http.Client{Timeout: cfg.QueryTimeout}
	var prom workflows.Prometheus
	var loki workflows.Loki
	jaeger := observability.NewJaeger(cfg.JaegerURL, httpClient, cfg.MaxTraceSpans)
	if cfg.UseGrafanaGateway() {
		grafana := observability.NewGrafana(observability.GrafanaConfig{
			BaseURL:                 cfg.GrafanaURL,
			Token:                   cfg.GrafanaToken,
			AccessClientID:          cfg.GrafanaAccessClientID,
			AccessClientSecret:      cfg.GrafanaAccessClientSecret,
			PrometheusDatasourceUID: cfg.GrafanaPrometheusDatasourceUID,
			LokiDatasourceUID:       cfg.GrafanaLokiDatasourceUID,
		}, httpClient)
		prom = grafana
		loki = grafana
		logger.Printf("observability MCP server running on stdio grafana=%s jaeger=%s", cfg.GrafanaURL, cfg.JaegerURL)
		if cfg.UsesDefaultJaegerURL() {
			logger.Printf("observability MCP grafana mode leaves jaeger on default direct URL; set OBS_JAEGER_URL if trace lookup should use a reachable endpoint")
		}
	} else {
		prom = observability.NewPrometheus(cfg.PrometheusURL, httpClient)
		loki = observability.NewLoki(cfg.LokiURL, httpClient)
		logger.Printf("observability MCP server running on stdio prometheus=%s loki=%s jaeger=%s", cfg.PrometheusURL, cfg.LokiURL, cfg.JaegerURL)
	}
	service := workflows.NewService(prom, loki, jaeger, cfg.MaxLogLines)
	if cfg.HistoryEnabled {
		historyDB, err := history.Open(cfg.HistoryDBPath)
		if err != nil {
			return fmt.Errorf("history open: %w", err)
		}
		defer historyDB.Close()
		if err := historyDB.Migrate(ctx); err != nil {
			return fmt.Errorf("history migrate: %w", err)
		}
		service.WithHistory(historyDB, cfg.HistoryAutoCapture)
	}
	return runServer(ctx, &app{service: service, cfg: cfg})
}

func runMCPServer(ctx context.Context, application *app) error {
	server := mcpserver.New(application.service, application.cfg)
	return server.Run(ctx, &sdkmcp.StdioTransport{})
}
