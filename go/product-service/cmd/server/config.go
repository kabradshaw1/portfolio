package main

import (
	"log"
	"os"
)

type Config struct {
	DatabaseURL    string
	AllowedOrigins string
	Port           string
	GRPCPort       string
	RedisURL       string
	OTELEndpoint   string
}

func loadConfig() Config {
	cfg := Config{
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		AllowedOrigins: os.Getenv("ALLOWED_ORIGINS"),
		Port:           os.Getenv("PORT"),
		GRPCPort:       os.Getenv("GRPC_PORT"),
		RedisURL:       os.Getenv("REDIS_URL"),
		OTELEndpoint:   os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
	}

	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	if cfg.AllowedOrigins == "" {
		cfg.AllowedOrigins = "http://localhost:3000"
	}
	if cfg.Port == "" {
		cfg.Port = "8095"
	}
	if cfg.GRPCPort == "" {
		cfg.GRPCPort = "9095"
	}

	return cfg
}
