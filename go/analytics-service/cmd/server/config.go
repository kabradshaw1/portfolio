package main

import (
	"log"
	"os"
)

// Config holds all configuration for the analytics service.
type Config struct {
	Port           string
	KafkaBrokers   string
	AllowedOrigins string
	OTELEndpoint   string
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
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
