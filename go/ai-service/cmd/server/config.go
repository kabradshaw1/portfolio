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
	EcommerceURL    string
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
		EcommerceURL:    getenv("ECOMMERCE_URL", "http://ecommerce-service:8092"),
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
