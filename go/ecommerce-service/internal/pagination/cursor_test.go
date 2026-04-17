package pagination

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestEncodeDecode_RoundTripTimestamp(t *testing.T) {
	id := uuid.New()
	sortValue := time.Now().UTC().Format(time.RFC3339Nano)

	cursor := EncodeCursor(sortValue, id)
	if cursor == "" {
		t.Fatal("EncodeCursor returned empty string")
	}

	gotValue, gotID, err := DecodeCursor(cursor)
	if err != nil {
		t.Fatalf("DecodeCursor returned error: %v", err)
	}
	if gotValue != sortValue {
		t.Errorf("sort value mismatch: got %q, want %q", gotValue, sortValue)
	}
	if gotID != id {
		t.Errorf("ID mismatch: got %v, want %v", gotID, id)
	}
}

func TestEncodeDecode_RoundTripPriceCents(t *testing.T) {
	id := uuid.New()
	sortValue := "1999" // price in cents

	cursor := EncodeCursor(sortValue, id)
	if cursor == "" {
		t.Fatal("EncodeCursor returned empty string")
	}

	gotValue, gotID, err := DecodeCursor(cursor)
	if err != nil {
		t.Fatalf("DecodeCursor returned error: %v", err)
	}
	if gotValue != sortValue {
		t.Errorf("sort value mismatch: got %q, want %q", gotValue, sortValue)
	}
	if gotID != id {
		t.Errorf("ID mismatch: got %v, want %v", gotID, id)
	}
}

func TestDecodeCursor_InvalidBase64(t *testing.T) {
	_, _, err := DecodeCursor("not!!valid$$base64")
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
}

func TestDecodeCursor_InvalidJSON(t *testing.T) {
	// Valid base64 but not valid JSON
	badJSON := base64.URLEncoding.EncodeToString([]byte("this is not json"))
	_, _, err := DecodeCursor(badJSON)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestDecodeCursor_EmptyString(t *testing.T) {
	_, _, err := DecodeCursor("")
	if err == nil {
		t.Fatal("expected error for empty cursor, got nil")
	}
}

func TestDecodeCursor_InvalidUUID(t *testing.T) {
	// Valid base64+JSON but UUID field is not a valid UUID
	badUUID := base64.URLEncoding.EncodeToString([]byte(`{"v":"some-value","id":"not-a-uuid"}`))
	_, _, err := DecodeCursor(badUUID)
	if err == nil {
		t.Fatal("expected error for invalid UUID in cursor, got nil")
	}
}
