package main

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/agent"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/auth"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/cache"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/guardrails"
	apphttp "github.com/kabradshaw1/portfolio/go/ai-service/internal/http"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/llm"
	mcpadapter "github.com/kabradshaw1/portfolio/go/ai-service/internal/mcp"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools/clients"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	pkgmw "github.com/kabradshaw1/portfolio/go/pkg/middleware"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "serve":
		runServe()
	case "mcp":
		runMCP()
	default:
		log.Fatalf("unknown command: %s (use 'serve' or 'mcp')", cmd)
	}
}

func runServe() {
	port := getenv("PORT", "8093")
	ollamaURL := getenv("OLLAMA_URL", "http://ollama:11434")
	ollamaModel := getenv("OLLAMA_MODEL", "qwen2.5:14b")
	ecommerceURL := getenv("ECOMMERCE_URL", "http://ecommerce-service:8092")
	ragChatURL := getenv("RAG_CHAT_URL", "http://chat-service:8001")
	ragIngestionURL := getenv("RAG_INGESTION_URL", "http://ingestion-service:8002")
	redisURL := getenv("REDIS_URL", "")

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET is required")
	}

	// Tracing
	ctx := context.Background()
	shutdownTracer, err := tracing.Init(ctx, "ai-service", os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if err != nil {
		log.Fatalf("tracing init: %v", err)
	}
	defer func() { _ = shutdownTracer(ctx) }()

	// Circuit breakers
	ollamaBreaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:          "ai-ollama",
		OnStateChange: resilience.ObserveStateChange,
	})
	ecomBreaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:          "ai-ecommerce",
		OnStateChange: resilience.ObserveStateChange,
	})
	ragBreaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:          "ai-rag",
		OnStateChange: resilience.ObserveStateChange,
	})
	redisBreaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:          "ai-redis",
		OnStateChange: resilience.ObserveStateChange,
	})

	// LLM client
	llmProvider := getenv("LLM_PROVIDER", "ollama")
	llmAPIKey := os.Getenv("LLM_API_KEY")
	var llmBaseURL string
	if llmProvider == "ollama" {
		llmBaseURL = ollamaURL
	} else {
		llmBaseURL = getenv("LLM_BASE_URL", "")
	}
	llmc, err := llm.NewClient(llmProvider, llmBaseURL, ollamaModel, llmAPIKey, ollamaBreaker)
	if err != nil {
		log.Fatalf("LLM client: %v", err)
	}

	// Tool registry
	ecomClient := clients.NewEcommerceClient(ecommerceURL, ecomBreaker)
	ragClient := clients.NewRAGClient(ragChatURL, ragIngestionURL, ragBreaker)

	var toolCache cache.Cache = cache.NopCache{}
	var limiter *guardrails.Limiter
	if redisURL != "" {
		opts, err := redis.ParseURL(redisURL)
		if err != nil {
			log.Fatalf("bad REDIS_URL: %v", err)
		}
		rc := redis.NewClient(opts)
		pingCtx, cancelPing := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancelPing()
		if err := rc.Ping(pingCtx).Err(); err != nil {
			slog.Warn("redis unreachable, caching + rate limit disabled", "error", err)
		} else {
			toolCache = cache.NewRedisCache(rc, "ai", redisBreaker)
			limiter = guardrails.NewLimiter(rc, 20, time.Minute, redisBreaker)
			slog.Info("redis connected, caching + rate limit enabled")
		}
	}

	registry := tools.NewMemRegistry()
	registry.Register(tools.Cached(tools.NewSearchProductsTool(ecomClient), toolCache, 60*time.Second))
	registry.Register(tools.Cached(tools.NewGetProductTool(ecomClient), toolCache, 60*time.Second))
	registry.Register(tools.Cached(tools.NewCheckInventoryTool(ecomClient), toolCache, 10*time.Second))
	registry.Register(tools.Cached(tools.NewListOrdersTool(ecomClient), toolCache, 10*time.Second))
	registry.Register(tools.Cached(tools.NewGetOrderTool(ecomClient), toolCache, 10*time.Second))
	registry.Register(tools.NewSummarizeOrdersTool(ecomClient, llmc))
	registry.Register(tools.NewViewCartTool(ecomClient))
	registry.Register(tools.NewAddToCartTool(ecomClient))
	registry.Register(tools.NewInitiateReturnTool(ecomClient))

	// RAG document tools
	registry.Register(tools.Cached(tools.NewSearchDocumentsTool(ragClient), toolCache, 30*time.Second))
	registry.Register(tools.NewAskDocumentTool(ragClient))
	registry.Register(tools.Cached(tools.NewListCollectionsTool(ragClient), toolCache, 60*time.Second))

	// MCP streamable HTTP endpoint
	mcpSrv := mcpadapter.NewServer(registry, mcpadapter.Defaults{})
	mcpHandler := sdkmcp.NewStreamableHTTPHandler(func(_ *http.Request) *sdkmcp.Server {
		return mcpSrv
	}, &sdkmcp.StreamableHTTPOptions{Stateless: true})
	authedMCPHandler := mcpadapter.OptionalJWTMiddleware(jwtSecret)(mcpHandler)

	// MCP client: discover and register tools from configured MCP servers.
	if mcpServersJSON := os.Getenv("MCP_SERVERS"); mcpServersJSON != "" {
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

	// Agent
	a := agent.New(llmc, registry, metrics.PromRecorder{}, 8, 30*time.Second).WithModel(ollamaModel)

	// HTTP
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(pkgmw.SecurityHeaders())
	router.Use(otelgin.Middleware("ai-service"))
	router.Use(corsMiddleware(getenv("ALLOWED_ORIGINS", "http://localhost:3000")))
	router.Use(apperror.ErrorHandler())

	apphttp.RegisterHealthRoutes(router, map[string]apphttp.ReadyCheck{
		"llm": func() error {
			if llmProvider == "ollama" {
				req, _ := http.NewRequest(http.MethodGet, ollamaURL+"/api/tags", nil)
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
			req, _ := http.NewRequest(http.MethodGet, ecommerceURL+"/health", nil)
			client := &http.Client{Timeout: 2 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			return nil
		},
	})
	apphttp.RegisterChatRoutes(router, a, jwtSecret, limiter)
	apphttp.RegisterMetricsRoute(router)
	router.Any("/mcp", gin.WrapH(authedMCPHandler))

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 120 * time.Second, // longer for LLM streaming responses
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("ai-service starting",
			"port", port,
			"ollama_url", ollamaURL,
			"ollama_model", ollamaModel,
			"ecommerce_url", ecommerceURL,
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("forced shutdown: %v", err)
	}
}

func runMCP() {
	jwtSecret := os.Getenv("JWT_SECRET")
	ecommerceURL := getenv("ECOMMERCE_URL", "http://ecommerce-service:8092")

	ecomBreaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:          "ai-ecommerce",
		OnStateChange: resilience.ObserveStateChange,
	})
	ecomClient := clients.NewEcommerceClient(ecommerceURL, ecomBreaker)

	registry := tools.NewMemRegistry()
	registry.Register(tools.NewSearchProductsTool(ecomClient))
	registry.Register(tools.NewGetProductTool(ecomClient))
	registry.Register(tools.NewCheckInventoryTool(ecomClient))
	registry.Register(tools.NewListOrdersTool(ecomClient))
	registry.Register(tools.NewGetOrderTool(ecomClient))
	registry.Register(tools.NewViewCartTool(ecomClient))
	registry.Register(tools.NewAddToCartTool(ecomClient))
	registry.Register(tools.NewInitiateReturnTool(ecomClient))
	// Note: summarize_orders is excluded — it requires an LLM client which
	// is not available in stdio mode.

	// Parse JWT from env for user-scoped tools.
	var defaults mcpadapter.Defaults
	if token := os.Getenv("AI_SERVICE_TOKEN"); token != "" && jwtSecret != "" {
		uid, err := auth.ParseBearer("Bearer "+token, jwtSecret)
		if err != nil {
			log.Fatalf("AI_SERVICE_TOKEN invalid: %v", err)
		}
		defaults = mcpadapter.Defaults{UserID: uid, JWT: token}
		slog.Info("stdio mode: authenticated", "user_id", uid)
	}

	mcpSrv := mcpadapter.NewServer(registry, defaults)
	slog.Info("ai-service MCP server starting (stdio)")
	if err := mcpSrv.Run(context.Background(), &sdkmcp.StdioTransport{}); err != nil {
		log.Fatalf("mcp server: %v", err)
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func corsMiddleware(allowedOrigins string) gin.HandlerFunc {
	originSet := make(map[string]bool)
	for _, o := range strings.Split(allowedOrigins, ",") {
		originSet[strings.TrimSpace(o)] = true
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if originSet[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Requested-With")
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
