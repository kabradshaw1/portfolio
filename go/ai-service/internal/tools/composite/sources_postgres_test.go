package composite

import (
	"context"
	"database/sql"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// ─── PostgresOrderSource ──────────────────────────────────────────────────────

func TestPostgresOrderSourceHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id", "status", "user_id", "saga_step", "created_unix", "updated_unix"}).
		AddRow("ord-1", "completed", "u1", "COMPLETED", int64(1000), int64(2000))
	mock.ExpectQuery(`SELECT id`).WithArgs("ord-1").WillReturnRows(rows)

	src := PostgresOrderSource{DB: db}
	got, err := src.FetchOrder(context.Background(), "ord-1")
	if err != nil {
		t.Fatalf("FetchOrder: %v", err)
	}
	if got.ID != "ord-1" {
		t.Fatalf("ID: want %q, got %q", "ord-1", got.ID)
	}
	if got.Status != "completed" {
		t.Fatalf("Status: want %q, got %q", "completed", got.Status)
	}
	if got.UserID != "u1" {
		t.Fatalf("UserID: want %q, got %q", "u1", got.UserID)
	}
	if got.CreatedUnix != 1000 {
		t.Fatalf("CreatedUnix: want 1000, got %d", got.CreatedUnix)
	}
	if got.UpdatedUnix != 2000 {
		t.Fatalf("UpdatedUnix: want 2000, got %d", got.UpdatedUnix)
	}
	// TraceID and CorrelationID are not stored in the DB.
	if got.TraceID != "" || got.CorrelationID != "" {
		t.Fatalf("expected empty TraceID/CorrelationID, got %q / %q", got.TraceID, got.CorrelationID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresOrderSourceNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT id`).WithArgs("missing").WillReturnError(sql.ErrNoRows)

	src := PostgresOrderSource{DB: db}
	_, fetchErr := src.FetchOrder(context.Background(), "missing")
	// OrderSource is the primary source — ErrNoRows propagates as a hard error.
	if fetchErr == nil {
		t.Fatal("expected error for missing order, got nil")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// ─── PostgresSagaSource ───────────────────────────────────────────────────────

func TestPostgresSagaSourceHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"saga_step"}).AddRow("PAYMENT_CONFIRMED")
	mock.ExpectQuery(`SELECT COALESCE`).WithArgs("ord-2").WillReturnRows(rows)

	src := PostgresSagaSource{DB: db}
	got, err := src.FetchSaga(context.Background(), "ord-2")
	if err != nil {
		t.Fatalf("FetchSaga: %v", err)
	}
	if got.Step != "PAYMENT_CONFIRMED" {
		t.Fatalf("Step: want %q, got %q", "PAYMENT_CONFIRMED", got.Step)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresSagaSourceNoRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT COALESCE`).WithArgs("ord-missing").WillReturnError(sql.ErrNoRows)

	src := PostgresSagaSource{DB: db}
	got, err := src.FetchSaga(context.Background(), "ord-missing")
	if err != nil {
		t.Fatalf("FetchSaga should return zero value on ErrNoRows, got: %v", err)
	}
	if got.Step != "" {
		t.Fatalf("expected empty step, got %q", got.Step)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// ─── PostgresPaymentSource ────────────────────────────────────────────────────

func TestPostgresPaymentSourceHappyPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"stripe_payment_intent_id", "status"}).
		AddRow("pi_test_123", "succeeded")
	mock.ExpectQuery(`SELECT COALESCE`).WithArgs("ord-3").WillReturnRows(rows)

	src := PostgresPaymentSource{DB: db}
	got, err := src.FetchPayment(context.Background(), "ord-3")
	if err != nil {
		t.Fatalf("FetchPayment: %v", err)
	}
	if got.StripeChargeID != "pi_test_123" {
		t.Fatalf("StripeChargeID: want %q, got %q", "pi_test_123", got.StripeChargeID)
	}
	if !got.WebhookReceived {
		t.Fatal("expected WebhookReceived=true for status=succeeded")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresPaymentSourcePendingStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"stripe_payment_intent_id", "status"}).
		AddRow("pi_test_456", "pending")
	mock.ExpectQuery(`SELECT COALESCE`).WithArgs("ord-4").WillReturnRows(rows)

	src := PostgresPaymentSource{DB: db}
	got, err := src.FetchPayment(context.Background(), "ord-4")
	if err != nil {
		t.Fatalf("FetchPayment: %v", err)
	}
	if got.WebhookReceived {
		t.Fatal("expected WebhookReceived=false for status=pending")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresPaymentSourceNoRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT COALESCE`).WithArgs("ord-missing").WillReturnError(sql.ErrNoRows)

	src := PostgresPaymentSource{DB: db}
	got, err := src.FetchPayment(context.Background(), "ord-missing")
	if err != nil {
		t.Fatalf("FetchPayment should return zero on ErrNoRows, got: %v", err)
	}
	if got.StripeChargeID != "" || got.WebhookReceived {
		t.Fatalf("expected zero PaymentRecord, got %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// ─── PostgresCartSource ───────────────────────────────────────────────────────

func TestPostgresCartSourceAllReleased(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"count"}).AddRow(0)
	mock.ExpectQuery(`SELECT COUNT`).WithArgs("user-1").WillReturnRows(rows)

	src := PostgresCartSource{DB: db}
	got, err := src.FetchCartReservation(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("FetchCartReservation: %v", err)
	}
	if !got.Released {
		t.Fatal("expected Released=true when no reserved items")
	}
	if got.ReleasedAt != 0 {
		t.Fatalf("ReleasedAt not stored in DB, expected 0, got %d", got.ReleasedAt)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresCartSourceStillReserved(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"count"}).AddRow(3)
	mock.ExpectQuery(`SELECT COUNT`).WithArgs("user-2").WillReturnRows(rows)

	src := PostgresCartSource{DB: db}
	got, err := src.FetchCartReservation(context.Background(), "user-2")
	if err != nil {
		t.Fatalf("FetchCartReservation: %v", err)
	}
	if got.Released {
		t.Fatal("expected Released=false when items are still reserved")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresCartSourceNoItems(t *testing.T) {
	// COUNT(*) always returns a row (with 0), so sql.ErrNoRows never fires for
	// this query. This test documents that the happy-path zero-count path is
	// the canonical "no items" case — covered by TestPostgresCartSourceAllReleased.
	// Kept as a lint-free no-op to record the design decision in test history.
	t.Skip("COUNT(*) always returns a row; zero-count is covered by AllReleased test")
}
