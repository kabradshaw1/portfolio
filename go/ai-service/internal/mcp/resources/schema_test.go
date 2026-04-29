package resources

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSchemaResourceReadsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "schema-ecommerce.md")
	if err := os.WriteFile(path, []byte("# Ecommerce schema\norders.id"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := NewSchemaResource(path)
	if err != nil {
		t.Fatal(err)
	}
	if r.URI() != "schema://ecommerce" {
		t.Fatalf("uri: %s", r.URI())
	}
	if r.MIMEType() != "text/markdown" {
		t.Fatalf("mime: %s", r.MIMEType())
	}
	got, err := r.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "# Ecommerce schema\norders.id" {
		t.Fatalf("got %q", got.Text)
	}
}

func TestSchemaResourceMissingPathErrorsAtConstruction(t *testing.T) {
	_, err := NewSchemaResource("")
	if err == nil {
		t.Fatalf("expected error on empty path")
	}
}
