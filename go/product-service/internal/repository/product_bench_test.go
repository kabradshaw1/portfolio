package repository

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kabradshaw1/portfolio/go/pkg/dbtest"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	"github.com/kabradshaw1/portfolio/go/product-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/product-service/internal/pagination"
)

var (
	benchPool *pgxpool.Pool
	benchTDB  *dbtest.TestDB
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	var err error
	benchTDB, err = dbtest.SetupPostgres(ctx, "../../migrations")
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: skipping DB benchmarks (no Docker): %v\n", err)
	} else {
		benchPool = benchTDB.Pool
		seedBenchData(ctx, benchPool)
	}

	code := m.Run()
	if benchTDB != nil {
		benchTDB.Teardown(ctx)
	}
	os.Exit(code)
}

func skipIfNoDocker(b *testing.B) {
	b.Helper()
	if benchPool == nil {
		b.Skip("skipping: Docker not available for testcontainers")
	}
}

func skipTestIfNoDocker(t *testing.T) {
	t.Helper()
	if benchPool == nil {
		t.Skip("skipping: Docker not available for testcontainers")
	}
}

func seedBenchData(ctx context.Context, pool *pgxpool.Pool) {
	categories := []string{"Electronics", "Clothing", "Home", "Books", "Sports"}
	for i := 0; i < 1000; i++ {
		_, err := pool.Exec(ctx,
			`INSERT INTO products (id, name, description, price, category, image_url, stock, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, '', $6, NOW() - ($7 || ' hours')::interval, NOW())`,
			uuid.New(),
			fmt.Sprintf("Bench Product %04d", i),
			fmt.Sprintf("Description for product %d", i),
			1000+rand.Intn(99000),
			categories[i%len(categories)],
			50+rand.Intn(200),
			fmt.Sprintf("%d", i),
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "seed product %d: %v\n", i, err)
			os.Exit(1)
		}
	}
}

func newBenchRepo() *ProductRepository {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "bench"})
	return NewProductRepository(benchPool, breaker)
}

func BenchmarkProductList_Cursor(b *testing.B) {
	skipIfNoDocker(b)
	repo := newBenchRepo()
	ctx := context.Background()

	products, _, _ := repo.List(ctx, model.ProductListParams{Limit: 20, Sort: "price_asc"})
	if len(products) == 0 {
		b.Fatal("no products seeded")
	}
	last := products[len(products)-1]
	cursor := pagination.EncodeCursor(fmt.Sprintf("%d", last.Price), last.ID)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := repo.List(ctx, model.ProductListParams{
			Limit:  20,
			Sort:   "price_asc",
			Cursor: cursor,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProductList_Offset(b *testing.B) {
	skipIfNoDocker(b)
	repo := newBenchRepo()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := repo.List(ctx, model.ProductListParams{
			Limit: 20,
			Page:  5,
			Sort:  "price_asc",
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProductList_CategoryFilter(b *testing.B) {
	skipIfNoDocker(b)
	repo := newBenchRepo()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := repo.List(ctx, model.ProductListParams{
			Limit:    20,
			Page:     1,
			Category: "Electronics",
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProductList_SearchQuery(b *testing.B) {
	skipIfNoDocker(b)
	repo := newBenchRepo()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := repo.List(ctx, model.ProductListParams{
			Limit: 20,
			Page:  1,
			Query: "Bench Product 05",
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProductFindByID(b *testing.B) {
	skipIfNoDocker(b)
	repo := newBenchRepo()
	ctx := context.Background()

	var id uuid.UUID
	err := benchPool.QueryRow(ctx, "SELECT id FROM products LIMIT 1").Scan(&id)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := repo.FindByID(ctx, id)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProductDecrementStock(b *testing.B) {
	skipIfNoDocker(b)
	repo := newBenchRepo()
	ctx := context.Background()

	ids := make([]uuid.UUID, 100)
	for i := range ids {
		id := uuid.New()
		ids[i] = id
		_, err := benchPool.Exec(ctx,
			`INSERT INTO products (id, name, description, price, category, image_url, stock)
			 VALUES ($1, $2, 'bench', 1000, 'Electronics', '', $3)`,
			id, fmt.Sprintf("Stock Bench %d", i), 1_000_000,
		)
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_ = repo.DecrementStock(ctx, ids[i%len(ids)], 1)
			i++
		}
	})
}

func BenchmarkProductCategories(b *testing.B) {
	skipIfNoDocker(b)
	repo := newBenchRepo()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := repo.Categories(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestCaptureExplainBaseline(t *testing.T) {
	skipTestIfNoDocker(t)
	if testing.Short() {
		t.Skip("skipping explain capture in short mode")
	}
	ctx := context.Background()

	basedir := "../../../benchdata/product-service/baseline"

	_ = dbtest.CaptureExplain(ctx, benchPool, basedir+"/list_offset_count.json",
		"SELECT COUNT(*) FROM products")
	_ = dbtest.CaptureExplain(ctx, benchPool, basedir+"/list_offset_data.json",
		"SELECT id, name, description, price, category, image_url, stock, created_at, updated_at FROM products ORDER BY price ASC, id ASC LIMIT $1 OFFSET $2",
		20, 80)

	var id uuid.UUID
	_ = benchPool.QueryRow(ctx, "SELECT id FROM products LIMIT 1").Scan(&id)
	_ = dbtest.CaptureExplain(ctx, benchPool, basedir+"/find_by_id.json",
		"SELECT id, name, description, price, category, image_url, stock, created_at, updated_at FROM products WHERE id = $1",
		id)

	_ = dbtest.CaptureExplain(ctx, benchPool, basedir+"/categories.json",
		"SELECT DISTINCT category FROM products ORDER BY category")

	_ = dbtest.CaptureExplain(ctx, benchPool, basedir+"/decrement_stock_select.json",
		"SELECT stock FROM products WHERE id = $1 FOR UPDATE",
		id)
}
