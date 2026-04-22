package repository

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/pkg/dbtest"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

var (
	benchPool *pgxpool.Pool
	benchTDB  *dbtest.TestDB
)

var benchUserID = uuid.MustParse("00000000-0000-0000-0000-000000000001")
var benchProductIDs []uuid.UUID

func TestMain(m *testing.M) {
	ctx := context.Background()
	var err error
	benchTDB, err = dbtest.SetupPostgres(ctx, "../../migrations")
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: skipping DB benchmarks (no Docker): %v\n", err)
	} else {
		benchPool = benchTDB.Pool
		seedOrderBenchData(ctx, benchPool)
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

func seedOrderBenchData(ctx context.Context, pool *pgxpool.Pool) {
	benchProductIDs = make([]uuid.UUID, 100)
	categories := []string{"Electronics", "Clothing", "Home", "Books", "Sports"}
	for i := range benchProductIDs {
		id := uuid.New()
		benchProductIDs[i] = id
		_, err := pool.Exec(ctx,
			`INSERT INTO products (id, name, description, price, category, image_url, stock)
			 VALUES ($1, $2, $3, $4, $5, '', $6)`,
			id,
			fmt.Sprintf("Order Bench Product %03d", i),
			fmt.Sprintf("Description %d", i),
			1000+rand.Intn(50000),
			categories[i%len(categories)],
			100000,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "seed product %d: %v\n", i, err)
			os.Exit(1)
		}
	}

	for i := 0; i < 200; i++ {
		orderID := uuid.New()
		itemCount := 1 + rand.Intn(5)
		total := 0
		type item struct {
			pid        uuid.UUID
			qty, price int
		}
		items := make([]item, itemCount)
		for j := 0; j < itemCount; j++ {
			price := 1000 + rand.Intn(50000)
			qty := 1 + rand.Intn(3)
			items[j] = item{
				pid:   benchProductIDs[rand.Intn(len(benchProductIDs))],
				qty:   qty,
				price: price,
			}
			total += price * qty
		}

		_, err := pool.Exec(ctx,
			`INSERT INTO orders (id, user_id, status, saga_step, total, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, NOW() - ($6 || ' hours')::interval, NOW())`,
			orderID, benchUserID,
			[]string{"pending", "completed", "processing"}[i%3],
			[]string{"CREATED", "COMPLETED", "STOCK_RESERVED", "PAYMENT_CONFIRMED"}[i%4],
			total, fmt.Sprintf("%d", i),
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "seed order %d: %v\n", i, err)
			os.Exit(1)
		}

		for _, it := range items {
			_, err := pool.Exec(ctx,
				`INSERT INTO order_items (id, order_id, product_id, quantity, price_at_purchase)
				 VALUES ($1, $2, $3, $4, $5)`,
				uuid.New(), orderID, it.pid, it.qty, it.price,
			)
			if err != nil {
				fmt.Fprintf(os.Stderr, "seed order item: %v\n", err)
				os.Exit(1)
			}
		}
	}
}

func newOrderBenchRepo() *OrderRepository {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "bench-order"})
	return NewOrderRepository(benchPool, breaker)
}

func benchOrderCreate(b *testing.B, itemCount int) {
	skipIfNoDocker(b)
	repo := newOrderBenchRepo()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		items := make([]model.OrderItem, itemCount)
		for j := range items {
			items[j] = model.OrderItem{
				ProductID:       benchProductIDs[j%len(benchProductIDs)],
				Quantity:        1,
				PriceAtPurchase: 2500,
			}
		}
		_, err := repo.Create(ctx, uuid.New(), 2500*itemCount, items)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOrderCreate_1Item(b *testing.B)   { benchOrderCreate(b, 1) }
func BenchmarkOrderCreate_5Items(b *testing.B)  { benchOrderCreate(b, 5) }
func BenchmarkOrderCreate_20Items(b *testing.B) { benchOrderCreate(b, 20) }

func BenchmarkOrderFindByID(b *testing.B) {
	skipIfNoDocker(b)
	repo := newOrderBenchRepo()
	ctx := context.Background()

	var id uuid.UUID
	err := benchPool.QueryRow(ctx, "SELECT id FROM orders LIMIT 1").Scan(&id)
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

func BenchmarkOrderListByUser_Simple(b *testing.B) {
	skipIfNoDocker(b)
	repo := newOrderBenchRepo()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := repo.ListByUser(ctx, benchUserID, model.OrderListParams{Limit: 20})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOrderFindIncompleteSagas(b *testing.B) {
	skipIfNoDocker(b)
	repo := newOrderBenchRepo()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := repo.FindIncompleteSagas(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestCaptureOrderExplainBaseline(t *testing.T) {
	skipTestIfNoDocker(t)
	if testing.Short() {
		t.Skip("skipping explain capture in short mode")
	}
	ctx := context.Background()
	basedir := "../../../benchdata/order-service/baseline"

	_ = dbtest.CaptureExplain(ctx, benchPool, basedir+"/find_incomplete_sagas.json",
		"SELECT id FROM orders WHERE saga_step NOT IN ($1, $2, $3)",
		"COMPLETED", "COMPENSATION_COMPLETE", "FAILED")

	var orderID uuid.UUID
	_ = benchPool.QueryRow(ctx, "SELECT id FROM orders LIMIT 1").Scan(&orderID)
	_ = dbtest.CaptureExplain(ctx, benchPool, basedir+"/find_by_id_order.json",
		"SELECT id, user_id, status, saga_step, total, created_at, updated_at FROM orders WHERE id = $1",
		orderID)
	_ = dbtest.CaptureExplain(ctx, benchPool, basedir+"/find_by_id_items.json",
		"SELECT oi.id, oi.order_id, oi.product_id, oi.quantity, oi.price_at_purchase, p.name FROM order_items oi JOIN products p ON p.id = oi.product_id WHERE oi.order_id = $1",
		orderID)
}
