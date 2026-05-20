package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/config"
	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/history"
	"github.com/kabradshaw1/portfolio/go/observability-mcp-service/internal/management"
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
	var historyDB *history.DB
	if cfg.HistoryEnabled {
		historyDB, err = history.Open(cfg.HistoryDBPath)
		if err != nil {
			logger.Printf("observability MCP history disabled: open %s: %v", cfg.HistoryDBPath, err)
		} else if err := historyDB.Migrate(ctx); err != nil {
			_ = historyDB.Close()
			logger.Printf("observability MCP history disabled: migrate %s: %v", cfg.HistoryDBPath, err)
		} else {
			defer historyDB.Close()
			service.WithHistory(historyDB, cfg.HistoryAutoCapture)
		}
	}
	root := repoRoot()
	catalog := management.DefaultCatalog()
	if err := catalog.ValidateScripts(root); err != nil {
		return fmt.Errorf("management catalog: %w", err)
	}
	runner := management.Runner{RepoRoot: root, MaxOutputBytes: cfg.ManagementMaxOutputBytes, MaxTimeout: cfg.ManagementActionTimeout}
	managementService := management.NewService(
		catalog,
		management.Policy{ActionsEnabled: cfg.ManagementActionsEnabled, AllowHighRisk: cfg.ManagementAllowHighRisk},
		runner,
		historyDB,
	)
	service.WithManagement(managementService)
	if cfg.UseGrafanaGateway() {
		if grafana, ok := prom.(*observability.GrafanaClient); ok {
			service.SetGrafanaAlerting(grafana)
		}
	}
	return runServer(ctx, &app{service: service, cfg: cfg})
}

func repoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}

func runMCPServer(ctx context.Context, application *app) error {
	server := mcpserver.New(application.service, application.cfg)
	return server.Run(ctx, &sdkmcp.StdioTransport{})
}
