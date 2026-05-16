package fixturecatalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogListsValidFixture(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "laptop-pro-15-specs.pdf", "%PDF-1.4")
	writeFile(t, root, "rag-eval-dataset-product-docs.json", `{
		"name": "product-docs-rag-v1",
		"items": [{
			"query": "How long does the battery last?",
			"expected_answer": "Up to 10 hours of mixed use.",
			"expected_sources": ["laptop-pro-15-specs.pdf"]
		}]
	}`)

	catalog := New([]string{root})
	fixtures, err := catalog.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(fixtures) != 1 {
		t.Fatalf("fixtures len = %d", len(fixtures))
	}
	got := fixtures[0]
	if got.ID != "rag-eval-dataset-product-docs.json" || got.Name != "product-docs-rag-v1" || got.ItemCount != 1 || !got.Valid {
		t.Fatalf("unexpected fixture: %#v", got)
	}
	if len(got.ReferencedSources) != 1 || got.ReferencedSources[0] != "laptop-pro-15-specs.pdf" {
		t.Fatalf("unexpected sources: %#v", got.ReferencedSources)
	}
}

func TestCatalogRejectsPathTraversalSource(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "fixture.json", `{
		"name": "bad",
		"items": [{
			"query": "q",
			"expected_answer": "a",
			"expected_sources": ["../secret.pdf"]
		}]
	}`)

	_, err := New([]string{root}).Load("fixture.json")
	if err == nil || !strings.Contains(err.Error(), "must stay under document root") {
		t.Fatalf("error = %v", err)
	}
}

func TestCatalogRejectsMissingSource(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "fixture.json", `{
		"name": "bad",
		"items": [{
			"query": "q",
			"expected_answer": "a",
			"expected_sources": ["missing.pdf"]
		}]
	}`)

	_, err := New([]string{root}).Load("fixture.json")
	if err == nil || !strings.Contains(err.Error(), "missing.pdf") {
		t.Fatalf("error = %v", err)
	}
}

func TestCatalogRejectsInvalidShape(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "fixture.json", `{"name": "bad name", "items": []}`)

	_, err := New([]string{root}).Load("fixture.json")
	if err == nil || !strings.Contains(err.Error(), "name must match") || !strings.Contains(err.Error(), "items must contain") {
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
		t.Fatalf("write file: %v", err)
	}
}
