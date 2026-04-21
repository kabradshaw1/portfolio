package main

import (
	"log"
	"os"
)

type Config struct {
	DatabaseURL     string
	JWTSecret       string
	AllowedOrigins  string
	Port            string
	GRPCPort        string
	RedisURL        string
	KafkaBrokers    string
	ProductGRPCAddr string
	OTELEndpoint    string
}

func loadConfig() Config {
	cfg := Config{
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		JWTSecret:       os.Getenv("JWT_SECRET"),
		AllowedOrigins:  os.Getenv("ALLOWED_ORIGINS"),
		Port:            os.Getenv("PORT"),
		GRPCPort:        os.Getenv("GRPC_PORT"),
		RedisURL:        os.Getenv("REDIS_URL"),
		KafkaBrokers:    os.Getenv("KAFKA_BROKERS"),
		ProductGRPCAddr: os.Getenv("PRODUCT_GRPC_ADDR"),
		OTELEndpoint:    os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
	}

	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		log.Fatal("JWT_SECRET is required")
	}
	if cfg.AllowedOrigins == "" {
		cfg.AllowedOrigins = "http://localhost:3000"
	}
	if cfg.Port == "" {
		cfg.Port = "8096"
	}
	if cfg.GRPCPort == "" {
		cfg.GRPCPort = "9096"
	}

	return cfg
}
