package tokenstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveThenLoadRoundTrip(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "auth", "tokens.json")
	store := NewFileStore(path)
	state := State{
		AccessToken:    "access-1",
		RefreshToken:   "refresh-1",
		AccessTokenExp: time.Date(2026, 5, 15, 12, 30, 0, 0, time.UTC),
		AuthEmail:      "user@example.test",
		AuthServiceURL: "http://auth.test/auth",
		WrittenAt:      time.Date(2026, 5, 15, 11, 30, 0, 0, time.UTC),
	}

	if err := store.Save(ctx, state); err != nil {
		t.Fatalf("Save error: %v", err)
	}
	got, ok, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if !ok {
		t.Fatal("Load ok = false")
	}
	if got.AccessToken != state.AccessToken || got.RefreshToken != state.RefreshToken || !got.AccessTokenExp.Equal(state.AccessTokenExp) || got.AuthEmail != state.AuthEmail || got.AuthServiceURL != state.AuthServiceURL || !got.WrittenAt.Equal(state.WrittenAt) {
		t.Fatalf("state = %#v, want %#v", got, state)
	}
}

func TestSaveWritesFileMode0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.json")
	store := NewFileStore(path)

	if err := store.Save(context.Background(), State{AccessToken: "access"}); err != nil {
		t.Fatalf("Save error: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat token file: %v", err)
	}
	if got := info.Mode().Perm(); got != fileMode {
		t.Fatalf("mode = %s, want %s", got, fileMode)
	}
}

func TestLoadMissingFileReturnsNotOK(t *testing.T) {
	store := NewFileStore(filepath.Join(t.TempDir(), "missing.json"))

	got, ok, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if ok {
		t.Fatal("Load ok = true")
	}
	if got != (State{}) {
		t.Fatalf("state = %#v", got)
	}
}

func TestLoadRejectsUnsafePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	_, ok, err := NewFileStore(path).Load(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if ok {
		t.Fatal("Load ok = true")
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("error = %v", err)
	}
}
