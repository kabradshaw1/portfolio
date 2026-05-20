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

func TestClientListCollectionSources(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/collections/documents/sources" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"collection": "documents",
			"sources": []map[string]any{
				{"filename": "laptop.pdf", "chunks": 2},
			},
		})
	}))
	defer server.Close()

	got, err := New(server.URL, "", server.Client()).ListCollectionSources(context.Background(), "documents")
	if err != nil {
		t.Fatalf("ListCollectionSources returned error: %v", err)
	}
	if len(got) != 1 || got[0].Filename != "laptop.pdf" || got[0].Chunks != 2 {
		t.Fatalf("unexpected sources: %#v", got)
	}
}
