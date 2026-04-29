package main

import (
	"context"
	"database/sql"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/agent"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/auth"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/cache"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/guardrails"
	appkafka "github.com/kabradshaw1/portfolio/go/ai-service/internal/kafka"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/llm"
	mcpadapter "github.com/kabradshaw1/portfolio/go/ai-service/internal/mcp"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools/clients"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools/composite"
	"github.com/kabradshaw1/portfolio/go/pkg/buildinfo"
	"github.com/kabradshaw1/portfolio/go/pkg/shutdown"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"

	_ "github.com/jackc/pgx/v5/stdlib" // registers "pgx" driver for database/sql
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
	cfg := loadConfig()

	// Tracing
	ctx := context.Background()
	shutdownTracer, err := tracing.Init(ctx, "ai-service", cfg.OTELEndpoint)
	if err != nil {
		log.Fatalf("tracing init: %v", err)
	}
	slog.SetDefault(slog.New(
		tracing.NewLogHandler(slog.NewJSONHandler(os.Stdout, nil)),
	))
	buildinfo.Log()

	// Circuit breakers
	ollamaBreaker := newCircuitBreaker("ai-ollama")
	ecomBreaker := newCircuitBreaker("ai-ecommerce")
	ragBreaker := newCircuitBreaker("ai-rag")
	redisBreaker := newCircuitBreaker("ai-redis")

	// LLM client
	llmc, err := llm.NewClient(cfg.LLMProvider, cfg.LLMBaseURL, cfg.OllamaModel, cfg.LLMAPIKey, ollamaBreaker)
	if err != nil {
		log.Fatalf("LLM client: %v", err)
	}

	// Tool dependencies
	ecomClient := clients.NewEcommerceClient(cfg.OrderURL, ecomBreaker)
	ragClient := clients.NewRAGClient(cfg.RAGChatURL, cfg.RAGIngestionURL, ragBreaker)

	var toolCache cache.Cache = cache.NopCache{}
	var limiter *guardrails.Limiter
	if cfg.RedisURL != "" {
		opts, err := redis.ParseURL(cfg.RedisURL)
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

	// Kafka producer (optional)
	var kafkaPub appkafka.Producer
	if cfg.KafkaBrokers != "" {
		kafkaPub = appkafka.NewProducer(strings.Split(cfg.KafkaBrokers, ","))
		defer kafkaPub.Close()
		slog.Info("kafka producer enabled", "brokers", cfg.KafkaBrokers)
	}

	// Tool registration (HTTP path: cached + optional Kafka)
	registry := tools.NewMemRegistry()
	if kafkaPub != nil {
		registry.Register(tools.Cached(tools.NewSearchProductsTool(ecomClient, kafkaPub), toolCache, 60*time.Second))
		registry.Register(tools.Cached(tools.NewGetProductTool(ecomClient, kafkaPub), toolCache, 60*time.Second))
	} else {
		registry.Register(tools.Cached(tools.NewSearchProductsTool(ecomClient), toolCache, 60*time.Second))
		registry.Register(tools.Cached(tools.NewGetProductTool(ecomClient), toolCache, 60*time.Second))
	}
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

	// Composite investigate_my_order tool: open bounded *sql.DB connections to each
	// ecommerce database. Errors from sql.Open are fatal (DSN is syntactically wrong);
	// PingContext failures are warn-only — the tool degrades gracefully per-call.
	investigateHTTP := &http.Client{Timeout: 5 * time.Second}
	orderDB := mustOpenDB(cfg.OrderDBURL, "orderdb")
	defer orderDB.Close()
	pingDB(ctx, orderDB, "orderdb")

	paymentDB := mustOpenDB(cfg.PaymentDBURL, "paymentdb")
	defer paymentDB.Close()
	pingDB(ctx, paymentDB, "paymentdb")

	cartDB := mustOpenDB(cfg.CartDBURL, "cartdb")
	defer cartDB.Close()
	pingDB(ctx, cartDB, "cartdb")

	investigateFetcher := composite.EvidenceFetcher{
		Order:   composite.PostgresOrderSource{DB: orderDB},
		Saga:    composite.PostgresSagaSource{DB: orderDB}, // saga_step lives on the orders row
		Payment: composite.PostgresPaymentSource{DB: paymentDB},
		Cart:    composite.PostgresCartSource{DB: cartDB},
		Rabbit:  composite.NopRabbitSource{},
		Trace:   composite.JaegerTraceSource{BaseURL: cfg.JaegerQueryURL, HTTP: investigateHTTP},
		Logs:    composite.LokiLogSource{BaseURL: cfg.LokiURL, HTTP: investigateHTTP},
	}
	registry.Register(composite.NewInvestigateMyOrderTool(investigateFetcher))

	// MCP streamable HTTP endpoint
	mcpSrv := mcpadapter.NewServer(registry, mcpadapter.Defaults{})
	mcpHandler := sdkmcp.NewStreamableHTTPHandler(func(_ *http.Request) *sdkmcp.Server {
		return mcpSrv
	}, &sdkmcp.StreamableHTTPOptions{Stateless: true})
	authedMCPHandler := mcpadapter.OptionalJWTMiddleware(cfg.JWTSecret)(mcpHandler)

	// MCP client: discover and register tools from configured MCP servers.
	if cfg.MCPServersJSON != "" {
		discoverMCPTools(ctx, registry, cfg.MCPServersJSON)
	}

	// Agent
	a := agent.New(llmc, registry, metrics.PromRecorder{}, 8, 90*time.Second).WithModel(cfg.OllamaModel)

	// HTTP server
	router := setupRouter(cfg, a, limiter, authedMCPHandler, llmc)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 120 * time.Second, // longer for LLM streaming responses
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("ai-service starting",
			"port", cfg.Port,
			"ollama_url", cfg.OllamaURL,
			"ollama_model", cfg.OllamaModel,
			"order_url", cfg.OrderURL,
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
		}
	}()

	// Graceful shutdown
	sm := shutdown.New(15 * time.Second)
	sm.Register("drain-http", 0, shutdown.DrainHTTP("ai-http", srv))
	sm.Register("otel", 30, func(sctx context.Context) error {
		return shutdownTracer(sctx)
	})
	sm.Wait()
}

func runMCP() {
	jwtSecret := os.Getenv("JWT_SECRET")
	orderURL := getenv("ORDER_URL", "http://order-service:8092")

	ecomBreaker := newCircuitBreaker("ai-ecommerce")
	ecomClient := clients.NewEcommerceClient(orderURL, ecomBreaker)

	registry := tools.NewMemRegistry()
	registerCoreTools(registry, ecomClient)
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

// mustOpenDB opens a *sql.DB for the pgx driver and applies standard pool
// limits for read-only composite tool queries. It fatals on a syntactically
// invalid DSN but does NOT ping — callers should call pingDB separately so
// a transient network failure at startup doesn't prevent the service from
// starting.
func mustOpenDB(dsn, name string) *sql.DB {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("sql.Open %s: %v", name, err)
	}
	// Small pool — these connections serve read-only diagnostic queries from
	// the composite investigate_my_order tool, not hot-path traffic.
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxIdleTime(2 * time.Minute)
	db.SetConnMaxLifetime(30 * time.Minute)
	return db
}

// pingDB verifies connectivity at startup. A failure is logged as a warning
// rather than a fatal: the investigate_my_order tool degrades gracefully and
// surfaces per-call errors; a transient DB blip at startup should not prevent
// the broader ai-service from starting.
func pingDB(ctx context.Context, db *sql.DB, name string) {
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		slog.Warn("composite tool DB unreachable at startup — will retry per-call",
			"db", name, "error", err)
	} else {
		slog.Info("composite tool DB connected", "db", name)
	}
}
