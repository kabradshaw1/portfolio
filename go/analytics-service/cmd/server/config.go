package main

import (
	"log"
	"os"
	"time"
)

// Config holds all configuration for the analytics service.
type Config struct {
	Port           string
	KafkaBrokers   string
	AllowedOrigins string
	OTELEndpoint   string

	// Redis (optional — service works without it).
	RedisURL string

	// Windowed aggregation settings.
	WindowFlushInterval   time.Duration
	RevenueWindowSize     time.Duration
	TrendingWindowSize    time.Duration
	TrendingSlideInterval time.Duration
	AbandonmentWindowSize time.Duration
	LateEventGrace        time.Duration
}

// loadConfig reads configuration from environment variables.
// It fatals if KAFKA_BROKERS is not set.
func loadConfig() Config {
	kafkaBrokers := os.Getenv("KAFKA_BROKERS")
	if kafkaBrokers == "" {
		log.Fatal("KAFKA_BROKERS is required")
	}

	return Config{
		Port:           getenv("PORT", "8094"),
		KafkaBrokers:   kafkaBrokers,
		AllowedOrigins: getenv("ALLOWED_ORIGINS", "http://localhost:3000"),
		OTELEndpoint:   os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),

		RedisURL: os.Getenv("REDIS_URL"),

		WindowFlushInterval:   getenvDuration("WINDOW_FLUSH_INTERVAL", 30*time.Second),
		RevenueWindowSize:     getenvDuration("REVENUE_WINDOW_SIZE", 1*time.Hour),
		TrendingWindowSize:    getenvDuration("TRENDING_WINDOW_SIZE", 15*time.Minute),
		TrendingSlideInterval: getenvDuration("TRENDING_SLIDE_INTERVAL", 1*time.Minute),
		AbandonmentWindowSize: getenvDuration("ABANDONMENT_WINDOW_SIZE", 30*time.Minute),
		LateEventGrace:        getenvDuration("LATE_EVENT_GRACE", 5*time.Minute),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		log.Fatalf("invalid duration for %s: %q: %v", key, v, err)
	}
	return d
}
