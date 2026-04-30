package main

import (
	"context"
	"log"
	"os"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kabradshaw1/portfolio/go/study-service/internal/mcpserver"
	"github.com/kabradshaw1/portfolio/go/study-service/internal/store"
	"github.com/kabradshaw1/portfolio/go/study-service/internal/study"
)

func main() {
	ctx := context.Background()
	dbPath := getenv("STUDY_DB_PATH", "data/study.db")
	materialPath := getenv("STUDY_MATERIAL_PATH", "../../docs/interview-prep/micro1-go-developer")

	db, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		log.Fatalf("migrate store: %v", err)
	}

	service := study.New(db, materialPath)
	if _, err := service.ImportMaterial(ctx); err != nil {
		log.Printf("initial material import failed: %v", err)
	}

	server := mcpserver.New(service)
	if err := server.Run(ctx, &sdkmcp.StdioTransport{}); err != nil {
		log.Fatalf("mcp server: %v", err)
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
