package ingestionapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientListCollections(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/collections" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"collections": []map[string]any{{"name": "documents", "points_count": 15}},
		})
	}))
	defer server.Close()

	got, err := New(server.URL, "", server.Client()).ListCollections(context.Background())
	if err != nil {
		t.Fatalf("ListCollections returned error: %v", err)
	}
	if len(got) != 1 || got[0].Name != "documents" || got[0].PointsCount != 15 {
		t.Fatalf("unexpected collections: %#v", got)
	}
}

func TestClientGetCollectionConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/collections/documents/config" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"chunk_size": 1000, "embedding_model": "nomic-embed-text"})
	}))
	defer server.Close()

	got, err := New(server.URL, "", server.Client()).GetCollectionConfig(context.Background(), "documents")
	if err != nil {
		t.Fatalf("GetCollectionConfig returned error: %v", err)
	}
	if got["embedding_model"] != "nomic-embed-text" {
		t.Fatalf("unexpected config: %#v", got)
	}
}

func TestClientUploadPDF(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/ingest" || r.URL.Query().Get("collection") != "eval_product_catalog_v1_a1b2c3d4" {
			t.Fatalf("unexpected request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("FormFile: %v", err)
		}
		defer file.Close()
		if header.Filename != "laptop.pdf" {
			t.Fatalf("filename = %q", header.Filename)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "chunks_created": 3})
	}))
	defer server.Close()

	got, err := New(server.URL, "token", server.Client()).UploadPDF(context.Background(), "eval_product_catalog_v1_a1b2c3d4", "laptop.pdf", []byte("%PDF-1.4"))
	if err != nil {
		t.Fatalf("UploadPDF returned error: %v", err)
	}
	if got.ChunksCreated != 3 {
		t.Fatalf("ChunksCreated = %d", got.ChunksCreated)
	}
}

func TestClientManifestRoundTripMethods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/collections/eval_product_catalog_v1_a1b2c3d4/manifest" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		switch r.Method {
		case http.MethodPut:
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "stored"})
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"fixture_id": "product_catalog_v1"})
		default:
			t.Fatalf("unexpected method: %s", r.Method)
		}
	}))
	defer server.Close()

	client := New(server.URL, "", server.Client())
	err := client.PutCollectionManifest(context.Background(), "eval_product_catalog_v1_a1b2c3d4", map[string]any{"fixture_id": "product_catalog_v1"})
	if err != nil {
		t.Fatalf("PutCollectionManifest returned error: %v", err)
	}
	got, err := client.GetCollectionManifest(context.Background(), "eval_product_catalog_v1_a1b2c3d4")
	if err != nil {
		t.Fatalf("GetCollectionManifest returned error: %v", err)
	}
	if got["fixture_id"] != "product_catalog_v1" {
		t.Fatalf("manifest = %#v", got)
	}
}
