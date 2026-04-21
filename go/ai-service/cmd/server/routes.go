package main

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	apphttp "github.com/kabradshaw1/portfolio/go/ai-service/internal/http"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/llm"
	mcpadapter "github.com/kabradshaw1/portfolio/go/ai-service/internal/mcp"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools/clients"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/agent"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/guardrails"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	pkgmw "github.com/kabradshaw1/portfolio/go/pkg/middleware"
)

// setupRouter creates the Gin engine with all middleware and routes.
func setupRouter(
	cfg Config,
	a *agent.Agent,
	limiter *guardrails.Limiter,
	authedMCPHandler http.Handler,
	llmc llm.Client,
) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(pkgmw.SecurityHeaders())
	router.Use(otelgin.Middleware("ai-service"))
	router.Use(apphttp.CORSMiddleware(cfg.AllowedOrigins))
	router.Use(apperror.ErrorHandler())

	apphttp.RegisterHealthRoutes(router, map[string]apphttp.ReadyCheck{
		"llm": func() error {
			if cfg.LLMProvider == "ollama" {
				req, _ := http.NewRequest(http.MethodGet, cfg.OllamaURL+"/api/tags", nil)
				client := &http.Client{Timeout: 2 * time.Second}
				resp, err := client.Do(req)
				if err != nil {
					return err
				}
				defer resp.Body.Close()
				return nil
			}
			// For API-based providers, do a lightweight chat call
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, err := llmc.Chat(ctx, []llm.Message{{Role: llm.RoleUser, Content: "ping"}}, nil)
			return err
		},
		"ecommerce": func() error {
			req, _ := http.NewRequest(http.MethodGet, cfg.OrderURL+"/health", nil)
			client := &http.Client{Timeout: 2 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			return nil
		},
	})
	apphttp.RegisterChatRoutes(router, a, cfg.JWTSecret, limiter)
	apphttp.RegisterMetricsRoute(router)
	router.Any("/mcp", gin.WrapH(authedMCPHandler))

	return router
}

// registerCoreTools registers the 8 ecommerce tools without caching or Kafka.
// Used by the MCP stdio path where those features are not available.
func registerCoreTools(registry *tools.MemRegistry, ecomClient *clients.EcommerceClient) {
	registry.Register(tools.NewSearchProductsTool(ecomClient))
	registry.Register(tools.NewGetProductTool(ecomClient))
	registry.Register(tools.NewCheckInventoryTool(ecomClient))
	registry.Register(tools.NewListOrdersTool(ecomClient))
	registry.Register(tools.NewGetOrderTool(ecomClient))
	registry.Register(tools.NewViewCartTool(ecomClient))
	registry.Register(tools.NewAddToCartTool(ecomClient))
	registry.Register(tools.NewInitiateReturnTool(ecomClient))
}

// discoverMCPTools connects to configured MCP servers and registers their tools.
func discoverMCPTools(ctx context.Context, registry *tools.MemRegistry, mcpServersJSON string) {
	var servers []struct {
		Name      string `json:"name"`
		Transport string `json:"transport"`
		URL       string `json:"url"`
	}
	if err := json.Unmarshal([]byte(mcpServersJSON), &servers); err != nil {
		log.Fatalf("bad MCP_SERVERS: %v", err)
	}
	for _, s := range servers {
		if s.Transport != "http" {
			log.Fatalf("unsupported MCP transport %q for server %q", s.Transport, s.Name)
		}
		mcpClient := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "ai-service", Version: "1.0.0"}, nil)
		session, err := mcpClient.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: s.URL}, nil)
		if err != nil {
			slog.Warn("mcp server unreachable, skipping", "name", s.Name, "url", s.URL, "error", err)
			continue
		}
		discovered, err := mcpadapter.DiscoverTools(ctx, session, s.Name)
		if err != nil {
			slog.Warn("mcp tool discovery failed, skipping", "name", s.Name, "error", err)
			continue
		}
		for _, t := range discovered {
			registry.Register(t)
		}
		slog.Info("mcp tools registered", "server", s.Name, "count", len(discovered))
	}
}
