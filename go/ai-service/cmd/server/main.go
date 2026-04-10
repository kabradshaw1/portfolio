package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/agent"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/cache"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/guardrails"
	apphttp "github.com/kabradshaw1/portfolio/go/ai-service/internal/http"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/llm"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools/clients"
)

func main() {
	port := getenv("PORT", "8093")
	ollamaURL := getenv("OLLAMA_URL", "http://ollama:11434")
	ollamaModel := getenv("OLLAMA_MODEL", "qwen2.5:14b")
	ecommerceURL := getenv("ECOMMERCE_URL", "http://ecommerce-service:8092")
	redisURL := getenv("REDIS_URL", "")

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET is required")
	}

	// LLM client
	llmProvider := getenv("LLM_PROVIDER", "ollama")
	llmAPIKey := os.Getenv("LLM_API_KEY")
	var llmBaseURL string
	if llmProvider == "ollama" {
		llmBaseURL = ollamaURL
	} else {
		llmBaseURL = getenv("LLM_BASE_URL", "")
	}
	llmc, err := llm.NewClient(llmProvider, llmBaseURL, ollamaModel, llmAPIKey)
	if err != nil {
		log.Fatalf("LLM client: %v", err)
	}

	// Tool registry
	ecomClient := clients.NewEcommerceClient(ecommerceURL)

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
			toolCache = cache.NewRedisCache(rc, "ai")
			limiter = guardrails.NewLimiter(rc, 20, time.Minute)
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

	// Agent
	a := agent.New(llmc, registry, metrics.PromRecorder{}, 8, 30*time.Second).WithModel(ollamaModel)

	// HTTP
	router := gin.New()
	router.Use(gin.Recovery())

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

	srv := &http.Server{Addr: ":" + port, Handler: router}

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

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
