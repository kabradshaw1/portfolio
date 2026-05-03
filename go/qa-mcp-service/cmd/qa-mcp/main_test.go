package main

import (
	"bytes"
	"context"
	"log"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunLogsReadyBeforeStartingServer(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := log.New(&logs, "", 0)
	dbPath := filepath.Join(t.TempDir(), "qa.db")

	called := false
	err := run(context.Background(), config{
		dbPath:       dbPath,
		materialPath: filepath.Join(t.TempDir(), "missing-material"),
	}, logger, func(context.Context, *app) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if !called {
		t.Fatalf("expected server runner to be called")
	}

	output := logs.String()
	if !strings.Contains(output, "QA MCP server running on stdio") {
		t.Fatalf("expected running log, got %q", output)
	}
	if !strings.Contains(output, "db_path="+dbPath) {
		t.Fatalf("expected db path in log, got %q", output)
	}
	if !strings.Contains(output, "initial material import failed") {
		t.Fatalf("expected import failure log, got %q", output)
	}
}
