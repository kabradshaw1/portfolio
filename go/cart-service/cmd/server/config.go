package main

import (
	"fmt"
	"log"
	"net/url"
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
	RabbitmqURL     string
	AuthGRPCURL     string
	OTELEndpoint    string
}

func loadConfig() Config {
	databaseURL, err := buildDatabaseURL()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	rabbitmqURL, err := buildRabbitMQURL()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	cfg := Config{
		DatabaseURL:     databaseURL,
		JWTSecret:       os.Getenv("JWT_SECRET"),
		AllowedOrigins:  os.Getenv("ALLOWED_ORIGINS"),
		Port:            os.Getenv("PORT"),
		GRPCPort:        os.Getenv("GRPC_PORT"),
		RedisURL:        os.Getenv("REDIS_URL"),
		KafkaBrokers:    os.Getenv("KAFKA_BROKERS"),
		ProductGRPCAddr: os.Getenv("PRODUCT_GRPC_ADDR"),
		RabbitmqURL:     rabbitmqURL,
		AuthGRPCURL:     getEnv("AUTH_GRPC_URL", "localhost:9091"),
		OTELEndpoint:    os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
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

func buildRabbitMQURL() (string, error) {
	host := os.Getenv("MQ_HOST")
	port := os.Getenv("MQ_PORT")
	vhost := os.Getenv("MQ_VHOST")
	user := os.Getenv("MQ_USER")
	password := os.Getenv("MQ_PASSWORD")

	for _, kv := range [][2]string{
		{"MQ_HOST", host},
		{"MQ_PORT", port},
		{"MQ_USER", user},
		{"MQ_PASSWORD", password},
	} {
		if kv[1] == "" {
			return "", fmt.Errorf("%s is required", kv[0])
		}
	}

	userinfo := url.QueryEscape(user) + ":" + url.QueryEscape(password)
	dsn := fmt.Sprintf("amqp://%s@%s:%s", userinfo, host, port)
	if vhost != "" {
		dsn += "/" + url.PathEscape(vhost)
	}
	return dsn, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
