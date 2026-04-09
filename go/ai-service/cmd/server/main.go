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

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/agent"
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

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET is required")
	}

	// LLM client
	llmc := llm.NewOllamaClient(ollamaURL, ollamaModel)

	// Tool registry
	ecomClient := clients.NewEcommerceClient(ecommerceURL)
	registry := tools.NewMemRegistry()
	registry.Register(tools.NewSearchProductsTool(ecomClient))
	registry.Register(tools.NewGetProductTool(ecomClient))
	registry.Register(tools.NewCheckInventoryTool(ecomClient))
	registry.Register(tools.NewListOrdersTool(ecomClient))
	registry.Register(tools.NewGetOrderTool(ecomClient))
	registry.Register(tools.NewSummarizeOrdersTool(ecomClient, llmc))
	registry.Register(tools.NewViewCartTool(ecomClient))
	registry.Register(tools.NewAddToCartTool(ecomClient))
	registry.Register(tools.NewInitiateReturnTool(ecomClient))

	// Agent
	a := agent.New(llmc, registry, metrics.PromRecorder{}, 8, 30*time.Second)

	// HTTP
	router := gin.New()
	router.Use(gin.Recovery())

	apphttp.RegisterHealthRoutes(router, map[string]apphttp.ReadyCheck{
		"ollama": func() error {
			req, _ := http.NewRequest(http.MethodGet, ollamaURL+"/api/tags", nil)
			client := &http.Client{Timeout: 2 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			return nil
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
	apphttp.RegisterChatRoutes(router, a, jwtSecret)
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
