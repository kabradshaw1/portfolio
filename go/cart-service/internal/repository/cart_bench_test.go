package repository

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kabradshaw1/portfolio/go/pkg/dbtest"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
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
		fmt.Fprintf(os.Stderr, "setup postgres: %v\n", err)
		os.Exit(1)
	}

	benchPool = benchTDB.Pool

	code := m.Run()
	benchTDB.Teardown(ctx)
	os.Exit(code)
}

func newCartBenchRepo() *CartRepository {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "bench-cart"})
	return NewCartRepository(benchPool, breaker)
}

func seedCartForUser(ctx context.Context, pool *pgxpool.Pool, count int) (uuid.UUID, []uuid.UUID) {
	userID := uuid.New()
	itemIDs := make([]uuid.UUID, count)
	for i := 0; i < count; i++ {
		itemID := uuid.New()
		itemIDs[i] = itemID
		_, err := pool.Exec(ctx,
			`INSERT INTO cart_items (id, user_id, product_id, quantity, created_at)
			 VALUES ($1, $2, $3, $4, NOW())`,
			itemID, userID, uuid.New(), 1+i%5,
		)
		if err != nil {
			panic(fmt.Sprintf("seed cart item %d: %v", i, err))
		}
	}
	return userID, itemIDs
}

func BenchmarkCartGetByUser_5Items(b *testing.B) {
	repo := newCartBenchRepo()
	ctx := context.Background()
	userID, _ := seedCartForUser(ctx, benchPool, 5)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := repo.GetByUser(ctx, userID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCartGetByUser_50Items(b *testing.B) {
	repo := newCartBenchRepo()
	ctx := context.Background()
	userID, _ := seedCartForUser(ctx, benchPool, 50)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := repo.GetByUser(ctx, userID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCartAddItem_New(b *testing.B) {
	repo := newCartBenchRepo()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := repo.AddItem(ctx, uuid.New(), uuid.New(), 1)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCartAddItem_Upsert(b *testing.B) {
	repo := newCartBenchRepo()
	ctx := context.Background()

	userID := uuid.New()
	productID := uuid.New()
	_, err := repo.AddItem(ctx, userID, productID, 1)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := repo.AddItem(ctx, userID, productID, 1)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCartReserve(b *testing.B) {
	repo := newCartBenchRepo()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		userID, _ := seedCartForUser(ctx, benchPool, 10)
		b.StartTimer()

		err := repo.Reserve(ctx, userID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCartUpdateQuantity_Success(b *testing.B) {
	repo := newCartBenchRepo()
	ctx := context.Background()
	userID, itemIDs := seedCartForUser(ctx, benchPool, 10)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := repo.UpdateQuantity(ctx, itemIDs[i%len(itemIDs)], userID, 1+(i%10))
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCartUpdateQuantity_ReservedConflict(b *testing.B) {
	repo := newCartBenchRepo()
	ctx := context.Background()

	userID, itemIDs := seedCartForUser(ctx, benchPool, 10)
	_ = repo.Reserve(ctx, userID)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = repo.UpdateQuantity(ctx, itemIDs[i%len(itemIDs)], userID, 5)
	}
}

func TestCaptureCartExplainBaseline(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping explain capture in short mode")
	}
	ctx := context.Background()
	basedir := "../../../benchdata/cart-service/baseline"

	userID, _ := seedCartForUser(ctx, benchPool, 20)

	_ = dbtest.CaptureExplain(ctx, benchPool, basedir+"/get_by_user.json",
		"SELECT id, user_id, product_id, quantity, created_at FROM cart_items WHERE user_id = $1 ORDER BY created_at DESC",
		userID)
	_ = dbtest.CaptureExplain(ctx, benchPool, basedir+"/reserve.json",
		"UPDATE cart_items SET reserved = true WHERE user_id = $1 AND reserved = false",
		userID)
}
