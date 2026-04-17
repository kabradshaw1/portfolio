//go:build integration

package testutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunMigrations reads all .up.sql files from the service migrations directory,
// sorts them lexicographically (matching golang-migrate ordering), creates the
// pgcrypto extension, and executes each file in sequence.
func RunMigrations(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	// Locate this file at runtime to build a stable relative path to migrations/.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	migrationsDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "migrations")

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("read migrations dir %s: %v", migrationsDir, err)
	}

	var upFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			upFiles = append(upFiles, filepath.Join(migrationsDir, e.Name()))
		}
	}
	sort.Strings(upFiles)

	// pgcrypto is required by the first migration (gen_random_uuid).
	if _, err := pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS "pgcrypto"`); err != nil {
		t.Fatalf("create pgcrypto extension: %v", err)
	}

	for _, f := range upFiles {
		sql, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read migration %s: %v", f, err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("execute migration %s: %v", f, err)
		}
	}
}

// TruncateAll removes all rows from every data table in FK-safe order so tests
// can start with a clean slate without recreating the schema.
func TruncateAll(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	// Order matters: child tables before parents.
	tables := []string{"returns", "order_items", "orders", "cart_items", "products"}
	query := fmt.Sprintf("TRUNCATE %s RESTART IDENTITY CASCADE", strings.Join(tables, ", "))
	if _, err := pool.Exec(ctx, query); err != nil {
		t.Fatalf("truncate tables: %v", err)
	}
}

// SeedProducts inserts n products with varied names, prices, categories, and
// staggered created_at timestamps. It returns the inserted IDs as strings.
func SeedProducts(ctx context.Context, t *testing.T, pool *pgxpool.Pool, n int) []string {
	t.Helper()

	categories := []string{"electronics", "clothing", "books", "home", "sports"}

	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		category := categories[i%len(categories)]
		name := fmt.Sprintf("Product %d", i+1)
		price := 1000 + (i * 100) // prices in cents: $10.00, $11.00, …
		createdAt := time.Now().UTC().Add(-time.Duration(n-i) * time.Minute)

		var id string
		err := pool.QueryRow(ctx,
			`INSERT INTO products (name, description, price, category, stock, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $6)
			 RETURNING id::text`,
			name,
			fmt.Sprintf("Description for %s", name),
			price,
			category,
			10+i, // varied stock
			createdAt,
		).Scan(&id)
		if err != nil {
			t.Fatalf("seed product %d: %v", i+1, err)
		}
		ids = append(ids, id)
	}
	return ids
}
