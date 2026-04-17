package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

func TestRAGClient_Search(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["query"] != "what is kubernetes" {
			t.Fatalf("expected query 'what is kubernetes', got %q", body["query"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"text":"Kubernetes is...","filename":"k8s.pdf","page_number":1,"score":0.95},
			{"text":"Pods are...","filename":"k8s.pdf","page_number":3,"score":0.82}
		]}`))
	}))
	defer server.Close()

	c := NewRAGClient(server.URL, "", resilience.NewBreaker(resilience.BreakerConfig{Name: "test"}))
	results, err := c.Search(context.Background(), "what is kubernetes", "", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Text != "Kubernetes is..." {
		t.Errorf("unexpected text: %s", results[0].Text)
	}
	if results[0].Score != 0.95 {
		t.Errorf("unexpected score: %f", results[0].Score)
	}
}

func TestRAGClient_Ask(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Fatalf("expected Accept: application/json, got %q", r.Header.Get("Accept"))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"answer": "Kubernetes is a container orchestration platform.",
			"sources": [{"file": "k8s.pdf", "page": 1}]
		}`))
	}))
	defer server.Close()

	c := NewRAGClient(server.URL, "", resilience.NewBreaker(resilience.BreakerConfig{Name: "test"}))
	answer, err := c.Ask(context.Background(), "what is kubernetes", "")
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if answer.Answer != "Kubernetes is a container orchestration platform." {
		t.Errorf("unexpected answer: %s", answer.Answer)
	}
	if len(answer.Sources) != 1 || answer.Sources[0].File != "k8s.pdf" {
		t.Errorf("unexpected sources: %+v", answer.Sources)
	}
}

func TestRAGClient_ListCollections(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/collections" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"collections":[
			{"name":"documents","point_count":150},
			{"name":"debug-myproject","point_count":42}
		]}`))
	}))
	defer server.Close()

	c := NewRAGClient("", server.URL, resilience.NewBreaker(resilience.BreakerConfig{Name: "test"}))
	collections, err := c.ListCollections(context.Background())
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(collections) != 2 {
		t.Fatalf("expected 2 collections, got %d", len(collections))
	}
	if collections[0].Name != "documents" || collections[0].PointCount != 150 {
		t.Errorf("unexpected collection: %+v", collections[0])
	}
}

func TestRAGClient_Search_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewRAGClient(server.URL, "", resilience.NewBreaker(resilience.BreakerConfig{Name: "test"}))
	_, err := c.Search(context.Background(), "test", "", 5)
	if err == nil {
		t.Fatal("expected error on 500")
	}
}
