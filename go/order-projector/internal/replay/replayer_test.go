package replay

import (
	"context"
	"testing"
)

func TestReplayerRejectsInvalidProjection(t *testing.T) {
	r := New(nil, nil) // nil is safe for validation-only test
	err := r.Start(context.Background(), "nonexistent")
	if err == nil {
		t.Errorf("expected error for invalid projection, got nil")
	}
	if err.Error() != "unknown projection: nonexistent" {
		t.Errorf("expected error message to contain 'unknown projection', got: %s", err.Error())
	}
}
