package main

import (
	"log"
	"os"
	"strconv"
)

// Config holds all environment-driven configuration for the ecommerce service.
type Config struct {
	DatabaseURL  string // required
	JWTSecret    string // required
	RabbitmqURL  string // required
	AllowedOrigins string // default "http://localhost:3000"
	Port           string // default "8092"
	RedisURL       string // optional
	KafkaBrokers   string // optional
	WorkerConcurrency int    // default 3, parsed from WORKER_CONCURRENCY
	OTELEndpoint      string // optional
	ProductGRPCAddr   string // optional, address of product-service gRPC
	CartGRPCAddr      string // optional, address of cart-service gRPC
	AuthGRPCURL       string // address of auth-service gRPC for denylist checks
	FrontendURL       string // default "http://localhost:3000", used for Stripe redirect URLs
}

// loadConfig reads environment variables and returns a validated Config.
// It fatals on missing required values.
func loadConfig() Config {
	cfg := Config{
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		JWTSecret:      os.Getenv("JWT_SECRET"),
		RabbitmqURL:    os.Getenv("RABBITMQ_URL"),
		AllowedOrigins: os.Getenv("ALLOWED_ORIGINS"),
		Port:           os.Getenv("PORT"),
		RedisURL:       os.Getenv("REDIS_URL"),
		KafkaBrokers:   os.Getenv("KAFKA_BROKERS"),
		OTELEndpoint:      os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		ProductGRPCAddr:   os.Getenv("PRODUCT_GRPC_ADDR"),
		CartGRPCAddr:      os.Getenv("CART_GRPC_ADDR"),
		AuthGRPCURL:       getEnv("AUTH_GRPC_URL", "localhost:9091"),
		FrontendURL:       getEnv("FRONTEND_URL", "http://localhost:3000"),
		WorkerConcurrency: 3,
	}

	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		log.Fatal("JWT_SECRET is required")
	}
	if cfg.RabbitmqURL == "" {
		log.Fatal("RABBITMQ_URL is required")
	}
	if cfg.AllowedOrigins == "" {
		cfg.AllowedOrigins = "http://localhost:3000"
	}
	if cfg.Port == "" {
		cfg.Port = "8092"
	}
	if v := os.Getenv("WORKER_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.WorkerConcurrency = n
		}
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
