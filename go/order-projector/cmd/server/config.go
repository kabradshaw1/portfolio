package main

import (
	"fmt"
	"log"
	"net/url"
	"os"
)

// Config holds all configuration for the order-projector service.
type Config struct {
	Port           string
	DatabaseURL    string
	KafkaBrokers   string
	AllowedOrigins string
	OTELEndpoint   string
}

// loadConfig reads configuration from environment variables. It fatals if
// any required value is missing. The Postgres DSN is assembled from
// component env vars (Phase 4 of the secrets-management migration) — the
// ConfigMap supplies host/port/db/options, the per-service Secret supplies
// user/password.
func loadConfig() Config {
	databaseURL, err := buildDatabaseURL()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	kafkaBrokers := os.Getenv("KAFKA_BROKERS")
	if kafkaBrokers == "" {
		log.Fatal("KAFKA_BROKERS is required")
	}

	return Config{
		Port:           getenv("PORT", "8097"),
		DatabaseURL:    databaseURL,
		KafkaBrokers:   kafkaBrokers,
		AllowedOrigins: getenv("ALLOWED_ORIGINS", "http://localhost:3000"),
		OTELEndpoint:   os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
	}
}

// buildDatabaseURL assembles a postgres:// DSN from the DB_* component env
// vars. Userinfo is URL-escaped so a future password rotation containing
// URL-reserved characters doesn't break parsing.
func buildDatabaseURL() (string, error) {
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	name := os.Getenv("DB_NAME")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")

	for _, kv := range [][2]string{
		{"DB_HOST", host},
		{"DB_PORT", port},
		{"DB_NAME", name},
		{"DB_USER", user},
		{"DB_PASSWORD", password},
	} {
		if kv[1] == "" {
			return "", fmt.Errorf("%s is required", kv[0])
		}
	}

	userinfo := url.QueryEscape(user) + ":" + url.QueryEscape(password)
	dsn := fmt.Sprintf("postgres://%s@%s:%s/%s", userinfo, host, port, name)
	if opts := os.Getenv("DB_OPTIONS"); opts != "" {
		dsn += "?" + opts
	}
	return dsn, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
