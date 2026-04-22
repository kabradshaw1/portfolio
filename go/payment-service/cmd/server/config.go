package main

import (
	"log"
	"os"
)

type Config struct {
	DatabaseURL         string
	Port                string
	GRPCPort            string
	StripeSecretKey     string
	StripeWebhookSecret string
	RabbitmqURL         string
	KafkaBrokers        string
	OTELEndpoint        string
	AllowedOrigins      string
	TLSCertDir          string
}

func loadConfig() Config {
	cfg := Config{
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		Port:                os.Getenv("PORT"),
		GRPCPort:            os.Getenv("GRPC_PORT"),
		StripeSecretKey:     os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
		RabbitmqURL:         os.Getenv("RABBITMQ_URL"),
		KafkaBrokers:        os.Getenv("KAFKA_BROKERS"),
		OTELEndpoint:        os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		AllowedOrigins:      os.Getenv("ALLOWED_ORIGINS"),
		TLSCertDir:          os.Getenv("TLS_CERT_DIR"),
	}

	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	if cfg.StripeSecretKey == "" {
		log.Fatal("STRIPE_SECRET_KEY is required")
	}
	if cfg.StripeWebhookSecret == "" {
		log.Fatal("STRIPE_WEBHOOK_SECRET is required")
	}
	if cfg.RabbitmqURL == "" {
		log.Fatal("RABBITMQ_URL is required")
	}
	if cfg.Port == "" {
		cfg.Port = "8098"
	}
	if cfg.GRPCPort == "" {
		cfg.GRPCPort = "9098"
	}
	if cfg.AllowedOrigins == "" {
		cfg.AllowedOrigins = "http://localhost:3000"
	}

	return cfg
}
