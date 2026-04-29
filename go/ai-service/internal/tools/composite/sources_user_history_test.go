package composite

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPostgresUserHistoryOrders(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"order_id", "product_id"}).
		AddRow("o1", "p1").
		AddRow("o2", "p2")
	// The query joins orders → order_items; match any SELECT that touches order_items.
	mock.ExpectQuery(`SELECT`).WithArgs("u1").WillReturnRows(rows)

	src := PostgresUserHistory{OrdersDB: db}
	got, err := src.Orders(context.Background(), "u1")
	if err != nil {
		t.Fatalf("Orders: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got))
	}
	if got[0].ProductID != "p1" || got[0].Source != "order:o1" {
		t.Fatalf("unexpected first item: %+v", got[0])
	}
	if got[1].ProductID != "p2" || got[1].Source != "order:o2" {
		t.Fatalf("unexpected second item: %+v", got[1])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresUserHistoryOrdersEmptyUserID(t *testing.T) {
	src := PostgresUserHistory{}
	got, err := src.Orders(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestPostgresUserHistoryCartItems(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"product_id"}).
		AddRow("p1").
		AddRow("p2")
	mock.ExpectQuery(`SELECT`).WithArgs("u1").WillReturnRows(rows)

	src := PostgresUserHistory{CartDB: db}
	got, err := src.CartItems(context.Background(), "u1")
	if err != nil {
		t.Fatalf("CartItems: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got))
	}
	if got[0].Source != "cart:current" || got[1].Source != "cart:current" {
		t.Fatalf("unexpected Source values: %+v", got)
	}
}

func TestPostgresUserHistoryCartItemsEmptyUserID(t *testing.T) {
	src := PostgresUserHistory{}
	got, err := src.CartItems(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestPostgresUserHistoryRecentlyViewedReturnsEmpty(t *testing.T) {
	src := PostgresUserHistory{}
	got, err := src.RecentlyViewed(context.Background(), "u1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}
