package main

import (
	"log"
	"os"

	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	"github.com/sony/gobreaker/v2"
)

// Config holds all environment-based configuration for the ai-service.
type Config struct {
	Port            string
	OllamaURL       string
	OllamaModel     string
	OrderURL        string
	RAGChatURL      string
	RAGIngestionURL string
	RedisURL        string
	JWTSecret       string
	LLMProvider     string
	LLMAPIKey       string
	LLMBaseURL      string
	AllowedOrigins  string
	KafkaBrokers    string
	MCPServersJSON  string
	OTELEndpoint    string

	// Database URLs for the composite investigate_my_order tool.
	// Each ecommerce service owns its own database, so we need three separate
	// connection URLs. These default to the in-cluster K8s addresses.
	OrderDBURL   string
	PaymentDBURL string
	CartDBURL    string

	// Observability endpoints for the composite investigate_my_order tool.
	JaegerQueryURL string
	LokiURL        string
}

// loadConfig reads environment variables and returns a populated Config.
// Fatals if required values (JWT_SECRET) are missing.
func loadConfig() Config {
	ollamaURL := getenv("OLLAMA_URL", "http://ollama:11434")
	llmProvider := getenv("LLM_PROVIDER", "ollama")

	var llmBaseURL string
	if llmProvider == "ollama" {
		llmBaseURL = ollamaURL
	} else {
		llmBaseURL = getenv("LLM_BASE_URL", "")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET is required")
	}

	return Config{
		Port:            getenv("PORT", "8093"),
		OllamaURL:       ollamaURL,
		OllamaModel:     getenv("OLLAMA_MODEL", "qwen2.5:14b"),
		OrderURL:        getenv("ORDER_URL", "http://order-service:8092"),
		RAGChatURL:      getenv("RAG_CHAT_URL", "http://chat-service:8001"),
		RAGIngestionURL: getenv("RAG_INGESTION_URL", "http://ingestion-service:8002"),
		RedisURL:        getenv("REDIS_URL", ""),
		JWTSecret:       jwtSecret,
		LLMProvider:     llmProvider,
		LLMAPIKey:       os.Getenv("LLM_API_KEY"),
		LLMBaseURL:      llmBaseURL,
		AllowedOrigins:  getenv("ALLOWED_ORIGINS", "http://localhost:3000"),
		KafkaBrokers:    os.Getenv("KAFKA_BROKERS"),
		MCPServersJSON:  os.Getenv("MCP_SERVERS"),
		OTELEndpoint:    os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),

		// Database URLs for the composite investigate_my_order tool.
		// Defaults point at the in-cluster Postgres server using the same
		// application_name convention as the owning services (ai-service suffix
		// makes these connections identifiable in pg_stat_activity).
		OrderDBURL:   getenv("ORDER_DB_URL", "postgres://taskuser:taskpass@postgres.java-tasks.svc.cluster.local:5432/orderdb?sslmode=disable&application_name=ai-service"),
		PaymentDBURL: getenv("PAYMENT_DB_URL", "postgres://taskuser:taskpass@postgres.java-tasks.svc.cluster.local:5432/paymentdb?sslmode=disable&application_name=ai-service"),
		CartDBURL:    getenv("CART_DB_URL", "postgres://taskuser:taskpass@postgres.java-tasks.svc.cluster.local:5432/cartdb?sslmode=disable&application_name=ai-service"),

		// Observability endpoints for the composite investigate_my_order tool.
		JaegerQueryURL: getenv("JAEGER_QUERY_URL", "http://jaeger-query.monitoring.svc.cluster.local:16686"),
		LokiURL:        getenv("LOKI_URL", "http://loki.monitoring.svc.cluster.local:3100"),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func newCircuitBreaker(name string) *gobreaker.CircuitBreaker[any] {
	return resilience.NewBreaker(resilience.BreakerConfig{
		Name:          name,
		OnStateChange: resilience.ObserveStateChange,
	})
}
