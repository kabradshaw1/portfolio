//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/integration/testutil"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/repository"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	gobreaker "github.com/sony/gobreaker/v2"
)

// sharedInfra is set up once in TestMain and torn down after all tests
// complete. Using TestMain avoids the t.Cleanup interaction where cleanup
// fires after the first test and closes the shared pool.
var sharedInfra *testutil.Infra

// mainTB is a testing.TB shim used during TestMain setup. Fatalf panics so
// that TestMain can detect setup failures. Cleanup is a no-op because
// teardown is handled explicitly by Infra.Teardown.
type mainTB struct {
	testing.TB
}

func (*mainTB) Helper()                           {}
func (*mainTB) Log(args ...any)                   { fmt.Println(args...) }
func (*mainTB) Logf(f string, args ...any)        { fmt.Printf(f+"\n", args...) }
func (*mainTB) Error(args ...any)                 { fmt.Println(args...) }
func (*mainTB) Errorf(f string, args ...any)      { fmt.Printf(f+"\n", args...) }
func (*mainTB) Fatal(args ...any)                 { panic(fmt.Sprint(args...)) }
func (*mainTB) Fatalf(f string, args ...any)      { panic(fmt.Sprintf(f, args...)) }
func (*mainTB) Cleanup(_ func())                  {} // no-op; Teardown handles cleanup
func (*mainTB) Setenv(_ string, _ string)         {}
func (*mainTB) TempDir() string                   { return os.TempDir() }
func (*mainTB) Parallel()                         {}
func (*mainTB) Skip(args ...any)                  {}
func (*mainTB) Skipf(f string, args ...any)       {}
func (*mainTB) SkipNow()                          {}
func (*mainTB) Skipped() bool                     { return false }
func (*mainTB) Failed() bool                      { return false }
func (*mainTB) FailNow()                          { panic("FailNow called during setup") }
func (*mainTB) Fail()                             {}
func (*mainTB) Name() string                      { return "TestMain" }
func (*mainTB) Run(_ string, _ func(t *testing.T)) bool { return true }

// TestMain starts shared infrastructure, runs all tests, then tears down.
func TestMain(m *testing.M) {
	ctx := context.Background()
	tb := &mainTB{}

	sharedInfra = testutil.SetupInfra(ctx, tb)
	testutil.RunMigrations(ctx, tb, sharedInfra.Pool)

	code := m.Run()

	sharedInfra.Teardown()
	os.Exit(code)
}

func getInfra(t *testing.T) *testutil.Infra {
	t.Helper()
	if sharedInfra == nil {
		t.Fatal("sharedInfra is nil — TestMain setup must have failed")
	}
	return sharedInfra
}

func newBreaker() *gobreaker.CircuitBreaker[any] {
	return resilience.NewBreaker(resilience.BreakerConfig{Name: "test"})
}

func TestProductRepository_ListAndFindByID(t *testing.T) {
	infra := getInfra(t)
	ctx := context.Background()
	testutil.TruncateAll(ctx, t, infra.Pool)

	ids := testutil.SeedProducts(ctx, t, infra.Pool, 5)

	repo := repository.NewProductRepository(infra.Pool, newBreaker())

	// List all products — offset mode (no cursor).
	products, total, err := repo.List(ctx, model.ProductListParams{Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total=5, got %d", total)
	}
	if len(products) != 5 {
		t.Errorf("expected 5 products, got %d", len(products))
	}

	// FindByID for the first seeded product.
	firstID, err := uuid.Parse(ids[0])
	if err != nil {
		t.Fatalf("parse product ID: %v", err)
	}
	p, err := repo.FindByID(ctx, firstID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if p.ID != firstID {
		t.Errorf("expected ID %s, got %s", firstID, p.ID)
	}
	if p.Name == "" {
		t.Error("expected non-empty product name")
	}
}

func TestOrderRepository_CreateAndFind(t *testing.T) {
	infra := getInfra(t)
	ctx := context.Background()
	testutil.TruncateAll(ctx, t, infra.Pool)

	ids := testutil.SeedProducts(ctx, t, infra.Pool, 1)

	productID, err := uuid.Parse(ids[0])
	if err != nil {
		t.Fatalf("parse product ID: %v", err)
	}
	userID := uuid.New()

	repo := repository.NewOrderRepository(infra.Pool, newBreaker())

	// Create order with 1 item.
	items := []model.OrderItem{
		{
			ProductID:       productID,
			Quantity:        2,
			PriceAtPurchase: 1000,
		},
	}
	order, err := repo.Create(ctx, userID, 2000, items)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if order.ID == uuid.Nil {
		t.Error("expected non-nil order ID")
	}
	if order.Status != model.OrderStatusPending {
		t.Errorf("expected status=pending, got %s", order.Status)
	}
	if order.Total != 2000 {
		t.Errorf("expected total=2000, got %d", order.Total)
	}

	// FindByID — verify items are returned.
	found, err := repo.FindByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if found.ID != order.ID {
		t.Errorf("expected order ID %s, got %s", order.ID, found.ID)
	}
	if len(found.Items) != 1 {
		t.Fatalf("expected 1 order item, got %d", len(found.Items))
	}
	if found.Items[0].ProductID != productID {
		t.Errorf("expected product ID %s, got %s", productID, found.Items[0].ProductID)
	}
	if found.Items[0].Quantity != 2 {
		t.Errorf("expected quantity=2, got %d", found.Items[0].Quantity)
	}

	// UpdateStatus to completed.
	if err := repo.UpdateStatus(ctx, order.ID, model.OrderStatusCompleted); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	completed, err := repo.FindByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("FindByID after status update: %v", err)
	}
	if completed.Status != model.OrderStatusCompleted {
		t.Errorf("expected status=completed, got %s", completed.Status)
	}
}
