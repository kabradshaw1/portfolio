package resources

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRunbookResourceReadsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runbook.md")
	if err := os.WriteFile(path, []byte("# Portfolio runbook\nhello"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := NewRunbookResource(path)
	if err != nil {
		t.Fatal(err)
	}
	if r.URI() != "runbook://how-this-portfolio-works" {
		t.Fatalf("uri: %s", r.URI())
	}
	if r.MIMEType() != "text/markdown" {
		t.Fatalf("mime: %s", r.MIMEType())
	}
	got, err := r.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "# Portfolio runbook\nhello" {
		t.Fatalf("got %q", got.Text)
	}
}

func TestRunbookResourceMissingPathErrorsAtConstruction(t *testing.T) {
	_, err := NewRunbookResource("")
	if err == nil {
		t.Fatalf("expected error on empty path")
	}
}

func TestRunbookResourceMissingFileErrorsAtRead(t *testing.T) {
	r, err := NewRunbookResource("/nonexistent/path/runbook.md")
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Read(context.Background())
	if err == nil {
		t.Fatalf("expected error reading missing file")
	}
	if errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("missing file should return os error, not ErrResourceNotFound")
	}
}

func TestRunbookResourceCachesContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runbook.md")
	if err := os.WriteFile(path, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := NewRunbookResource(path)
	if err != nil {
		t.Fatal(err)
	}
	got1, err := r.Read(context.Background())
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	if err := os.WriteFile(path, []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	got2, err := r.Read(context.Background())
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if got1.Text != got2.Text {
		t.Fatalf("expected cached content; got1=%q got2=%q", got1.Text, got2.Text)
	}
}
