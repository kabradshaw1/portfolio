package corpusfixture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogListsFixtureWithDeterministicCollection(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "laptop.pdf", "%PDF-1.4\nlaptop")
	writeFile(t, root, "product_catalog_v1.corpus.json", `{
		"id": "product_catalog_v1",
		"name": "Product Catalog v1",
		"description": "Product catalog PDFs",
		"documents": ["laptop.pdf"],
		"expected_collection_prefix": "eval_product_catalog_v1"
	}`)
	writeFile(t, root, "rag-eval-dataset-product-docs.json", `{"name":"dataset","items":[]}`)

	fixtures, err := New([]string{root}).List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(fixtures) != 1 {
		t.Fatalf("fixture count = %d", len(fixtures))
	}
	got := fixtures[0]
	if got.ID != "product_catalog_v1" || got.DocumentCount != 1 || got.SourceHash == "" {
		t.Fatalf("unexpected fixture: %#v", got)
	}
	if !strings.HasPrefix(got.DefaultCollection, "eval_product_catalog_v1_") {
		t.Fatalf("default collection = %q", got.DefaultCollection)
	}
}

func TestLoadResolvesFixtureIDToCorpusManifest(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "laptop.pdf", "%PDF-1.4\nlaptop")
	writeFile(t, root, "product_catalog_v1.corpus.json", `{
		"id": "product_catalog_v1",
		"name": "Product Catalog v1",
		"description": "Product catalog PDFs",
		"documents": ["laptop.pdf"],
		"expected_collection_prefix": "eval_product_catalog_v1"
	}`)

	got, err := New([]string{root}).Load("product_catalog_v1")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got.ID != "product_catalog_v1" {
		t.Fatalf("ID = %q", got.ID)
	}
}

func TestLoadRejectsEscapingDocument(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "bad.corpus.json", `{
		"id": "bad",
		"name": "Bad",
		"description": "Bad fixture",
		"documents": ["../secret.pdf"],
		"expected_collection_prefix": "eval_bad"
	}`)

	_, err := New([]string{root}).Load("bad")
	if err == nil || !strings.Contains(err.Error(), "must stay under fixture root") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadRejectsSymlinkEscapingFixtureRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFile(t, outside, "secret.pdf", "%PDF-1.4\nsecret")
	if err := os.Symlink(filepath.Join(outside, "secret.pdf"), filepath.Join(root, "linked.pdf")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	writeFile(t, root, "bad.corpus.json", `{
		"id": "bad",
		"name": "Bad",
		"description": "Bad fixture",
		"documents": ["linked.pdf"],
		"expected_collection_prefix": "eval_bad"
	}`)

	_, err := New([]string{root}).Load("bad")
	if err == nil || !strings.Contains(err.Error(), "must stay under fixture root") {
		t.Fatalf("error = %v", err)
	}
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}
